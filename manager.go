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

// Register starts tracking a new job. If a job with the same name is already running, it cancels the previous
func (jm *JobManager) Register(jobName string) (context.Context, func()) {
	// Check if a job is already running
	jm.mu.Lock()
	existingJob, exists := jm.running[jobName]
	jm.mu.Unlock()

	// If it exists, stop it OUTSIDE the lock
	if exists {
		logger.log(SeverityDebug, OpWarn, "JobManager: Stopping previously running job: %s", jobName)
		existingJob.cancel()
		logger.log(SeverityDebug, OpWarn, "JobManager: Calling stopCommand for %s", jobName)
		<-stopCommand(jobName)
		logger.log(SeverityDebug, OpSuccess, "JobManager: stopCommand for %s finished", jobName)
	}

	// Now re-acquire the lock to register the new job
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Create a new job instance
	ctx, cancel := context.WithCancel(context.Background())
	logger.log(SeverityDebug, OpWarn, "JobManager: Creating new context for job: %s", jobName)

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
func (jm *JobManager) StopAll() {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	var stoppedChs []<-chan struct{}
	for name, job := range jm.running {
		logger.log(SeverityDebug, OpWarn, "JobManager: Stopping job on exit: %s", name)
		job.cancel()
		stoppedChs = append(stoppedChs, stopCommand(name))
	}

	for _, ch := range stoppedChs {
		<-ch
	}
}
