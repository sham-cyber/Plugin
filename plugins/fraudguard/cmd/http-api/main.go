// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

// fraudguard-http-api runs the standalone observe-only metadata scoring API.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
	"github.com/maximhq/bifrost/plugins/fraudguard/httpapi"
)

func main() {
	listenAddress := envOr("FRAUDGUARD_API_LISTEN", "127.0.0.1:8787")
	rawMappings := os.Getenv("FRAUDGUARD_API_KEY_HASHES")
	mappings, err := httpapi.ParseKeyHashMappings(rawMappings)
	if err != nil {
		log.Fatalf("FraudGuard API configuration error: %v", err)
	}
	rateLimit, err := envInt("FRAUDGUARD_API_REQUESTS_PER_MINUTE", 120)
	if err != nil {
		log.Fatalf("FraudGuard API configuration error: %v", err)
	}

	engine, err := fraudguard.NewEngine(fraudguard.Config{
		WarmupRequests: 5,
		Window:         15 * time.Minute,
		ProfileTTL:     2 * time.Hour,
		MaxProfiles:    10_000,
		AlertThreshold: 60,
	})
	if err != nil {
		log.Fatalf("Could not initialize FraudGuard engine: %v", err)
	}
	handler, err := httpapi.NewHandler(engine, httpapi.Config{
		KeyHashes:         mappings,
		RequestsPerMinute: rateLimit,
	})
	if err != nil {
		log.Fatalf("FraudGuard API configuration error: %v", err)
	}

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("FraudGuard observe-only API listening on %s", listenAddress)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("FraudGuard API stopped: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	if parsed < 1 || parsed > 100_000 {
		return 0, errors.New(name + " must be between 1 and 100000")
	}
	return parsed, nil
}
