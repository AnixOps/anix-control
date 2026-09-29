package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.cron == nil {
		t.Error("cron not initialized")
	}
	if s.jobs == nil {
		t.Error("jobs map not initialized")
	}
}

func TestStartStop(t *testing.T) {
	s := New()
	s.Start()
	time.Sleep(10 * time.Millisecond)
	s.Stop()
}

func TestAddJob(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Name:     "Test Job",
		Schedule: "*/5 * * * * *",
		Handler: func(ctx context.Context) error {
			return nil
		},
		Enabled: true,
	}

	err := s.AddJob(job)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, ok := s.GetJob("test-job")
	if !ok {
		t.Error("job not found after adding")
	}
	if retrieved.Name != "Test Job" {
		t.Errorf("expected name 'Test Job', got '%s'", retrieved.Name)
	}
}

func TestAddJob_Duplicate(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
	}

	s.AddJob(job)
	err := s.AddJob(job)

	if err != ErrJobExists {
		t.Errorf("expected ErrJobExists, got %v", err)
	}
}

func TestAddJob_InvalidSchedule(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Schedule: "invalid-cron",
		Handler:  func(ctx context.Context) error { return nil },
	}

	err := s.AddJob(job)
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}

func TestRemoveJob(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
	}

	s.AddJob(job)
	s.RemoveJob("test-job")

	_, ok := s.GetJob("test-job")
	if ok {
		t.Error("job still exists after removal")
	}
}

func TestRemoveJob_NonExistent(t *testing.T) {
	s := New()
	// Should not panic
	s.RemoveJob("non-existent")
}

func TestGetJob(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
	}

	s.AddJob(job)

	retrieved, ok := s.GetJob("test-job")
	if !ok {
		t.Error("job not found")
	}
	if retrieved.ID != "test-job" {
		t.Errorf("expected ID 'test-job', got '%s'", retrieved.ID)
	}

	_, ok = s.GetJob("non-existent")
	if ok {
		t.Error("should not find non-existent job")
	}
}

func TestListJobs(t *testing.T) {
	s := New()
	job1 := &Job{
		ID:       "job1",
		Schedule: "*/5 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
	}
	job2 := &Job{
		ID:       "job2",
		Schedule: "*/10 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
	}

	s.AddJob(job1)
	s.AddJob(job2)

	jobs := s.ListJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestEnableDisableJob(t *testing.T) {
	s := New()
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler:  func(ctx context.Context) error { return nil },
		Enabled:  true,
	}

	s.AddJob(job)

	s.DisableJob("test-job")
	retrieved, _ := s.GetJob("test-job")
	if retrieved.Enabled {
		t.Error("job should be disabled")
	}

	s.EnableJob("test-job")
	retrieved, _ = s.GetJob("test-job")
	if !retrieved.Enabled {
		t.Error("job should be enabled")
	}
}

func TestEnableDisableJob_NonExistent(t *testing.T) {
	s := New()
	// Should not panic
	s.EnableJob("non-existent")
	s.DisableJob("non-existent")
}

func TestRunJob(t *testing.T) {
	s := New()
	called := false
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	s.AddJob(job)
	err := s.RunJob("test-job")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestRunJob_Error(t *testing.T) {
	s := New()
	expectedErr := errors.New("job error")
	job := &Job{
		ID:       "test-job",
		Schedule: "*/5 * * * * *",
		Handler: func(ctx context.Context) error {
			return expectedErr
		},
	}

	s.AddJob(job)
	err := s.RunJob("test-job")

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestRunJob_NotFound(t *testing.T) {
	s := New()
	err := s.RunJob("non-existent")

	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestJobError(t *testing.T) {
	err := ErrJobExists
	if err.Error() != "job already exists" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	err = ErrJobNotFound
	if err.Error() != "job not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
