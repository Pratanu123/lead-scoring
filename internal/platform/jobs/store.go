package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const QueueKey = "lead-scoring:jobs"
const EventsChannel = "lead-scoring:job-events"

const (
	TypeEmbed = "embed"
	TypeScore = "score"

	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var ErrNotFound = errors.New("job not found")

type Job struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	LeadID    string          `json:"lead_id"`
	Status    string          `json:"status"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Event struct {
	JobID  string `json:"job_id"`
	LeadID string `json:"lead_id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Store struct {
	db    *sql.DB
	redis *redis.Client
}

func NewStore(db *sql.DB, redisClient *redis.Client) *Store {
	return &Store{db: db, redis: redisClient}
}

func (s *Store) Enqueue(ctx context.Context, jobType string, leadID string, payload any) (Job, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}

	const query = `
INSERT INTO jobs (type, lead_id, status, payload)
VALUES ($1, $2, $3, $4::jsonb)
RETURNING id, type, lead_id, status, payload, COALESCE(result::text, ''), COALESCE(error, ''), created_at, updated_at;
`

	var job Job
	var resultText string
	err = s.db.QueryRowContext(ctx, query, jobType, leadID, StatusQueued, string(payloadJSON)).Scan(
		&job.ID,
		&job.Type,
		&job.LeadID,
		&job.Status,
		&job.Payload,
		&resultText,
		&job.Error,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return Job{}, err
	}
	if resultText != "" {
		job.Result = json.RawMessage(resultText)
	}

	if s.redis != nil {
		_ = s.redis.LPush(ctx, QueueKey, job.ID).Err()
		_ = s.publish(ctx, Event{
			JobID:  job.ID,
			LeadID: job.LeadID,
			Type:   job.Type,
			Status: job.Status,
		})
	}

	return job, nil
}

func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	const query = `
SELECT id, type, lead_id, status, payload, COALESCE(result::text, ''), COALESCE(error, ''), created_at, updated_at
FROM jobs
WHERE id = $1;
`
	var job Job
	var resultText string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
		&job.Type,
		&job.LeadID,
		&job.Status,
		&job.Payload,
		&resultText,
		&job.Error,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if resultText != "" {
		job.Result = json.RawMessage(resultText)
	}
	return job, nil
}

func (s *Store) ClaimNext(ctx context.Context) (Job, error) {
	if s.redis == nil {
		return Job{}, sql.ErrNoRows
	}

	jobID, err := s.redis.BRPop(ctx, 5*time.Second, QueueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return Job{}, sql.ErrNoRows
		}
		return Job{}, err
	}
	if len(jobID) < 2 {
		return Job{}, sql.ErrNoRows
	}

	const query = `
UPDATE jobs
SET status = $2, updated_at = now()
WHERE id = $1 AND status = $3
RETURNING id, type, lead_id, status, payload, COALESCE(result::text, ''), COALESCE(error, ''), created_at, updated_at;
`
	var job Job
	var resultText string
	err = s.db.QueryRowContext(ctx, query, jobID[1], StatusRunning, StatusQueued).Scan(
		&job.ID,
		&job.Type,
		&job.LeadID,
		&job.Status,
		&job.Payload,
		&resultText,
		&job.Error,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, sql.ErrNoRows
	}
	if err != nil {
		return Job{}, err
	}
	if resultText != "" {
		job.Result = json.RawMessage(resultText)
	}

	_ = s.publish(ctx, Event{
		JobID:  job.ID,
		LeadID: job.LeadID,
		Type:   job.Type,
		Status: job.Status,
	})
	return job, nil
}

func (s *Store) Complete(ctx context.Context, id string, result any) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	const query = `
UPDATE jobs
SET status = $2, result = $3::jsonb, error = NULL, updated_at = now()
WHERE id = $1
RETURNING lead_id, type;
`
	var leadID string
	var jobType string
	if err := s.db.QueryRowContext(ctx, query, id, StatusCompleted, string(resultJSON)).Scan(&leadID, &jobType); err != nil {
		return err
	}

	return s.publish(ctx, Event{
		JobID:  id,
		LeadID: leadID,
		Type:   jobType,
		Status: StatusCompleted,
	})
}

func (s *Store) Fail(ctx context.Context, id string, jobErr error) error {
	const query = `
UPDATE jobs
SET status = $2, error = $3, updated_at = now()
WHERE id = $1
RETURNING lead_id, type;
`
	var leadID string
	var jobType string
	errMsg := "job failed"
	if jobErr != nil {
		errMsg = jobErr.Error()
	}
	if err := s.db.QueryRowContext(ctx, query, id, StatusFailed, errMsg).Scan(&leadID, &jobType); err != nil {
		return err
	}

	return s.publish(ctx, Event{
		JobID:  id,
		LeadID: leadID,
		Type:   jobType,
		Status: StatusFailed,
		Error:  errMsg,
	})
}

func (s *Store) PublishEvent(ctx context.Context, event Event) error {
	return s.publish(ctx, event)
}

func (s *Store) Subscribe(ctx context.Context) *redis.PubSub {
	if s.redis == nil {
		return nil
	}
	return s.redis.Subscribe(ctx, EventsChannel)
}

func (s *Store) publish(ctx context.Context, event Event) error {
	if s.redis == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, EventsChannel, payload).Err()
}
