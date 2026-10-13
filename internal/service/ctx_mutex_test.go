package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "dingoscheduler/pkg/proto/manager"
)

func TestWriteLockWaitersLeaveWhenCallerGivesUp(t *testing.T) {
	s := &SchedulerService{}
	if err := s.writeMu.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A stalled holder (e.g. a 60s database query) must not make timed-out RPCs queue behind it.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.SchedulerFile(ctx, &pb.SchedulerFileRequest{DataType: "models", Org: "Qwen", Repo: "demo", Name: "a.bin", Etag: "e", InstanceId: "hd-01"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("SchedulerFile waited %s: %v", time.Since(start), err)
	}
	if _, err := s.SingleFileProcess(ctx, &pb.FileProcessEntry{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SingleFileProcess: %v", err)
	}
	s.writeMu.Unlock()
	// The abandoned waiters left nothing behind: the lock is immediately available.
	quick, cancelQuick := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelQuick()
	if err := s.writeMu.Lock(quick); err != nil {
		t.Fatalf("lock leaked: %v", err)
	}
	s.writeMu.Unlock()
}

func TestWriteLockRejectsAlreadyCanceledCaller(t *testing.T) {
	var m ctxMutex
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Lock(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller acquired the lock: %v", err)
	}
	if err := m.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Unlock()
}
