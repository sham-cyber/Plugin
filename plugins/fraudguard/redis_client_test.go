// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisClientFromURLUsesConfiguredBoundedPool(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := NewRedisClientFromURL(context.Background(), "redis://"+server.Addr(), RedisClientConfig{
		OperationTimeout: time.Second,
		Pool: RedisPoolOptions{
			PoolSize:     8,
			MinIdleConns: 1,
			MaxIdleConns: 4,
			PoolTimeout:  100 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	options := client.Options()
	if options.PoolSize != 8 || options.MinIdleConns != 1 ||
		options.MaxIdleConns != 4 || options.PoolTimeout != 100*time.Millisecond {
		t.Fatalf("unexpected Redis pool configuration: %+v", options)
	}
}

func TestRedisClientRejectsUnsafePoolBoundsWithoutEchoingURL(t *testing.T) {
	_, err := NewRedisClientFromURL(context.Background(), "redis://user:secret@example.invalid:6379", RedisClientConfig{
		OperationTimeout: time.Second,
		Pool:             RedisPoolOptions{PoolSize: maxRedisPoolSize + 1},
	})
	if err == nil {
		t.Fatal("oversized Redis pool was accepted")
	}
	if got := err.Error(); got == "" || strings.Contains(got, "secret") || strings.Contains(got, "example.invalid") {
		t.Fatalf("Redis error leaked connection details: %q", got)
	}

	_, err = NewRedisClientFromURL(context.Background(), "redis://localhost:6379", RedisClientConfig{
		OperationTimeout: time.Second,
		Pool:             RedisPoolOptions{PoolSize: 2, MinIdleConns: 2, MaxIdleConns: 1},
	})
	if err == nil {
		t.Fatal("inconsistent idle connection bounds were accepted")
	}
}

func TestRedisPoolDefaultsStayWithinOperationTimeout(t *testing.T) {
	pool, err := normalizeRedisPool(RedisPoolOptions{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if pool.PoolSize != defaultRedisPoolSize || pool.MaxIdleConns != defaultRedisPoolSize ||
		pool.PoolTimeout > time.Second {
		t.Fatalf("unexpected defaults: %+v", pool)
	}
}
