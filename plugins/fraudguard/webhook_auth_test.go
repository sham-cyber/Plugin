// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import "testing"

func TestWebhookPayloadSignatureVerifiesExactBodyInConstantTimeHelper(t *testing.T) {
	key := []byte("test-only-webhook-signing-key-01234567890123456789")
	body := []byte(`{"event":"risk_finding","score":75}`)
	signature, err := SignWebhookPayload(key, body)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyWebhookPayload(key, body, signature) {
		t.Fatal("valid webhook signature did not verify")
	}
	if VerifyWebhookPayload(key, []byte(`{"event":"risk_finding","score":76}`), signature) {
		t.Fatal("modified webhook body verified")
	}
	if VerifyWebhookPayload([]byte("different-key-that-is-long-enough-123456"), body, signature) {
		t.Fatal("signature verified with the wrong key")
	}
	if VerifyWebhookPayload(key, body, signature[len(webhookSignaturePrefix):]) ||
		VerifyWebhookPayload(key, body, "sha256=not-hex") {
		t.Fatal("malformed or unprefixed signature verified")
	}
	if _, err := SignWebhookPayload([]byte("short"), body); err == nil {
		t.Fatal("short HMAC key was accepted")
	}
	if VerifyWebhookPayload([]byte("short"), body, signature) {
		t.Fatal("short HMAC key verified")
	}
}
