// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the defect-diagnoser-agent binary.
//
// All factory functions follow the Create* prefix convention and contain
// zero business logic — they compose constructors with config.
package factory

import (
	"context"
	"path/filepath"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	delivery "github.com/bborbe/agent/delivery"
	healthcheck "github.com/bborbe/agent/healthcheck"
	"github.com/bborbe/cqrs/base"
	libkafka "github.com/bborbe/kafka"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"

	"github.com/bborbe/defect-diagnoser-agent/pkg/prompts"
	"github.com/bborbe/defect-diagnoser-agent/pkg/verdict"
)

const serviceName = "defect-diagnoser-agent"

// TaskTypeDefectDiagnose is the work task type this binary exists to serve:
// stage 1 of the defect lane. It is not exported by github.com/bborbe/agent,
// so it is declared here.
const TaskTypeDefectDiagnose agentlib.TaskType = "defect-diagnose"

// CreateClaudeRunner constructs a ClaudeRunner pre-configured with tools,
// model, working directory, and CLI environment.
func CreateClaudeRunner(
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	allowedTools claudelib.AllowedTools,
	model claudelib.ClaudeModel,
	env map[string]string,
) claudelib.ClaudeRunner {
	return claudelib.NewClaudeRunner(claudelib.ClaudeRunnerConfig{
		ClaudeConfigDir:  claudeConfigDir,
		AllowedTools:     allowedTools,
		Model:            model,
		WorkingDirectory: agentDir,
		Env:              env,
	})
}

// CreateSyncProducer creates a Kafka sync producer.
func CreateSyncProducer(
	ctx context.Context,
	brokers libkafka.Brokers,
) (libkafka.SyncProducer, error) {
	return libkafka.NewSyncProducerWithName(ctx, brokers, serviceName)
}

// CreateKafkaResultDeliverer creates a ResultDeliverer that publishes task
// updates to Kafka via CQRS commands. Uses the passthrough content generator
// — the agent framework's StepRunner already produces the full marshaled
// task in result.Output; the deliverer publishes it as-is and overrides
// status/phase frontmatter based on the result Status.
func CreateKafkaResultDeliverer(
	syncProducer libkafka.SyncProducer,
	topicPrefix base.TopicPrefix,
	taskID agentlib.TaskIdentifier,
	originalContent string,
	currentDateTime libtime.CurrentDateTimeGetter,
) agentlib.ResultDeliverer {
	return delivery.NewKafkaResultDeliverer(
		syncProducer,
		topicPrefix,
		taskID,
		originalContent,
		delivery.NewPassthroughContentGenerator(),
		currentDateTime,
	)
}

// CreateFileResultDeliverer creates a ResultDeliverer that writes the agent's
// output back to a markdown file (local CLI mode). Uses the passthrough
// content generator (same rationale as Kafka).
func CreateFileResultDeliverer(filePath string) agentlib.ResultDeliverer {
	return delivery.NewFileResultDeliverer(
		delivery.NewPassthroughContentGenerator(),
		filePath,
	)
}

// CreateAgent assembles the full 3-phase claude agent. Each phase has its own
// step and instructions: planning writes ## Plan, execution writes ## Verdict
// and ai_review writes ## Review, then advances to done.
func CreateAgent(
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	allowedTools claudelib.AllowedTools,
	model claudelib.ClaudeModel,
	claudeEnv map[string]string,
	envContext map[string]string,
) *agentlib.Agent {
	return CreateAgentFromRunner(
		CreateClaudeRunner(claudeConfigDir, agentDir, allowedTools, model, claudeEnv),
		agentDir,
		envContext,
	)
}

// CreateAgentFromRunner builds the 3-phase claude agent given a pre-constructed
// ClaudeRunner and the agent directory (whose repos/ subdirectory is the clone
// root the verdict gate reads). Used by CreateAgentProvider to share one runner
// across the domain agent and the healthcheck-Claude liveness agent.
func CreateAgentFromRunner(
	runner claudelib.ClaudeRunner,
	agentDir claudelib.AgentDir,
	envContext map[string]string,
) *agentlib.Agent {
	gate := verdict.NewGate(envContext, filepath.Join(string(agentDir), "repos"))
	planningStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "planning",
		Runner:        runner,
		Instructions:  prompts.BuildInstructions(domain.TaskPhasePlanning),
		EnvContext:    envContext,
		OutputSection: "## Plan",
		NextPhase:     string(domain.TaskPhaseExecution),
	})
	executionStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "execution",
		Runner:        runner,
		Instructions:  prompts.BuildInstructions(domain.TaskPhaseExecution),
		EnvContext:    envContext,
		OutputSection: "## Verdict",
		NextPhase:     string(domain.TaskPhaseAIReview),
	})
	aiReviewStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "ai_review",
		Runner:        runner,
		Instructions:  prompts.BuildInstructions(domain.TaskPhaseAIReview),
		EnvContext:    envContext,
		OutputSection: "## Review",
		NextPhase:     "done",
	})
	return agentlib.NewAgent(
		agentlib.NewPhase(domain.TaskPhasePlanning, verdict.NewPlanningStep(planningStep)),
		agentlib.NewPhase(domain.TaskPhaseExecution, verdict.NewExecutionStep(executionStep, gate)),
		agentlib.NewPhase(domain.TaskPhaseAIReview, verdict.NewAIReviewStep(aiReviewStep, gate)),
	)
}

// CreateAgentProvider wires the per-task-type dispatch table for defect-diagnoser-agent.
// Returns lib.AgentProvider — main.go calls Get(ctx, taskType) to select the
// appropriate *Agent. Pure plumbing; no conditional, no error.
//
// TaskTypeDefectDiagnose routes to the 3-phase diagnoser. TaskTypeHealthcheck
// and TaskTypeOAuthProbe (transition alias) both route to the shared
// healthcheck-Claude liveness agent, reusing the same ClaudeRunner.
func CreateAgentProvider(
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	allowedTools claudelib.AllowedTools,
	model claudelib.ClaudeModel,
	claudeEnv map[string]string,
	envContext map[string]string,
) agentlib.AgentProvider {
	runner := CreateClaudeRunner(claudeConfigDir, agentDir, allowedTools, model, claudeEnv)
	diagnoser := CreateAgentFromRunner(runner, agentDir, envContext)
	livenessAgent := healthcheck.NewAgent(healthcheck.NewClaudeStep(runner))
	return agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{
		TaskTypeDefectDiagnose:       diagnoser,
		agentlib.TaskTypeHealthcheck: livenessAgent,
		agentlib.TaskTypeOAuthProbe:  livenessAgent,
	})
}
