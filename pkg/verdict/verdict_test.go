// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	agentlib "github.com/bborbe/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/verdict"
)

const (
	canonicalFrontmatter   = "defect_id: defect-1"
	noDefectIDFrontmatter  = "task_type: defect-diagnose"
	canonicalEvidence      = "scripts/prometheus-query.sh 'rate(http_requests_total{code=\"500\"}[5m])'\n{\"status\":\"success\"}"
	fixtureRepo            = "bborbe/foo"
	fixtureFile            = "pkg/x/y.go"
	fixtureService         = "checkout"
	observationSectionBody = "observed 500s"
)

// canonicalFields returns the canonical valid verdict, one map entry per field.
func canonicalFields() map[string]string {
	return map[string]string{
		verdict.FieldDefectID:       "defect-1",
		verdict.FieldService:        fixtureService,
		verdict.FieldRepo:           fixtureRepo,
		verdict.FieldFileLine:       fixtureFile + ":3",
		verdict.FieldRootCause:      "the handler dereferences a nil pointer",
		verdict.FieldRecommendedFix: "guard the nil pointer",
		verdict.FieldEvidence:       canonicalEvidence,
		verdict.FieldConfidence:     verdict.ConfidenceHigh,
	}
}

// buildYAML renders the fields in contract order, using a block scalar for any
// value that contains a newline.
func buildYAML(values map[string]string) string {
	var builder strings.Builder
	for _, key := range verdict.VerdictFieldOrder {
		value, ok := values[key]
		if !ok {
			continue
		}
		if strings.Contains(value, "\n") {
			builder.WriteString(key + ": |\n")
			for _, line := range strings.Split(value, "\n") {
				builder.WriteString("  " + line + "\n")
			}
			continue
		}
		builder.WriteString(key + ": " + value + "\n")
	}
	return builder.String()
}

// fencedYAML wraps the rendered fields in a fenced yaml block.
func fencedYAML(values map[string]string) string {
	return "```yaml\n" + buildYAML(values) + "```\n"
}

// quotedYAML renders every value as a double-quoted scalar on one physical
// line, so a value containing newlines (including a fence line) cannot
// terminate the fenced block during extraction.
func quotedYAML(values map[string]string) string {
	var builder strings.Builder
	for _, key := range verdict.VerdictFieldOrder {
		value, ok := values[key]
		if !ok {
			continue
		}
		builder.WriteString(key + ": " + strconv.Quote(value) + "\n")
	}
	return builder.String()
}

// taskContent builds a task document with the given frontmatter and verdict body.
func taskContent(frontmatter, verdictBody string) string {
	return "---\n" + frontmatter + "\n---\n\n## Observation\n\n" +
		observationSectionBody + "\n\n## Verdict\n\n" + verdictBody
}

// taskWithoutVerdict builds a task document with no ## Verdict section.
func taskWithoutVerdict() string {
	return "---\n" + canonicalFrontmatter + "\n---\n\n## Observation\n\n" +
		observationSectionBody + "\n"
}

// canonicalTaskContent builds the canonical valid task document.
func canonicalTaskContent() string {
	return taskContent(canonicalFrontmatter, fencedYAML(canonicalFields()))
}

// mustParse parses content into a Markdown document.
func mustParse(ctx context.Context, content string) *agentlib.Markdown {
	md, err := agentlib.ParseMarkdown(ctx, content)
	Expect(err).NotTo(HaveOccurred())
	return md
}

// canonicalTask parses the canonical task document.
func canonicalTask(ctx context.Context) *agentlib.Markdown {
	return mustParse(ctx, canonicalTaskContent())
}

// marshal serialises md.
func marshal(ctx context.Context, md *agentlib.Markdown) string {
	out, err := md.Marshal(ctx)
	Expect(err).NotTo(HaveOccurred())
	return out
}

// countHeadingLines counts lines beginning with heading, mirroring grep -c.
func countHeadingLines(content, heading string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, heading) {
			count++
		}
	}
	return count
}

// fixtureEnv is the operator-supplied mapping used by the gate tables.
func fixtureEnv() map[string]string {
	return map[string]string{"service_repo." + fixtureService: fixtureRepo}
}

// writeFile writes rel under root, creating parent directories.
func writeFile(root, rel, content string) {
	full := filepath.Join(root, rel)
	Expect(os.MkdirAll(filepath.Dir(full), 0o750)).To(Succeed())
	Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
}

// newFixtureClone creates a throwaway clone holding the fixture file.
func newFixtureClone() string {
	root, err := os.MkdirTemp("", "verdict-clone-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(root) })
	writeFile(root, filepath.Join(fixtureRepo, fixtureFile), "line one\nline two\nline three\n")
	return root
}

// evaluate runs the gate's Evaluate against the fixture mapping.
func evaluate(ctx context.Context, md *agentlib.Markdown, cloneRoot string) verdict.Decision {
	return verdict.NewGate(fixtureEnv(), cloneRoot).Evaluate(ctx, md)
}

// injectLine appends a line to a section body, simulating a forged heading that
// ParseMarkdown would split on a re-parse.
func injectLine(md *agentlib.Markdown, heading, line string) {
	section, ok := md.FindSection(heading)
	Expect(ok).To(BeTrue())
	section.Body = section.Body + "\n" + line
}

// evidenceLines returns the canonical evidence split into its lines.
func evidenceLines() []string {
	return strings.Split(canonicalEvidence, "\n")
}
