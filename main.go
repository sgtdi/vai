package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/sgtdi/fswatcher"
)

var version = "1.1.1"
var logger *Logger

// Vai contains vai fields
type Vai struct {
	Config     Config            `yaml:"config"`
	Jobs       map[string]Job    `yaml:"jobs"`
	jobManager *JobManager       `yaml:"-"`
	fswatcher  fswatcher.Watcher `yaml:"-"`
}

// Config options for file vai.yml
type Config struct {
	Path             string        `yaml:"path"`
	Severity         string        `yaml:"severity,omitempty"`
	ClearCli         bool          `yaml:"clearCli,omitempty"`
	Cooldown         time.Duration `yaml:"cooldown,omitempty"`
	BufferSize       int           `yaml:"bufferSize,omitempty"`
	BatchingDuration time.Duration `yaml:"batchingDuration,omitempty"`

	serverityLevel fswatcher.Severity
}

func main() {
	args := os.Args[1:]

	cmdFlags, positionalArgs, path, regex, env, configFile, saveFile, help, debug, versionFlag, saveIsSet := parseCLIArgs(args)

	severity := SeverityWarn
	if debug {
		severity = SeverityDebug
	}
	logger = New(severity)

	fmt.Print(purple("\n--------------\n"))
	fmt.Printf("%sVai v%s%s\n", ColorPurple, version, ColorPurple)
	fmt.Print(purple("--------------\n\n"))

	// Print current version and exit
	if versionFlag {
		os.Exit(0)
	}

	v := NewVai(
		cmdFlags,
		positionalArgs,
		path,
		regex,
		env,
		configFile,
		help,
		severity,
	)

	v.jobManager = NewJobManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		logger.log(SeverityDebug, OpSuccess, "Shutdown signal received")
		cancel()
	}()

	// Start the watcher in a goroutine
	var wg sync.WaitGroup
	wg.Go(func() {
		startWatch(ctx, v)
	})

	logger.log(SeverityWarn, OpSuccess, "File watcher started...")

	// Wait for the context to be canceled
	<-ctx.Done()

	// Wait for the watcher to finish
	wg.Wait()

	logger.log(SeverityInfo, OpWarn, "Shutting down...")
	v.jobManager.StopAll()

	if saveIsSet {
		logger.log(SeverityInfo, OpWarn, "Saving configuration to %s...", saveFile)
		if err := v.Save(saveFile); err != nil {
			logger.log(SeverityError, OpError, "Failed to save config file: %v", err)
		}
		logger.log(SeverityInfo, OpSuccess, "Configuration saved successfully")

	}
}

func parseCLIArgs(args []string) (cmdFlags, positionalArgs []string, path, regex, env, configFile, saveFile string, help, debug, versionFlag, saveIsSet bool) {
	configFile = "vai.yml"
	saveFile = "vai.yml"

	knownFlagsWithArg := map[string]bool{
		"cmd": true, "path": true, "env": true, "regex": true,
	}
	knownBoolFlags := map[string]bool{
		"help": true, "debug": true, "version": true, "save": true,
	}
	shortFlags := map[string]string{
		"c": "cmd", "p": "path", "e": "env", "r": "regex", "s": "save",
		"h": "help", "d": "debug", "v": "version",
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		isKnownFlag, flagName := identifyFlag(arg, knownFlagsWithArg, knownBoolFlags, shortFlags)

		if isKnownFlag {
			if flagName == "cmd" {
				var cmd string
				cmd, i = parseCmdFlag(args, i, knownFlagsWithArg, knownBoolFlags)
				if cmd != "" {
					cmdFlags = append(cmdFlags, cmd)
				}
			} else if knownFlagsWithArg[flagName] {
				var value string
				value, i = parseValueFlag(args, i)
				switch flagName {
				case "regex":
					regex = value
				case "env":
					env = value
				case "path":
					path = value
				}
			} else if knownBoolFlags[flagName] {
				switch flagName {
				case "help":
					help = true
				case "debug":
					debug = true
				case "version":
					versionFlag = true
				case "save":
					saveIsSet = true
				}
			}
		} else {
			// The rest of the args belong to the cmd
			positionalArgs = args[i:]
			break
		}
		i++
	}
	return
}

func identifyFlag(arg string, knownFlagsWithArg, knownBoolFlags map[string]bool, shortFlags map[string]string) (bool, string) {
	if name, found := strings.CutPrefix(arg, "--"); found {
		if knownFlagsWithArg[name] || knownBoolFlags[name] {
			return true, name
		}
	} else if name, found := strings.CutPrefix(arg, "-"); found {
		if longName, ok := shortFlags[name]; ok {
			return true, longName
		}
	}
	return false, ""
}

func parseCmdFlag(args []string, currentIndex int, knownFlagsWithArg, knownBoolFlags map[string]bool) (string, int) {
	var cmdParts []string
	i := currentIndex + 1
	for i < len(args) {
		nextArg := args[i]
		isNextArgAFlag := false
		if strings.HasPrefix(nextArg, "-") {
			nextFlagName := strings.TrimLeft(nextArg, "-")
			if knownFlagsWithArg[nextFlagName] || knownBoolFlags[nextFlagName] {
				isNextArgAFlag = true
			}
		}

		if isNextArgAFlag {
			i--
			break
		}
		cmdParts = append(cmdParts, nextArg)
		i++
	}
	if len(cmdParts) > 0 {
		return strings.Join(cmdParts, " "), i
	}
	return "", i
}

func parseValueFlag(args []string, currentIndex int) (string, int) {
	if currentIndex+1 < len(args) && !strings.HasPrefix(args[currentIndex+1], "-") {
		return args[currentIndex+1], currentIndex + 1
	}
	return "", currentIndex
}

// NewVai parse config struct with all possible flags and args
func NewVai(cmdFlags, positionalArgs []string, path, regex, env, configFile string, help bool, severity Severity) *Vai {
	var err error
	v := &Vai{}

	// Handle help flag
	if help {
		v.printHelp()
	}

	if len(cmdFlags) > 0 || len(positionalArgs) > 0 {
		logger.log(SeverityDebug, OpInfo, "Loading commands from CLI")
		seriesCmds, singleCmd := v.handleCmds(cmdFlags, positionalArgs)
		patterns := parseRegex(regex)
		envMap := parseEnv(env)

		// Default to current directory if path is not specified in CLI mode
		cliPath := path
		if cliPath == "" {
			cliPath = "."
		}
		v = FromCLI(seriesCmds, singleCmd, cliPath, patterns, envMap)
	} else {
		// Fallback to config with no cmds
		if fileExists(configFile) {
			logger.log(SeverityDebug, OpInfo, "Loading config from file")
			v, err = FromFile(configFile, path)
			if err != nil {
				logger.log(SeverityError, OpError, "Failed to load config file: %v", err)
				os.Exit(1)
			}
			// Update logger level if config file has severity and no CLI override
			if v.Config.Severity != "" && severity == SeverityWarn {
				logger = New(ParseSeverity(v.Config.Severity))
			}
			logger.log(SeverityInfo, OpSuccess, "Using config %s", cyan(configFile))
		} else {
			// If none show help
			logger.log(SeverityError, OpError, "No config file found and no command given")
			v.printHelp()
		}
	}

	// Set debug verbosity if requested by flag
	if severity == SeverityDebug {
		v.Config.Severity = severity.String()
	}

	// Set defaults values
	v.SetDefaults()
	// Print current Vai configuration
	if v.Config.Severity == SeverityDebug.String() {
		v.printConfig()
	}
	return v
}

// printHelp prints usage help info
func (v *Vai) printHelp() {
	// Usage
	fmt.Println(
		yellow("Usage:"),
		"vai",
		cyan("[flags]"),
		cyan("[command...]..."),
	)

	fmt.Println()
	fmt.Println("A tool to run commands when files change, configured via CLI or a vai.yml file")

	// Configuration Modes
	fmt.Println()
	fmt.Println(yellow("Configuration Modes:"))

	fmt.Println(
		"  1.",
		white("CLI Mode:"),
		"Provide a command directly (e.g., `vai go run .`)",
	)

	fmt.Println(
		"  2.",
		white("File Mode:"),
		"Use a vai.yml file for complex workflows (e.g., `vai`)",
	)

	// Flags
	fmt.Println()
	fmt.Println(yellow("Flags:"))

	fmt.Println(
		"  ",
		cyan("-c, --cmd"),
		"<command>",
		"Command to run. Can be specified multiple times",
	)

	fmt.Println(
		"  ",
		cyan("-p, --path"),
		"<path>",
		"Path to watch. (default: .)",
	)

	fmt.Println(
		"  ",
		cyan("-e, --env"),
		"<vars>",
		"KEY=VALUE environment variables",
	)

	fmt.Println(
		"  ",
		cyan("-r, --regex"),
		"<patterns>",
		"Glob patterns to watch",
	)

	fmt.Println(
		"  ",
		cyan("-s, --save"),
		"Save CLI flags to a new vai.yml file and exit",
	)

	fmt.Println(
		"  ",
		cyan("-h, --help"),
		"Show this help message",
	)

	os.Exit(0)
}

// printConfig prints the current config
func (v *Vai) printConfig() {
	fmt.Println(yellow("--- Global Config ---"))

	fmt.Println(cyan("- Path:"), v.Config.Path)
	fmt.Println(cyan("- Cooldown:"), v.Config.Cooldown)
	fmt.Println(cyan("- Batching Duration:"), v.Config.BatchingDuration)
	fmt.Println(cyan("- Buffer Size:"), v.Config.BufferSize)
	fmt.Println(cyan("- Severity:"), v.Config.Severity)
	fmt.Println(cyan("- Clear CLI:"), v.Config.ClearCli)

	fmt.Println(yellow("---------------------"))

	if len(v.Jobs) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(yellow("--- Jobs ---"))

	for name, job := range v.Jobs {
		fmt.Println(cyan("- Job:"), name)

		if job.Trigger != nil {
			if len(job.Trigger.Paths) > 0 {
				fmt.Println(
					"  ",
					cyan("- Watch Paths:"),
					strings.Join(job.Trigger.Paths, ", "),
				)
			}

			if len(job.Trigger.Regex) > 0 {
				fmt.Println(
					"  ",
					cyan("- Inclusion Regex:"),
					strings.Join(job.Trigger.Regex, ", "),
				)
			}
		}

		if len(job.Series) > 0 {
			fmt.Println("  ", cyan("- Commands:"))

			var printSubJobs func([]Job, string)
			printSubJobs = func(jobs []Job, indent string) {
				for _, j := range jobs {
					if j.Cmd != "" {
						cmd := j.Cmd
						if len(j.Params) > 0 {
							cmd += " " + strings.Join(j.Params, " ")
						}
						fmt.Println(indent, white("- ", cmd))
					} else if len(j.Parallel) > 0 {
						fmt.Println(indent, white("- Parallel:"))
						printSubJobs(j.Parallel, indent+"  ")
					} else if len(j.Series) > 0 {
						fmt.Println(indent, white("- Series:"))
						printSubJobs(j.Series, indent+"  ")
					}
				}
			}
			printSubJobs(job.Series, "    ")
		}

		if len(job.Env) > 0 {
			fmt.Println("  ", cyan("- Environment:"))

			for key, val := range job.Env {
				fmt.Println(
					"    ",
					white("- ", key+":"),
					val,
				)
			}
		}
	}

	fmt.Println(yellow("------------"))
}

// handleCmds determines the commands to run
func (v *Vai) handleCmds(cmdFlags, positionalArgs []string) ([]string, []string) {
	if len(cmdFlags) > 0 {
		// --cmd flags take precedence
		return cmdFlags, nil
	}
	if len(positionalArgs) > 0 {
		// Positional args are treated as a single command with args
		return nil, positionalArgs
	}
	logger.log(SeverityError, OpError, "No command provided, use --help for usage details")
	os.Exit(1)
	return nil, nil
}

// fileExists checks if a file exists and is not a dir
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
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

// handleEnv parses the env variables
func parseEnv(envFlag string) map[string]string {
	envMap := make(map[string]string)

	if envFlag != "" {
		pairs := strings.Split(envFlag, ",")
		for _, pair := range pairs {
			trimmedPair := strings.TrimSpace(pair)
			if key, value, found := strings.Cut(trimmedPair, "="); found {
				envMap[key] = value
			}
		}
	}
	return envMap
}
