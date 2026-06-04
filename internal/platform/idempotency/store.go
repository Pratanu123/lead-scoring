package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultResultTTL = 24 * time.Hour
	defaultLockTTL   = 2 * time.Minute
	keyPrefix        = "lead-scoring:idempotency:create-lead:"
)

type Status int

const (
	StatusProceed Status = iota
	StatusReplay
	StatusConflict
	StatusInProgress
)

type Result struct {
	Status   Status
	Response []byte
	Token    string
}

type Store struct {
	client    *redis.Client
	resultTTL time.Duration
	lockTTL   time.Duration
}

func NewStore(client *redis.Client) *Store {
	return &Store{
		client:    client,
		resultTTL: defaultResultTTL,
		lockTTL:   defaultLockTTL,
	}
}

func (s *Store) Begin(ctx context.Context, key string, requestHash string) (Result, error) {
	if s == nil || s.client == nil || key == "" {
		return Result{Status: StatusProceed}, nil
	}

	baseKey := storageKey(key)
	token := operationToken(key, requestHash)
	status, err := beginScript.Run(
		ctx,
		s.client,
		[]string{baseKey + ":request-hash", baseKey + ":response", baseKey + ":lock"},
		requestHash,
		token,
		s.resultTTL.Milliseconds(),
		s.lockTTL.Milliseconds(),
	).Int()
	if err != nil {
		return Result{}, fmt.Errorf("begin idempotent request: %w", err)
	}

	switch status {
	case -1:
		return Result{Status: StatusConflict}, nil
	case 1:
		response, err := s.client.Get(ctx, baseKey+":response").Bytes()
		if err != nil {
			return Result{}, fmt.Errorf("read idempotent response: %w", err)
		}
		return Result{Status: StatusReplay, Response: response}, nil
	case 2:
		return Result{Status: StatusProceed, Token: token}, nil
	case 3:
		return Result{Status: StatusInProgress}, nil
	default:
		return Result{}, fmt.Errorf("unexpected idempotency status: %d", status)
	}
}

func (s *Store) Complete(ctx context.Context, key string, token string, response []byte) error {
	if s == nil || s.client == nil || key == "" || token == "" {
		return nil
	}

	baseKey := storageKey(key)
	_, err := completeScript.Run(
		ctx,
		s.client,
		[]string{baseKey + ":response", baseKey + ":lock"},
		token,
		response,
		s.resultTTL.Milliseconds(),
	).Result()
	if err != nil {
		return fmt.Errorf("complete idempotent request: %w", err)
	}
	return nil
}

func (s *Store) Abort(ctx context.Context, key string, token string) {
	if s == nil || s.client == nil || key == "" || token == "" {
		return
	}

	baseKey := storageKey(key)
	_, _ = abortScript.Run(ctx, s.client, []string{baseKey + ":lock"}, token).Result()
}

func storageKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return keyPrefix + hex.EncodeToString(hash[:])
}

func operationToken(key string, requestHash string) string {
	value := fmt.Sprintf("%s:%s:%d", key, requestHash, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

var beginScript = redis.NewScript(`
local stored_hash = redis.call("GET", KEYS[1])
if stored_hash and stored_hash ~= ARGV[1] then
	return -1
end

if redis.call("EXISTS", KEYS[2]) == 1 then
	return 1
end

local acquired = redis.call("SET", KEYS[3], ARGV[2], "NX", "PX", ARGV[4])
if not acquired then
	return 3
end

redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[3])
return 2
`)

var completeScript = redis.NewScript(`
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
	return 0
end

redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
redis.call("DEL", KEYS[2])
return 1
`)

var abortScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)
