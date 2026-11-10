// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync/atomic"
)

const (
	maxCustomPromptRules          = 16
	maxCustomPromptRuleBytes      = 256
	maxCustomPromptRulesJSONBytes = 16 * 1024
	maxCustomPromptScanBytes      = 64 * 1024
)

// CustomPromptRule is a trusted operator-supplied RE2 expression. Rule text is
// never emitted in findings, logs, or telemetry.
type CustomPromptRule struct {
	Pattern string `json:"pattern"`
}

// CustomPromptRules atomically swaps bounded, compiled advisory rules. Updates
// must come from trusted host code; this manager has no Redis or HTTP writer.
type CustomPromptRules struct {
	active atomic.Pointer[compiledPromptRules]
}

type compiledPromptRules struct {
	patterns []*regexp.Regexp
}

func NewCustomPromptRules() *CustomPromptRules {
	rules := &CustomPromptRules{}
	rules.active.Store(&compiledPromptRules{})
	return rules
}

// ReplaceJSON validates and atomically installs a complete rule set. Invalid
// updates leave the previous compiled set in place.
func (rules *CustomPromptRules) ReplaceJSON(data []byte) error {
	if rules == nil {
		return errors.New("fraudguard: custom prompt rules are not initialized")
	}
	if len(data) == 0 || len(data) > maxCustomPromptRulesJSONBytes {
		return errors.New("fraudguard: custom prompt rule document must be 1 to 16384 bytes")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var entries []CustomPromptRule
	if err := decoder.Decode(&entries); err != nil || entries == nil {
		return errors.New("fraudguard: invalid custom prompt rule document")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("fraudguard: custom prompt rule document must contain one JSON value")
	}
	if len(entries) > maxCustomPromptRules {
		return errors.New("fraudguard: custom prompt rule count cannot exceed 16")
	}

	compiled := &compiledPromptRules{patterns: make([]*regexp.Regexp, 0, len(entries))}
	for _, entry := range entries {
		if strings.TrimSpace(entry.Pattern) == "" || len(entry.Pattern) > maxCustomPromptRuleBytes {
			return errors.New("fraudguard: custom prompt rule pattern must be 1 to 256 bytes")
		}
		pattern, err := regexp.Compile(entry.Pattern)
		if err != nil {
			return errors.New("fraudguard: custom prompt rule expression is invalid")
		}
		if pattern == nil {
			return errors.New("fraudguard: custom prompt rule expression is invalid")
		}
		compiled.patterns = append(compiled.patterns, pattern)
	}
	rules.active.Store(compiled)
	return nil
}

func customPromptRulesForConfig(config Config) *CustomPromptRules {
	if !config.EnableCustomPromptRules {
		return nil
	}
	return config.CustomPromptRules
}

// MatchText applies the current rule snapshot to at most 64 KiB of text.
func (rules *CustomPromptRules) MatchText(text string) bool {
	if len(text) > maxCustomPromptScanBytes {
		text = text[:maxCustomPromptScanBytes]
	}
	return rules.snapshot().matches(text)
}

func (rules *CustomPromptRules) snapshot() *compiledPromptRules {
	if rules == nil {
		return nil
	}
	return rules.active.Load()
}

func (rules *compiledPromptRules) matches(text string) bool {
	if rules == nil {
		return false
	}
	for _, pattern := range rules.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func (rules *compiledPromptRules) hasRules() bool {
	return rules != nil && len(rules.patterns) > 0
}
