// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package prompts provides embedded prompt fragments for the Claude agent.
package prompts

import (
	_ "embed"

	claudelib "github.com/bborbe/agent/claude"
	"github.com/bborbe/vault-cli/pkg/domain"
)

//go:embed planning.md
var planning string

//go:embed execution.md
var execution string

//go:embed ai_review.md
var aiReview string

//go:embed output-format.md
var outputFormat string

// BuildInstructions assembles the prompt for one diagnoser phase: that phase's
// instructions followed by the shared output-format envelope.
func BuildInstructions(phase domain.TaskPhase) claudelib.Instructions {
	var phaseInstruction claudelib.Instruction
	switch phase {
	case domain.TaskPhasePlanning:
		phaseInstruction = claudelib.Instruction{Name: "planning", Content: planning}
	case domain.TaskPhaseExecution:
		phaseInstruction = claudelib.Instruction{Name: "execution", Content: execution}
	case domain.TaskPhaseAIReview:
		phaseInstruction = claudelib.Instruction{Name: "ai_review", Content: aiReview}
	default:
		return claudelib.Instructions{{Name: "output-format", Content: outputFormat}}
	}
	return claudelib.Instructions{
		phaseInstruction,
		{Name: "output-format", Content: outputFormat},
	}
}
