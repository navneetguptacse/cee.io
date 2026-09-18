package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"cee.io/pkg/config"
	"github.com/redis/go-redis/v9"
)

// Store defines persistence operations for API keys.
type Store interface {
	CreateKey(ctx context.Context, key *Key) error
	GetKey(ctx context.Context, id string) (*Key, error)
	GetKeyByHash(ctx context.Context, hash string) (*Key, error)
	ListKeys(ctx context.Context) ([]*Key, error)
	RevokeKey(ctx context.Context, id string) error
	CountActiveMasterAuthKeys(ctx context.Context) (int, error)
	TouchKey(ctx context.Context, id string) error
	Close() error
}

// -----------------------------------------------------------------------------
// MemoryStore: In-memory thread-safe implementation
// -----------------------------------------------------------------------------

type MemoryStore struct {
	mu         sync.RWMutex
	keysByID   map[string]*Key
	keysByHash map[string]*Key
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		keysByID:   make(map[string]*Key),
		keysByHash: make(map[string]*Key),
	}
}

func (s *MemoryStore) CreateKey(ctx context.Context, key *Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.keysByID[key.ID]; exists {
		return fmt.Errorf("key with id %s already exists", key.ID)
	}
	if _, exists := s.keysByHash[key.KeyHash]; exists {
		return fmt.Errorf("duplicate key hash")
	}

	clone := *key
	s.keysByID[key.ID] = &clone
	s.keysByHash[key.KeyHash] = &clone
	return nil
}

func (s *MemoryStore) GetKey(ctx context.Context, id string) (*Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keysByID[id]
	if !exists {
		return nil, ErrKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (s *MemoryStore) GetKeyByHash(ctx context.Context, hash string) (*Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keysByHash[hash]
	if !exists {
		return nil, ErrKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (s *MemoryStore) ListKeys(ctx context.Context) ([]*Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Key, 0, len(s.keysByID))
	for _, key := range s.keysByID {
		clone := *key
		clone.KeyHash = "" // never expose hash
		list = append(list, &clone)
	}
	return list, nil
}

func (s *MemoryStore) CountActiveMasterAuthKeys(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, k := range s.keysByID {
		if k.Type == TypeAuth && k.Role == RoleMaster && k.Status == StatusActive {
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) RevokeKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.keysByID[id]
	if !exists {
		return ErrKeyNotFound
	}
	if key.Status == StatusRevoked {
		return nil
	}

	// Safety check: protect the last active Master AUTH key
	if key.Type == TypeAuth && key.Role == RoleMaster {
		count := 0
		for _, k := range s.keysByID {
			if k.Type == TypeAuth && k.Role == RoleMaster && k.Status == StatusActive {
				count++
			}
		}
		if count <= 1 {
			return ErrLastMaster
		}
	}

	key.Status = StatusRevoked
	return nil
}

func (s *MemoryStore) TouchKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.keysByID[id]
	if exists {
		now := time.Now().UTC()
		key.LastUsedAt = &now
	}
	return nil
}

func (s *MemoryStore) Close() error {
	return nil
}

// -----------------------------------------------------------------------------
// RedisStore: Redis-backed persistence with in-memory sync
// -----------------------------------------------------------------------------

const (
	RedisKeyMetaPrefix = "cee:keys:meta:"
	RedisKeyHashPrefix = "cee:keys:hash:"
	RedisKeyAllSet     = "cee:keys:all"
)

type RedisStore struct {
	rdb    *redis.Client
	memory *MemoryStore
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
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

	store := &RedisStore{
		rdb:    rdb,
		memory: NewMemoryStore(),
	}

	// Hydrate local cache from Redis
	if err := store.hydrate(context.Background()); err != nil {
		slog.Warn("redis_keys_hydrate_failed", "error", err)
	}

	return store, nil
}

func (s *RedisStore) hydrate(ctx context.Context) error {
	ids, err := s.rdb.SMembers(ctx, RedisKeyAllSet).Result()
	if err != nil {
		return err
	}

	for _, id := range ids {
		data, err := s.rdb.Get(ctx, RedisKeyMetaPrefix+id).Bytes()
		if err != nil {
			continue
		}
		var k Key
		if err := json.Unmarshal(data, &k); err == nil {
			_ = s.memory.CreateKey(ctx, &k)
		}
	}
	return nil
}

func (s *RedisStore) CreateKey(ctx context.Context, key *Key) error {
	data, err := json.Marshal(key)
	if err != nil {
		return err
	}

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, RedisKeyMetaPrefix+key.ID, data, 0)
	pipe.Set(ctx, RedisKeyHashPrefix+key.KeyHash, key.ID, 0)
	pipe.SAdd(ctx, RedisKeyAllSet, key.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipe error: %w", err)
	}

	return s.memory.CreateKey(ctx, key)
}

func (s *RedisStore) GetKey(ctx context.Context, id string) (*Key, error) {
	return s.memory.GetKey(ctx, id)
}

func (s *RedisStore) GetKeyByHash(ctx context.Context, hash string) (*Key, error) {
	return s.memory.GetKeyByHash(ctx, hash)
}

func (s *RedisStore) ListKeys(ctx context.Context) ([]*Key, error) {
	return s.memory.ListKeys(ctx)
}

func (s *RedisStore) CountActiveMasterAuthKeys(ctx context.Context) (int, error) {
	return s.memory.CountActiveMasterAuthKeys(ctx)
}

func (s *RedisStore) RevokeKey(ctx context.Context, id string) error {
	// First enforce last master protection in memory
	if err := s.memory.RevokeKey(ctx, id); err != nil {
		return err
	}

	key, err := s.memory.GetKey(ctx, id)
	if err != nil {
		return err
	}

	data, err := json.Marshal(key)
	if err != nil {
		return err
	}

	return s.rdb.Set(ctx, RedisKeyMetaPrefix+id, data, 0).Err()
}

func (s *RedisStore) TouchKey(ctx context.Context, id string) error {
	_ = s.memory.TouchKey(ctx, id)
	return nil
}

func (s *RedisStore) Close() error {
	return s.rdb.Close()
}

// -----------------------------------------------------------------------------
// Factory & Bootstrap
// -----------------------------------------------------------------------------

// Bootstrap registers initial master credentials from .env and config.
func Bootstrap(ctx context.Context, store Store, authTokens []string, metricsToken string) error {
	// 1. Ingest Master AUTH credentials
	for i, token := range authTokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		hash := HashKey(token)
		if _, err := store.GetKeyByHash(ctx, hash); errors.Is(err, ErrKeyNotFound) {
			id := fmt.Sprintf("key_bootstrap_auth_%d", i+1)
			k := &Key{
				ID:          id,
				Prefix:      ExtractPrefix(token),
				KeyHash:     hash,
				Type:        TypeAuth,
				Role:        RoleMaster,
				Status:      StatusActive,
				CreatedAt:   time.Now().UTC(),
				CreatedBy:   "system_bootstrap",
				Description: "Default Master AUTH API key from configuration",
			}
			if err := store.CreateKey(ctx, k); err != nil {
				slog.Error("failed_registering_bootstrap_auth_key", "error", err)
			} else {
				slog.Info("registered_bootstrap_auth_key", "id", id, "prefix", k.Prefix)
			}
		}
	}

	// 2. Ingest Master METRICS credential
	metricsToken = strings.TrimSpace(metricsToken)
	if metricsToken != "" {
		hash := HashKey(metricsToken)
		if _, err := store.GetKeyByHash(ctx, hash); errors.Is(err, ErrKeyNotFound) {
			id := "key_bootstrap_metrics"
			k := &Key{
				ID:          id,
				Prefix:      ExtractPrefix(metricsToken),
				KeyHash:     hash,
				Type:        TypeMetrics,
				Role:        RoleMaster,
				Status:      StatusActive,
				CreatedAt:   time.Now().UTC(),
				CreatedBy:   "system_bootstrap",
				Description: "Default Master METRICS API key from configuration",
			}
			if err := store.CreateKey(ctx, k); err != nil {
				slog.Error("failed_registering_bootstrap_metrics_key", "error", err)
			} else {
				slog.Info("registered_bootstrap_metrics_key", "id", id, "prefix", k.Prefix)
			}
		}
	}

	return nil
}

// NewStore initializes the appropriate key store and seeds bootstrap credentials.
func NewStore(cfg *config.Config) (Store, error) {
	var store Store

	if cfg.Redis.URL != "" && cfg.Redis.URL != "none" {
		rs, err := NewRedisStore(cfg.Redis.URL)
		if err != nil {
			slog.Warn("redis_keys_store_unavailable_fallback_memory", "error", err)
			store = NewMemoryStore()
		} else {
			slog.Info("connected_to_redis_key_store", "url", cfg.Redis.URL)
			store = rs
		}
	} else {
		store = NewMemoryStore()
	}

	metricsToken := os.Getenv("METRICS_TOKEN")
	if err := Bootstrap(context.Background(), store, cfg.Auth.Tokens, metricsToken); err != nil {
		slog.Error("failed_bootstrapping_key_store", "error", err)
	}

	return store, nil
}
