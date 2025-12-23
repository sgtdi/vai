package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// parallelCtxKey is used to indicate parallel execution
type parallelCtxKey struct{}

var (
	runningProcesses = make(map[string][]*exec.Cmd)
	processMutex     = &sync.Mutex{}
)

// Execute runs a given job
func Execute(ctx context.Context, job Job, debug bool) {
	// Execute 'Before' jobs
	for _, beforeJob := range job.Before {
		select {
		case <-ctx.Done():
			return
		default:
			Execute(ctx, beforeJob, debug)
		}
	}

	// Execute
	executeJob(ctx, job, debug)

	// Execute 'After' jobs
	for _, afterJob := range job.After {
		select {
		case <-ctx.Done():
			return
		default:
			Execute(ctx, afterJob, debug)
		}
	}
}

// stopCommand stops a running command by its job name
func stopCommand(jobName string, debug bool) <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		processMutex.Lock()
		cmds, ok := runningProcesses[jobName]
		if !ok {
			processMutex.Unlock()
			return
		}
		delete(runningProcesses, jobName)
		if debug {
			Logf(SeverityInfo, "Executor: Removed job %s from running processes map.", jobName)
		}
		processMutex.Unlock()

		for _, cmd := range cmds {
			if cmd.Process == nil {
				continue
			}

			if debug {
				Logf(SeverityInfo, "Executor: Stopping process with PID: %d for job: %s", cmd.Process.Pid, jobName)
			}
			// Kill the process group to ensure child processes are also killed
			err := killProcess(cmd)
			if err != nil {
				Logf(SeverityError, "Failed to stop process: %v", err)
			} else if debug {
				Logf(SeverityInfo, "Executor: Successfully sent kill signal to PID: %d", cmd.Process.Pid)
			}
			// Wait for the process to exit to release resources
			if debug {
				Logf(SeverityInfo, "Executor: Waiting for process %d to exit...", cmd.Process.Pid)
			}
			_, _ = cmd.Process.Wait()
			if debug {
				Logf(SeverityInfo, "Executor: Process %d finished waiting.", cmd.Process.Pid)
			}
		}
		time.Sleep(1 * time.Second)
	}()
	return stopped
}

// executeJob handles the core execution
func executeJob(ctx context.Context, job Job, debug bool) {
	select {
	case <-ctx.Done():
		return // Job was canceled
	default:
		// Continue
	}

	if job.Cmd != "" {
		runCommand(ctx, job, debug)
	} else if len(job.Series) > 0 {
		for i := range job.Series {
			seriesJob := &job.Series[i]
			seriesJob.Name = job.Name
			Execute(ctx, *seriesJob, debug)
		}
	} else if len(job.Parallel) > 0 {
		var commandStrings []string
		for _, pJob := range job.Parallel {
			cmdStr := pJob.Cmd
			if len(pJob.Params) > 0 {
				cmdStr += " " + strings.Join(pJob.Params, " ")
			}
			commandStrings = append(commandStrings, fmt.Sprintf("%s[%s]%s", ColorYellow, cmdStr, ColorReset))
		}
		Logf(SeverityWarn, "Running cmds: %s", strings.Join(commandStrings, ", "))

		var wg sync.WaitGroup
		for i := range job.Parallel {
			jobToRun := job.Parallel[i]
			jobToRun.Name = job.Name
			wg.Add(1)
			go func(j Job) {
				defer wg.Done()
				pCtx := context.WithValue(ctx, parallelCtxKey{}, true)
				Execute(pCtx, j, debug)
			}(jobToRun)
		}
		wg.Wait()
	}
}

// ClearConsole clears the cli, it's by default to true
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
func runCommand(ctx context.Context, job Job, debug bool) {
	if p, _ := ctx.Value(parallelCtxKey{}).(bool); !p {
		Logf(SeverityWarn, "Running cmd: %s%s %v%s", ColorYellow, job.Cmd, job.Params, ColorReset)
	}
	cmd := exec.CommandContext(ctx, job.Cmd, job.Params...)

	// Set up environment variables
	cmd.Env = os.Environ()
	for key, val := range job.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	// Set the process group ID
	setpgid(cmd)

	// Capture stdout and stderr
	var outputBuf bytes.Buffer
	cmd.Stdout = &outputBuf
	cmd.Stderr = &outputBuf

	// Run and wait
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		if ctx.Err() == nil {
			Logf(SeverityError, "Failed to start cmd: %v", err)
		}
		return
	}

	if debug {
		Logf(SeverityInfo, "Executor: Started new process with PID: %d for job: %s", cmd.Process.Pid, job.Name)
	}

	// Register the command
	if job.Name != "" {
		processMutex.Lock()
		runningProcesses[job.Name] = append(runningProcesses[job.Name], cmd)
		processMutex.Unlock()
	}

	err := cmd.Wait()
	duration := time.Since(startTime)

	// If the command finishes on its own, remove it from the running processes map
	if job.Name != "" {
		processMutex.Lock()
		// Find and remove the specific command from the slice
		if cmds, ok := runningProcesses[job.Name]; ok {
			for i, c := range cmds {
				if c == cmd {
					runningProcesses[job.Name] = slices.Delete(cmds, i, i+1)
					break
				}
			}
		}
		processMutex.Unlock()
	}

	cmdStr := job.Cmd
	if len(job.Params) > 0 {
		cmdStr += " " + strings.Join(job.Params, " ")
	}

	if err != nil {
		// Killed by the context
		if ctx.Err() == nil {
			Logf(SeverityError, "Cmd with error: %s[%s]%s %v (%s%s%s)", ColorGreen, cmdStr, ColorReset, err, ColorCyan, duration.Round(time.Millisecond), ColorReset)
		}
	} else {
		Logf(SeveritySuccess, "Cmd successfully: %s[%s]%s (%s%s%s)", ColorGreen, cmdStr, ColorReset, ColorCyan, duration.Round(time.Millisecond), ColorReset)
	}

	// Print the captured output
	if outputBuf.Len() > 0 {
		fmt.Printf("%s%s%s", ColorGray, outputBuf.String(), ColorReset)
	}
}
