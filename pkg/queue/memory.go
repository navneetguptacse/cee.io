package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/metrics"
)

type MemoryQueue struct {
	jobs       chan *SubmissionJob
	store      sync.Map // token -> *SubmissionJob
	listeners  sync.Map // token -> chan *SubmissionJob
	inQueue    int64
	processing int64
	completed  int64
	failed     int64
	closed     atomic.Bool
}

func NewMemoryQueue(bufferSize int) *MemoryQueue {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	return &MemoryQueue{
		jobs: make(chan *SubmissionJob, bufferSize),
	}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, job *SubmissionJob) error {
	if q.closed.Load() {
		return fmt.Errorf("queue is closed")
	}

	job.SetStatus(languages.GetStatusByID(languages.StatusInQueue))
	q.store.Store(job.Token, job)

	select {
	case q.jobs <- job:
		atomic.AddInt64(&q.inQueue, 1)
		metrics.QueueSize.Set(float64(atomic.LoadInt64(&q.inQueue)))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue buffer is full")
	}
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (*SubmissionJob, error) {
	select {
	case job, ok := <-q.jobs:
		if !ok {
			return nil, fmt.Errorf("queue is closed")
		}
		atomic.AddInt64(&q.inQueue, -1)
		atomic.AddInt64(&q.processing, 1)
		metrics.QueueSize.Set(float64(atomic.LoadInt64(&q.inQueue)))
		metrics.QueueProcessing.Set(float64(atomic.LoadInt64(&q.processing)))
		return job, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *MemoryQueue) GetSubmission(ctx context.Context, token string) (*SubmissionJob, error) {
	val, ok := q.store.Load(token)
	if !ok {
		return nil, nil
	}
	job := val.(*SubmissionJob)
	return job.Snapshot(), nil
}

func (q *MemoryQueue) UpdateSubmission(ctx context.Context, job *SubmissionJob) error {
	q.store.Store(job.Token, job)
	return nil
}

func (q *MemoryQueue) DeleteSubmission(ctx context.Context, token string) error {
	_, ok := q.store.LoadAndDelete(token)
	if !ok {
		return fmt.Errorf("submission not found")
	}
	return nil
}

func (q *MemoryQueue) GetStats(ctx context.Context) (QueueStats, error) {
	inQ := atomic.LoadInt64(&q.inQueue)
	proc := atomic.LoadInt64(&q.processing)
	comp := atomic.LoadInt64(&q.completed)
	fail := atomic.LoadInt64(&q.failed)

	return QueueStats{
		InQueue:    inQ,
		Processing: proc,
		Completed:  comp,
		Failed:     fail,
		Total:      inQ + proc + comp + fail,
	}, nil
}

func (q *MemoryQueue) SignalCompleted(token string, job *SubmissionJob) {
	atomic.AddInt64(&q.processing, -1)
	status := job.GetStatus()
	if status.ID == languages.StatusAccepted {
		atomic.AddInt64(&q.completed, 1)
	} else {
		atomic.AddInt64(&q.failed, 1)
	}
	metrics.QueueProcessing.Set(float64(atomic.LoadInt64(&q.processing)))

	snap := job.Snapshot()
	// Update in store
	q.store.Store(token, snap)

	// Notify waiting synchronous callers immediately
	if chVal, ok := q.listeners.LoadAndDelete(token); ok {
		ch := chVal.(chan *SubmissionJob)
		select {
		case ch <- snap:
		default:
		}
		close(ch)
	}
}

func (q *MemoryQueue) WaitForResult(ctx context.Context, token string, timeout time.Duration) (*SubmissionJob, error) {
	// First check if already finished
	if job, _ := q.GetSubmission(ctx, token); job != nil && job.IsFinished() {
		return job, nil
	}

	ch := make(chan *SubmissionJob, 1)
	q.listeners.Store(token, ch)

	// Re-check after storing listener in case completed right in between
	if job, _ := q.GetSubmission(ctx, token); job != nil && job.IsFinished() {
		q.listeners.Delete(token)
		return job, nil
	}

	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case job := <-ch:
		return job, nil
	case <-t.C:
		q.listeners.Delete(token)
		return q.GetSubmission(ctx, token)
	case <-ctx.Done():
		q.listeners.Delete(token)
		return nil, ctx.Err()
	}
}

func (q *MemoryQueue) Close() error {
	if q.closed.CompareAndSwap(false, true) {
		close(q.jobs)
	}
	return nil
}
