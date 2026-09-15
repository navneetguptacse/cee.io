package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/metrics"
	"github.com/redis/go-redis/v9"
)

const (
	QueueNameSubmissions    = "cee:submissions"
	ChannelSubmissionPrefix = "cee:done:"
)

type RedisQueue struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisQueue(redisURL string, ttlSeconds int) (*RedisQueue, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed connecting to redis: %w", err)
	}

	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}

	return &RedisQueue{
		rdb: rdb,
		ttl: time.Duration(ttlSeconds) * time.Second,
	}, nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, job *SubmissionJob) error {
	job.Status = languages.GetStatusByID(languages.StatusInQueue)
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("submission:%s", job.Token)
	if err := q.rdb.Set(ctx, key, data, q.ttl).Err(); err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}

	if err := q.rdb.LPush(ctx, QueueNameSubmissions, job.Token).Err(); err != nil {
		return fmt.Errorf("redis lpush error: %w", err)
	}

	q.updateMetrics(ctx)
	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context) (*SubmissionJob, error) {
	for {
		res, err := q.rdb.BRPop(ctx, 2*time.Second, QueueNameSubmissions).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, err
		}

		if len(res) < 2 {
			continue
		}
		token := res[1]

		job, err := q.GetSubmission(ctx, token)
		if err != nil || job == nil {
			continue
		}

		q.updateMetrics(ctx)
		return job, nil
	}
}

func (q *RedisQueue) GetSubmission(ctx context.Context, token string) (*SubmissionJob, error) {
	key := fmt.Sprintf("submission:%s", token)
	val, err := q.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var job SubmissionJob
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (q *RedisQueue) UpdateSubmission(ctx context.Context, job *SubmissionJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("submission:%s", job.Token)
	return q.rdb.Set(ctx, key, data, q.ttl).Err()
}

func (q *RedisQueue) DeleteSubmission(ctx context.Context, token string) error {
	key := fmt.Sprintf("submission:%s", token)
	res, err := q.rdb.Del(ctx, key).Result()
	if err != nil {
		return err
	}
	if res == 0 {
		return fmt.Errorf("submission not found")
	}
	return nil
}

func (q *RedisQueue) GetStats(ctx context.Context) (QueueStats, error) {
	len, err := q.rdb.LLen(ctx, QueueNameSubmissions).Result()
	if err != nil {
		return QueueStats{}, err
	}
	return QueueStats{
		InQueue: len,
		Total:   len,
	}, nil
}

func (q *RedisQueue) SignalCompleted(token string, job *SubmissionJob) {
	ctx := context.Background()
	_ = q.UpdateSubmission(ctx, job)

	channel := ChannelSubmissionPrefix + token
	_ = q.rdb.Publish(ctx, channel, token).Err()
}

func (q *RedisQueue) WaitForResult(ctx context.Context, token string, timeout time.Duration) (*SubmissionJob, error) {
	job, _ := q.GetSubmission(ctx, token)
	if job != nil && job.Status.ID > languages.StatusProcessing {
		return job, nil
	}

	channel := ChannelSubmissionPrefix + token
	pubsub := q.rdb.Subscribe(ctx, channel)
	defer pubsub.Close()

	ch := pubsub.Channel()

	job, _ = q.GetSubmission(ctx, token)
	if job != nil && job.Status.ID > languages.StatusProcessing {
		return job, nil
	}

	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case <-ch:
		return q.GetSubmission(ctx, token)
	case <-t.C:
		return q.GetSubmission(ctx, token)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *RedisQueue) updateMetrics(ctx context.Context) {
	l, err := q.rdb.LLen(ctx, QueueNameSubmissions).Result()
	if err == nil {
		metrics.QueueSize.Set(float64(l))
	}
}

func (q *RedisQueue) Close() error {
	return q.rdb.Close()
}
