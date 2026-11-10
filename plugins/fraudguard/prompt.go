// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

var promptOverridePhrases = [...]string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"disregard previous instructions",
	"override the system prompt",
	"reveal the system prompt",
	"ignore the system message",
	"developer mode enabled",
	"act as an unrestricted",
}

type promptInspection struct {
	overrideDetected  bool
	customRuleMatched bool
	userBytes         int64
}

// inspectBifrostRequest reads only user-authored text fields, transiently. It
// ignores system/developer messages, raw request bodies, tool data, and media.
// The returned value contains no text and is not stored in profile history.
func inspectBifrostRequest(req *schemas.BifrostRequest, maxScanBytes int, scanForOverride bool) promptInspection {
	return inspectBifrostRequestWithRules(req, maxScanBytes, scanForOverride, nil)
}

func inspectBifrostRequestWithRules(
	req *schemas.BifrostRequest,
	maxScanBytes int,
	scanForOverride bool,
	customRules *CustomPromptRules,
) promptInspection {
	if req == nil {
		return promptInspection{}
	}
	if maxScanBytes <= 0 {
		maxScanBytes = 64 * 1024
	}

	result := promptInspection{}
	scanned := 0
	ruleSnapshot := customRules.snapshot()
	scanForCustomRule := ruleSnapshot.hasRules()
	visit := func(text string) {
		if len(text) == 0 {
			return
		}
		if int64(len(text)) > int64(^uint64(0)>>1)-result.userBytes {
			result.userBytes = int64(^uint64(0) >> 1)
		} else {
			result.userBytes += int64(len(text))
		}
		if scanned >= maxScanBytes ||
			(scanForOverride && result.overrideDetected && (!scanForCustomRule || result.customRuleMatched)) ||
			(!scanForOverride && (!scanForCustomRule || result.customRuleMatched)) {
			return
		}
		remaining := maxScanBytes - scanned
		if len(text) > remaining {
			text = text[:remaining]
		}
		scanned += len(text)
		if scanForOverride && !result.overrideDetected {
			lower := strings.ToLower(text)
			for _, phrase := range promptOverridePhrases {
				if strings.Contains(lower, phrase) {
					result.overrideDetected = true
					break
				}
			}
		}
		if scanForCustomRule && !result.customRuleMatched {
			result.customRuleMatched = ruleSnapshot.matches(text)
		}
	}

	if request := req.ChatRequest; request != nil {
		for _, message := range request.Input {
			if message.Role != schemas.ChatMessageRoleUser || message.Content == nil {
				continue
			}
			if message.Content.ContentStr != nil {
				visit(*message.Content.ContentStr)
			}
			for _, block := range message.Content.ContentBlocks {
				if block.Type == schemas.ChatContentBlockTypeText && block.Text != nil {
					visit(*block.Text)
				}
			}
		}
	}

	if request := req.TextCompletionRequest; request != nil && request.Input != nil {
		if request.Input.PromptStr != nil {
			visit(*request.Input.PromptStr)
		}
		for _, prompt := range request.Input.PromptArray {
			visit(prompt)
		}
	}

	if request := req.ResponsesRequest; request != nil {
		for _, message := range request.Input {
			if message.Role == nil || *message.Role != schemas.ResponsesInputMessageRoleUser || message.Content == nil {
				continue
			}
			if message.Content.ContentStr != nil {
				visit(*message.Content.ContentStr)
			}
			for _, block := range message.Content.ContentBlocks {
				if block.Type == schemas.ResponsesInputMessageContentBlockTypeText && block.Text != nil {
					visit(*block.Text)
				}
			}
		}
	}

	return result
}
