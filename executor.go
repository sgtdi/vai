package main

import (
	"context"
	"fmt"
	"io"
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
func stopCommand(jobName string) <-chan struct{} {
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
		logger.log(SeverityDebug, OpSuccess, "Executor: Removed job %s from running processes map.", jobName)
		processMutex.Unlock()

		for _, cmd := range cmds {
			if cmd.Process == nil {
				continue
			}
			logger.log(SeverityInfo, OpSuccess, "Executor: Stopping process with PID: %d for job: %s", cmd.Process.Pid, jobName)
			// Kill the process group to ensure child processes are also killed
			err := killProcess(cmd)
			if err != nil {
				logger.log(SeverityError, OpError, "Failed to stop process: %v", err)
			} else {
				logger.log(SeverityInfo, OpSuccess, "Executor: Successfully sent kill signal to PID: %d", cmd.Process.Pid)
			}
		}
	}()
	return stopped
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
		for i := range job.Series {
			seriesJob := &job.Series[i]
			seriesJob.Name = job.Name
			Execute(ctx, *seriesJob)
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
		logger.log(SeverityWarn, OpWarn, "Running cmds: %s", strings.Join(commandStrings, ", "))

		var wg sync.WaitGroup
		for i := range job.Parallel {
			jobToRun := job.Parallel[i]
			jobToRun.Name = job.Name
			wg.Add(1)
			go func(j Job) {
				defer wg.Done()
				pCtx := context.WithValue(ctx, parallelCtxKey{}, true)
				Execute(pCtx, j)
			}(jobToRun)
		}
		wg.Wait()
	}
}

// ClearConsole clears the cli
func ClearConsole() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

// runCommand executes the command and streams its output
func runCommand(ctx context.Context, job Job) {
	if p, _ := ctx.Value(parallelCtxKey{}).(bool); !p {
		logger.log(SeverityWarn, OpWarn, "Running cmd: %s", yellow(job.Cmd, " ", job.Params))
	}
	cmd := exec.CommandContext(ctx, job.Cmd, job.Params...)

	// Set up environment variables
	cmd.Env = os.Environ()
	for key, val := range job.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	// Set the process group ID
	setpgid(cmd)

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to create stdout pipe: %v", err)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to create stderr pipe: %v", err)
		return
	}

	// Run and wait
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		if ctx.Err() == nil {
			logger.log(SeverityError, OpError, "Failed to start cmd: %v", err)
		}
		return
	}
	logger.log(SeverityDebug, OpWarn, "Executor: Started new process with PID: %d for job: %s", cmd.Process.Pid, job.Name)

	streamOutput := func(reader io.Reader, writer io.Writer) {
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				fmt.Fprint(writer, gray(string(buf[:n])))
			}
			if err != nil {
				break
			}
		}
	}

	// Stream stdout and stderr in goroutines
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamOutput(stdoutPipe, os.Stdout)
	}()
	go func() {
		defer wg.Done()
		streamOutput(stderrPipe, os.Stderr)
	}()

	// Register the command
	if job.Name != "" {
		processMutex.Lock()
		runningProcesses[job.Name] = append(runningProcesses[job.Name], cmd)
		processMutex.Unlock()
	}

	err = cmd.Wait()
	wg.Wait() // Wait for IO to finish

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
			logger.log(SeverityError, OpError, "Cmd with error: %s %v (%s)", green("[", cmdStr, "]"), red(err), cyan(duration.Round(time.Millisecond)))
		}
	} else {
		logger.log(SeverityWarn, OpSuccess, "Cmd successfully: %s (%s)", green(cmdStr), cyan(duration.Round(time.Millisecond)))
	}
}
