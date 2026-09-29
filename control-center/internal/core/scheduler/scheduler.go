package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Job represents a scheduled job
type Job struct {
	ID       string
	Name     string
	Schedule string // cron expression
	Handler  func(ctx context.Context) error
	LastRun  time.Time
	NextRun  time.Time
	Enabled  bool
	Metadata map[string]interface{}
}

// Scheduler manages scheduled jobs
type Scheduler struct {
	mu    sync.RWMutex
	cron  *cron.Cron
	jobs  map[string]*Job
	entry map[string]cron.EntryID
}

// New creates a new scheduler
func New() *Scheduler {
	return &Scheduler{
		cron:  cron.New(cron.WithSeconds()),
		jobs:  make(map[string]*Job),
		entry: make(map[string]cron.EntryID),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// AddJob adds a new scheduled job
func (s *Scheduler) AddJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; exists {
		return ErrJobExists
	}

	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		if !job.Enabled {
			return
		}
		job.LastRun = time.Now()
		if err := job.Handler(context.Background()); err != nil {
			// Log error
		}
	})
	if err != nil {
		return err
	}

	s.jobs[job.ID] = job
	s.entry[job.ID] = entryID
	job.NextRun = s.cron.Entry(entryID).Next

	return nil
}

// RemoveJob removes a scheduled job
func (s *Scheduler) RemoveJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entry[id]; exists {
		s.cron.Remove(entryID)
		delete(s.entry, id)
	}
	delete(s.jobs, id)
}

// GetJob retrieves a job by ID
func (s *Scheduler) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

// ListJobs returns all scheduled jobs
func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		// Update next run time
		if entryID, ok := s.entry[job.ID]; ok {
			job.NextRun = s.cron.Entry(entryID).Next
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// EnableJob enables a job
func (s *Scheduler) EnableJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = true
	}
}

// DisableJob disables a job
func (s *Scheduler) DisableJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = false
	}
}

// RunJob runs a job immediately
func (s *Scheduler) RunJob(id string) error {
	s.mu.RLock()
	job, ok := s.jobs[id]
	s.mu.RUnlock()

	if !ok {
		return ErrJobNotFound
	}

	job.LastRun = time.Now()
	return job.Handler(context.Background())
}

// Errors
var (
	ErrJobExists   = &JobError{Message: "job already exists"}
	ErrJobNotFound = &JobError{Message: "job not found"}
)

// JobError represents a scheduler error
type JobError struct {
	Message string
}

func (e *JobError) Error() string {
	return e.Message
}
