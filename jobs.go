package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	runningProcesses = make(map[string][]*exec.Cmd)
	processMutex     = &sync.Mutex{}
)

// parallelCtxKey is used to indicate parallel execution
type parallelCtxKey struct{}

// Job is the unified struct for any unit of work
type Job struct {
	Name     string            `yaml:"-"`
	Cmd      string            `yaml:"cmd,omitempty"`
	Params   []string          `yaml:"params,omitempty"`
	Series   []Job             `yaml:"series,omitempty"`
	Parallel []Job             `yaml:"parallel,omitempty"`
	Before   []Job             `yaml:"before,omitempty"`
	After    []Job             `yaml:"after,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Trigger  *Trigger          `yaml:"trigger,omitempty"`
}

// Trigger defines file paths and regex patterns to watch on
type Trigger struct {
	Paths []string `yaml:"paths,omitempty"`
	Regex []string `yaml:"regex,omitempty"`
}

// start handles the core execution
func (j *Job) start(ctx context.Context) {
	// Execute 'Before' jobs
	for _, beforeJob := range j.Before {
		select {
		case <-ctx.Done():
			return
		default:
			beforeJob.start(ctx)
		}
	}

	// Execute
	j.run(ctx)

	// Execute 'After' jobs
	for _, afterJob := range j.After {
		select {
		case <-ctx.Done():
			return
		default:
			afterJob.start(ctx)
		}
	}
}

// stop stops a running command by its job name
func (j Job) stop() <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		processMutex.Lock()
		cmds, ok := runningProcesses[j.Name]
		if !ok {
			processMutex.Unlock()
			return
		}
		delete(runningProcesses, j.Name)
		logger.log(SeverityDebug, OpSuccess, "Executor: Removed job %s from running processes map.", j.Name)
		processMutex.Unlock()

		for _, cmd := range cmds {
			if cmd.Process == nil {
				continue
			}
			logger.log(SeverityInfo, OpSuccess, "Executor: Stopping process with PID: %d for job: %s", cmd.Process.Pid, j.Name)
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

// run handles the core execution
func (j *Job) run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return // Job was canceled
	default:
		// Continue
	}

	if j.Cmd != "" {
		j.execute(ctx)
	} else if len(j.Series) > 0 {
		for i := range j.Series {
			seriesJob := &j.Series[i]
			seriesJob.Name = j.Name
			seriesJob.run(ctx)
		}
	} else if len(j.Parallel) > 0 {
		var commandStrings []string
		for _, pJob := range j.Parallel {
			cmdStr := pJob.Cmd
			if len(pJob.Params) > 0 {
				cmdStr += " " + strings.Join(pJob.Params, " ")
			}
			commandStrings = append(commandStrings, fmt.Sprintf("%s[%s]%s", ColorYellow, cmdStr, ColorReset))
		}
		logger.log(SeverityWarn, OpWarn, "Running cmds: %s", strings.Join(commandStrings, ", "))

		var wg sync.WaitGroup
		for i := range j.Parallel {
			jobToRun := j.Parallel[i]
			jobToRun.Name = j.Name
			wg.Add(1)
			go func(job Job) {
				defer wg.Done()
				pCtx := context.WithValue(ctx, parallelCtxKey{}, true)
				job.run(pCtx)
			}(jobToRun)
		}
		wg.Wait()
	}
}

// execute executes the command and streams its output
func (j *Job) execute(ctx context.Context) {
	if p, _ := ctx.Value(parallelCtxKey{}).(bool); !p {
		logger.log(SeverityWarn, OpWarn, "Running cmd: %s", yellow(j.Cmd, " ", j.Params))
	}

	cmd, stdoutPipe, stderrPipe, err := j.setupCmd(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger.log(SeverityError, OpError, "%v", err)
		}
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
	registerProcess(j.Name, cmd)
	logger.log(SeverityDebug, OpWarn, "Executor: Started new process with PID: %d for job: %s", cmd.Process.Pid, j.Name)

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

	err = cmd.Wait()
	wg.Wait() // Wait for IO to finish

	duration := time.Since(startTime)

	cleanupProcess(j.Name, cmd)

	cmdStr := j.Cmd
	if len(j.Params) > 0 {
		cmdStr += " " + strings.Join(j.Params, " ")
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

// UnmarshalYAML is the custom parser for the Action struct
func (j *Job) UnmarshalYAML(node *yaml.Node) error {
	// Try to unmarshal as a command string
	var simpleCmd string
	if err := node.Decode(&simpleCmd); err == nil {
		parts := strings.Fields(simpleCmd)
		if len(parts) > 0 {
			j.Cmd = parts[0]
			j.Params = parts[1:]
		}
		return nil
	}

	// Unmarshal it into a temporary struct to avoid recursion
	var raw struct {
		Name     string            `yaml:"name,omitempty"`
		Cmd      string            `yaml:"cmd,omitempty"`
		Params   []string          `yaml:"params,omitempty"`
		Series   []Job             `yaml:"series,omitempty"`
		Parallel []Job             `yaml:"parallel,omitempty"`
		Before   []Job             `yaml:"before,omitempty"`
		After    []Job             `yaml:"after,omitempty"`
		Env      map[string]string `yaml:"env,omitempty"`
		Trigger  *Trigger          `yaml:"trigger,omitempty"`
	}

	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Validate that only one of cmd, series, or parallel is set
	types := 0
	if raw.Cmd != "" {
		types++
	}
	if len(raw.Series) > 0 {
		types++
	}
	if len(raw.Parallel) > 0 {
		types++
	}
	if types > 1 {
		return &yaml.TypeError{Errors: []string{"action map can only contain one of 'cmd', 'series', or 'parallel' keys"}}
	}

	// Assign the fields from the temporary struct to the actual Job struct
	j.Name = raw.Name
	j.Cmd = raw.Cmd
	j.Params = raw.Params
	j.Series = raw.Series
	j.Parallel = raw.Parallel
	j.Before = raw.Before
	j.After = raw.After
	j.Env = raw.Env
	j.Trigger = raw.Trigger

	return nil
}

// setupCmd prepares the command for execution
func (j *Job) setupCmd(ctx context.Context) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, j.Cmd, j.Params...)

	// Set up environment variables
	cmd.Env = os.Environ()
	for key, val := range j.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	// Set the process group ID
	setpgid(cmd)

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	return cmd, stdoutPipe, stderrPipe, nil
}

// cleanupProcess removes a process from the running list
func cleanupProcess(jobName string, cmd *exec.Cmd) {
	if jobName != "" {
		processMutex.Lock()
		// Find and remove the specific command from the slice
		if cmds, ok := runningProcesses[jobName]; ok {
			for i, c := range cmds {
				if c == cmd {
					runningProcesses[jobName] = slices.Delete(cmds, i, i+1)
					break
				}
			}
		}
		processMutex.Unlock()
	}
}

// registerProcess adds a process to the running list
func registerProcess(jobName string, cmd *exec.Cmd) {
	if jobName != "" {
		processMutex.Lock()
		runningProcesses[jobName] = append(runningProcesses[jobName], cmd)
		processMutex.Unlock()
	}
}

// streamOutput pipes the reader to the writer
func streamOutput(reader io.Reader, writer io.Writer) {
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

// instance contains a job execution
type instance struct {
	cancel context.CancelFunc
	id     uint64
}

// Manager tracks running jobs
type Manager struct {
	mu      sync.Mutex
	running map[string]instance
	nextID  uint64
}

// newManager creates a new Manager for jobs
func newManager() *Manager {
	return &Manager{
		running: make(map[string]instance),
	}
}

// stop stops all running jobs
func (m *Manager) stop() {
	m.mu.Lock()

	var stoppedChs []<-chan struct{}
	for name, job := range m.running {
		logger.log(SeverityDebug, OpWarn, "JobManager: Stopping job on exit: %s", name)
		job.cancel()
		stoppedChs = append(stoppedChs, Job{Name: name}.stop())
	}
	m.mu.Unlock()

	for _, ch := range stoppedChs {
		<-ch
	}
}

// register starts tracking a new job. If a job with the same name is already running, it cancels the previous
func (m *Manager) register(jobName string) (context.Context, func()) {
	// Check if a job is already running
	m.mu.Lock()
	existingJob, exists := m.running[jobName]
	m.mu.Unlock()

	// If it exists, stop it OUTSIDE the lock
	if exists {
		logger.log(SeverityDebug, OpWarn, "JobManager: Stopping previously running job: %s", jobName)
		existingJob.cancel()
		logger.log(SeverityDebug, OpWarn, "JobManager: Calling stopCommand for %s", jobName)
		<-Job{Name: jobName}.stop()
		logger.log(SeverityDebug, OpSuccess, "JobManager: stopCommand for %s finished", jobName)
	}

	// Now re-acquire the lock to register the new job
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a new job instance
	ctx, cancel := context.WithCancel(context.Background())
	logger.log(SeverityDebug, OpWarn, "JobManager: Creating new context for job: %s", jobName)

	// Assign a unique ID
	m.nextID++
	id := m.nextID

	m.running[jobName] = instance{
		cancel: cancel,
		id:     id,
	}

	// Return a function that will deregister the job
	return ctx, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if job, ok := m.running[jobName]; ok && job.id == id {
			delete(m.running, jobName)
		}
	}
}
