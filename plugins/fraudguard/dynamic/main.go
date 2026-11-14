// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

// Package main exposes FraudGuard through Bifrost's shared-object plugin
// loader. Build this package with -buildmode=plugin from the matching Bifrost
// checkout and Go toolchain.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
)

var activePlugin struct {
	sync.RWMutex
	instance   *fraudguard.Plugin
	references int
}

// Init is called by Bifrost when it loads the shared object.
func Init(rawConfig any) error {
	config, err := decodeConfig(rawConfig)
	if err != nil {
		return err
	}
	if config.EnableAIAnalysis {
		return errors.New("fraudguard: dynamic plugin AI review needs a host-supplied AIAnalyzer; disable it or use the direct Go integration")
	}
	if config.OnFinding == nil {
		config.OnFinding = logFinding
	}

	instance, err := fraudguard.NewFromEnvironment(config)
	if err != nil {
		return err
	}

	activePlugin.Lock()
	previous := activePlugin.instance
	activePlugin.instance = instance
	activePlugin.references++
	activePlugin.Unlock()

	// Bifrost can load the same .so again during a plugin config reload. Its
	// old DynamicPlugin wrapper still points to this package's Cleanup symbol,
	// so keep a reference count: cleaning up the old wrapper must not close the
	// replacement instance.
	if previous != nil {
		if err := previous.Cleanup(); err != nil {
			log.Printf("FraudGuard previous instance cleanup failed: %v", err)
		}
	}
	return nil
}

// GetName is required by Bifrost's shared-object loader.
func GetName() string { return fraudguard.PluginName }

// Cleanup is required by Bifrost's shared-object loader.
func Cleanup() error {
	activePlugin.Lock()
	if activePlugin.references > 0 {
		activePlugin.references--
	}
	if activePlugin.references > 0 {
		activePlugin.Unlock()
		return nil
	}
	instance := activePlugin.instance
	activePlugin.instance = nil
	activePlugin.references = 0
	activePlugin.Unlock()
	if instance == nil {
		return nil
	}
	return instance.Cleanup()
}

// PreRequestHook preserves the LLMPlugin contract; FraudGuard does not route.
func PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	instance, unlock := lockPlugin()
	defer unlock()
	if instance == nil {
		return nil
	}
	return instance.PreRequestHook(ctx, req)
}

// PreLLMHook forwards to FraudGuard and always preserves observe-only behavior.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	instance, unlock := lockPlugin()
	defer unlock()
	if instance == nil {
		return req, nil, nil
	}
	return instance.PreLLMHook(ctx, req)
}

// PostLLMHook forwards the result hook without changing provider output.
func PostLLMHook(ctx *schemas.BifrostContext, response *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	instance, unlock := lockPlugin()
	defer unlock()
	if instance == nil {
		return response, bifrostErr, nil
	}
	return instance.PostLLMHook(ctx, response, bifrostErr)
}

func lockPlugin() (*fraudguard.Plugin, func()) {
	activePlugin.RLock()
	return activePlugin.instance, activePlugin.RUnlock
}

func logFinding(finding fraudguard.Finding) {
	if !finding.Alert || len(finding.Signals) == 0 {
		return
	}
	names := make([]string, 0, len(finding.Signals))
	for _, signal := range finding.Signals {
		names = append(names, signal.Name)
	}
	// Findings contain only fixed signal names and numeric scores; never log
	// requests, prompts, virtual-key values, or tenant identifiers here.
	log.Printf("FraudGuard observe-only alert: score=%d threshold=%d signals=%v",
		finding.Score, finding.Threshold, names)
}

// decodeConfig accepts Bifrost's JSON config map using snake_case field names.
// Runtime callbacks and adapters cannot be supplied through JSON.
func decodeConfig(raw any) (fraudguard.Config, error) {
	var config fraudguard.Config
	if raw == nil {
		return config, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return config, fmt.Errorf("fraudguard: encode plugin config: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return config, fmt.Errorf("fraudguard: plugin config must be an object: %w", err)
	}

	configValue := reflect.ValueOf(&config).Elem()
	configType := configValue.Type()
	for key, value := range fields {
		fieldName := normalizeFieldName(key)
		if fieldName == "version" {
			var version int
			if err := json.Unmarshal(value, &version); err != nil || version != 1 {
				return config, fmt.Errorf("fraudguard: unsupported config version in %q", key)
			}
			continue
		}
		fieldIndex := -1
		for i := 0; i < configType.NumField(); i++ {
			field := configType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			if normalizeFieldName(field.Name) == fieldName {
				fieldIndex = i
				break
			}
		}
		if fieldIndex < 0 {
			return config, fmt.Errorf("fraudguard: unknown plugin config field %q", key)
		}

		field := configType.Field(fieldIndex)
		switch field.Name {
		case "AIAnalyzer", "OnAIReview", "OnFinding", "ThreatIntelFeed":
			return config, fmt.Errorf("fraudguard: %s is runtime-only and cannot be set through Bifrost JSON config", key)
		}
		target := reflect.New(field.Type)
		if field.Type == reflect.TypeOf(time.Duration(0)) {
			var durationString string
			if err := json.Unmarshal(value, &durationString); err == nil {
				duration, err := time.ParseDuration(durationString)
				if err != nil {
					return config, fmt.Errorf("fraudguard: invalid duration for %s: %w", key, err)
				}
				target.Elem().SetInt(int64(duration))
			} else if err := json.Unmarshal(value, target.Interface()); err != nil {
				return config, fmt.Errorf("fraudguard: invalid duration for %s: use a Go duration string or nanoseconds", key)
			}
			configValue.Field(fieldIndex).Set(target.Elem())
			continue
		}
		if err := json.Unmarshal(value, target.Interface()); err != nil {
			return config, fmt.Errorf("fraudguard: invalid value for %s: %w", key, err)
		}
		configValue.Field(fieldIndex).Set(target.Elem())
	}
	return config, nil
}

func normalizeFieldName(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, r := range value {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		normalized.WriteRune(r)
	}
	return strings.ToLower(normalized.String())
}
