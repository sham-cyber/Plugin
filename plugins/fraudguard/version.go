// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"regexp"
	"strings"
	"time"
)

// These variables can be set by the host build with Go linker -X flags.
// They are intentionally not populated from environment variables.
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildTime    = "unknown"
)

var (
	semanticVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	commitHashPattern      = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

// BuildInfo contains only bounded, validated release metadata.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// BuildMetadata returns sanitized metadata for authenticated health/admin JSON.
// Version values are deliberately omitted from metric labels to avoid
// high-cardinality time series.
func BuildMetadata() BuildInfo {
	return buildMetadataFor(BuildVersion, BuildCommit, BuildTime)
}

func buildMetadataFor(version, commit, buildTime string) BuildInfo {
	version = strings.TrimSpace(version)
	if version != "dev" && !semanticVersionPattern.MatchString(version) {
		version = "dev"
	}

	commit = strings.TrimSpace(commit)
	if !commitHashPattern.MatchString(commit) {
		commit = "unknown"
	}

	safeBuildTime := "unknown"
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(buildTime)); err == nil {
		safeBuildTime = parsed.UTC().Format(time.RFC3339)
	}

	return BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildTime: safeBuildTime,
	}
}
