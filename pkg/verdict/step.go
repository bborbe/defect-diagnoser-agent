// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict

import (
	"context"
	"strings"

	agentlib "github.com/bborbe/agent"
	"github.com/bborbe/errors"
	"github.com/bborbe/vault-cli/pkg/domain"
)

const (
	planSection    = "## Plan"
	reviewSection  = "## Review"
	failureSection = "## Failure"
)

// PrefixMessage prefixes every line of message with "> ".
func PrefixMessage(message string) string {
	lines := strings.Split(message, "\n")
	for index := range lines {
		lines[index] = "> " + lines[index]
	}
	return strings.Join(lines, "\n")
}

// stepWrapper records its phase on entry, applies the heading rule to the
// section its inner step wrote, runs the gate where the phase needs it, and
// prefixes failure messages once on the way out.
type stepWrapper struct {
	inner    agentlib.Step
	phase    domain.TaskPhase
	section  string
	gate     *Gate
	isReview bool
}

// NewPlanningStep wraps the planning step. It records phase planning and
// applies the heading rule to "## Plan".
func NewPlanningStep(inner agentlib.Step) agentlib.Step {
	return &stepWrapper{
		inner:   inner,
		phase:   domain.TaskPhasePlanning,
		section: planSection,
	}
}

// NewExecutionStep wraps the execution step. It records phase execution,
// applies the heading rule to "## Verdict" and evaluates the gate.
func NewExecutionStep(inner agentlib.Step, gate *Gate) agentlib.Step {
	return &stepWrapper{
		inner:   inner,
		phase:   domain.TaskPhaseExecution,
		section: VerdictSectionHeading,
		gate:    gate,
	}
}

// NewAIReviewStep wraps the ai_review step. It records phase ai_review,
// applies the heading rule to "## Review", re-runs the gate and then the line
// guard; on success it removes any stale "## Failure" and completes the task.
func NewAIReviewStep(inner agentlib.Step, gate *Gate) agentlib.Step {
	return &stepWrapper{
		inner:    inner,
		phase:    domain.TaskPhaseAIReview,
		section:  reviewSection,
		gate:     gate,
		isReview: true,
	}
}

// Name delegates to the inner step.
func (w *stepWrapper) Name() string { return w.inner.Name() }

// ShouldRun delegates to the inner step.
func (w *stepWrapper) ShouldRun(ctx context.Context, md *agentlib.Markdown) (bool, error) {
	return w.inner.ShouldRun(ctx, md)
}

// Run implements agentlib.Step.
func (w *stepWrapper) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	w.recordPhase(md)
	result, err := w.inner.Run(ctx, md)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "%s run", w.inner.Name())
	}
	if result == nil {
		return nil, errors.Errorf(ctx, "%s returned nil result", w.inner.Name())
	}
	if result.Status != agentlib.AgentStatusDone {
		result.Message = prefixFailureMessage(result.Status, result.Message)
		return result, nil
	}
	if decision, fired := checkHeading(ctx, md, w.section); fired {
		return prefixedResult(decision), nil
	}
	return w.afterHeading(ctx, md, result), nil
}

// afterHeading runs the phase-specific logic once the heading rule passed.
func (w *stepWrapper) afterHeading(
	ctx context.Context,
	md *agentlib.Markdown,
	result *agentlib.Result,
) *agentlib.Result {
	switch {
	case w.gate == nil:
		return result
	case w.isReview:
		decision := w.gate.Review(ctx, md)
		if decision.Status != agentlib.AgentStatusDone {
			return prefixedResult(decision)
		}
		removeSection(md, failureSection)
		return &agentlib.Result{
			Status:    agentlib.AgentStatusDone,
			NextPhase: string(domain.TaskPhaseDone),
		}
	default:
		decision := w.gate.Evaluate(ctx, md)
		if decision.Status != agentlib.AgentStatusDone {
			return prefixedResult(decision)
		}
		return result
	}
}

// recordPhase stamps the task's phase on entry so an escalation leaves the
// task on the phase that escalated.
func (w *stepWrapper) recordPhase(md *agentlib.Markdown) {
	if md.Frontmatter == nil {
		md.Frontmatter = agentlib.TaskFrontmatter{}
	}
	md.Frontmatter["phase"] = string(w.phase)
}

// prefixedResult builds a result from a gate decision with its message
// prefixed once.
func prefixedResult(decision Decision) *agentlib.Result {
	return &agentlib.Result{
		Status:  decision.Status,
		Message: prefixFailureMessage(decision.Status, decision.Message),
	}
}

// prefixFailureMessage prefixes the message only for failed/needs_input.
func prefixFailureMessage(status agentlib.AgentStatus, message string) string {
	if status == agentlib.AgentStatusFailed || status == agentlib.AgentStatusNeedsInput {
		return PrefixMessage(message)
	}
	return message
}
