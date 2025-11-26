package main

import (
	"testing"
	"time"
)

func TestNewJobManager(t *testing.T) {
	jm := NewJobManager()
	if jm == nil {
		t.Fatal("NewJobManager returned nil")
	}
	if jm.running == nil {
		t.Error("JobManager running map is nil")
	}
}

func TestJobManager_Register(t *testing.T) {
	t.Run("Register a new job", func(t *testing.T) {
		jm := NewJobManager()
		jobName := "test-job"

		ctx, deregister := jm.Register(jobName)
		if ctx == nil {
			t.Fatal("Register returned a nil context")
		}
		if deregister == nil {
			t.Fatal("Register returned a nil deregister function")
		}

		jm.mu.Lock()
		if _, ok := jm.running[jobName]; !ok {
			t.Error("Job was not registered in the running map")
		}
		jm.mu.Unlock()
	})

	t.Run("Deregister a job", func(t *testing.T) {
		jm := NewJobManager()
		jobName := "test-job"

		_, deregister := jm.Register(jobName)
		deregister()

		jm.mu.Lock()
		if _, ok := jm.running[jobName]; ok {
			t.Error("Job was not deregistered from the running map")
		}
		jm.mu.Unlock()
	})

	t.Run("Registering a duplicate job cancels the previous one", func(t *testing.T) {
		jm := NewJobManager()
		jobName := "test-job"

		ctx1, deregister1 := jm.Register(jobName)
		defer deregister1()

		ctx2, deregister2 := jm.Register(jobName)
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
		jm := NewJobManager()
		jobName := "test-job"

		_, deregister1 := jm.Register(jobName)

		_, deregister2 := jm.Register(jobName)
		defer deregister2()

		deregister1()

		jm.mu.Lock()
		if _, ok := jm.running[jobName]; !ok {
			t.Error("The new job was incorrectly deregistered by a stale deregister function")
		}
		jm.mu.Unlock()
	})
}

func TestJobManager_Concurrency(t *testing.T) {
	jm := NewJobManager()
	jobName := "concurrent-job"

	for range 100 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			_, deregister := jm.Register(jobName)
			time.Sleep(time.Millisecond)
			deregister()
		})
	}
	jm.mu.Lock()
	if len(jm.running) != 0 {
		t.Errorf("Expected running map to be empty, but it has %d items", len(jm.running))
	}
	jm.mu.Unlock()
}
