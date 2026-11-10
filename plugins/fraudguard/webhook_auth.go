// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const webhookSignaturePrefix = "sha256="

var errorsWebhookSigningKeyTooShort = errors.New("fraudguard: webhook signing key must be at least 32 bytes")

// SignWebhookPayload signs the exact HTTP body bytes. Receivers must verify the
// raw bytes before decoding JSON. Keys shorter than 32 bytes are rejected.
func SignWebhookPayload(key, payload []byte) (string, error) {
	if len(key) < 32 {
		return "", errorsWebhookSigningKeyTooShort
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return webhookSignaturePrefix + hex.EncodeToString(mac.Sum(nil)), nil
}

// webhookSignature preserves the package-private helper used by existing
// internal tests and call sites. New code should use SignWebhookPayload.
func webhookSignature(key, payload []byte) string {
	signature, _ := SignWebhookPayload(key, payload)
	return strings.TrimPrefix(signature, webhookSignaturePrefix)
}

// VerifyWebhookPayload checks a sha256=<hex> HMAC using constant-time compare.
// It authenticates the body but does not prevent replay; receivers should also
// validate the event ID and timestamp headers.
func VerifyWebhookPayload(key, payload []byte, signature string) bool {
	if len(key) < 32 || !strings.HasPrefix(signature, webhookSignaturePrefix) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, webhookSignaturePrefix))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return hmac.Equal(provided, mac.Sum(nil))
}
