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
func (jm *JobManager) Register(jobName string) (context.Context, func()) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Stop all previously running jobs
	for name, job := range jm.running {
		Logf(SeverityInfo, "Stopping previously running job: %s", name)
		stopCommand(name)
		job.cancel()
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
