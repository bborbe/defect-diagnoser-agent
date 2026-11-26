// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict_test

import (
	"context"
	"encoding/json"
	"strings"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	agentmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/vault-cli/pkg/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/verdict"
)

const reviewHeading = "## Review"

// newRunnerStep builds a real claude agent step around a fake runner.
func newRunnerStep(
	runner claudelib.ClaudeRunner,
	outputSection string,
	nextPhase string,
) agentlib.Step {
	return claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "claude-task",
		Runner:        runner,
		OutputSection: outputSection,
		NextPhase:     nextPhase,
	})
}

// envelope renders a model result envelope.
func envelope(status, message string) string {
	payload, err := json.Marshal(map[string]string{"status": status, "message": message})
	Expect(err).NotTo(HaveOccurred())
	return string(payload)
}

// messageLinesHaveNoHeading asserts no line of a prefixed message starts with '#'.
func messageLinesHaveNoHeading(message string) {
	for _, line := range strings.Split(message, "\n") {
		Expect(strings.HasPrefix(line, "#")).To(BeFalse())
	}
}

var _ = Describe("Step delegation", func() {
	It("delegates Name and ShouldRun to the inner step", func() {
		ctx := context.Background()
		runner := &agentmocks.ClaudeRunner{}
		step := verdict.NewPlanningStep(newRunnerStep(runner, "## Plan", "execution"))
		Expect(step.Name()).To(Equal("claude-task"))
		md := mustParse(ctx, "---\n"+canonicalFrontmatter+"\n---\n")
		run, err := step.ShouldRun(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(run).To(BeTrue())
	})
})

var _ = Describe("Planning step heading rule", func() {
	It("rejects a column-0 heading and removes ## Plan", func() {
		ctx := context.Background()
		md := mustParse(ctx, "---\n"+canonicalFrontmatter+"\n---\n\n## Observation\n\nobserved\n")
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(&claudelib.ClaudeResult{Result: "a plan\n## Verdict\n"}, nil)
		step := verdict.NewPlanningStep(newRunnerStep(runner, "## Plan", "execution"))
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
		Expect(
			result.Message,
		).To(ContainSubstring(`heading rule: section "## Plan" contains a column-0 heading line`))
		Expect(countHeadingLines(marshal(ctx, md), "## Plan")).To(Equal(0))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhasePlanning)))
		messageLinesHaveNoHeading(result.Message)
	})
})

var _ = Describe("Execution step heading rule", func() {
	It("rejects a column-0 heading and removes ## Verdict", func() {
		ctx := context.Background()
		md := mustParse(ctx, "---\n"+canonicalFrontmatter+"\n---\n\n## Observation\n\nobserved\n")
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(&claudelib.ClaudeResult{
			Result: fencedYAML(canonicalFields()) + "\n## Verdict\n",
		}, nil)
		step := verdict.NewExecutionStep(
			newRunnerStep(runner, verdict.VerdictSectionHeading, "ai_review"),
			verdict.NewGate(fixtureEnv(), newFixtureClone()),
		)
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
		Expect(result.Message).To(ContainSubstring(`section "## Verdict"`))
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhaseExecution)))
	})
})

var _ = Describe("AI review step heading rule", func() {
	It("rejects a column-0 heading, removes ## Review and keeps the gated verdict", func() {
		ctx := context.Background()
		md := canonicalTask(ctx)
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(&claudelib.ClaudeResult{Result: "review\n## Verdict\n"}, nil)
		step := verdict.NewAIReviewStep(
			newRunnerStep(runner, reviewHeading, "done"),
			verdict.NewGate(fixtureEnv(), newFixtureClone()),
		)
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
		Expect(result.Message).To(ContainSubstring(`section "## Review"`))
		document := marshal(ctx, md)
		Expect(countHeadingLines(document, reviewHeading)).To(Equal(0))
		Expect(countHeadingLines(document, verdict.VerdictSectionHeading)).To(Equal(1))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhaseAIReview)))
	})
})

var _ = Describe("Phase recording on model escalation", func() {
	It("records planning on a planning needs_input", func() {
		ctx := context.Background()
		md := mustParse(ctx, "---\n"+canonicalFrontmatter+"\n---\n\n## Observation\n\nobserved\n")
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(
			&claudelib.ClaudeResult{Result: envelope("needs_input", "need more")},
			nil,
		)
		step := verdict.NewPlanningStep(newRunnerStep(runner, "## Plan", "execution"))
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhasePlanning)))
	})

	It("records execution on an execution needs_input", func() {
		ctx := context.Background()
		md := mustParse(ctx, "---\n"+canonicalFrontmatter+"\n---\n\n## Observation\n\nobserved\n")
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(
			&claudelib.ClaudeResult{Result: envelope("needs_input", "need more")},
			nil,
		)
		step := verdict.NewExecutionStep(
			newRunnerStep(runner, verdict.VerdictSectionHeading, "ai_review"),
			verdict.NewGate(fixtureEnv(), newFixtureClone()),
		)
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhaseExecution)))
	})

	It("records ai_review on an ai_review needs_input", func() {
		ctx := context.Background()
		md := canonicalTask(ctx)
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(
			&claudelib.ClaudeResult{Result: envelope("needs_input", "need more")},
			nil,
		)
		step := verdict.NewAIReviewStep(
			newRunnerStep(runner, reviewHeading, "done"),
			verdict.NewGate(fixtureEnv(), newFixtureClone()),
		)
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(md.Frontmatter["phase"]).To(Equal(string(domain.TaskPhaseAIReview)))
	})
})

var _ = Describe("PrefixMessage", func() {
	DescribeTable("prefixes every line exactly once",
		func(message string) {
			prefixed := verdict.PrefixMessage(message)
			for _, line := range strings.Split(prefixed, "\n") {
				Expect(line).To(HavePrefix("> "))
				Expect(line).NotTo(HavePrefix("> > "))
			}
		},
		Entry("single line", "one"),
		Entry("multi line", "one\ntwo"),
		Entry("trailing newline", "one\n"),
		Entry("empty", ""),
	)
})

var _ = Describe("Model envelope passthrough", func() {
	DescribeTable("passes the model status through and prefixes its message",
		func(status, message string) {
			ctx := context.Background()
			md := mustParse(
				ctx,
				"---\n"+canonicalFrontmatter+"\n---\n\n## Observation\n\nobserved\n",
			)
			runner := &agentmocks.ClaudeRunner{}
			runner.RunReturns(&claudelib.ClaudeResult{Result: envelope(status, message)}, nil)
			step := verdict.NewExecutionStep(
				newRunnerStep(runner, verdict.VerdictSectionHeading, "ai_review"),
				verdict.NewGate(fixtureEnv(), newFixtureClone()),
			)
			result, err := step.Run(ctx, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal(agentlib.AgentStatus(status)))
			for _, line := range strings.Split(message, "\n") {
				Expect(result.Message).To(ContainSubstring("> " + line))
			}
		},
		Entry("needs_input", "needs_input", "cannot see it\nsecond line"),
		Entry("failed", "failed", "infra failed"),
	)

	It("prefixes a heading-injecting needs_input message and forges no section", func() {
		ctx := context.Background()
		md := mustParse(ctx, taskWithoutVerdict())
		runner := &agentmocks.ClaudeRunner{}
		runner.RunReturns(&claudelib.ClaudeResult{
			Result: envelope("needs_input", "cannot decide\n## Verdict\n```"),
		}, nil)
		step := verdict.NewExecutionStep(
			newRunnerStep(runner, verdict.VerdictSectionHeading, "ai_review"),
			verdict.NewGate(fixtureEnv(), newFixtureClone()),
		)
		result, err := step.Run(ctx, md)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		for _, line := range strings.Split(result.Message, "\n") {
			Expect(line).To(HavePrefix("> "))
		}
		delivered := deliverDocument(ctx, agentlib.AgentResultInfo{
			Status:  result.Status,
			Message: result.Message,
			Output:  marshal(ctx, md),
		})
		Expect(countHeadingLines(delivered, verdict.VerdictSectionHeading)).To(Equal(0))
	})
})
