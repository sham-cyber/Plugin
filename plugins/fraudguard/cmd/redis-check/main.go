// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	address := os.Getenv("FRAUDGUARD_REDIS_URL")
	if address == "" {
		fmt.Fprintln(os.Stderr, "FRAUDGUARD_REDIS_URL is required")
		os.Exit(2)
	}
	options, err := redis.ParseURL(address)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FRAUDGUARD_REDIS_URL is not a valid Redis URL")
		os.Exit(2)
	}
	options.MaxRetries = -1
	client := redis.NewClient(options)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Redis PING failed; check connectivity and credentials were not printed")
		os.Exit(1)
	}
	fmt.Println("Redis PING succeeded")
}
