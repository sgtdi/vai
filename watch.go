package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sgtdi/fswatcher"
	"gopkg.in/yaml.v3"
)

// Save writes the Vai configuration to a YAML file
func (w *Vai) Save(filePath string) error {
	var b bytes.Buffer
	encoder := yaml.NewEncoder(&b)
	encoder.SetIndent(2)
	err := encoder.Encode(w)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, b.Bytes(), 0644)
}

// SetDefaults applies default values to the Vai configuration
func (v *Vai) SetDefaults() {
	if v.Config.Path == "" {
		v.Config.Path = "."
	}
	for i, job := range v.Jobs {
		if job.Trigger != nil && len(job.Trigger.Paths) == 0 {
			logger.log(SeverityDebug, OpInfo, "Defaulting paths for job %s to %s", job.Name, v.Config.Path)
			job.Trigger.Paths = []string{v.Config.Path}
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

// aggregateRegex collects all unique regex patterns from all jobs
func aggregateRegex(vai *Vai) (incPatterns, excPatterns []string) {
	incRegexMap := make(map[string]struct{})
	excRegexMap := make(map[string]struct{})

	for _, job := range vai.Jobs {
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

// startWatch dispatches events to the appropriate jobs
func startWatch(ctx context.Context, w *Vai) {
	var err error
	var jobNames []string
	for jobName := range w.Jobs {
		jobNames = append(jobNames, jobName)
	}

	logger.log(SeverityInfo, OpSuccess, "Jobs successfully imported: %s%s%s", ColorGreen, strings.Join(jobNames, ", "), ColorReset)
	// Run jobs on startup
	logger.log(SeverityInfo, OpWarn, "Running jobs...")
	for jobName, job := range w.Jobs {
		logger.log(SeverityInfo, OpWarn, "Triggering job: %s%s%s", ColorGreen, jobName, ColorReset)
		jobCtx, deregister := w.jobManager.Register(jobName)
		job.Name = jobName
		go func(j Job) {
			defer deregister()
			Execute(jobCtx, j)
		}(job)
	}

	if w.Config.Path == "" {
		logger.log(SeverityError, OpError, "No path defined, nothing to vai")
		return
	}

	// Aggregate regex patterns
	incRegex, excRegex := aggregateRegex(w)

	// Create a fswatcher instance
	opts := []fswatcher.WatcherOpt{
		fswatcher.WithCooldown(w.Config.Cooldown),
		fswatcher.WithBufferSize(w.Config.BufferSize),
	}
	if w.Config.BatchingDuration > 0 {
		opts = append(opts, fswatcher.WithEventBatching(w.Config.BatchingDuration))
	}
	if len(incRegex) > 0 {
		opts = append(opts, fswatcher.WithIncRegex(incRegex...))
	}
	if len(excRegex) > 0 {
		opts = append(opts, fswatcher.WithExcRegex(excRegex...))
	}
	if w.Config.Severity == SeverityDebug.String() {
		opts = append(opts, fswatcher.WithSeverity(fswatcher.SeverityDebug))
		opts = append(opts, fswatcher.WithLogFile("debug.log"))
	}

	w.fswatcher, err = fswatcher.New(opts...)
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to create watcher: %v", err)
		return
	}

	// Get the current working directory to display relative paths
	cwd, err := os.Getwd()
	if err != nil {
		logger.log(SeverityError, OpError, "Could not get current working directory: %v", err)
		cwd = "" // Ensure cwd is empty on error
	}

	// Start the event listener
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.fswatcher.Events():
				if !ok {
					return
				}
				if w.Config.ClearCli {
					// Clear the console before displaying the change
					ClearConsole()
				}

				// Determine the path to display
				displayPath := event.Path
				if len(cwd) > 0 {
					if relPath, err := filepath.Rel(cwd, event.Path); err == nil {
						displayPath = relPath
					}
				}

				logger.log(SeverityWarn, OpTrigger, "%s", purple(fmt.Sprintf("Change detected: %s", displayPath)))
				// Dispatch the event
				dispatch(event.Path, w)
			case err, ok := <-w.fswatcher.Dropped():
				if !ok {
					return
				}
				logger.log(SeverityError, OpError, "Watch error: %v", err)
			}
		}
	}()

	// Start watching
	if err := w.fswatcher.Watch(ctx); err != nil {
		logger.log(SeverityError, OpError, "Failed to start vai: %v", err)
	}

	// Add the global path
	if err := w.fswatcher.AddPath(w.Config.Path); err != nil {
		if ctx.Err() == nil {
			logger.log(SeverityError, OpError, "Failed to vai path %s: %v", w.Config.Path, err)
		}
	}
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

// dispatch checks an event and triggers the ones that match
func dispatch(eventPath string, w *Vai) {
	if len(w.Jobs) == 0 {
		logger.log(SeverityError, OpError, "No jobs to dispatch event to")
		return
	}

	for jobName, job := range w.Jobs {
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
			ctx, deregister := w.jobManager.Register(name)
			j.Name = name

			defer deregister() // Deregister on complete
			Execute(ctx, j)
		}(jobName, job)
	}
}
