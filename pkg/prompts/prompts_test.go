// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts_test

import (
	"github.com/bborbe/vault-cli/pkg/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/prompts"
)

// headingSentence is the response rule every phase prompt must carry.
const headingSentence = "Never start a line of your response with #."

// verdictFields are the eight keys the execution prompt must name.
var verdictFields = []string{
	"defect_id",
	"service",
	"repo",
	"file:line",
	"root_cause",
	"recommended_fix",
	"evidence",
	"confidence",
}

var _ = Describe("BuildInstructions", func() {
	DescribeTable("selects the phase prompt",
		func(phase domain.TaskPhase, required []string) {
			instrs := prompts.BuildInstructions(phase)
			Expect(instrs).To(HaveLen(2))
			Expect(instrs[0].Name).To(Equal(string(phase)))
			Expect(instrs[0].Content).NotTo(BeEmpty())
			Expect(instrs[1].Name).To(Equal("output-format"))
			Expect(instrs[1].Content).NotTo(BeEmpty())

			rendered := instrs.String()
			Expect(rendered).To(ContainSubstring(headingSentence))
			Expect(rendered).NotTo(ContainSubstring("You are a task execution agent"))
			for _, want := range required {
				Expect(rendered).To(ContainSubstring(want))
			}
		},
		Entry(
			"planning",
			domain.TaskPhasePlanning,
			[]string{"scripts/repo-clone.sh", "service_repo."},
		),
		Entry(
			"execution",
			domain.TaskPhaseExecution,
			append(
				[]string{
					"scripts/vault-read.sh",
					"scripts/vault-list.sh",
					"scripts/prometheus-query.sh",
				},
				verdictFields...,
			),
		),
		Entry(
			"ai_review",
			domain.TaskPhaseAIReview,
			[]string{"## Observation", "file:line"},
		),
	)

	It("falls back to output-format only for an unknown phase", func() {
		instrs := prompts.BuildInstructions(domain.TaskPhase("bogus"))
		Expect(instrs).To(HaveLen(1))
		Expect(instrs[0].Name).To(Equal("output-format"))
		Expect(instrs[0].Content).NotTo(BeEmpty())
	})
})
