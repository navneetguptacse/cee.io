package tests

import (
	"context"
	"testing"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/queue"
)

func TestMemoryQueue_EnqueueDequeue(t *testing.T) {
	q := queue.NewMemoryQueue(100)
	defer q.Close()

	ctx := context.Background()
	job := &queue.SubmissionJob{
		Token:      "tok-1",
		SourceCode: "print(1)",
		LanguageID: 71,
	}

	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	deqJob, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if deqJob.Token != "tok-1" {
		t.Errorf("expected token tok-1, got %s", deqJob.Token)
	}
}

func TestMemoryQueue_WaitForResultSync(t *testing.T) {
	q := queue.NewMemoryQueue(100)
	defer q.Close()

	ctx := context.Background()
	token := "sync-test-token"
	job := &queue.SubmissionJob{
		Token:      token,
		SourceCode: "print(42)",
		LanguageID: 71,
	}

	_ = q.Enqueue(ctx, job)

	// Simulate background worker finishing in 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdout := "42\n"
		job.SetStatus(languages.GetStatusByID(languages.StatusAccepted))
		job.Stdout = &stdout
		q.SignalCompleted(token, job)
	}()

	start := time.Now()
	completed, err := q.WaitForResult(ctx, token, 2*time.Second)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("WaitForResult error: %v", err)
	}
	if completed == nil || completed.Status.ID != languages.StatusAccepted {
		t.Fatalf("expected accepted status, got %+v", completed)
	}
	if completed.Stdout == nil || *completed.Stdout != "42\n" {
		t.Fatalf("expected stdout '42\n', got %v", completed.Stdout)
	}

	if duration > 500*time.Millisecond {
		t.Errorf("WaitForResult took too long: %v (expected ~50ms)", duration)
	}
}

func TestMemoryQueue_TTLEviction(t *testing.T) {
	q := queue.NewMemoryQueueWithTTL(100, 50*time.Millisecond)
	defer q.Close()

	ctx := context.Background()
	token := "ttl-eviction-token"
	job := &queue.SubmissionJob{
		Token:      token,
		SourceCode: "print('ttl test')",
		LanguageID: 71,
	}

	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Complete job with timestamp 100ms in the past
	oldTime := time.Now().Add(-100 * time.Millisecond).UTC().Format(time.RFC3339)
	job.FinishedAt = &oldTime
	job.SetStatus(languages.GetStatusByID(languages.StatusAccepted))
	q.SignalCompleted(token, job)

	// Ensure job is currently retrievable
	beforeCleanup, err := q.GetSubmission(ctx, token)
	if err != nil || beforeCleanup == nil {
		t.Fatalf("expected job to exist before cleanup, got err=%v", err)
	}

	// Run TTL cleanup
	q.CleanupExpired()

	// Verify job was evicted
	afterCleanup, err := q.GetSubmission(ctx, token)
	if err != nil {
		t.Fatalf("GetSubmission error: %v", err)
	}
	if afterCleanup != nil {
		t.Errorf("expected job %s to be evicted after TTL expiration, but still exists", token)
	}
}
