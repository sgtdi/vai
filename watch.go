package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sgtdi/fswatcher"
	"gopkg.in/yaml.v3"
)

// Vai contains vai fields
type Vai struct {
	cwd       string            `yaml:"-"`
	Config    Config            `yaml:"config"`
	Jobs      map[string]Job    `yaml:"jobs"`
	manager   *Manager          `yaml:"-"`
	fswatcher fswatcher.Watcher `yaml:"-"`
}

// Config options for file vai.yml
type Config struct {
	Severity         string        `yaml:"severity,omitempty"`
	ClearCli         bool          `yaml:"clearCli,omitempty"`
	Cooldown         time.Duration `yaml:"cooldown,omitempty"`
	BufferSize       int           `yaml:"bufferSize,omitempty"`
	BatchingDuration time.Duration `yaml:"batchingDuration,omitempty"`
	serverityLevel   fswatcher.Severity
}

// newVai parse config struct with all possible flags and args
func newVai(args *Args) (*Vai, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Could not get current working directory: %v", err)
	}

	v := &Vai{
		cwd:     cwd,
		manager: newManager(),
	}

	hasCLI := len(args.CmdFlags) > 0 || len(args.PositionalArgs) > 0
	hasConfig := fileExists(args.ConfigFile)

	// Nothing provided, show help
	if !hasCLI && !hasConfig || args.Help {
		return nil, fmt.Errorf("No config file and no command provided")
	}

	// Load config file as base if exist
	if hasConfig {
		logger.log(SeverityDebug, OpInfo, "Loading config from %s", args.ConfigFile)
		v.applyConfig(args.ConfigFile)
		logger.log(SeverityInfo, OpSuccess, "Using config %s", cyan(args.ConfigFile))
	}

	// Override config or CLI mode
	if hasCLI {
		v.applyCLI(args)
	}

	// Set defaults
	v.setDefaults()

	return v, nil
}

// startJobs starts all defined jobs
func (v *Vai) startJobs() {
	var jobNames []string
	for jobName := range v.Jobs {
		jobNames = append(jobNames, jobName)
	}

	logger.log(SeverityInfo, OpSuccess, "Jobs successfully imported: %s%s%s", ColorGreen, strings.Join(jobNames, ", "), ColorReset)
	// Run jobs on startup
	logger.log(SeverityInfo, OpWarn, "Running jobs...")
	for jobName, job := range v.Jobs {
		logger.log(SeverityInfo, OpWarn, "Triggering job: %s%s%s", ColorGreen, jobName, ColorReset)
		jobCtx, deregister := v.manager.register(jobName)
		job.Name = jobName
		go func(j Job) {
			defer deregister()
			j.start(jobCtx)
		}(job)
	}
}

// setDefaults applies default values to the Vai configuration
func (v *Vai) setDefaults() {
	for i, job := range v.Jobs {
		if job.Trigger != nil && len(job.Trigger.Paths) == 0 {
			logger.log(SeverityDebug, OpInfo, "Defaulting paths for job %s to .", job.Name)
			job.Trigger.Paths = []string{v.cwd}
			v.Jobs[i] = job
		}
	}
	if v.Config.BufferSize == 0 {
		logger.log(SeverityDebug, OpInfo, "Setting default buffer size to %d", 4096)
		v.Config.BufferSize = 4096
	}
	if v.Config.Severity == "" {
		logger.log(SeverityDebug, OpInfo, "Setting default severity to %s", SeverityWarn.String())
		v.Config.Severity = SeverityWarn.String()
	}
	if v.Config.Cooldown == 0 {
		logger.log(SeverityDebug, OpInfo, "Setting default cooldown to %s", (100 * time.Millisecond).String())
		v.Config.Cooldown = 100 * time.Millisecond
	}
}

// applyCLI applies configuration from command line arguments
func (v *Vai) applyCLI(args *Args) {
	cmds := parseFlags(args.CmdFlags, args.PositionalArgs)
	if len(cmds) == 0 {
		return
	}

	patterns := parseRegex(args.Regex)
	env := parseEnv(args.Env)
	path := parsePath(args.Path)

	var actions []Job
	for _, cmdStr := range cmds {
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}
		actions = append(actions, Job{
			Cmd:    parts[0],
			Params: parts[1:],
		})
	}

	job := Job{
		Series: actions,
	}

	// CLI explicitly sets trigger
	if path != "" || len(patterns) > 0 {
		job.Trigger = &Trigger{
			Paths: []string{path},
			Regex: patterns,
		}
	}

	if len(env) > 0 {
		job.Env = env
	}

	if v.Jobs == nil {
		v.Jobs = map[string]Job{}
	}

	// CLI defines / overrides the default job
	v.Jobs["default"] = job
}

// applyConfig loads configuration from a file
func (v *Vai) applyConfig(path string) {
	cfg, err := fromFile(path)
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to load config file: %v", err)
		os.Exit(1)
	}

	// Save runtime
	cwd := v.cwd

	// Apply config
	v.Config = cfg.Config
	v.Jobs = cfg.Jobs

	// Restore runtime
	v.cwd = cwd

	if v.Config.Severity != "" {
		logger = newLogger(parseSeverity(v.Config.Severity))
	}
}

// dispatch checks an event and triggers the ones that match
func (v *Vai) dispatch(eventPath string) {
	if len(v.Jobs) == 0 {
		logger.log(SeverityError, OpError, "No jobs to dispatch event to")
		return
	}

	for jobName, job := range v.Jobs {
		if job.Trigger == nil || len(job.Trigger.Paths) == 0 {
			logger.log(SeverityWarn, OpError, "Skipping job '%s': no paths defined", jobName)
			continue
		}

		// Check if the event path is in job's vai paths
		pathMatch := false
		absEventPath, _ := filepath.Abs(eventPath)
		canonicalEventPath, _ := filepath.EvalSymlinks(absEventPath)
		if canonicalEventPath == "" {
			canonicalEventPath = absEventPath
		}

		for _, watchPath := range job.Trigger.Paths {
			absWatchPath, _ := filepath.Abs(watchPath)
			canonicalWatchPath, _ := filepath.EvalSymlinks(absWatchPath)
			if canonicalWatchPath == "" {
				canonicalWatchPath = absWatchPath
			}

			if strings.HasPrefix(canonicalEventPath, canonicalWatchPath) {
				pathMatch = true
				break
			}
		}

		if !pathMatch {
			logger.log(SeverityDebug, OpWarn, "Skipping job '%s': event path '%s' is not in watched paths", jobName, eventPath)
			continue
		}

		// Check regex
		if !matchRegex(eventPath, job.Trigger.Regex) {
			logger.log(SeverityDebug, OpWarn, "Skipping job '%s': event path '%s' does not match regex", jobName, eventPath)
			continue
		}

		// Job is a match
		logger.log(SeverityDebug, OpSuccess, "Triggering job: %s", green("[", jobName, "]"))

		go func(name string, j Job) {
			// Register the job
			ctx, deregister := v.manager.register(name)
			j.Name = name

			defer deregister() // Deregister on complete
			j.start(ctx)
		}(jobName, job)
	}
}

// save writes the Vai configuration to a YAML file
func (v *Vai) save(filePath string) error {
	var b bytes.Buffer
	encoder := yaml.NewEncoder(&b)
	encoder.SetIndent(2)
	err := encoder.Encode(v)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, b.Bytes(), 0644)
}

// startWatch dispatches events to the appropriate jobs
func (v *Vai) startWatch(ctx context.Context) {
	var err error

	v.startJobs()

	v.fswatcher, err = v.newWatcher()
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to create watcher: %v", err)
		return
	}

	// Start the event listener
	go v.runEventLoop(ctx)

	// Start watching
	if err := v.fswatcher.Watch(ctx); err != nil {
		logger.log(SeverityError, OpError, "Failed to start vai: %v", err)
	}

	// Collect unique paths from all jobs
	pathsToWatch := make(map[string]struct{})
	for _, job := range v.Jobs {
		if job.Trigger != nil {
			for _, p := range job.Trigger.Paths {
				pathsToWatch[filepath.Clean(p)] = struct{}{}
			}
		}
	}

	// If no paths found from jobs, use current working directory as fallback
	if len(pathsToWatch) == 0 {
		pathsToWatch[v.cwd] = struct{}{}
	}

	// Add all paths to the watcher
	for path := range pathsToWatch {
		if err := v.fswatcher.AddPath(path); err != nil {
			if ctx.Err() == nil {
				logger.log(SeverityError, OpError, "Failed to watch path %s: %v", path, err)
			}
		}
	}
}

// runEventLoop listens for file events and dispatches them
func (v *Vai) runEventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-v.fswatcher.Events():
			if !ok {
				return
			}
			if v.Config.ClearCli {
				// Clear the console before displaying the change
				clearCLI()
			}

			// Determine the path to display
			displayPath := event.Path
			if len(v.cwd) > 0 {
				if relPath, err := filepath.Rel(v.cwd, event.Path); err == nil {
					displayPath = relPath
				}
			}

			logger.log(SeverityWarn, OpTrigger, "%s", purple(fmt.Sprintf("Change detected: %s", displayPath)))
			// Dispatch the event
			v.dispatch(event.Path)
		case err, ok := <-v.fswatcher.Dropped():
			if !ok {
				return
			}
			logger.log(SeverityError, OpError, "Watch error: %v", err)
		}
	}
}

// newWatcher sets up the file watcher
func (v *Vai) newWatcher() (fswatcher.Watcher, error) {
	// Aggregate regex patterns
	incRegex, excRegex := v.aggregateRegex()

	// Create a fswatcher instance
	opts := []fswatcher.WatcherOpt{
		fswatcher.WithCooldown(v.Config.Cooldown),
		fswatcher.WithBufferSize(v.Config.BufferSize),
	}
	if v.Config.BatchingDuration > 0 {
		opts = append(opts, fswatcher.WithEventBatching(v.Config.BatchingDuration))
	}
	if len(incRegex) > 0 {
		opts = append(opts, fswatcher.WithIncRegex(incRegex...))
	}
	if len(excRegex) > 0 {
		opts = append(opts, fswatcher.WithExcRegex(excRegex...))
	}
	if v.Config.Severity == SeverityDebug.String() {
		opts = append(opts, fswatcher.WithSeverity(fswatcher.SeverityDebug))
		opts = append(opts, fswatcher.WithLogFile("debug.log"))
	}

	return fswatcher.New(opts...)
}

// aggregateRegex collects all unique regex patterns from all jobs
func (v *Vai) aggregateRegex() (incPatterns, excPatterns []string) {
	incRegexMap := make(map[string]struct{})
	excRegexMap := make(map[string]struct{})

	for _, job := range v.Jobs {
		if job.Trigger != nil {
			for _, rx := range job.Trigger.Regex {
				if trimmedRx, found := strings.CutPrefix(rx, "!"); found {
					excRegexMap[trimmedRx] = struct{}{}
				} else {
					incRegexMap[rx] = struct{}{}
				}
			}
		}
	}

	incPatterns = make([]string, 0, len(incRegexMap))
	for rx := range incRegexMap {
		incPatterns = append(incPatterns, rx)
	}

	excPatterns = make([]string, 0, len(excRegexMap))
	for rx := range excRegexMap {
		excPatterns = append(excPatterns, rx)
	}

	return incPatterns, excPatterns
}

// clearCLI clears the cli
func clearCLI() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

// fileExists checks if a file exists and is not a dir
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// parsePath parses the path to watch
func parsePath(pathFlag string) string {
	if pathFlag != "" {
		return pathFlag
	}
	cwd, _ := os.Getwd()
	return cwd
}

// parseRegex determines the file patterns to watch
func parseRegex(regexFlag string) []string {
	if regexFlag != "" {
		patterns := strings.Split(regexFlag, ",")
		for i, p := range patterns {
			patterns[i] = strings.TrimSpace(p)
		}
		return patterns
	}
	// Default patterns
	return []string{".*\\.go$", "^go\\.mod$", "^go\\.sum$"}
}

// fromFile loads a Workflow from a YAML configuration file
func fromFile(filePath string) (*Vai, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var vai Vai
	if err := yaml.Unmarshal(data, &vai); err != nil {
		return nil, err
	}
	for name, job := range vai.Jobs {
		job.Name = name
		vai.Jobs[name] = job
	}
	return &vai, nil
}

// parseEnv parses the env variables
func parseEnv(envFlag string) map[string]string {
	envMap := make(map[string]string)

	if envFlag != "" {
		for pair := range strings.SplitSeq(envFlag, ",") {
			trimmedPair := strings.TrimSpace(pair)
			if key, value, found := strings.Cut(trimmedPair, "="); found {
				envMap[key] = value
			}
		}
	}
	return envMap
}

// matchRegex checks if the file matches the regex patterns
func matchRegex(path string, regex []string) bool {
	if len(regex) == 0 {
		return true
	}

	included := false
	hasInclusion := false

	for _, rx := range regex {
		// Check for exclusion
		if strings.HasPrefix(rx, "!") {
			pattern := strings.TrimPrefix(rx, "!")
			matched, err := regexp.MatchString(pattern, path)
			if err == nil && matched {
				return false
			}
			continue
		}

		// Check for inclusion
		hasInclusion = true
		matched, err := regexp.MatchString(rx, path)
		if err == nil && matched {
			included = true
		}
	}

	// If no inclusion patterns are defined, we default to including (unless excluded above)
	if !hasInclusion {
		return true
	}

	return included
}

// parseFlags determines the commands to run and the flags to use
func parseFlags(cmdFlags, positionalArgs []string) []string {
	if len(cmdFlags) > 0 {
		// --cmd flags take precedence
		return cmdFlags
	}
	if len(positionalArgs) > 0 {
		// Positional args are treated as a single command with args
		return []string{strings.Join(positionalArgs, " ")}
	}
	logger.log(SeverityError, OpError, "No command provided, use --help for usage details")
	return nil
}
