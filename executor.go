package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

var (
	runningProcesses = make(map[string]*exec.Cmd)
	processMutex     = &sync.Mutex{}
)

// Execute runs a given job
func Execute(ctx context.Context, job Job) {
	// Execute 'Before' jobs
	for _, beforeJob := range job.Before {
		select {
		case <-ctx.Done():
			return
		default:
			Execute(ctx, beforeJob)
		}
	}

	// Execute
	executeJob(ctx, job)

	// Execute 'After' jobs
	for _, afterJob := range job.After {
		select {
		case <-ctx.Done():
			return
		default:
			Execute(ctx, afterJob)
		}
	}
}

// stopCommand stops a running command by its job name
func stopCommand(jobName string, debug bool) {
	processMutex.Lock()
	defer processMutex.Unlock()

	if cmd, ok := runningProcesses[jobName]; ok {
		if cmd.Process != nil {
			if debug {
				Logf(SeverityWarn, "Stopping process with PID: %d", cmd.Process.Pid)
			}
			// Kill the process group to ensure child processes are also killed
			err := killProcess(cmd)
			if err != nil {
				Logf(SeverityError, "Failed to stop process: %v", err)
			}
			// Wait for the process to exit to release resources
			_, _ = cmd.Process.Wait()
		}
	}
}

// executeJob handles the core execution
func executeJob(ctx context.Context, job Job) {
	select {
	case <-ctx.Done():
		return // Job was canceled
	default:
		// Continue
	}

	if job.Cmd != "" {
		runCommand(ctx, job)
	} else if len(job.Series) > 0 {
		for _, seriesJob := range job.Series {
			seriesJob.Name = job.Name
			Execute(ctx, seriesJob)
		}
	} else if len(job.Parallel) > 0 {
		var wg sync.WaitGroup
		for _, parallelJob := range job.Parallel {
			jobToRun := parallelJob
			jobToRun.Name = job.Name
			wg.Go(func() {
				Execute(ctx, jobToRun)
			})
		}
		wg.Wait()
	}
}

// ClearConsole clears the cli
func ClearConsole() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

// runCommand executes the command and streams its output
func runCommand(ctx context.Context, job Job) {
	Logf(SeveritySuccess, "Running command: %s%s %v%s", ColorGreen, job.Cmd, job.Params, ColorReset)
	cmd := exec.CommandContext(ctx, job.Cmd, job.Params...)

	// Set up environment variables
	cmd.Env = os.Environ()
	for key, val := range job.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	// Set the process group ID
	setpgid(cmd)

	// stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run and wait
	if err := cmd.Start(); err != nil {
		if ctx.Err() == nil {
			Logf(SeverityError, "Failed to start command: %v", err)
		}
		return
	}

	// Register the command
	if job.Name != "" {
		processMutex.Lock()
		runningProcesses[job.Name] = cmd
		processMutex.Unlock()

		// Unregister the command when it's done
		defer func() {
			processMutex.Lock()
			delete(runningProcesses, job.Name)
			processMutex.Unlock()
		}()
	}

	err := cmd.Wait()
	if err != nil {
		// Killed by the context
		if ctx.Err() == nil {
			Logf(SeverityError, "Command finished with error: %v", err)
		}
	} else {
		Logf(SeveritySuccess, "Command finished successfully: %s", job.Cmd)
	}
}
