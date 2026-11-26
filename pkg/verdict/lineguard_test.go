// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict_test

import (
	"context"
	"os"
	"path/filepath"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	delivery "github.com/bborbe/agent/delivery"
	agentmocks "github.com/bborbe/agent/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/verdict"
)

// newOutsideDir creates a second throwaway directory that a symlink can escape to.
func newOutsideDir() string {
	dir, err := os.MkdirTemp("", "verdict-outside-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// deliverDocument runs the real passthrough content generator.
func deliverDocument(ctx context.Context, info agentlib.AgentResultInfo) string {
	document, err := delivery.NewPassthroughContentGenerator().Generate(ctx, "", info)
	Expect(err).NotTo(HaveOccurred())
	return document
}

// runAIReview drives a real claude agent step wrapped by the ai_review wrapper.
func runAIReview(
	ctx context.Context,
	md *agentlib.Markdown,
	cloneRoot string,
	reviewText string,
) *agentlib.Result {
	runner := &agentmocks.ClaudeRunner{}
	runner.RunReturns(&claudelib.ClaudeResult{Result: reviewText}, nil)
	inner := claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "claude-task",
		Runner:        runner,
		OutputSection: reviewHeading,
		NextPhase:     "done",
	})
	step := verdict.NewAIReviewStep(inner, verdict.NewGate(fixtureEnv(), cloneRoot))
	result, err := step.Run(ctx, md)
	Expect(err).NotTo(HaveOccurred())
	return result
}

var _ = Describe("CheckLineGuard", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("accepts a line that exists", func() {
		Expect(
			verdict.CheckLineGuard(ctx, newFixtureClone(), fixtureRepo, fixtureFile+":2"),
		).To(Succeed())
	})

	It("reports an absent path as not found", func() {
		err := verdict.CheckLineGuard(ctx, newFixtureClone(), fixtureRepo, "pkg/x/missing.go:1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"pkg/x/missing.go" not found in clone`))
	})

	It("reports a line beyond the end of file with the count", func() {
		err := verdict.CheckLineGuard(ctx, newFixtureClone(), fixtureRepo, fixtureFile+":99")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"pkg/x/y.go:99" beyond end of file (3 lines)`))
	})

	It("reports a symlinked file escaping the clone", func() {
		outside := newOutsideDir()
		writeFile(outside, "secret.go", "secret\n")
		clone := newFixtureClone()
		Expect(os.Symlink(
			filepath.Join(outside, "secret.go"),
			filepath.Join(clone, fixtureRepo, "pkg/x/link.go"),
		)).To(Succeed())
		err := verdict.CheckLineGuard(ctx, clone, fixtureRepo, "pkg/x/link.go:1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"pkg/x/link.go" resolves outside the clone`))
	})

	It("reports a symlinked directory escaping the clone without touching its target", func() {
		outside := newOutsideDir()
		writeFile(outside, "sentinel", "keep me\n")
		clone := newFixtureClone()
		Expect(os.Symlink(outside, filepath.Join(clone, fixtureRepo, "pkg/linkdir"))).To(Succeed())
		err := verdict.CheckLineGuard(ctx, clone, fixtureRepo, "pkg/linkdir/sentinel:1")
		Expect(err).To(HaveOccurred())
		Expect(
			err.Error(),
		).To(ContainSubstring(`"pkg/linkdir/sentinel" resolves outside the clone`))
		content, readErr := os.ReadFile(filepath.Join(outside, "sentinel"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("keep me\n"))
	})

	It("reports a dangling symlinked directory escaping the clone", func() {
		outside := newOutsideDir()
		clone := newFixtureClone()
		Expect(os.Symlink(outside, filepath.Join(clone, fixtureRepo, "pkg/linkdir"))).To(Succeed())
		err := verdict.CheckLineGuard(ctx, clone, fixtureRepo, "pkg/linkdir/absent:1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"pkg/linkdir/absent" resolves outside the clone`))
	})

	It("reports an absent repo as not found", func() {
		err := verdict.CheckLineGuard(ctx, newFixtureClone(), "bborbe/absent", fixtureFile+":1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found in clone"))
	})
})

var _ = Describe("AI review line guard", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("fails from rule a when the verdict section is missing", func() {
		md := mustParse(ctx, taskWithoutVerdict())
		result := runAIReview(ctx, md, newFixtureClone(), "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("fails from rule a when the yaml does not parse", func() {
		md := mustParse(ctx, taskContent(canonicalFrontmatter, "```yaml\nkey: [unclosed\n```\n"))
		result := runAIReview(ctx, md, newFixtureClone(), "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
		Expect(result.Message).To(ContainSubstring("verdict yaml does not parse"))
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("escalates when the path is absent", func() {
		md := mustParse(
			ctx,
			taskContent(
				canonicalFrontmatter,
				fencedYAML(fieldsWith(verdict.FieldFileLine, "pkg/x/missing.go:1")),
			),
		)
		result := runAIReview(ctx, md, newFixtureClone(), "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(result.Message).To(ContainSubstring("not found in clone"))
		Expect(result.Message).To(ContainSubstring("evidence:"))
		for _, line := range evidenceLines() {
			Expect(result.Message).To(ContainSubstring(line))
		}
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("escalates when the line is beyond the end of file", func() {
		md := mustParse(
			ctx,
			taskContent(
				canonicalFrontmatter,
				fencedYAML(fieldsWith(verdict.FieldFileLine, fixtureFile+":99")),
			),
		)
		result := runAIReview(ctx, md, newFixtureClone(), "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(result.Message).To(ContainSubstring("beyond end of file (3 lines)"))
		for _, line := range evidenceLines() {
			Expect(result.Message).To(ContainSubstring(line))
		}
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("escalates when the cited file is a symlink escaping the clone", func() {
		outside := newOutsideDir()
		writeFile(outside, "secret.go", "secret\n")
		clone := newFixtureClone()
		Expect(os.Symlink(
			filepath.Join(outside, "secret.go"),
			filepath.Join(clone, fixtureRepo, "pkg/x/link.go"),
		)).To(Succeed())
		md := mustParse(
			ctx,
			taskContent(
				canonicalFrontmatter,
				fencedYAML(fieldsWith(verdict.FieldFileLine, "pkg/x/link.go:1")),
			),
		)
		result := runAIReview(ctx, md, clone, "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(result.Message).To(ContainSubstring("resolves outside the clone"))
		for _, line := range evidenceLines() {
			Expect(result.Message).To(ContainSubstring(line))
		}
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("escalates when the cited file sits in a symlinked directory escaping the clone", func() {
		outside := newOutsideDir()
		writeFile(outside, "sentinel", "keep me\n")
		clone := newFixtureClone()
		Expect(os.Symlink(outside, filepath.Join(clone, fixtureRepo, "pkg/linkdir"))).To(Succeed())
		md := mustParse(
			ctx,
			taskContent(
				canonicalFrontmatter,
				fencedYAML(fieldsWith(verdict.FieldFileLine, "pkg/linkdir/sentinel:1")),
			),
		)
		result := runAIReview(ctx, md, clone, "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
		Expect(result.Message).To(ContainSubstring("resolves outside the clone"))
		Expect(countHeadingLines(marshal(ctx, md), verdict.VerdictSectionHeading)).To(Equal(0))
	})

	It("completes and removes a stale failure when the line resolves", func() {
		content := canonicalTaskContent() + "\n## Failure\n\nold failure\n"
		md := mustParse(ctx, content)
		result := runAIReview(ctx, md, newFixtureClone(), "review looks good")
		Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
		Expect(result.NextPhase).To(Equal("done"))
		document := marshal(ctx, md)
		Expect(countHeadingLines(document, verdict.VerdictSectionHeading)).To(Equal(1))
		Expect(countHeadingLines(document, "## Failure")).To(Equal(0))
		delivered := deliverDocument(ctx, agentlib.AgentResultInfo{
			Status:    result.Status,
			NextPhase: result.NextPhase,
			Output:    document,
		})
		Expect(delivered).To(ContainSubstring("phase: done"))
	})
})
