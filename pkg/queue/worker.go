package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"cee.io/pkg/metrics"
	"cee.io/pkg/security"
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

	// 1. Update status to Processing (2)
	job.SetStatus(languages.GetStatusByID(languages.StatusProcessing))
	_ = wp.queue.UpdateSubmission(wp.ctx, job)

	// 2. Pre-execution static security scan
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

	// 3. Run execution
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

	// 4. Populate execution results with mutex protection
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

	// 5. Compare with Expected Output
	if job.ExpectedOutput != "" && job.Status.ID == languages.StatusAccepted {
		actual := ""
		if job.Stdout != nil {
			actual = *job.Stdout
		}
		if !compareOutput(actual, job.ExpectedOutput) {
			job.Status = languages.GetStatusByID(languages.StatusWrongAnswer)
		}
	}
	job.mu.Unlock()

	durationSec := time.Since(start).Seconds()
	wp.finalizeJob(job, durationSec)
}

func (wp *WorkerPool) finalizeJob(job *SubmissionJob, durationSec float64) {
	status := job.GetStatus()

	// Record metrics
	langName := strconv.Itoa(job.LanguageID)
	if job.Language != nil {
		langName = job.Language.Name
	}
	metrics.RecordSubmission(status.Description, langName, durationSec)

	// Send webhook callback if configured
	if job.CallbackURL != "" {
		go wp.sendCallback(job)
	}

	// Signal queue that job is complete (instant wakeup for sync callers)
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

func compareOutput(actual, expected string) bool {
	normActual := strings.TrimRight(actual, " \t\r\n")
	normExpected := strings.TrimRight(expected, " \t\r\n")
	return normActual == normExpected
}
