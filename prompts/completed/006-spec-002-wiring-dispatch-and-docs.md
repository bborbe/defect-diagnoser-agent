---
status: completed
spec: [002-diagnose-observed-defect-with-verdict-gate]
summary: Routed defect-diagnose to the gate-wrapped three-phase diagnoser, retired llm, proved the chain end to end, and documented the service_repo mapping
execution_id: defect-diagnoser-agent-diagnose-v1-exec-006-spec-002-wiring-dispatch-and-docs
dark-factory-version: v0.198.17
created: "2026-09-28T01:40:00Z"
queued: "2026-11-26T18:43:49Z"
started: "2026-11-26T19:02:35Z"
completed: "2026-11-26T19:09:43Z"
branch: dark-factory/diagnose-observed-defect-with-verdict-gate
---

# Wire the diagnoser: dispatch, phase chain, README mapping and changelog

<summary>

- A `defect-diagnose` task finally routes to the diagnoser instead of being rejected as an unknown task type.
- The old generic `llm` task type is no longer accepted, so a task can no longer reach a prompt that says nothing about diagnosis.
- The liveness task types keep routing to the liveness agent, unchanged.
- The three phases run one after another in a single job, each wrapped by the binary checks that decide whether its output stands.
- The local command-line path builds exactly the same diagnoser as the in-cluster path, so a local run proves the deployed behaviour.
- An end-to-end test drives the whole chain with a fake model and asserts the task file that comes out, for ten different outcomes including a happy path, a low-confidence escalation, a stale failure marker and a forged heading.
- The README documents the operator-supplied service-to-repo mapping and links the rule set the binary enforces.
- The changelog records the change.

</summary>

<objective>

Join the two halves this spec built. The verdict gate and the line guard exist as agent steps after prompt 1; the three per-phase prompts exist after prompt 2; nothing routes a `defect-diagnose` task to any of it. This prompt changes the dispatch table so `defect-diagnose` reaches the diagnoser and `llm` no longer does, extends the runner seam so the factory can build the gate against the clone root, wraps each phase step with its binary checks, proves the whole chain end to end with a fake model, and documents the operator-supplied mapping the diagnoser depends on.

</objective>

<context>

Read these before writing anything:

- `specs/in-progress/002-diagnose-observed-defect-with-verdict-gate.md` — read `## Desired Behavior` 1 and `## Acceptance Criteria` AC2, AC3 and AC9. AC3's ten cases (a)–(j) are the test matrix this prompt must implement.
- `docs/verdict-gate.md` — the authoritative rule set. You need § The verdict block (the eight fields, in order, and the template), § Service → repo mapping input (the `service_repo.<service>` form the README must document), and § Model-output heading rule.
- `pkg/verdict/` — **prompt 1's deliverable**. Read `step.go`, `gate.go` and `lineguard.go` before wiring anything. If the package is missing, see the sentinel rule in `<requirements>`.
- `pkg/prompts/` — **prompt 2's deliverable**. `prompts.BuildInstructions(phase)` and the three `*.md` files must already exist. If they do not, see the sentinel rule in `<requirements>`.
- `pkg/factory/factory.go` — the file you change. Read all of it: `CreateClaudeRunner`, `CreateAgent`, `CreateAgentFromRunner`, `CreateAgentProvider`, `serviceName`.
- `pkg/factory/factory_test.go`, `pkg/factory/factory_suite_test.go`, `pkg/factory/factory_kafka_topic_test.go` — the existing specs. `factory_test.go`'s dispatch specs assert the accepted-types list `[healthcheck llm oauth-probe]` and that `TaskTypeLLM` returns the domain agent; both change here.
- `cmd/run-task/main.go` — the local-CLI entry point. It calls `factory.CreateAgent(claudeConfigDir, agentDir, allowedTools, model, claudeEnv, envContext)` and then `agent.Run(ctx, a.Phase, string(taskContent), deliverer)`. **Read it; do not edit it.** Its call must keep compiling unchanged.
- `main.go` — the Kafka entry point. It calls `provider.Get(ctx, agentlib.TaskType(a.TaskType))`. **Read it; do not edit it.**
- `README.md` — where the mapping section goes. Read the existing `### Config CRD env pattern` and `### Claude subprocess env allowlist` sections; add to them rather than restructuring the file.
- `CHANGELOG.md` — the file carrying the existing `## Unreleased` section; append one bullet to it.
- `scenarios/001-diagnose-one-observed-defect.md` — already updated alongside this spec (its Setup carries a `service_repo.` entry and its Step 1 writes the task file with `defect_id`). **Do not edit it**; AC9 only checks it.
- `agent/.claude/CLAUDE.md` — the guardrails prose. Do not edit it.
- `docs/dod.md` — the Definition of Done for this repo.
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite and `DescribeTable` conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — `Create*` prefix, zero business logic in a factory, constructor shape.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors`; never `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100, `dupl`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format and the required `feat:` / `fix:` prefix.

**Verified facts about the current code, read while writing this prompt:**

- `pkg/factory/factory.go` declares `const serviceName = "defect-diagnoser-agent"` and these exported constructors: `CreateClaudeRunner(claudeConfigDir, agentDir, allowedTools, model, env)`; `CreateSyncProducer(ctx, brokers)`; `CreateKafkaResultDeliverer(...)`; `CreateFileResultDeliverer(filePath string) agentlib.ResultDeliverer`; `CreateAgent(claudeConfigDir, agentDir, allowedTools, model, claudeEnv, envContext) *agentlib.Agent`; `CreateAgentFromRunner(runner, envContext) *agentlib.Agent`; `CreateAgentProvider(claudeConfigDir, agentDir, allowedTools, model, claudeEnv, envContext) agentlib.AgentProvider`.
- `CreateAgentFromRunner` is the seam `CreateAgent` delegates to; `CreateAgentProvider` also calls it once and reuses the same `runner` for the liveness agent. There are exactly **two** call sites of `CreateAgentFromRunner` in the repo, both inside `factory.go`. Grep for `CreateAgentFromRunner` and for `factory.CreateAgent` before you start, and confirm that list has not grown.
- `agentlib.NewAgentProvider(name string, agents map[TaskType]*Agent) AgentProvider`; `Get` returns `errors.Errorf(ctx, "unknown task_type %q for %s; accepted: %v", …)` on a map miss, with the accepted keys sorted. `agentlib.TaskType` values are validated by `TaskType.Validate(ctx)` against `^[a-z0-9-]+$` and a 63-character limit.
- `github.com/bborbe/agent` exports `TaskTypeLLM = "llm"`, `TaskTypeHealthcheck = "healthcheck"` and the deprecated alias `TaskTypeOAuthProbe = "oauth-probe"`. It does **not** export a `defect-diagnose` constant, so this prompt defines one locally in `pkg/factory`.
- `healthcheck.NewAgent(step agentlib.Step) *agentlib.Agent` and `healthcheck.NewClaudeStep(runner claudelib.ClaudeRunner) agentlib.Step` are unchanged and stay in use. The liveness step's prompt is the literal `reply 'ok'`.
- `delivery.NewFileResultDeliverer(generator delivery.ContentGenerator, filePath string) agentlib.ResultDeliverer` and `factory.CreateFileResultDeliverer(filePath string)` both exist; the latter wires `delivery.NewPassthroughContentGenerator()`. `delivery.NewPassthroughContentGenerator()` applies `status` / `phase` frontmatter and splices the `## Failure` section from `result.Message` on `failed` / `needs_input`.
- The counterfeiter fake is importable as `agentmocks "github.com/bborbe/agent/mocks"` and offers `RunReturns(result *claude.ClaudeResult, err error)`, `RunReturnsOnCall(i int, result *claude.ClaudeResult, err error)`, `RunCallCount() int`, `RunArgsForCall(i int) (context.Context, string)`. Call index `i` is zero-based.
- `claudelib.AgentDir` is a string type used as the Claude runner's working directory. With a fake runner it is only read by the factory, so a test can pass a `t.TempDir()`.

**AC10 (the rung-1 live run) is the spec's operator-executable rung.** It needs two port-forwards, the router token and a real dev service, so it cannot run inside this container. Do not attempt it and do not add it to `<verification>`.


<!-- Open questions for the reviewer — decisions taken here, each with the alternative:

     1. `CreateAgentFromRunner` gains a third parameter (`agentDir`) even though AC2 only
        says the seam "takes the runner". The gate and the line guard need the clone root
        (`<agent dir>/repos`), and nothing else on the seam can supply it: the
        `claude.ClaudeRunner` interface exposes only `Run`, so the runner's working
        directory is not readable from outside. Both call sites are inside `factory.go` and
        are updated here; `CreateAgent`'s own signature — the one `cmd/run-task/main.go`
        calls — is unchanged, and `factory_test.go`'s `CreateAgent` spec keeps compiling.

     2. AC3 is implemented as ten specs in `pkg/factory/phase_chain_test.go` driving the
        real seam (`CreateAgentFromRunner` with the fake runner) and the real
        `CreateFileResultDeliverer`, rather than as more step
        tests. AC3 asserts "the written task", which only the real content generator
        produces (it is what applies `status` / `phase` and splices `## Failure`); the
        step-level tables live in prompt 1.

     3. AC3(b)(d)(e)(f) assert "the fake runner captured exactly two prompts". That is
        implemented with `RunCallCount()` on the counterfeiter fake.

     4. The task fixture sets `trigger_count: 1` and `max_triggers: 3`. Without a count
        below the cap, the framework's `failed` path clears the assignee at cap exhaustion
        and case (e) — which asserts the assignee is KEPT on `failed` — would fail for the
        wrong reason.

     5. The new tests construct `service_repo.<service>` keys in their own local
        `envContext`. That is the only way to exercise gate rules h and i, and it is
        simulated operator input rather than committed mapping data — consistent with the
        spec's Non-goal and with AC9's `grep -rn 'service_repo\.' k8s/` returning 0 lines.
        Reviewer: confirm the Non-goal is not meant to forbid these test fixtures; if it
        is, rules h and i become untestable and the Non-goal needs amending.

     6. The README change is a new `### Service → repo mapping` subsection under
        `### Config CRD env pattern`. AC9 only greps for `service_repo.` and
        `docs/verdict-gate.md` anywhere in `README.md`, so the placement is a readability
        choice, not a pinned one.

     7. AC2's liveness assertion is implemented by running the liveness agent with the same
        fake runner and asserting the captured prompt contains none of
        `scripts/repo-clone.sh`, `scripts/vault-read.sh`, `scripts/prometheus-query.sh`
        and `## Verdict`.

     8. Removing `llm` changes `pkg/factory/factory_test.go`'s accepted-types assertion to
        the literal `[defect-diagnose healthcheck oauth-probe]` (the sorted order of the
        new map's keys). Spec 002's constraint that "all other existing tests keep passing"
        is read as excluding these dispatch specs, which cannot survive the Non-goal that
        drops `llm`. -->

</context>

<requirements>

0. **Fail-loud sentinels for the cross-prompt boundary.** Before writing anything, confirm `pkg/verdict/gate.go`, `pkg/verdict/step.go`, `pkg/verdict/lineguard.go`, `pkg/prompts/planning.md`, `pkg/prompts/execution.md` and `pkg/prompts/ai_review.md` all exist and that `prompts.BuildInstructions` takes a phase argument. If `pkg/verdict/` is missing or does not export the names below, stop and report `"status":"failed"` with the message `verdict gate not yet deployed (prompt 1 of spec 002)`. If the per-phase prompts or the phase-taking `BuildInstructions` are missing, stop and report `"status":"failed"` with the message `per-phase prompts not yet deployed (prompt 2 of spec 002)`. Never report `needs_input` for either — a missing prerequisite is a broken ship order, not a question for the operator.

   The names you depend on from `pkg/verdict`, all exported: `NewGate(envContext map[string]string, cloneRoot string) *Gate`; `(*Gate).Evaluate(ctx, md) Decision`; `(*Gate).Review(ctx, md) Decision`; `type Decision struct { Status agentlib.AgentStatus; Message string }`; `NewPlanningStep(inner agentlib.Step) agentlib.Step`; `NewExecutionStep(inner agentlib.Step, gate *Gate) agentlib.Step`; `NewAIReviewStep(inner agentlib.Step, gate *Gate) agentlib.Step`.

1. **Extend the runner seam.** Change `CreateAgentFromRunner` to

   ```go
   func CreateAgentFromRunner(
       runner claudelib.ClaudeRunner,
       agentDir claudelib.AgentDir,
       envContext map[string]string,
   ) *agentlib.Agent
   ```

   and wrap each phase step prompt 2 built with its binary checks:

   - build the gate once: `gate := verdict.NewGate(envContext, filepath.Join(string(agentDir), "repos"))`. The clone root is exactly where `scripts/repo-clone.sh` writes — the fixed `repos/` directory beside `scripts/`, i.e. `<agent dir>/repos`. Use `path/filepath`, not string concatenation.
   - planning: `verdict.NewPlanningStep(planningStep)`.
   - execution: `verdict.NewExecutionStep(executionStep, gate)`.
   - ai_review: `verdict.NewAIReviewStep(aiReviewStep, gate)`.
   - register the wrapped steps under `domain.TaskPhasePlanning`, `domain.TaskPhaseExecution` and `domain.TaskPhaseAIReview`, in that order, and keep the three `claude.AgentStepConfig` values prompt 2 set (`OutputSection` `## Plan` / `## Verdict` / `## Review`, `NextPhase` `execution` / `ai_review` / `done`, and `EnvContext: envContext` on all three).
   - the body stays free of business logic: no conditional, no loop, no error path. It is composition only.

2. **Keep `CreateAgent` delegating to the seam.** `CreateAgent`'s own signature does not change — `cmd/run-task/main.go` calls it unchanged. Its body must still be exactly a call to `CreateAgentFromRunner`, now passing `agentDir` through:

   ```go
   return CreateAgentFromRunner(
       CreateClaudeRunner(claudeConfigDir, agentDir, allowedTools, model, claudeEnv),
       agentDir,
       envContext,
   )
   ```

   `grep -n 'CreateAgentFromRunner' pkg/factory/factory.go` must show `CreateAgent` calling it.

3. **Dispatch.** Add the local task-type constant next to `serviceName`:

   ```go
   // TaskTypeDefectDiagnose is the work task type this binary exists to serve:
   // stage 1 of the defect lane. It is not exported by github.com/bborbe/agent,
   // so it is declared here.
   const TaskTypeDefectDiagnose agentlib.TaskType = "defect-diagnose"
   ```

   Then change `CreateAgentProvider` to build the diagnoser through the seam and to drop `llm` from the table:

   ```go
   runner := CreateClaudeRunner(claudeConfigDir, agentDir, allowedTools, model, claudeEnv)
   diagnoser := CreateAgentFromRunner(runner, agentDir, envContext)
   livenessAgent := healthcheck.NewAgent(healthcheck.NewClaudeStep(runner))
   return agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{
       TaskTypeDefectDiagnose:       diagnoser,
       agentlib.TaskTypeHealthcheck: livenessAgent,
       agentlib.TaskTypeOAuthProbe:  livenessAgent,
   })
   ```

   The map has exactly three entries. `TaskTypeLLM` must not appear anywhere in `pkg/factory/` — including the existing `CreateAgentProvider` doc comment, which still says `TaskTypeLLM routes to the existing 3-phase domain agent`; rewrite that comment to describe `defect-diagnose`. `healthcheck` and `oauth-probe` keep returning the same liveness agent instance. The liveness agent's prompt must contain none of the diagnoser's instructions — it is built from `healthcheck.NewClaudeStep`, not from `prompts.BuildInstructions`.

4. **Update the existing dispatch specs** in `pkg/factory/factory_test.go` so they describe the new table. The three that change are the ones asserting `TaskTypeLLM` returns the domain agent and the two asserting the accepted-types list. `Get(TaskTypeDefectDiagnose)` replaces `Get(agentlib.TaskTypeLLM)`; the accepted-types assertion becomes the literal `[defect-diagnose healthcheck oauth-probe]` (that is the sorted order of the new map's keys). Keep the assertions on the quoted offending value and on the binary name. Add one spec asserting `agentlib.TaskType("llm")` now misses with `unknown task_type` (use the literal, not the constant, so the `! grep -rq 'TaskTypeLLM'` check holds). Leave `factory_kafka_topic_test.go` and every other spec in the file untouched.

5. **Prove the diagnoser is reachable, with a fake runner** — add `pkg/factory/diagnoser_dispatch_test.go`, `package factory_test`, using the existing `pkg/factory/factory_suite_test.go`:

   - `Get(ctx, factory.TaskTypeDefectDiagnose)` returns a non-nil agent — a build-only reachability assertion; do not run it, because `CreateAgentProvider` builds its own real `CreateClaudeRunner` and cannot receive the fake. Drive the seam instead: `factory.CreateAgentFromRunner(fakeRunner, claudelib.AgentDir(tmp), envContext)` at `domain.TaskPhasePlanning` against a minimal task document and a `factory.CreateFileResultDeliverer(<temp file>)`, with an `agentmocks.ClaudeRunner` that records the prompt; assert the captured prompt contains `scripts/repo-clone.sh`. The prompt is `RunArgsForCall(0)`'s second argument.
   - `Get(ctx, agentlib.TaskTypeHealthcheck)` and `Get(ctx, agentlib.TaskTypeOAuthProbe)` each return a non-nil liveness agent and the same instance — a build-only assertion. For the prompt content, build the liveness agent directly as `healthcheck.NewAgent(healthcheck.NewClaudeStep(fakeRunner))`, run it with the fake runner, and assert the captured prompt contains none of `scripts/repo-clone.sh`, `scripts/vault-read.sh`, `scripts/prometheus-query.sh` and `## Verdict` (the provider's own liveness agent cannot receive the fake).
   - `Get(ctx, agentlib.TaskType("llm"))` returns an error whose message contains `unknown task_type`.
   - a boundary check for the new constant: `Expect(factory.TaskTypeDefectDiagnose.Validate(ctx)).To(Succeed())`, so a future rename that breaks the `^[a-z0-9-]+$` contract fails here rather than at deploy.

   Build the provider with a temp directory as the agent dir, so nothing the test does touches `agent/repos/`.

6. **Wired phase-chain tests** — add `pkg/factory/phase_chain_test.go`, `package factory_test`. This is AC3. Every case drives the **real** seam (`CreateAgentFromRunner` with the fake runner) and the **real** file deliverer, and asserts on the task file that `factory.CreateFileResultDeliverer` wrote.

   **Harness.** One helper builds, for a case: a temp directory used as both the agent dir and the clone root (`<tmp>/repos/<owner>/<repo>/<path>`), the `envContext` mapping `{"service_repo.<service>": "<owner>/<repo>"}`, a task file written to a second temp path, the `agentmocks.ClaudeRunner` with one `RunReturnsOnCall` per model step, and the agent built through the seam as `factory.CreateAgentFromRunner(fakeRunner, claudelib.AgentDir(tmp), envContext)` — `CreateAgentProvider` constructs its own real runner and cannot receive the fake, so assert `factory.CreateAgentProvider(...).Get(ctx, factory.TaskTypeDefectDiagnose)` is non-nil as a separate, build-only check. The runner is then driven with `agent.Run(ctx, domain.TaskPhasePlanning, taskContent, factory.CreateFileResultDeliverer(taskPath))` and the resulting file is read back.

   Set the fake's default `RunReturns` to a benign `{"status":"failed","message":"unexpected extra model call"}` envelope as well as the per-call stubs, so an extra model call fails the case with a readable message instead of panicking on a nil result.

   The base task document for every case except (j) is a file whose frontmatter carries `task_type: defect-diagnose`, `phase: planning`, `status: in_progress`, `assignee: defect-diagnoser-agent`, `defect_id: defect-1`, `trigger_count: 1`, `max_triggers: 3`, and whose body carries `## Observation` naming the mapped service, with **no** `## Plan`. The `trigger_count` below `max_triggers` matters for the `failed` cases: at the cap the framework clears the assignee, which would hide the retry-versus-escalate assertion.

   Assert on the written file's content, not on a copy: frontmatter fields (`status`, `phase`, `assignee`), the presence and order of `## Plan`, `## Verdict`, `## Review`, the absence of `## Verdict`, the absence of `## Failure`, `## Failure` message substrings, and `grep -c '^## Verdict'` / `grep -c '^## Review'` counts. Factor the reading and the counting into helpers so no `It` body is a copy of another — `dupl` analyses test files.

   **The ten cases.** Implement each as its own spec (a `DescribeTable` over a case struct is fine, as long as each row asserts the whole list below).

   - **(a) Happy path.** The fake runner returns, in order: a plan, then a `High` verdict that passes every gate rule and whose cited line exists in the fixture clone, then a passing review. The input task carries a stale `## Failure` from a prior attempt, placed **after** `## Observation`. The written task ends `status: completed`, `phase: done`, contains `## Plan`, `## Verdict` and `## Review` in that order, and has no `## Failure`.
   - **(b) Medium verdict.** The verdict is valid with `confidence: Medium` and its cited line exists in the fixture clone. The written task has `assignee: ""`, `phase: execution`, and a `## Failure` containing `below High`. `## Verdict` is present and `## Review` is absent — ai_review never ran. Assert the fake runner captured exactly two prompts, the second containing `scripts/prometheus-query.sh`.
   - **(c) Missing line.** The verdict is `High` but its cited line exceeds the fixture file's length; the review passes. The written task has `assignee: ""`, a `## Failure` containing `beyond end of file`, the fixture file's line count, and `evidence:` followed by each line of the fixture's evidence prefixed `> `. It has `phase: ai_review`, **no** `## Verdict`, and is not `status: completed`.
   - **(d) `defect_id` mismatch.** The verdict's `defect_id` differs from the frontmatter's. The written task has `assignee: ""`, `phase: execution`, and **no** `## Verdict` — the removal happens on the task's parsed sections, so assert on the written file and on the fake runner capturing exactly two prompts.
   - **(e) Malformed verdict (rule a).** The execution response contains no yaml block. The written task has `phase: execution`, the assignee kept (`defect-diagnoser-agent`), a `## Failure`, and **no** `## Verdict`. This is the `failed` path, so the assignee must survive.
   - **(f) Model escalates at execution.** The second fake response is `{"status":"needs_input","message":"…"}`. The written task has `phase: execution` and `assignee: ""`. This pins the phase cursor on entry, not only in the gate's reject branches.
   - **(g) Heading rule, planning.** The planning response contains a column-0 `## Verdict` line. The result is `failed` naming `## Plan`; the fake runner captured exactly one prompt; the written task has `phase: planning`, no `## Plan` section, and `grep -c '^## Verdict'` equal to 0. The `## Failure` body has no line starting with `#`.
   - **(h) Heading rule, execution.** The execution response carries a valid verdict block plus a column-0 `## Verdict` line. The result is `failed` naming `## Verdict`, and the written task has `phase: execution` and `grep -c '^## Verdict'` equal to 0.
   - **(i) Heading rule, ai_review.** The review response contains a column-0 `## Verdict` line. The result is `failed` naming `## Review`, and the written task has `phase: ai_review`, no `## Review`, and exactly one `^## Verdict` line — the gated verdict execution wrote.
   - **(j) Entry at ai_review.** The task enters at `phase: ai_review` with a pre-written, otherwise valid `confidence: Medium` verdict whose line exists in the fixture clone, and the fake runner returns a passing review. The written task is not `status: completed`: it has `assignee: ""`, `phase: ai_review`, and a `## Failure` containing `below High`.

   **Fixtures.** The fixture clone is a temp directory holding `<repo>/<path>` with a known number of lines, so cases (a), (b), (j) pass the line guard and case (c) fails it. The fixture verdict is the eight keys in the contract's order with `evidence` naming `scripts/prometheus-query.sh` and quoting an output line. Give the fake runner's plan response a `## Plan`-shaped body naming `service`, `repo`, `clone_path` and `head_sha`, and its review response a body with no column-0 `#` line. Keep every fixture string in a shared helper so the cases differ only in the field each one changes.

   For the review response to be "passing" it only has to be a `done` envelope whose body has no column-0 `#` line — the binary checks the review's shape, not its prose.

7. **README — document the mapping input.** Add a `### Service → repo mapping` subsection under `### Config CRD env pattern`, before `### Claude subprocess env allowlist`. It must:

   - explain that the diagnoser owns no mapping data, and that the operator supplies one entry per service through the existing `ENV_CONTEXT` channel, in the form `service_repo.<service>=<owner>/<repo>`, comma-separated when there is more than one;
   - state that the mapping renders into the prompt's `## Environment` section as `service_repo.<service>: <owner>/<repo>`, and that a service resolves **only** on an exact, case-sensitive key match — no fuzzy match, no fallback, and no repo from the model's own knowledge;
   - say that the mapping is deliberately **not** committed to this repo's `k8s/` Config, and that an unmapped service escalates to the operator instead of guessing;
   - link the rule set the binary enforces: a line containing the literal `docs/verdict-gate.md`.

   `grep -c 'service_repo\.' README.md` must be at least 1 and `grep -c 'docs/verdict-gate.md' README.md` must be at least 1. Leave the rest of the README's structure alone — in particular do not re-add any mention of pod logs or events, and do not touch the generic `Tune ALLOWED_TOOLS per task shape` table.

8. **CHANGELOG.** Append one `feat:` bullet to the existing `## Unreleased` section at the top of `CHANGELOG.md` (it already carries two bullets from spec 001); do not create a second `## Unreleased` heading. Follow `changelog-guide.md`: the bullet needs a `feat:` prefix and must mention the diagnoser. Do not describe verification steps, do not list files, and do not use a prompt filename as the entry. This prompt owns the only `## Unreleased` entry for this spec — the sibling prompts deliberately add none.

9. **AC9 checks that need no edit, only confirmation.** `scenarios/001-diagnose-one-observed-defect.md` was already updated alongside this spec. Do not edit it. Confirm with the `<verification>` greps that its Setup block still carries a `service_repo.` entry and its Step 1 block still carries `defect_id`.

10. **Self-check before finishing.** Re-run `<verification>` and confirm every line passes. Then walk AC2, AC3 and AC9 bullet by bullet against your change and confirm each is actually asserted — a case that asserts only the status and not the written task does not count. Confirm `cmd/run-task/main.go` and `main.go` still compile unchanged, and that the two `CreateAgentFromRunner` call sites are both updated.

</requirements>

<constraints>

- **Contract** ([[Defect Lane Task Chain]], amended 2026-09-27; copied into `docs/verdict-gate.md` because the container cannot read the vault). Fixed: the `## Verdict` field names and their order; the `confidence` values `High | Medium | Low`; and the escalation split (no line → no Verdict; line + Medium/Low → Verdict kept; stage 2 only from High). `## Observation` is read and never rewritten.
- **Gate rules are frozen in `docs/verdict-gate.md`.** Do not change that page, and do not change the rule behaviour prompt 1 shipped.
- **Frozen envelope:** the final-response JSON `{status, message, files}` that the delivery layer parses is unchanged.
- **Escalation mechanics are the framework's.** `needs_input` clears `assignee`, sets `previous_assignee` and writes `## Failure`. `failed` keeps the assignee for the trigger-count retry, and clears it only at cap exhaustion. Do not reimplement any of it.
- **Phase cursor.** The diagnoser writes no frontmatter field of its own except `phase`. Prompt 1's step wrappers already set it on entry; do not add a second writer.
- `healthcheck` / `oauth-probe` routing and the liveness agent are unchanged, and they share the one `runner` instance.
- **Do NOT add a confidence-threshold knob and do NOT add a gate opt-out flag.** `High`-only is the contract; the spec's Non-goals forbid both outright.
- **No committed mapping data.** No `service_repo.<service>` key/value pair may be hard-coded in the Config, in non-test Go code, or in any docs file. The README documents the **form** only. The new tests are the one exception and the reason the exception exists: they construct `service_repo.<service>` keys in their own local `envContext` to exercise rules h and i, which cannot be tested without them — that is simulated operator input, not committed mapping data.
- **Do NOT add a `pod-logs.sh` or any pod-log / event read**, and do not re-introduce a pod-log mention into the README. That waits on the Config CRD gaining a service-account field and is an explicit spec Non-goal.
- Do NOT change the `github.com/bborbe/agent` module or its version. No `go get`, no version bump, and never `-mod=vendor`.
- **Scope.** This prompt touches `pkg/factory/factory.go`, `pkg/factory/factory_test.go`, the two new `pkg/factory/*_test.go` files, `README.md` and `CHANGELOG.md`. Do NOT edit `pkg/verdict/`, `pkg/prompts/`, `main.go`, `cmd/run-task/main.go`, `k8s/`, `agent/`, `scenarios/`, `docs/verdict-gate.md` or `docs/observation-scripts.md`.
- **Prompts 1 and 2 of this spec must have landed first.** If `pkg/verdict/` or the per-phase prompts are missing, follow requirement 0 — report `"status":"failed"` naming the missing prompt. Never `needs_input`.
- Do NOT create or edit the four read-only scripts. They are spec `read-only-observation-toolkit`'s deliverable, and **that spec must be merged before this one starts**.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass. The only specs this prompt changes are the three dispatch assertions in `pkg/factory/factory_test.go`; everything else stays green.

</constraints>

<verification>

Run every line from the repo root. All must pass.

```bash
make test          # the new dispatch and phase-chain suites run here
make precommit     # AC1 — must exit 0

# AC2 — the seam exists and CreateAgent delegates to it
grep -n 'CreateAgentFromRunner' pkg/factory/factory.go
test "$(grep -c 'CreateAgentFromRunner(' pkg/factory/factory.go)" -ge 3

# the dispatch table has the new task type and no longer accepts llm
grep -qF 'TaskTypeDefectDiagnose:' pkg/factory/factory.go
grep -qE 'agentlib\.TaskTypeHealthcheck: +livenessAgent' pkg/factory/factory.go
grep -qE 'agentlib\.TaskTypeOAuthProbe: +livenessAgent' pkg/factory/factory.go
! grep -rq 'TaskTypeLLM' pkg/factory/

# AC9 — README documents the mapping form and links the rule set
test "$(grep -c 'service_repo\.' README.md)" -ge 1
test "$(grep -c 'docs/verdict-gate.md' README.md)" -ge 1

# AC9 — no mapping entry is committed to the Config
! grep -rq 'service_repo\.' k8s/

# AC9 — scenario 001 already carries both edits; this prompt only confirms them
test "$(sed -n '/^1\. \*\*Create the task/,/^2\. /p' scenarios/001-diagnose-one-observed-defect.md | grep -c defect_id)" -ge 1
test "$(sed -n '/^\*\*Setup\*\*/,/^\*\*Acceptance\*\*/p' scenarios/001-diagnose-one-observed-defect.md | grep -c 'service_repo\.')" -ge 1

# AC9 — the changelog records the change, and the README keeps no pod-log claim
test "$(sed -n '/^## Unreleased/,/^## v/p' CHANGELOG.md | grep -ci 'diagnos')" -ge 1
test "$(grep -ciE 'pod[- ]?logs' README.md)" = "0"

# the factory tests leave no clone behind and never touch the shared local one
test -z "$(find agent/repos -mindepth 1 2>/dev/null)"
! grep -rn 'agent/repos' pkg/factory/

# changed code reaches at least 80% statement coverage
go test -mod=mod -coverprofile=/tmp/factory-cover.out ./pkg/factory/...
test "$(go tool cover -func=/tmp/factory-cover.out | tail -1 | sed -E 's/.* ([0-9]+)\.[0-9]+%/\1/')" -ge 80
```

</verification>
