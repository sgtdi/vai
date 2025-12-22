package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sgtdi/fswatcher"
	"gopkg.in/yaml.v3"
)

// Vai contains vai fields
type Vai struct {
	Config     Config            `yaml:"config"`
	Jobs       map[string]Job    `yaml:"jobs"`
	jobManager *JobManager       `yaml:"-"`
	fswatcher  fswatcher.Watcher `yaml:"-"`
}

// Save writes to a YAML file
func (w *Vai) Save(filePath string) error {
	data, err := yaml.Marshal(w)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// SetDefaults applies default values
func (w *Vai) SetDefaults() {
	if w.Config.Path == "" {
		w.Config.Path = "."
	}
	for i, job := range w.Jobs {
		if job.On != nil && len(job.On.Paths) == 0 {
			job.On.Paths = []string{w.Config.Path}
			w.Jobs[i] = job
		}
	}
	if w.Config.BufferSize == 0 {
		w.Config.BufferSize = 4096
	}
	if w.Config.LogLevel == "" {
		w.Config.LogLevel = "warn"
	}
	if w.Config.Cooldown == 0 {
		w.Config.Cooldown = 100 * time.Millisecond
	}
}

// aggregateRegex collects all unique regex patterns from all jobs
func aggregateRegex(vai *Vai) (incPatterns, excPatterns []string) {
	incRegexMap := make(map[string]struct{})
	excRegexMap := make(map[string]struct{})

	for _, job := range vai.Jobs {
		if job.On != nil {
			for _, rx := range job.On.Regex {
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
	Logf(SeveritySuccess, "Jobs successfully imported: %s%s%s", ColorGreen, strings.Join(jobNames, ", "), ColorReset)

	// Run jobs on startup
	Log(SeverityInfo, "Running jobs on startup...")
	for jobName, job := range w.Jobs {
		Logf(SeveritySuccess, "Triggering job: %s%s%s", ColorGreen, jobName, ColorReset)
		jobCtx, deregister := w.jobManager.Register(jobName, w.Config.Debug)
		go func(j Job) {
			defer deregister()
			Execute(jobCtx, j)
		}(job)
	}

	if w.Config.Path == "" {
		Log(SeverityWarn, "No path defined, nothing to vai.")
		return
	}

	// Aggregate regex patterns
	incRegex, excRegex := aggregateRegex(w)

	// Create a fswatcher instance
	opts := []fswatcher.WatcherOpt{
		fswatcher.WithCooldown(w.Config.Cooldown),
		fswatcher.WithBufferSize(w.Config.BufferSize),
		fswatcher.WithLogSeverity(logLevelString(w.Config.LogLevel)),
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
	if w.Config.Debug {
		opts = append(opts, fswatcher.WithLogSeverity(fswatcher.SeverityDebug))
		opts = append(opts, fswatcher.WithLogFile("debug.log"))
	}

	w.fswatcher, err = fswatcher.New(opts...)
	if err != nil {
		Logf(SeverityError, "Failed to create watcher: %v", err)
		return
	}

	// Get the current working directory to display relative paths
	cwd, err := os.Getwd()
	if err != nil {
		Logf(SeverityWarn, "Could not get current working directory: %v", err)
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
				ClearConsole()

				// Determine the path to display
				displayPath := event.Path
				if cwd != "" {
					if relPath, err := filepath.Rel(cwd, event.Path); err == nil {
						displayPath = relPath
					}
				}

				Logf(SeveritySuccess, "Change detected: %s%s%s", ColorGreen, displayPath, ColorReset)
				// Dispatch the event
				dispatch(event.Path, w)
			case err, ok := <-w.fswatcher.Dropped():
				if !ok {
					return
				}
				Logf(SeverityError, "Watch error: %v", err)
			}
		}
	}()

	// Start watching
	if err := w.fswatcher.Watch(ctx); err != nil {
		Logf(SeverityError, "Failed to start vai: %v", err)
	}

	// Add the global path
	if err := w.fswatcher.AddPath(w.Config.Path); err != nil {
		if ctx.Err() == nil {
			Logf(SeverityError, "Failed to vai path %s: %v", w.Config.Path, err)
		}
	}
}

// dispatch checks an event and triggers the ones that match
func dispatch(eventPath string, w *Vai) {
	if len(w.Jobs) == 0 {
		if w.Config.Debug {
			Logf(SeverityWarn, "No jobs to dispatch event to")
		}
		return
	}

	for jobName, job := range w.Jobs {
		if job.On == nil || len(job.On.Paths) == 0 {
			if w.Config.Debug {
				Logf(SeverityWarn, "Skipping job '%s': no paths defined", jobName)
			}
			continue
		}

		// Check if the event path is in job's vai paths
		pathMatch := false
		for _, watchPath := range w.fswatcher.Paths() {
			if strings.HasPrefix(eventPath, watchPath) {
				pathMatch = true
				break
			}
		}

		if !pathMatch {
			if w.Config.Debug {
				Logf(SeverityWarn, "Skipping job '%s': event path '%s' is not in vai paths %v", jobName, eventPath, w.fswatcher.Paths())
			}
			continue
		}

		// Job is a match
		Logf(SeveritySuccess, "Triggering job: %s%s%s", ColorGreen, jobName, ColorReset)

		// Register the job
		ctx, deregister := w.jobManager.Register(jobName, w.Config.Debug)

		// Run the job
		go func(j Job) {
			defer deregister() // Deregister on complete
			Execute(ctx, j)
		}(job)
	}
}
