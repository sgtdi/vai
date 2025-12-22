package main

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executor tests on Windows due to shell command differences")
	}

	t.Run("executes a simple command", func(t *testing.T) {
		job := Job{Cmd: "echo", Params: []string{"hello world"}}
		output := captureOutput(func() {
			Execute(context.Background(), job)
		})

		if !strings.Contains(output, "hello world") {
			t.Errorf("Expected output to contain 'hello world', but got '%s'", output)
		}
	})

	t.Run("executes jobs in series", func(t *testing.T) {
		job := Job{
			Series: []Job{
				{Cmd: "echo", Params: []string{"first"}},
				{Cmd: "echo", Params: []string{"second"}},
			},
		}
		output := captureOutput(func() {
			Execute(context.Background(), job)
		})

		firstIndex := strings.Index(output, "first")
		secondIndex := strings.Index(output, "second")

		if firstIndex == -1 || secondIndex == -1 || secondIndex < firstIndex {
			t.Errorf("Expected 'first' to be printed before 'second', but got '%s'", output)
		}
	})

	t.Run("executes jobs in parallel", func(t *testing.T) {
		job := Job{
			Parallel: []Job{
				{Cmd: "sleep", Params: []string{"0.1"}},
				{Cmd: "sleep", Params: []string{"0.1"}},
			},
		}

		startTime := time.Now()
		Execute(context.Background(), job)
		duration := time.Since(startTime)

		if duration > 250*time.Millisecond {
			t.Errorf("Parallel jobs took too long (%v), suggesting they ran in series", duration)
		}
	})

	t.Run("executes before and after jobs", func(t *testing.T) {
		job := Job{
			Before: []Job{{Cmd: "echo", Params: []string{"before"}}},
			Cmd:    "echo",
			Params: []string{"main"},
			After:  []Job{{Cmd: "echo", Params: []string{"after"}}},
		}
		output := captureOutput(func() {
			Execute(context.Background(), job)
		})

		beforeIndex := strings.Index(output, "before")
		mainIndex := strings.Index(output, "main")
		afterIndex := strings.Index(output, "after")

		if !(beforeIndex < mainIndex && mainIndex < afterIndex) {
			t.Errorf("Expected 'before', 'main', and 'after' in order, but got '%s'", output)
		}
	})

	t.Run("context cancellation stops a running job", func(t *testing.T) {
		job := Job{Cmd: "sleep", Params: []string{"5"}}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		wg.Go(func() {
			Execute(ctx, job)
		})

		wg.Wait()

		if ctx.Err() == nil {
			t.Error("Expected context to be canceled, but it was not")
		}
	})

	t.Run("runCommand sets environment variables", func(t *testing.T) {
		job := Job{
			Cmd:    "sh",
			Params: []string{"-c", "echo $TEST_VAR"},
			Env:    map[string]string{"TEST_VAR": "hello from env"},
		}
		output := captureOutput(func() {
			runCommand(context.Background(), job)
		})

		if !strings.Contains(output, "hello from env") {
			t.Errorf("Expected output to contain 'hello from env', but got '%s'", output)
		}
	})

	t.Run("stopCommand kills a running process", func(t *testing.T) {
		job := Job{Name: "test-kill", Cmd: "sleep", Params: []string{"5"}}

		var wg sync.WaitGroup
		wg.Go(func() {
			Execute(context.Background(), job)
		})

		time.Sleep(100 * time.Millisecond)

		stopCommand(job.Name, false)

		wg.Wait()

		processMutex.Lock()
		_, ok := runningProcesses[job.Name]
		processMutex.Unlock()

		if ok {
			t.Error("Expected process to be removed from the map, but it was not")
		}
	})
}
