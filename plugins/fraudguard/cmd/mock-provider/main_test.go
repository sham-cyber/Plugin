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

func TestServeCompletionReturnsSyntheticResponseWithoutEchoingPrompt(t *testing.T) {
	body := `{"model":"model-a","messages":[{"role":"user","content":"private prompt must not be echoed"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	response := httptest.NewRecorder()

	serveCompletion(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("private prompt")) {
		t.Fatal("mock provider echoed request content")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"prompt_tokens":24`)) {
		t.Fatal("mock provider omitted synthetic usage")
	}
}

func TestServeCompletionRejectsMissingModel(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	response := httptest.NewRecorder()

	serveCompletion(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestServeCompletionBoundsRequestBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"model-a","extra":"`+strings.Repeat("x", maxRequestBody)+`"}`,
	))
	response := httptest.NewRecorder()

	serveCompletion(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
