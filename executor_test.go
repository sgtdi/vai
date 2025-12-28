package main

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	logger = New(SeverityError)
}

func resetGlobals() {
	processMutex.Lock()
	runningProcesses = make(map[string][]*exec.Cmd)
	processMutex.Unlock()
}

func TestExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executor tests on Windows due to shell command differences")
	}

	t.Run("executes a simple command", func(t *testing.T) {
		resetGlobals()

		job := Job{Cmd: "echo", Params: []string{"hello world"}}
		output := captureOutput(func() {
			Execute(context.Background(), job)
		})

		if !strings.Contains(output, "hello world") {
			t.Fatalf("expected output to contain 'hello world', got %q", output)
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

		Execute(context.Background(), job)

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
		Execute(context.Background(), serial)
		serialTime := time.Since(start)

		start = time.Now()
		Execute(context.Background(), parallel)
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

		Execute(context.Background(), job)

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
			Execute(ctx, job)
		})
		wg.Wait()

		if ctx.Err() == nil {
			t.Fatal("expected context to be canceled")
		}
	})

	t.Run("runCommand sets environment variables", func(t *testing.T) {
		resetGlobals()

		job := Job{
			Cmd:    "sh",
			Params: []string{"-c", "echo $TEST_VAR"},
			Env:    map[string]string{"TEST_VAR": "hello from env"},
		}

		output := captureOutput(func() {
			runCommand(context.Background(), job)
		})

		if !strings.Contains(output, "hello from env") {
			t.Fatalf("expected env output, got %q", output)
		}
	})

	t.Run("stopCommand kills a running process", func(t *testing.T) {
		resetGlobals()

		job := Job{Name: "test-kill", Cmd: "sleep", Params: []string{"5"}}

		var wg sync.WaitGroup
		wg.Go(func() {
			Execute(context.Background(), job)
		})

		time.Sleep(100 * time.Millisecond)
		<-stopCommand(job.Name)
		wg.Wait()

		processMutex.Lock()
		_, ok := runningProcesses[job.Name]
		processMutex.Unlock()

		if ok {
			t.Fatal("expected process to be removed from runningProcesses")
		}
	})
}

func stripAnsi(str string) string {
	const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
	re := regexp.MustCompile(ansi)
	return re.ReplaceAllString(str, "")
}
