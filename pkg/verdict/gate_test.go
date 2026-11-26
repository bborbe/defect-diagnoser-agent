// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict_test

import (
	"context"
	"strings"

	agentlib "github.com/bborbe/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/verdict"
)

// gateCase is one row of a gate table.
type gateCase struct {
	Name          string
	Build         func(context.Context) *agentlib.Markdown
	Env           map[string]string
	Status        agentlib.AgentStatus
	Contains      []string
	Absent        []string
	AbsentDoc     []string
	VerdictLeft   int
	MessageEquals string
	CheckMessage  func(string)
}

// fieldsWith returns the canonical fields with one value replaced.
func fieldsWith(key, value string) map[string]string {
	values := canonicalFields()
	values[key] = value
	return values
}

// fieldsWithout returns the canonical fields with one key removed.
func fieldsWithout(key string) map[string]string {
	values := canonicalFields()
	delete(values, key)
	return values
}

// buildTask parses a document built from the given fields and frontmatter.
func buildTask(
	frontmatter string,
	values map[string]string,
) func(context.Context) *agentlib.Markdown {
	return func(ctx context.Context) *agentlib.Markdown {
		return mustParse(ctx, taskContent(frontmatter, fencedYAML(values)))
	}
}

// runGateCase evaluates one row and asserts its expectations.
func runGateCase(row gateCase) {
	By(row.Name)
	ctx := context.Background()
	md := row.Build(ctx)
	env := row.Env
	if env == nil {
		env = fixtureEnv()
	}
	decision := verdict.NewGate(env, newFixtureClone()).Evaluate(ctx, md)
	Expect(decision.Status).To(Equal(row.Status))
	if row.MessageEquals != "" {
		Expect(decision.Message).To(Equal(row.MessageEquals))
	}
	for _, want := range row.Contains {
		Expect(decision.Message).To(ContainSubstring(want))
	}
	for _, unwanted := range row.Absent {
		Expect(decision.Message).NotTo(ContainSubstring(unwanted))
	}
	document := marshal(ctx, md)
	for _, unwanted := range row.AbsentDoc {
		Expect(document).NotTo(ContainSubstring(unwanted))
	}
	Expect(countHeadingLines(document, verdict.VerdictSectionHeading)).To(Equal(row.VerdictLeft))
	if row.CheckMessage != nil {
		row.CheckMessage(decision.Message)
	}
}

var _ = Describe("Gate failed rules", func() {
	DescribeTable("returns failed and removes the verdict",
		runGateCase,
		Entry("section missing", gateCase{
			Name: "section missing",
			Build: func(ctx context.Context) *agentlib.Markdown {
				return mustParse(ctx, taskWithoutVerdict())
			},
			Status:   agentlib.AgentStatusFailed,
			Contains: []string{"verdict section missing"},
		}),
		Entry("no yaml block", gateCase{
			Name: "no yaml block",
			Build: func(ctx context.Context) *agentlib.Markdown {
				return mustParse(ctx, taskContent(canonicalFrontmatter, "plain text only\n"))
			},
			Status:   agentlib.AgentStatusFailed,
			Contains: []string{"verdict yaml block missing"},
		}),
		Entry("unparseable yaml", gateCase{
			Name: "unparseable yaml",
			Build: func(ctx context.Context) *agentlib.Markdown {
				return mustParse(
					ctx,
					taskContent(canonicalFrontmatter, "```yaml\nkey: [unclosed\n```\n"),
				)
			},
			Status:   agentlib.AgentStatusFailed,
			Contains: []string{"verdict yaml does not parse"},
		}),
		Entry("heading injection in verdict body", gateCase{
			Name: "heading injection in verdict body",
			Build: func(ctx context.Context) *agentlib.Markdown {
				md := canonicalTask(ctx)
				injectLine(md, verdict.VerdictSectionHeading, "## Notes")
				return md
			},
			Status: agentlib.AgentStatusFailed,
			Contains: []string{
				`heading rule: section "## Verdict" contains a column-0 heading line`,
			},
			MessageEquals: `heading rule: section "## Verdict" contains a column-0 heading line`,
		}),
		Entry("defect_id newline", gateCase{
			Name: "defect_id newline",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldDefectID, "defect-1\nsecond"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "defect_id" contains a newline`},
			Absent:      []string{"second"},
			VerdictLeft: 0,
		}),
		Entry("service newline", gateCase{
			Name: "service newline",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldService, "checkout\nsecond"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "service" contains a newline`},
			Absent:      []string{"second"},
			VerdictLeft: 0,
		}),
		Entry("repo newline", gateCase{
			Name: "repo newline",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldRepo, "bborbe/foo\nsecond"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "repo" contains a newline`},
			Absent:      []string{"second"},
			VerdictLeft: 0,
		}),
		Entry("file:line newline", gateCase{
			Name: "file:line newline",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "pkg/x/y.go:3\nsecond"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "file:line" contains a newline`},
			Absent:      []string{"second"},
			VerdictLeft: 0,
		}),
		Entry("confidence newline", gateCase{
			Name: "confidence newline",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldConfidence, "High\n## Review"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "confidence" contains a newline`},
			Absent:      []string{"## Review"},
			AbsentDoc:   []string{"## Review"},
			VerdictLeft: 0,
		}),
		Entry("defect_id missing", gateCase{
			Name:        "defect_id missing",
			Build:       buildTask(canonicalFrontmatter, fieldsWithout(verdict.FieldDefectID)),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "defect_id" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("service missing", gateCase{
			Name:        "service missing",
			Build:       buildTask(canonicalFrontmatter, fieldsWithout(verdict.FieldService)),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "service" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("repo missing", gateCase{
			Name:        "repo missing",
			Build:       buildTask(canonicalFrontmatter, fieldsWithout(verdict.FieldRepo)),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "repo" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("file:line empty at High", gateCase{
			Name:        "file:line empty at High",
			Build:       buildTask(canonicalFrontmatter, fieldsWith(verdict.FieldFileLine, "")),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "file:line" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("root_cause missing", gateCase{
			Name:        "root_cause missing",
			Build:       buildTask(canonicalFrontmatter, fieldsWithout(verdict.FieldRootCause)),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "root_cause" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("recommended_fix missing", gateCase{
			Name: "recommended_fix missing",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWithout(verdict.FieldRecommendedFix),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "recommended_fix" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("evidence missing", gateCase{
			Name:        "evidence missing",
			Build:       buildTask(canonicalFrontmatter, fieldsWithout(verdict.FieldEvidence)),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`field "evidence" is missing or empty`},
			VerdictLeft: 0,
		}),
		Entry("confidence high lowercase", gateCase{
			Name: "confidence high lowercase",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldConfidence, "high"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`confidence "high" is not one of High, Medium, Low`},
			VerdictLeft: 0,
		}),
		Entry("confidence Certain", gateCase{
			Name: "confidence Certain",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldConfidence, "Certain"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`confidence "Certain" is not one of High, Medium, Low`},
			VerdictLeft: 0,
		}),
		Entry("confidence empty", gateCase{
			Name:        "confidence empty",
			Build:       buildTask(canonicalFrontmatter, fieldsWith(verdict.FieldConfidence, "")),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{`confidence "" is not one of High, Medium, Low`},
			VerdictLeft: 0,
		}),
		Entry("file:line without line", gateCase{
			Name: "file:line without line",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "pkg/x.go"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			VerdictLeft: 0,
		}),
		Entry("file:line zero", gateCase{
			Name: "file:line zero",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "pkg/x.go:0"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			VerdictLeft: 0,
		}),
		Entry("file:line negative", gateCase{
			Name: "file:line negative",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "pkg/x.go:-3"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			VerdictLeft: 0,
		}),
		Entry("file:line not numeric", gateCase{
			Name: "file:line not numeric",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "pkg/x.go:abc"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			VerdictLeft: 0,
		}),
		Entry("file:line absolute", gateCase{
			Name: "file:line absolute",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "/abs/x.go:3"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			AbsentDoc:   []string{"/abs/x.go:3"},
			VerdictLeft: 0,
		}),
		Entry("file:line traversal", gateCase{
			Name: "file:line traversal",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldFileLine, "a/../x.go:3"),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"file:line"},
			VerdictLeft: 0,
		}),
	)
})

var _ = Describe("Gate needs_input rules", func() {
	DescribeTable("returns needs_input with the documented message",
		runGateCase,
		Entry("Low with no line", gateCase{
			Name:        "Low with no line",
			Build:       buildTask(canonicalFrontmatter, lowFields("", verdict.ConfidenceLow)),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    append([]string{"no line cited; evidence:"}, evidenceLines()...),
			VerdictLeft: 0,
		}),
		Entry("Medium with no line", gateCase{
			Name: "Medium with no line",
			Build: buildTask(
				canonicalFrontmatter,
				lowFields("", verdict.ConfidenceMedium),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"no line cited; evidence:"},
			VerdictLeft: 0,
		}),
		Entry("heading injection through evidence", gateCase{
			Name: "heading injection through evidence",
			Build: func(ctx context.Context) *agentlib.Markdown {
				values := lowFields("", verdict.ConfidenceLow, "## Verdict", "```")
				return mustParse(ctx, taskContent(
					canonicalFrontmatter,
					"```yaml\n"+quotedYAML(values)+"```\n",
				))
			},
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"no line cited; evidence:"},
			VerdictLeft: 0,
			CheckMessage: func(message string) {
				for _, line := range strings.Split(verdict.PrefixMessage(message), "\n") {
					Expect(strings.HasPrefix(line, "#")).To(BeFalse())
					Expect(strings.TrimSpace(line)).NotTo(Equal("```"))
				}
			},
		}),
		Entry("evidence cites no live read", gateCase{
			Name: "evidence cites no live read",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldEvidence, "nothing useful"),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"evidence cites no live read"},
			VerdictLeft: 0,
		}),
		Entry("frontmatter without defect_id", gateCase{
			Name:        "frontmatter without defect_id",
			Build:       buildTask(noDefectIDFrontmatter, canonicalFields()),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"defect_id mismatch"},
			VerdictLeft: 0,
		}),
		Entry("defect_id mismatch", gateCase{
			Name: "defect_id mismatch",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldDefectID, "defect-2"),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"defect-2", "defect-1"},
			VerdictLeft: 0,
		}),
		Entry("service does not resolve", gateCase{
			Name: "service does not resolve",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldService, "Agent-Task-Controller"),
			),
			Env:         map[string]string{"service_repo.agent-task-controller": fixtureRepo},
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"does not resolve"},
			VerdictLeft: 0,
		}),
		Entry("repo mismatch", gateCase{
			Name: "repo mismatch",
			Build: buildTask(
				canonicalFrontmatter,
				fieldsWith(verdict.FieldRepo, "bborbe/bar"),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"bborbe/bar", "bborbe/foo"},
			VerdictLeft: 0,
		}),
		Entry("Low with valid line", gateCase{
			Name: "Low with valid line",
			Build: buildTask(
				canonicalFrontmatter,
				lowFields(fixtureFile+":2", verdict.ConfidenceLow),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"below High"},
			VerdictLeft: 1,
		}),
		Entry("Low with line beyond end", gateCase{
			Name: "Low with line beyond end",
			Build: buildTask(
				canonicalFrontmatter,
				lowFields(fixtureFile+":99", verdict.ConfidenceLow),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    append([]string{"beyond end of file"}, evidenceLines()...),
			VerdictLeft: 0,
		}),
	)
})

var _ = Describe("Gate precedence", func() {
	DescribeTable("the first rule that fires is the only one reported",
		runGateCase,
		Entry("rule d fires before rule e", gateCase{
			Name: "rule d fires before rule e",
			Build: buildTask(
				canonicalFrontmatter,
				missingRootCauseWithAbsoluteLine(),
			),
			Status:      agentlib.AgentStatusFailed,
			Contains:    []string{"root_cause"},
			Absent:      []string{"file:line", "/abs/x.go:3"},
			VerdictLeft: 0,
		}),
		Entry("rule c fires before rule d", gateCase{
			Name: "rule c fires before rule d",
			Build: buildTask(
				canonicalFrontmatter,
				mediumNoLineMissingRootCause(),
			),
			Status:      agentlib.AgentStatusNeedsInput,
			Contains:    []string{"no line cited"},
			Absent:      []string{"root_cause"},
			VerdictLeft: 0,
		}),
	)
})

var _ = Describe("Rule e passing row", func() {
	It("keeps the verdict for a repo-relative path with a positive line", func() {
		ctx := context.Background()
		md := mustParse(
			ctx,
			taskContent(
				canonicalFrontmatter,
				fencedYAML(fieldsWith(verdict.FieldFileLine, fixtureFile+":42")),
			),
		)
		decision := evaluate(ctx, md, newFixtureClone())
		Expect(decision.Status).To(Equal(agentlib.AgentStatusDone))
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(1))
	})
})

// lowFields builds fields with a Medium/Low confidence and an optional evidence
// suffix appended.
func lowFields(fileLine, confidence string, evidenceSuffix ...string) map[string]string {
	values := canonicalFields()
	values[verdict.FieldFileLine] = fileLine
	values[verdict.FieldConfidence] = confidence
	if len(evidenceSuffix) > 0 {
		values[verdict.FieldEvidence] = canonicalEvidence + "\n" + strings.Join(
			evidenceSuffix,
			"\n",
		)
	}
	return values
}

// missingRootCauseWithAbsoluteLine removes root_cause and sets an absolute line.
func missingRootCauseWithAbsoluteLine() map[string]string {
	values := fieldsWith(verdict.FieldFileLine, "/abs/x.go:3")
	delete(values, verdict.FieldRootCause)
	return values
}

// mediumNoLineMissingRootCause removes root_cause and clears the line at Medium.
func mediumNoLineMissingRootCause() map[string]string {
	values := fieldsWith(verdict.FieldFileLine, "")
	values[verdict.FieldConfidence] = verdict.ConfidenceMedium
	delete(values, verdict.FieldRootCause)
	return values
}
