// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	defaultContextMessages  = 32
	maxContextMessages      = 256
	defaultContextScanBytes = 64 * 1024
	maxContextScanBytes     = 1024 * 1024
)

// ContextMessage is one transient, caller-provided conversation entry.
// InspectConversationContext reads only entries whose role is "user".
type ContextMessage struct {
	Role string
	Text string
}

type ContextGuardOptions struct {
	MaxMessages  int
	MaxScanBytes int
}

// ContextGuardAssessment contains fixed booleans and bounded counters only.
// It never returns or retains message text.
type ContextGuardAssessment struct {
	ProgressiveJailbreak bool `json:"progressive_jailbreak"`
	DataExfiltration     bool `json:"data_exfiltration"`
	SchemaManipulation   bool `json:"schema_manipulation"`
	MessagesScanned      int  `json:"messages_scanned"`
	BytesScanned         int  `json:"bytes_scanned"`
}

var (
	contextFramingPhrases = [...]string{
		"for the next message",
		"for your next response",
		"when i ask again",
		"in the next step",
		"roleplay as",
		"pretend we are",
		"hypothetically,",
	}
	contextOverridePhrases = [...]string{
		"ignore previous instructions",
		"ignore all previous instructions",
		"disregard your instructions",
		"override the system prompt",
		"developer mode",
		"bypass your restrictions",
		"reveal hidden instructions",
	}
	contextDisclosureActions = [...]string{
		"reveal", "print", "show", "send", "dump", "exfiltrate", "export",
	}
	contextSensitiveTargets = [...]string{
		"system prompt", "hidden instructions", "api key", "api keys", "secret",
		"credential", "environment variable", "access token",
	}
	contextSchemaMarkers = [...]string{
		"$schema", "additionalproperties", "tool_choice", "response_format",
		"function schema", "tool schema", `"required"`,
	}
	contextSchemaDirectives = [...]string{
		"override", "replace", "modify", "remove", "bypass", "ignore", "change",
	}
)

// InspectConversationContext applies small local phrase heuristics to a bounded
// view of user turns. It is opt-in, transient, and returns only booleans/counts.
// It is not an intent classifier and can produce false positives.
func InspectConversationContext(messages []ContextMessage, options ContextGuardOptions) ContextGuardAssessment {
	maxMessages := options.MaxMessages
	if maxMessages <= 0 {
		maxMessages = defaultContextMessages
	}
	if maxMessages > maxContextMessages {
		maxMessages = maxContextMessages
	}
	maxBytes := options.MaxScanBytes
	if maxBytes <= 0 {
		maxBytes = defaultContextScanBytes
	}
	if maxBytes > maxContextScanBytes {
		maxBytes = maxContextScanBytes
	}

	result := ContextGuardAssessment{}
	remaining := maxBytes
	priorFraming := false
	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			continue
		}
		if result.MessagesScanned >= maxMessages || remaining <= 0 {
			break
		}
		text := message.Text
		if text == "" {
			continue
		}
		if len(text) > remaining {
			text = text[:remaining]
		}
		remaining -= len(text)
		result.BytesScanned += len(text)
		result.MessagesScanned++

		lower := strings.ToLower(text)
		if priorFraming && containsAny(lower, contextOverridePhrases[:]) {
			result.ProgressiveJailbreak = true
		}
		if containsAny(lower, contextDisclosureActions[:]) &&
			containsAny(lower, contextSensitiveTargets[:]) {
			result.DataExfiltration = true
		}
		if containsAny(lower, contextSchemaMarkers[:]) &&
			containsAny(lower, contextSchemaDirectives[:]) {
			result.SchemaManipulation = true
		}
		if containsAny(lower, contextFramingPhrases[:]) {
			priorFraming = true
		}
	}
	return result
}

// inspectBifrostContext adapts typed Bifrost request messages to the same
// bounded, user-only inspection path. Non-user roles and non-text payloads are
// deliberately ignored.
func inspectBifrostContext(req *schemas.BifrostRequest, options ContextGuardOptions) ContextGuardAssessment {
	if req == nil {
		return ContextGuardAssessment{}
	}
	messages := make([]ContextMessage, 0, 8)
	addContent := func(content *schemas.ChatMessageContent) {
		if content == nil {
			return
		}
		if content.ContentStr != nil {
			messages = append(messages, ContextMessage{Role: "user", Text: *content.ContentStr})
		}
		for _, block := range content.ContentBlocks {
			if block.Type == schemas.ChatContentBlockTypeText && block.Text != nil {
				messages = append(messages, ContextMessage{Role: "user", Text: *block.Text})
			}
		}
	}
	if chat := req.ChatRequest; chat != nil {
		for _, message := range chat.Input {
			if message.Role == schemas.ChatMessageRoleUser {
				addContent(message.Content)
			}
		}
	}
	if text := req.TextCompletionRequest; text != nil && text.Input != nil {
		if text.Input.PromptStr != nil {
			messages = append(messages, ContextMessage{Role: "user", Text: *text.Input.PromptStr})
		}
		for _, prompt := range text.Input.PromptArray {
			messages = append(messages, ContextMessage{Role: "user", Text: prompt})
		}
	}
	if responses := req.ResponsesRequest; responses != nil {
		for _, message := range responses.Input {
			if message.Role == nil || *message.Role != schemas.ResponsesInputMessageRoleUser {
				continue
			}
			if message.Content == nil {
				continue
			}
			if message.Content.ContentStr != nil {
				messages = append(messages, ContextMessage{Role: "user", Text: *message.Content.ContentStr})
			}
			for _, block := range message.Content.ContentBlocks {
				if block.Type == schemas.ResponsesInputMessageContentBlockTypeText && block.Text != nil {
					messages = append(messages, ContextMessage{Role: "user", Text: *block.Text})
				}
			}
		}
	}
	return InspectConversationContext(messages, options)
}

func containsAny(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
