// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package verdict implements the deterministic referee for the defect
// diagnoser. The model writes the "## Verdict"; this package decides whether
// that verdict stands. It owns the model-output heading rule, the verdict gate
// rules a-k, the line guard and the step wrappers that run them inside the
// phase chain.
package verdict

import (
	"context"
	"fmt"
	"strings"

	agentlib "github.com/bborbe/agent"
	"gopkg.in/yaml.v3"
)

// Field names, in the contract's order.
const (
	FieldDefectID       = "defect_id"
	FieldService        = "service"
	FieldRepo           = "repo"
	FieldFileLine       = "file:line"
	FieldRootCause      = "root_cause"
	FieldRecommendedFix = "recommended_fix"
	FieldEvidence       = "evidence"
	FieldConfidence     = "confidence"
)

// Confidence values, case-sensitive.
const (
	ConfidenceHigh   = "High"
	ConfidenceMedium = "Medium"
	ConfidenceLow    = "Low"
)

// VerdictSectionHeading is the heading of the section that carries the verdict.
const VerdictSectionHeading = "## Verdict"

// VerdictFieldOrder is the eight field names in the contract's order.
var VerdictFieldOrder = []string{
	FieldDefectID,
	FieldService,
	FieldRepo,
	FieldFileLine,
	FieldRootCause,
	FieldRecommendedFix,
	FieldEvidence,
	FieldConfidence,
}

// Verdict is the parsed ## Verdict contract block.
type Verdict struct {
	DefectID       string
	Service        string
	Repo           string
	FileLine       string
	RootCause      string
	RecommendedFix string
	Evidence       string
	Confidence     string
}

// parseOutcome distinguishes rule a's four failures.
type parseOutcome int

const (
	parseOK parseOutcome = iota
	parseNoSection
	parseNoBlock
	parseInvalidYAML
)

// fields holds the ## Verdict yaml block's values keyed by the contract's field
// names. A key is absent when the field is missing from the block; a nil yaml
// value is normalised to the empty string (never the literal "<nil>").
type fields map[string]string

// newVerdict reads the eight contract fields out of fields in order.
func newVerdict(f fields) Verdict {
	return Verdict{
		DefectID:       f[FieldDefectID],
		Service:        f[FieldService],
		Repo:           f[FieldRepo],
		FileLine:       f[FieldFileLine],
		RootCause:      f[FieldRootCause],
		RecommendedFix: f[FieldRecommendedFix],
		Evidence:       f[FieldEvidence],
		Confidence:     f[FieldConfidence],
	}
}

// extractFields reads the last fenced ```yaml block of the ## Verdict section.
func extractFields(ctx context.Context, md *agentlib.Markdown) (fields, parseOutcome) {
	section, ok := md.FindSection(VerdictSectionHeading)
	if !ok {
		return nil, parseNoSection
	}
	block, ok := lastYAMLBlock(section.Body)
	if !ok {
		return nil, parseNoBlock
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil, parseInvalidYAML
	}
	return normaliseFields(ctx, raw), parseOK
}

// lastYAMLBlock returns the body of the last complete fenced ```yaml block.
// A block that opens and never closes is not counted.
func lastYAMLBlock(body string) (string, bool) {
	var (
		blocks  []string
		current []string
		inBlock bool
	)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "```yaml" {
				inBlock = true
				current = nil
			}
			continue
		}
		if trimmed == "```" {
			blocks = append(blocks, strings.Join(current, "\n"))
			inBlock = false
			continue
		}
		current = append(current, line)
	}
	if len(blocks) == 0 {
		return "", false
	}
	return blocks[len(blocks)-1], true
}

// normaliseFields converts every present contract key to a string. A nil value
// becomes the empty string; a key absent from the block stays absent.
func normaliseFields(_ context.Context, raw map[string]interface{}) fields {
	out := make(fields, len(VerdictFieldOrder))
	for _, name := range VerdictFieldOrder {
		value, present := raw[name]
		if !present {
			continue
		}
		if value == nil {
			out[name] = ""
			continue
		}
		out[name] = fmt.Sprintf("%v", value)
	}
	return out
}
