// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	healthcheck "github.com/bborbe/agent/healthcheck"
	agentmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/vault-cli/pkg/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/defect-diagnoser-agent/pkg/factory"
)

const dispatchTask = `---
task_type: defect-diagnose
phase: planning
status: in_progress
assignee: defect-diagnoser-agent
defect_id: defect-1
trigger_count: 1
max_triggers: 3
---

## Observation

the checkout service returns 500s
`

// doneResult builds a ClaudeResult whose raw text is a done envelope carrying
// output as the section body.
func doneResult(output string) *claudelib.ClaudeResult {
	blob, err := json.Marshal(map[string]string{"status": "done", "output": output})
	Expect(err).NotTo(HaveOccurred())
	return &claudelib.ClaudeResult{Result: string(blob)}
}

var _ = Describe("defect-diagnose dispatch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("routes TaskTypeDefectDiagnose to a non-nil diagnoser", func() {
		provider := factory.CreateAgentProvider(
			claudelib.ClaudeConfigDir(""),
			claudelib.AgentDir(GinkgoT().TempDir()),
			claudelib.AllowedTools{},
			claudelib.ClaudeModel("sonnet"),
			map[string]string{},
			map[string]string{},
		)
		agent, err := provider.Get(ctx, factory.TaskTypeDefectDiagnose)
		Expect(err).NotTo(HaveOccurred())
		Expect(agent).NotTo(BeNil())
	})

	It("routes healthcheck and oauth-probe to the same liveness agent", func() {
		provider := factory.CreateAgentProvider(
			claudelib.ClaudeConfigDir(""),
			claudelib.AgentDir(GinkgoT().TempDir()),
			claudelib.AllowedTools{},
			claudelib.ClaudeModel("sonnet"),
			map[string]string{},
			map[string]string{},
		)
		healthcheckAgent, err := provider.Get(ctx, agentlib.TaskTypeHealthcheck)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthcheckAgent).NotTo(BeNil())
		oauthProbeAgent, err := provider.Get(ctx, agentlib.TaskTypeOAuthProbe)
		Expect(err).NotTo(HaveOccurred())
		Expect(oauthProbeAgent).To(BeIdenticalTo(healthcheckAgent))
	})

	It("rejects the retired llm task type", func() {
		provider := factory.CreateAgentProvider(
			claudelib.ClaudeConfigDir(""),
			claudelib.AgentDir(GinkgoT().TempDir()),
			claudelib.AllowedTools{},
			claudelib.ClaudeModel("sonnet"),
			map[string]string{},
			map[string]string{},
		)
		_, err := provider.Get(ctx, agentlib.TaskType("llm"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown task_type"))
	})

	It("keeps the defect-diagnose constant inside the TaskType contract", func() {
		Expect(factory.TaskTypeDefectDiagnose.Validate(ctx)).To(Succeed())
	})

	It("builds a diagnoser whose planning prompt names the clone script", func() {
		tmp := GinkgoT().TempDir()
		fakeRunner := &agentmocks.ClaudeRunner{}
		fakeRunner.RunReturns(doneResult("## Plan\n\nservice: checkout\n"), nil)
		envContext := map[string]string{"service_repo.checkout": "bborbe/foo"}
		agent := factory.CreateAgentFromRunner(fakeRunner, claudelib.AgentDir(tmp), envContext)
		Expect(agent).NotTo(BeNil())

		taskPath := filepath.Join(GinkgoT().TempDir(), "task.md")
		Expect(os.WriteFile(taskPath, []byte(dispatchTask), 0o600)).To(Succeed())

		_, err := agent.Run(
			ctx,
			domain.TaskPhasePlanning,
			dispatchTask,
			factory.CreateFileResultDeliverer(taskPath),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeRunner.RunCallCount()).To(BeNumerically(">=", 1))
		_, prompt := fakeRunner.RunArgsForCall(0)
		Expect(prompt).To(ContainSubstring("scripts/repo-clone.sh"))
	})

	It("builds a liveness agent whose prompt carries none of the diagnoser instructions", func() {
		fakeRunner := &agentmocks.ClaudeRunner{}
		fakeRunner.RunReturns(&claudelib.ClaudeResult{Result: "ok"}, nil)
		liveness := healthcheck.NewAgent(healthcheck.NewClaudeStep(fakeRunner))

		_, err := liveness.Run(
			ctx,
			domain.TaskPhasePlanning,
			dispatchTask,
			&agentmocks.AgentResultDeliverer{},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeRunner.RunCallCount()).To(Equal(1))
		_, prompt := fakeRunner.RunArgsForCall(0)
		for _, forbidden := range []string{
			"scripts/repo-clone.sh",
			"scripts/vault-read.sh",
			"scripts/prometheus-query.sh",
			"## Verdict",
		} {
			Expect(strings.Contains(prompt, forbidden)).To(BeFalse(),
				"liveness prompt must not contain %q", forbidden)
		}
	})
})
