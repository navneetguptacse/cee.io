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
