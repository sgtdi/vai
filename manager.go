package main

import (
	"context"
	"sync"
)

// runningJob contains a job instance
type runningJob struct {
	cancel context.CancelFunc
	id     uint64
}

// JobManager tracks running jobs
type JobManager struct {
	mu      sync.Mutex
	running map[string]runningJob
	nextID  uint64
}

// NewJobManager creates a new JobManager
func NewJobManager() *JobManager {
	return &JobManager{
		running: make(map[string]runningJob),
	}
}

// Register starts tracking a new job. If a job with the same name is already running, it cancels the previous one
func (jm *JobManager) Register(jobName string, debug bool) (context.Context, func()) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Stop previously running job with the same name
	if job, ok := jm.running[jobName]; ok {
		Logf(SeverityInfo, "Stopping previously running job: %s", jobName)
		job.cancel()
		stopCommand(jobName, debug)
	}

	// Create a new job instance
	ctx, cancel := context.WithCancel(context.Background())

	// Assign a unique ID
	jm.nextID++
	jobID := jm.nextID

	jm.running[jobName] = runningJob{
		cancel: cancel,
		id:     jobID,
	}

	// Return a function that will deregister the job
	return ctx, func() {
		jm.mu.Lock()
		defer jm.mu.Unlock()

		if job, ok := jm.running[jobName]; ok && job.id == jobID {
			delete(jm.running, jobName)
		}
	}
}

// StopAll stops all running jobs
func (jm *JobManager) StopAll(debug bool) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	for name, job := range jm.running {
		Logf(SeverityInfo, "Stopping job on exit: %s", name)
		job.cancel()
		stopCommand(name, debug)
	}
}
