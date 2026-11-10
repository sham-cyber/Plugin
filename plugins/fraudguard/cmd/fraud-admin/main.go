// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

// Command fraud-admin reads aggregate-only FraudGuard diagnostics from a host
// that mounts the authenticated admin handler. It intentionally has no
// unblocking, tenant lookup, Redis-key inspection, or policy-mutation command.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

type lookupEnvFunc func(string) (string, bool)

func main() {
	if err := run(os.Args[1:], os.LookupEnv, newHTTPClient(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fraud-admin:", err)
		os.Exit(1)
	}
}

func run(args []string, lookup lookupEnvFunc, client *http.Client, output io.Writer) error {
	if len(args) != 1 || args[0] == "help" || args[0] == "--help" {
		_, _ = fmt.Fprintln(output, "Usage: fraud-admin <health|status|metrics>")
		_, _ = fmt.Fprintln(output, "Read-only aggregate diagnostics; configure FRAUDGUARD_ADMIN_URL and optional FRAUDGUARD_ADMIN_TOKEN.")
		_, _ = fmt.Fprintln(output, "Tenant unblocking, raw Redis inspection, policy mutation, and cache-clearing commands are not provided.")
		return nil
	}

	var endpoint string
	switch args[0] {
	case "health":
		endpoint = "/admin/fraudguard/healthz"
	case "status":
		endpoint = "/admin/fraudguard/status"
	case "metrics":
		endpoint = "/admin/fraudguard/metrics"
	default:
		return errors.New("unsupported command; only health, status, and metrics are available")
	}
	if lookup == nil || client == nil || output == nil {
		return errors.New("admin client is not initialized")
	}

	base := "http://127.0.0.1:8081"
	if configured, ok := lookup("FRAUDGUARD_ADMIN_URL"); ok && strings.TrimSpace(configured) != "" {
		base = strings.TrimSpace(configured)
	}
	target, err := adminEndpointURL(base, endpoint)
	if err != nil {
		return errors.New("invalid FRAUDGUARD_ADMIN_URL")
	}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return errors.New("could not create admin request")
	}
	if token, ok := lookup("FRAUDGUARD_ADMIN_TOKEN"); ok && token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Accept", "application/json, text/plain")

	response, err := client.Do(request)
	if err != nil {
		return errors.New("admin endpoint request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("admin endpoint returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return errors.New("admin response could not be safely read")
	}
	if args[0] != "metrics" {
		var compact bytes.Buffer
		if err := json.Compact(&compact, body); err != nil {
			return errors.New("admin endpoint returned invalid JSON")
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
			return errors.New("admin endpoint returned invalid JSON")
		}
		body = pretty.Bytes()
	}
	_, err = output.Write(body)
	if err != nil {
		return errors.New("could not write admin output")
	}
	if len(body) == 0 || body[len(body)-1] != '\n' {
		_, err = io.WriteString(output, "\n")
	}
	return err
}

func adminEndpointURL(base, endpoint string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid admin URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpoint
	return parsed.String(), nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
