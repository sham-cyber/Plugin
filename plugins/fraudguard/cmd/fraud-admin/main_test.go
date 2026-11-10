// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunReadsAggregateMetricsWithOptionalToken(t *testing.T) {
	const token = "private-admin-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/fraudguard/metrics" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("authorization header = %q", got)
		}
		_, _ = w.Write([]byte("# TYPE fraudguard_alerts_total counter\nfraudguard_alerts_total 3\n"))
	}))
	defer server.Close()

	var output bytes.Buffer
	err := run([]string{"metrics"}, func(key string) (string, bool) {
		switch key {
		case "FRAUDGUARD_ADMIN_URL":
			return server.URL, true
		case "FRAUDGUARD_ADMIN_TOKEN":
			return token, true
		default:
			return "", false
		}
	}, server.Client(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), token) || !strings.Contains(output.String(), "fraudguard_alerts_total 3") {
		t.Fatalf("unexpected or sensitive output: %q", output.String())
	}
}

func TestRunRejectsMutationCommandsBeforeNetwork(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})}
	err := run([]string{"unblock"}, func(string) (string, bool) { return "", false }, client, &bytes.Buffer{})
	if err == nil || called {
		t.Fatal("mutation command was accepted or made a network call")
	}
}

func TestRunDoesNotPrintUnauthorizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("private response detail"))
	}))
	defer server.Close()
	var output bytes.Buffer
	err := run([]string{"status"}, func(key string) (string, bool) {
		if key == "FRAUDGUARD_ADMIN_URL" {
			return server.URL, true
		}
		return "", false
	}, server.Client(), &output)
	if err == nil || strings.Contains(output.String(), "private response detail") {
		t.Fatal("unauthorized response was accepted or leaked")
	}
}

func TestAdminEndpointURLRejectsCredentialAndQueryData(t *testing.T) {
	for _, input := range []string{
		"https://user:password@example.test",
		"https://example.test?token=secret",
		"file:///tmp/admin",
	} {
		if _, err := adminEndpointURL(input, "/admin/fraudguard/status"); err == nil {
			t.Fatalf("unsafe URL accepted: %q", input)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
