package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"cee.io/pkg/metrics"
	"cee.io/pkg/security"
	"cee.io/pkg/utils"
)

type WorkerPool struct {
	queue       Queue
	executor    executor.Executor
	concurrency int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	client      *http.Client
}

func NewWorkerPool(q Queue, exec executor.Executor, concurrency int) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		queue:       q,
		executor:    exec,
		concurrency: concurrency,
		ctx:         ctx,
		cancel:      cancel,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (wp *WorkerPool) Start() {
	metrics.WorkersActive.Set(float64(wp.concurrency))
	slog.Info("worker_pool_started", "concurrency", wp.concurrency, "executor", wp.executor.Type())

	for i := 0; i < wp.concurrency; i++ {
		wp.wg.Add(1)
		go wp.workerLoop(i)
	}
}

func (wp *WorkerPool) Stop() {
	wp.cancel()
	wp.wg.Wait()
	metrics.WorkersActive.Set(0)
	slog.Info("worker_pool_stopped")
}

func (wp *WorkerPool) workerLoop(workerID int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		default:
		}

		job, err := wp.queue.Dequeue(wp.ctx)
		if err != nil {
			if wp.ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		wp.processJob(job)
	}
}

func (wp *WorkerPool) processJob(job *SubmissionJob) {
	start := time.Now()
	token := job.Token
	slog.Info("job_started", "token", token, "language_id", job.LanguageID)

	job.SetStatus(languages.GetStatusByID(languages.StatusProcessing))
	_ = wp.queue.UpdateSubmission(wp.ctx, job)

	scan := security.AnalyzeCode(job.SourceCode, job.LanguageID)
	if scan.Rejected {
		slog.Warn("code_rejected", "token", token, "reason", scan.Reason)
		output := "Rejected: " + scan.Reason
		nowStr := time.Now().UTC().Format(time.RFC3339)

		job.mu.Lock()
		job.Status = languages.GetStatusByID(languages.StatusCompilationError)
		job.CompileOutput = &output
		job.FinishedAt = &nowStr
		job.mu.Unlock()

		wp.finalizeJob(job, 0)
		return
	}

	sub := JobToExecutionSubmission(job)
	execRes, err := wp.executor.Execute(wp.ctx, sub)
	if err != nil {
		slog.Error("execution_internal_error", "token", token, "error", err)
		msg := err.Error()
		nowStr := time.Now().UTC().Format(time.RFC3339)

		job.mu.Lock()
		job.Status = languages.GetStatusByID(languages.StatusInternalError)
		job.Message = &msg
		job.FinishedAt = &nowStr
		job.mu.Unlock()

		wp.finalizeJob(job, time.Since(start).Seconds())
		return
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	job.mu.Lock()
	job.Status = execRes.Status
	job.Stdout = execRes.Stdout
	job.Stderr = execRes.Stderr
	job.CompileOutput = execRes.CompileOutput
	job.Message = execRes.Message
	job.Time = execRes.Time
	job.WallTime = execRes.WallTime
	job.Memory = execRes.Memory
	job.ExitCode = execRes.ExitCode
	job.ExitSignal = execRes.ExitSignal
	job.FinishedAt = &nowStr

	if job.ExpectedOutput != "" && job.Status.ID == languages.StatusAccepted {
		actual := ""
		if job.Stdout != nil {
			actual = *job.Stdout
		}
		if !utils.CompareOutput(actual, job.ExpectedOutput) {
			job.Status = languages.GetStatusByID(languages.StatusWrongAnswer)
		}
	}
	job.mu.Unlock()

	durationSec := time.Since(start).Seconds()
	wp.finalizeJob(job, durationSec)
}

func (wp *WorkerPool) finalizeJob(job *SubmissionJob, durationSec float64) {
	status := job.GetStatus()

	langName := strconv.Itoa(job.LanguageID)
	if job.Language != nil {
		langName = job.Language.Name
	}
	metrics.RecordSubmission(status.Description, langName, durationSec)

	if job.CallbackURL != "" {
		go wp.sendCallback(job)
	}

	wp.queue.SignalCompleted(job.Token, job)

	slog.Info("job_completed",
		"token", job.Token,
		"status_id", status.ID,
		"status", status.Description,
		"duration_sec", durationSec,
	)
}

func (wp *WorkerPool) sendCallback(job *SubmissionJob) {
	if err := security.ValidateCallbackURL(job.CallbackURL); err != nil {
		slog.Error("callback_blocked_ssrf", "token", job.Token, "url", job.CallbackURL, "error", err)
		return
	}

	snap := job.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, job.CallbackURL, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := wp.client.Do(req)
	if err != nil {
		slog.Error("callback_failed", "token", job.Token, "url", job.CallbackURL, "error", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("callback_sent", "token", job.Token, "url", job.CallbackURL, "http_status", resp.StatusCode)
}
