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
		if debug {
			Logf(SeverityInfo, "JobManager: Stopping previously running job: %s", jobName)
		}
		job.cancel()
		if debug {
			Logf(SeverityInfo, "JobManager: Calling stopCommand for %s", jobName)
		}
		<-stopCommand(jobName, debug)
		if debug {
			Logf(SeverityInfo, "JobManager: stopCommand for %s finished", jobName)
		}
	}

	// Create a new job instance
	ctx, cancel := context.WithCancel(context.Background())
	if debug {
		Logf(SeverityInfo, "JobManager: Creating new context for job: %s", jobName)
	}

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

	var stoppedChs []<-chan struct{}
	for name, job := range jm.running {
		if debug {
			Logf(SeverityInfo, "JobManager: Stopping job on exit: %s", name)
		}
		job.cancel()
		stoppedChs = append(stoppedChs, stopCommand(name, debug))
	}

	for _, ch := range stoppedChs {
		<-ch
	}
}
