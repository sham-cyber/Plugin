// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisPoolSize = 32
	maxRedisPoolSize     = 512
	defaultRedisPoolWait = 250 * time.Millisecond
	maxRedisPoolWait     = 5 * time.Second
)

// RedisPoolOptions configures go-redis's standard connection pool. This pool is
// for optional product services, not behavioral-profile or request-rate state.
type RedisPoolOptions struct {
	PoolSize     int
	MinIdleConns int
	MaxIdleConns int
	PoolTimeout  time.Duration
}

// RedisClientConfig bounds connection and pool waits. It cannot guarantee a
// particular Redis latency; host, network, server load, and pool contention vary.
type RedisClientConfig struct {
	OperationTimeout  time.Duration
	Pool              RedisPoolOptions
	FailOpenOnStartup bool
}

// NewRedisClientFromURL builds a bounded go-redis client. The URL must come from
// a secret manager; parse, dial, and ping errors never include the URL.
func NewRedisClientFromURL(ctx context.Context, redisURL string, config RedisClientConfig) (*redis.Client, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, errors.New("fraudguard: Redis URL is required")
	}
	timeout := boundedRedisTimeout(config.OperationTimeout)
	pool, err := normalizeRedisPool(config.Pool, timeout)
	if err != nil {
		return nil, err
	}
	parsed, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, errors.New("fraudguard: invalid Redis URL")
	}
	parsed.ContextTimeoutEnabled = true
	if parsed.DialTimeout <= 0 || parsed.DialTimeout > timeout {
		parsed.DialTimeout = timeout
	}
	if parsed.ReadTimeout <= 0 || parsed.ReadTimeout > timeout {
		parsed.ReadTimeout = timeout
	}
	if parsed.WriteTimeout <= 0 || parsed.WriteTimeout > timeout {
		parsed.WriteTimeout = timeout
	}
	parsed.PoolSize = pool.PoolSize
	parsed.MinIdleConns = pool.MinIdleConns
	parsed.MaxIdleConns = pool.MaxIdleConns
	parsed.PoolTimeout = pool.PoolTimeout

	client := redis.NewClient(parsed)
	if !config.FailOpenOnStartup {
		if ctx == nil {
			ctx = context.Background()
		}
		pingTimeout := timeout
		if pingTimeout > 2*time.Second {
			pingTimeout = 2 * time.Second
		}
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		err := client.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			_ = client.Close()
			return nil, errors.New("fraudguard: Redis startup ping failed")
		}
	}
	return client, nil
}

func normalizeRedisPool(pool RedisPoolOptions, operationTimeout time.Duration) (RedisPoolOptions, error) {
	if pool.PoolSize == 0 {
		pool.PoolSize = defaultRedisPoolSize
	}
	if pool.PoolSize < 1 || pool.PoolSize > maxRedisPoolSize {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis pool size must be between 1 and 512")
	}
	if pool.MinIdleConns < 0 || pool.MinIdleConns > pool.PoolSize {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis minimum idle connections must not exceed pool size")
	}
	if pool.MaxIdleConns == 0 {
		pool.MaxIdleConns = pool.PoolSize
	}
	if pool.MaxIdleConns < 1 || pool.MaxIdleConns > pool.PoolSize {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis maximum idle connections must be between 1 and pool size")
	}
	if pool.MinIdleConns > pool.MaxIdleConns {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis minimum idle connections must not exceed maximum idle connections")
	}
	if pool.PoolTimeout == 0 {
		pool.PoolTimeout = defaultRedisPoolWait
		if pool.PoolTimeout > operationTimeout {
			pool.PoolTimeout = operationTimeout
		}
	}
	if pool.PoolTimeout < time.Millisecond || pool.PoolTimeout > maxRedisPoolWait {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis pool timeout must be between 1ms and 5s")
	}
	if pool.PoolTimeout > operationTimeout {
		return RedisPoolOptions{}, errors.New("fraudguard: Redis pool timeout cannot exceed operation timeout")
	}
	return pool, nil
}
