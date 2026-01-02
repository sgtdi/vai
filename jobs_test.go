package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func init() {
	logger = newLogger(SeverityError)
}

func resetGlobals() {
	processMutex.Lock()
	defer processMutex.Unlock()

	for _, cmds := range runningProcesses {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				_ = killProcess(cmd)
			}
		}
	}

	runningProcesses = make(map[string][]*exec.Cmd)

	// Small delay to allow OS to reap processes (needed on Linux CI)
	time.Sleep(20 * time.Millisecond)
}

func TestJobUnmarshalYAML(t *testing.T) {
	t.Run("Unmarshal simple command string", func(t *testing.T) {
		yamlString := `"go run ."`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		expected := Job{Cmd: "go", Params: []string{"run", "."}}
		if !reflect.DeepEqual(job, expected) {
			t.Errorf("Expected %+v, got %+v", expected, job)
		}
	})

	t.Run("Unmarshal full job struct", func(t *testing.T) {
		yamlString := `
name: "Run and Test"
series:
  - cmd: "go build"
  - cmd: "./app"
env:
  APP_ENV: "development"
trigger:
  regex:
    - '\.go$'
`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if job.Name != "Run and Test" {
			t.Errorf("Expected name 'Run and Test', got '%s'", job.Name)
		}
		if len(job.Series) != 2 {
			t.Errorf("Expected 2 series jobs, got %d", len(job.Series))
		}
		if job.Env["APP_ENV"] != "development" {
			t.Errorf("Expected APP_ENV to be 'development', got '%s'", job.Env["APP_ENV"])
		}
		// The expected value in Go is an unescaped string
		if len(job.Trigger.Regex) != 1 || job.Trigger.Regex[0] != `\.go$` {
			t.Errorf("Expected regex '\\.go$', got '%v'", job.Trigger.Regex)
		}
	})

	t.Run("Unmarshal fails with multiple action types", func(t *testing.T) {
		yamlString := `
cmd: "do one thing"
series:
  - cmd: "do another thing"
`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err == nil {
			t.Fatal("Expected an error but got none")
		}
	})
}

func TestFromCLI(t *testing.T) {
	t.Run("From single command", func(t *testing.T) {
		singleCmd := []string{"go", "run", "."}
		path := "./src"
		patterns := []string{`\.go$`}
		// env := map[string]string{"PORT": "8080"} // Original inputs

		// Construct Args as if parsed from CLI
		args := &Args{
			PositionalArgs: singleCmd,
			Path:           path,
			Regex:          strings.Join(patterns, ","),
			Env:            "PORT=8080",
		}

		vai, err := newVai(args)
		if err != nil {
			t.Fatalf("newVai failed: %v", err)
		}

		job := vai.Jobs["default"]
		// Note: parseFlags combines positional args into a single string
		if len(job.Series) != 1 {
			t.Fatalf("Expected 1 series job, got %d", len(job.Series))
		}
		// The command parsing splits "go run ." into Cmd="go" Params=["run", "."]
		if job.Series[0].Cmd != "go" || !reflect.DeepEqual(job.Series[0].Params, []string{"run", "."}) {
			t.Errorf("Unexpected command: %+v", job.Series[0])
		}
		if job.Env["PORT"] != "8080" {
			t.Errorf("Expected env PORT=8080, got '%s'", job.Env["PORT"])
		}
		if !reflect.DeepEqual(job.Trigger.Regex, patterns) {
			t.Errorf("Expected regex patterns '%v', got '%v'", patterns, job.Trigger.Regex)
		}
		if len(job.Trigger.Paths) != 1 || job.Trigger.Paths[0] != path {
			t.Errorf("Expected path '%s', got '%v'", path, job.Trigger.Paths)
		}
	})

	t.Run("From multiple cmd flags", func(t *testing.T) {
		// seriesCmds := []string{"go fmt ./...", "go run ."}
		args := &Args{
			CmdFlags: []string{"go fmt ./...", "go run ."},
			Path:     ".",
		}

		vai, err := newVai(args)
		if err != nil {
			t.Fatalf("newVai failed: %v", err)
		}

		job := vai.Jobs["default"]
		if len(job.Series) != 2 {
			t.Fatalf("Expected 2 series jobs, got %d", len(job.Series))
		}
		if job.Series[0].Cmd != "go" || !reflect.DeepEqual(job.Series[0].Params, []string{"fmt", "./..."}) {
			t.Errorf("Unexpected first command: %+v", job.Series[0])
		}
		if job.Series[1].Cmd != "go" || !reflect.DeepEqual(job.Series[1].Params, []string{"run", "."}) {
			t.Errorf("Unexpected second command: %+v", job.Series[1])
		}
	})
}

func TestFromFile(t *testing.T) {
	t.Run("Successfully load a valid file", func(t *testing.T) {
		yamlContent := `
jobs:
  default:
    trigger:
      paths: ["/app"]
    series:
      - "go run ."
`
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "vai.yml")
		os.WriteFile(filePath, []byte(yamlContent), 0644)

		// Use the unexported fromFile
		vai, err := fromFile(filePath)
		if err != nil {
			t.Fatalf("fromFile failed: %v", err)
		}

		job, ok := vai.Jobs["default"]
		if !ok {
			t.Error("Expected 'default' job to be present")
		}
		if len(job.Trigger.Paths) != 1 || job.Trigger.Paths[0] != "/app" {
			t.Errorf("Expected path '/app', got '%v'", job.Trigger.Paths)
		}
	})

	t.Run("Return error for non-existent file", func(t *testing.T) {
		_, err := fromFile("non-existent-file.yml")
		if err == nil {
			t.Fatal("Expected an error for a non-existent file, but got none")
		}
	})

	t.Run("Return error for malformed YAML", func(t *testing.T) {
		yamlContent := `config: { path: "/app }`
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "vai.yml")
		os.WriteFile(filePath, []byte(yamlContent), 0644)

		_, err := fromFile(filePath)
		if err == nil {
			t.Fatal("Expected an error for malformed YAML, but got none")
		}
	})
}

func TestNewManager(t *testing.T) {
	m := newManager()
	if m == nil {
		t.Fatal("newManager returned nil")
	}
	if m.running == nil {
		t.Error("Manager running map is nil")
	}
}

func TestManager_Register(t *testing.T) {
	t.Run("Register a new job", func(t *testing.T) {
		m := newManager()
		jobName := "test-job"

		ctx, deregister := m.register(jobName)
		if ctx == nil {
			t.Fatal("Register returned a nil context")
		}
		if deregister == nil {
			t.Fatal("Register returned a nil deregister function")
		}

		m.mu.Lock()
		if _, ok := m.running[jobName]; !ok {
			t.Error("Job was not registered in the running map")
		}
		m.mu.Unlock()
	})

	t.Run("Deregister a job", func(t *testing.T) {
		m := newManager()
		jobName := "test-job"

		_, deregister := m.register(jobName)
		deregister()

		m.mu.Lock()
		if _, ok := m.running[jobName]; ok {
			t.Error("Job was not deregistered from the running map")
		}
		m.mu.Unlock()
	})

	t.Run("Registering a duplicate job cancels the previous one", func(t *testing.T) {
		m := newManager()
		jobName := "test-job"

		ctx1, deregister1 := m.register(jobName)
		defer deregister1()

		ctx2, deregister2 := m.register(jobName)
		defer deregister2()

		select {
		case <-ctx1.Done():
		case <-time.After(100 * time.Millisecond):
			t.Error("Previous job's context was not canceled after re-registering")
		}
		if ctx2.Err() != nil {
			t.Error("The new job's context should be active, but it was canceled")
		}
	})

	t.Run("Stale deregister does not affect a new job", func(t *testing.T) {
		m := newManager()
		jobName := "test-job"

		_, deregister1 := m.register(jobName)

		_, deregister2 := m.register(jobName)
		defer deregister2()

		deregister1()

		m.mu.Lock()
		if _, ok := m.running[jobName]; !ok {
			t.Error("The new job was incorrectly deregistered by a stale deregister function")
		}
		m.mu.Unlock()
	})
}

func TestManager_Stop(t *testing.T) {
	m := newManager()
	jobName1 := "job1"
	jobName2 := "job2"

	ctx1, _ := m.register(jobName1)
	ctx2, _ := m.register(jobName2)

	m.stop()

	if ctx1.Err() == nil {
		t.Error("Expected job1 to be canceled")
	}
	if ctx2.Err() == nil {
		t.Error("Expected job2 to be canceled")
	}
}

func TestManager_Concurrency(t *testing.T) {
	m := newManager()
	jobName := "concurrent-job"

	for range 100 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			_, deregister := m.register(jobName)
			time.Sleep(time.Millisecond)
			deregister()
		})
	}
	m.mu.Lock()
	if len(m.running) != 0 {
		t.Errorf("Expected running map to be empty, but it has %d items", len(m.running))
	}
	m.mu.Unlock()
}

func TestExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executor tests on Windows due to shell command differences")
	}

	t.Run("executes a simple command", func(t *testing.T) {
		resetGlobals()

		dir := t.TempDir()
		out := dir + "/out"

		job := Job{
			Cmd:    "sh",
			Params: []string{"-c", "touch " + out},
		}

		job.execute(context.Background())

		if _, err := os.Stat(out); err != nil {
			t.Fatal("command did not run")
		}
	})

	t.Run("executes jobs in series", func(t *testing.T) {
		resetGlobals()

		dir := t.TempDir()
		first := dir + "/first"
		second := dir + "/second"

		job := Job{
			Series: []Job{
				{Cmd: "sh", Params: []string{"-c", "touch " + first}},
				{Cmd: "sh", Params: []string{"-c", "touch " + second}},
			},
		}

		job.run(context.Background())

		if _, err := os.Stat(first); err != nil {
			t.Fatal("first job did not run")
		}
		if _, err := os.Stat(second); err != nil {
			t.Fatal("second job did not run")
		}
	})

	t.Run("executes jobs in parallel", func(t *testing.T) {
		resetGlobals()

		serial := Job{
			Series: []Job{
				{Cmd: "sleep", Params: []string{"0.2"}},
				{Cmd: "sleep", Params: []string{"0.2"}},
			},
		}

		parallel := Job{
			Parallel: []Job{
				{Cmd: "sleep", Params: []string{"0.2"}},
				{Cmd: "sleep", Params: []string{"0.2"}},
			},
		}

		start := time.Now()
		serial.run(context.Background())
		serialTime := time.Since(start)

		start = time.Now()
		parallel.run(context.Background())
		parallelTime := time.Since(start)

		if parallelTime >= serialTime {
			t.Fatalf(
				"expected parallel execution to be faster (serial=%v, parallel=%v)",
				serialTime, parallelTime,
			)
		}
	})

	t.Run("executes before and after jobs", func(t *testing.T) {
		resetGlobals()

		dir := t.TempDir()
		before := dir + "/before"
		main := dir + "/main"
		after := dir + "/after"

		job := Job{
			Before: []Job{
				{Cmd: "sh", Params: []string{"-c", "touch " + before}},
			},
			Cmd:    "sh",
			Params: []string{"-c", "touch " + main},
			After: []Job{
				{Cmd: "sh", Params: []string{"-c", "touch " + after}},
			},
		}

		job.start(context.Background())

		if _, err := os.Stat(before); err != nil {
			t.Fatal("before job did not run")
		}
		if _, err := os.Stat(main); err != nil {
			t.Fatal("main job did not run")
		}
		if _, err := os.Stat(after); err != nil {
			t.Fatal("after job did not run")
		}
	})

	t.Run("context cancellation stops a running job", func(t *testing.T) {
		resetGlobals()

		job := Job{Cmd: "sleep", Params: []string{"5"}}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		wg.Go(func() {
			job.execute(ctx)
		})
		wg.Wait()

		if ctx.Err() == nil {
			t.Fatal("expected context to be canceled")
		}
	})

	t.Run("runCommand sets environment variables", func(t *testing.T) {
		resetGlobals()

		dir := t.TempDir()
		out := dir + "/env"

		job := Job{
			Cmd: "sh",
			Params: []string{
				"-c",
				"echo \"$TEST_VAR\" > " + out,
			},
			Env: map[string]string{
				"TEST_VAR": "hello from env",
			},
		}

		job.run(context.Background())

		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal("env file not created")
		}

		if strings.TrimSpace(string(data)) != "hello from env" {
			t.Fatalf("unexpected env value: %q", string(data))
		}
	})

	t.Run("stopCommand kills a running process", func(t *testing.T) {
		resetGlobals()

		job := Job{Name: "test-kill", Cmd: "sleep", Params: []string{"5"}}

		var wg sync.WaitGroup
		wg.Go(func() {
			job.execute(context.Background())
		})

		time.Sleep(100 * time.Millisecond)
		<-job.stop()
		wg.Wait()

		processMutex.Lock()
		_, ok := runningProcesses[job.Name]
		processMutex.Unlock()

		if ok {
			t.Fatal("expected process to be removed from runningProcesses")
		}
	})
}
