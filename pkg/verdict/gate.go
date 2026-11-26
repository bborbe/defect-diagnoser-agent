// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict

import (
	"context"
	"strings"

	agentlib "github.com/bborbe/agent"
	"github.com/bborbe/errors"
)

// requiredFields are the seven non-confidence fields that must be present and
// non-empty (rule d), derived from the single source of field order.
var requiredFields = fieldsExcept(FieldConfidence)

// newlineCheckedFields are the five scalar fields rule a screens for newlines
// before any value can be echoed.
var newlineCheckedFields = []string{
	FieldDefectID,
	FieldService,
	FieldRepo,
	FieldFileLine,
	FieldConfidence,
}

// fieldsExcept returns every contract field except the named one, in order.
func fieldsExcept(excluded string) []string {
	result := make([]string, 0, len(VerdictFieldOrder))
	for _, name := range VerdictFieldOrder {
		if name != excluded {
			result = append(result, name)
		}
	}
	return result
}

// Decision is the outcome of one gate evaluation.
type Decision struct {
	Status  agentlib.AgentStatus // Failed, NeedsInput, or Done
	Message string               // binary-generated; NOT yet "> "-prefixed
}

// Gate applies the deterministic verdict rules to a task document.
type Gate struct {
	envContext map[string]string
	cloneRoot  string
}

// NewGate constructs the gate. envContext carries the operator-supplied
// service_repo.<service> entries. cloneRoot is <agent dir>/repos.
func NewGate(envContext map[string]string, cloneRoot string) *Gate {
	return &Gate{envContext: envContext, cloneRoot: cloneRoot}
}

// Evaluate applies gate rules a-j to md after the execution step, in order.
// Done means rules a-j passed (rule k): the caller advances to ai_review.
func (g *Gate) Evaluate(ctx context.Context, md *agentlib.Markdown) Decision {
	if decision, fired := g.check(ctx, md); fired {
		return decision
	}
	return Decision{Status: agentlib.AgentStatusDone}
}

// Review applies gate rules a-j to md again — so a Job entering at phase:
// ai_review can never complete an ungated verdict — and, when they pass, the
// line guard. Done means the cited line resolves: the caller completes.
func (g *Gate) Review(ctx context.Context, md *agentlib.Markdown) Decision {
	if decision, fired := g.check(ctx, md); fired {
		return decision
	}
	verdict := newVerdict(mustFields(ctx, md))
	if err := CheckLineGuard(ctx, g.cloneRoot, verdict.Repo, verdict.FileLine); err != nil {
		removeVerdict(md)
		return Decision{
			Status:  agentlib.AgentStatusNeedsInput,
			Message: lineGuardMessage(ctx, err, verdict.Evidence),
		}
	}
	return Decision{Status: agentlib.AgentStatusDone}
}

// mustFields re-extracts the fields after check passed; check guarantees a
// parseable block, so a failure here is impossible.
func mustFields(ctx context.Context, md *agentlib.Markdown) fields {
	parsed, _ := extractFields(ctx, md)
	return parsed
}

// check walks rules a-j in order and returns the first that fires.
func (g *Gate) check(ctx context.Context, md *agentlib.Markdown) (Decision, bool) {
	if decision, fired := checkHeading(ctx, md, VerdictSectionHeading); fired {
		return decision, true
	}
	parsed, outcome := extractFields(ctx, md)
	if decision, fired := checkParseOutcome(ctx, md, parsed, outcome); fired {
		return decision, true
	}
	verdict := newVerdict(parsed)
	if decision, fired := checkConfidence(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := checkNoLine(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := checkRequired(ctx, md, parsed); fired {
		return decision, true
	}
	if decision, fired := checkFileLine(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := checkEvidence(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := checkDefectID(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := g.checkService(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := g.checkRepo(ctx, md, verdict); fired {
		return decision, true
	}
	if decision, fired := g.checkBelowHigh(ctx, md, verdict); fired {
		return decision, true
	}
	return Decision{}, false
}

// checkHeading is the model-output heading rule: any column-0 line starting
// with '#' in the section body rejects the phase.
func checkHeading(ctx context.Context, md *agentlib.Markdown, heading string) (Decision, bool) {
	section, ok := md.FindSection(heading)
	if !ok {
		return Decision{}, false
	}
	for _, line := range strings.Split(section.Body, "\n") {
		if strings.HasPrefix(line, "#") {
			removeSection(md, heading)
			return Decision{
				Status: agentlib.AgentStatusFailed,
				Message: errors.Errorf(
					ctx,
					"heading rule: section %q contains a column-0 heading line",
					heading,
				).Error(),
			}, true
		}
	}
	return Decision{}, false
}

// checkParseOutcome implements rule a.
func checkParseOutcome(
	ctx context.Context,
	md *agentlib.Markdown,
	parsed fields,
	outcome parseOutcome,
) (Decision, bool) {
	switch outcome {
	case parseNoSection:
		return failed(ctx, md, "verdict section missing")
	case parseNoBlock:
		return failed(ctx, md, "verdict yaml block missing")
	case parseInvalidYAML:
		return failed(ctx, md, "verdict yaml does not parse")
	case parseOK:
	}
	for _, name := range newlineCheckedFields {
		if strings.Contains(parsed[name], "\n") {
			return failed(ctx, md, "field %q contains a newline", name)
		}
	}
	return Decision{}, false
}

// checkConfidence implements rule b.
func checkConfidence(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	switch verdict.Confidence {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return Decision{}, false
	default:
		return failed(
			ctx,
			md,
			"confidence %q is not one of High, Medium, Low",
			verdict.Confidence,
		)
	}
}

// checkNoLine implements rule c.
func checkNoLine(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	if verdict.Confidence != ConfidenceMedium && verdict.Confidence != ConfidenceLow {
		return Decision{}, false
	}
	if verdict.FileLine != "" {
		return Decision{}, false
	}
	removeVerdict(md)
	return Decision{
		Status:  agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(ctx, "no line cited; evidence:\n%s", verdict.Evidence).Error(),
	}, true
}

// checkRequired implements rule d.
func checkRequired(
	ctx context.Context,
	md *agentlib.Markdown,
	parsed fields,
) (Decision, bool) {
	for _, name := range requiredFields {
		if parsed[name] == "" {
			return failed(ctx, md, "field %q is missing or empty", name)
		}
	}
	return Decision{}, false
}

// checkFileLine implements rule e.
func checkFileLine(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	if _, _, ok := parseFileLine(verdict.FileLine); ok {
		return Decision{}, false
	}
	return failed(ctx, md, "file:line is not a repo-relative <path>:<positive line>")
}

// checkEvidence implements rule f.
func checkEvidence(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	if strings.Contains(verdict.Evidence, "scripts/vault-") ||
		strings.Contains(verdict.Evidence, "scripts/prometheus-query.sh") {
		return Decision{}, false
	}
	removeVerdict(md)
	return Decision{
		Status:  agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(ctx, "evidence cites no live read").Error(),
	}, true
}

// checkDefectID implements rule g.
func checkDefectID(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	frontmatterID, ok := md.Frontmatter.String(FieldDefectID)
	if ok && frontmatterID == verdict.DefectID {
		return Decision{}, false
	}
	removeVerdict(md)
	return Decision{
		Status: agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(
			ctx,
			"defect_id mismatch: verdict %q, frontmatter %q",
			verdict.DefectID,
			frontmatterID,
		).Error(),
	}, true
}

// checkService implements rule h.
func (g *Gate) checkService(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	if _, ok := g.lookupService(verdict.Service); ok {
		return Decision{}, false
	}
	removeVerdict(md)
	return Decision{
		Status:  agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(ctx, "service %q does not resolve", verdict.Service).Error(),
	}, true
}

// checkRepo implements rule i.
func (g *Gate) checkRepo(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	mapped, ok := g.lookupService(verdict.Service)
	if !ok || mapped == verdict.Repo {
		return Decision{}, false
	}
	removeVerdict(md)
	return Decision{
		Status: agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(
			ctx,
			"repo %q does not match mapped repo %q",
			verdict.Repo,
			mapped,
		).Error(),
	}, true
}

// checkBelowHigh implements rule j.
func (g *Gate) checkBelowHigh(
	ctx context.Context,
	md *agentlib.Markdown,
	verdict Verdict,
) (Decision, bool) {
	if verdict.Confidence != ConfidenceMedium && verdict.Confidence != ConfidenceLow {
		return Decision{}, false
	}
	err := CheckLineGuard(ctx, g.cloneRoot, verdict.Repo, verdict.FileLine)
	if err != nil {
		removeVerdict(md)
		return Decision{
			Status:  agentlib.AgentStatusNeedsInput,
			Message: lineGuardMessage(ctx, err, verdict.Evidence),
		}, true
	}
	return Decision{
		Status:  agentlib.AgentStatusNeedsInput,
		Message: errors.Errorf(ctx, "confidence %s below High", verdict.Confidence).Error(),
	}, true
}

// lookupService resolves a service through the operator-supplied mapping. The
// match is exact and case-sensitive; no fuzzy match, no fallback.
func (g *Gate) lookupService(service string) (string, bool) {
	repo, ok := g.envContext["service_repo."+service]
	return repo, ok
}

// failed removes the verdict and returns a failed decision.
func failed(ctx context.Context, md *agentlib.Markdown, format string, args ...interface{}) (
	Decision, bool,
) {
	removeVerdict(md)
	return Decision{
		Status:  agentlib.AgentStatusFailed,
		Message: errors.Errorf(ctx, format, args...).Error(),
	}, true
}

// lineGuardMessage renders a line-guard reason with the evidence quoted raw.
func lineGuardMessage(ctx context.Context, err error, evidence string) string {
	return errors.Errorf(ctx, "%s; evidence:\n%s", err.Error(), evidence).Error()
}

// removeVerdict removes every "## Verdict" section from the parsed sections.
func removeVerdict(md *agentlib.Markdown) {
	removeSection(md, VerdictSectionHeading)
}

// removeSection filters every section with the given heading out of md.Sections.
func removeSection(md *agentlib.Markdown, heading string) {
	kept := md.Sections[:0]
	for _, section := range md.Sections {
		if section.Heading != heading {
			kept = append(kept, section)
		}
	}
	md.Sections = kept
}
