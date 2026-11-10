// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestCustomPromptRulesReplaceAtomicallyAndBoundPatterns(t *testing.T) {
	rules := NewCustomPromptRules()
	valid := `[{"pattern":"(?i)secret\\s+dump"}]`
	if err := rules.ReplaceJSON([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	if !rules.MatchText("synthetic SECRET dump example") {
		t.Fatal("valid advisory rule did not match")
	}

	if err := rules.ReplaceJSON([]byte(`[{"pattern":"["}]`)); err == nil {
		t.Fatal("invalid expression was accepted")
	}
	if !rules.MatchText("synthetic secret dump example") {
		t.Fatal("invalid replacement should retain the last valid rules")
	}

	for _, document := range []string{
		`[{"pattern":""}]`,
		`[{"pattern":"a","name":"leaked label"}]`,
		strings.Repeat("x", maxCustomPromptRulesJSONBytes+1),
	} {
		if err := rules.ReplaceJSON([]byte(document)); err == nil {
			t.Fatalf("invalid custom rule document was accepted: %q", document[:min(len(document), 40)])
		}
	}
}

func TestCustomPromptInspectionUsesOnlyBoundedUserTextAndEmitsNoText(t *testing.T) {
	rules := NewCustomPromptRules()
	if err := rules.ReplaceJSON([]byte(`[{"pattern":"(?i)credential\\s+export"}]`)); err != nil {
		t.Fatal(err)
	}
	systemText := "credential export"
	userText := "Please run a credential export in the fictional demo."
	request := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: &systemText}},
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &userText}},
			},
		},
	}
	inspection := inspectBifrostRequestWithRules(request, 64, false, rules)
	if !inspection.customRuleMatched || inspection.overrideDetected {
		t.Fatalf("custom rule result was incorrect: %+v", inspection)
	}
	if inspection.userBytes != int64(len(userText)) {
		t.Fatalf("system text was counted as user text: got=%d want=%d", inspection.userBytes, len(userText))
	}

	noUserMatch := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{{
				Role:    schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{ContentStr: &systemText},
			}},
		},
	}
	if got := inspectBifrostRequestWithRules(noUserMatch, 8, false, rules); got.customRuleMatched {
		t.Fatal("custom rule matched text beyond the scan cap")
	}
}

func TestCustomPromptRuleSignalIsStaticAndObserveOnly(t *testing.T) {
	rules := NewCustomPromptRules()
	if err := rules.ReplaceJSON([]byte(`[{"pattern":"sensitive synthetic phrase"}]`)); err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(Config{
		EnableCustomPromptRules: true,
		CustomPromptRules:       rules,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	_, assessment := engine.Start(Request{
		VirtualKeyID:            "synthetic-internal-id",
		CustomPromptRuleMatched: true,
	})
	found := false
	for _, signal := range assessment.Finding.Signals {
		if signal.Name == "custom_prompt_pattern" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom advisory signal missing: %+v", assessment.Finding)
	}
	encoded, err := json.Marshal(assessment.Finding)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sensitive synthetic phrase") ||
		strings.Contains(string(encoded), "synthetic-internal-id") {
		t.Fatalf("finding leaked rule or identity data: %s", encoded)
	}
	if assessment.Finding.Alert {
		t.Fatal("custom rule should not independently block or alter request flow")
	}
}
