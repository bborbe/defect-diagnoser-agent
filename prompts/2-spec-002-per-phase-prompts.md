---
status: draft
spec: [002-diagnose-observed-defect-with-verdict-gate]
created: "2026-09-28T01:40:00Z"
branch: dark-factory/diagnose-observed-defect-with-verdict-gate
---

# Replace the scaffold prompt with one prompt per diagnoser phase

<summary>

- Each of the three diagnoser phases stops sharing one generic scaffold prompt and gets instructions written for that phase.
- Planning is told to resolve the observed service against the operator's mapping and to clone the mapped repo, and never to guess a repo.
- Execution is told which read-only scripts to call, in what order, and how to quote each call and its output as evidence.
- Review is told to re-check the verdict against the original observation, including whether the cited line supports the stated cause.
- Every phase is told never to start a line of its response with a hash sign, because the step supplies the heading.
- Every phase ends with the same JSON envelope the delivery layer already parses.
- The generic "you are a task execution agent" scaffold prompt is gone.
- The three phase steps are built in the factory with their own section heading and next phase, so the chain advances planning to execution to review to done.
- The instructions are embedded and checked by tests, so a phase prompt that loses a required instruction fails the build.

</summary>

<objective>

Give each diagnoser phase instructions of its own. Today every phase shares one scaffold prompt that says nothing about defect diagnosis, so planning never resolves a service, execution never observes the running system, and review never checks the verdict. After this change `pkg/prompts/` carries `planning.md`, `execution.md` and `ai_review.md`, each embedded and selected by phase, and `pkg/factory` builds three distinct steps from them. The generic scaffold prompt is deleted, so no phase can fall back to it.

</objective>

<context>

Read these before writing anything:

- `specs/in-progress/002-diagnose-observed-defect-with-verdict-gate.md` — read `## Desired Behavior` 2, 3 and 5, and `## Acceptance Criteria` AC8. AC8 pins the exact strings each phase prompt must contain.
- `docs/verdict-gate.md` — § The verdict block (the eight fields and their order, and the template you must copy into the execution prompt) and § Service → repo mapping input (the `service_repo.<service>` form the planning prompt must describe). Where it and any other source disagree, it wins.
- `docs/observation-scripts.md` — the authoritative contract for the four read-only scripts the prompts tell the model to call. The prompts must not contradict it, and must not invent a script.
- `specs/in-progress/001-read-only-observation-toolkit.md` — the dependency spec. It provides the four scripts, the `GIT_REST_URL` / `PROMETHEUS_URL` forwarding and the `ALLOWED_TOOLS` string. **Spec 001 must be merged before this prompt runs.** Do not reimplement any of it, and do not edit `agent/scripts/`.
- `prompts/completed/001-spec-001-http-observation-scripts.md` — house style for this repo.
- `prompts/in-progress/003-spec-001-config-allowlist-and-consistency.md` — the three-way consistency test that scans `pkg/prompts/*.md` for `scripts/<name>.sh` mentions and asserts each mentioned script is allowlisted. Every script name you write into a prompt must be one of the four allowlisted names, spelled exactly `scripts/vault-read.sh`, `scripts/vault-list.sh`, `scripts/prometheus-query.sh`, `scripts/repo-clone.sh`.
- `pkg/prompts/prompts.go`, `pkg/prompts/workflow.md`, `pkg/prompts/output-format.md`, `pkg/prompts/prompts_test.go`, `pkg/prompts/prompts_suite_test.go` — the files this prompt changes.
- `pkg/factory/factory.go` — `CreateAgentFromRunner` builds the steps; read it before changing it.
- `agent/.claude/CLAUDE.md` — the guardrails prose the model reads alongside these prompts. Do not edit it; make the prompts consistent with it.
- `docs/dod.md` — the Definition of Done for this repo.
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite and spec conventions, including `DescribeTable` / `Entry`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100, `dupl`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — GoDoc comments start with the name and are full sentences.

**Verified facts about the current code, read while writing this prompt:**

- `pkg/prompts/prompts.go` embeds `workflow.md` and `output-format.md` and exposes `func BuildInstructions() claudelib.Instructions` returning a two-element `claudelib.Instructions` (`type Instructions []Instruction`, `type Instruction struct { Name, Content string }`). `claudelib.Instructions.String()` renders each element as `<name>content</name>` joined by newlines, and `claude.BuildPrompt(instructions, envContext, taskContent)` appends `## Environment` (the `ENV_CONTEXT` pairs, sorted) and `## Task` (the marshaled task document).
- `pkg/prompts/prompts_test.go` asserts exactly three things about the scaffold builder: two instructions, the first named `workflow`, the second named `output-format`. Spec 002's `## Constraints` says those length/name tests are **replaced** by per-phase assertions; every other existing test keeps passing.
- `pkg/factory/factory.go` currently builds **one** `claude.NewAgentStep` with `OutputSection: "## Result"`, `NextPhase: "done"`, and registers it under `planning`, `domain.TaskPhaseExecution` and `ai_review`.
- `workflow.md` opens with `You are a task execution agent. Execute the task described below.` — the exact sentence AC8 forbids in any phase.
- `output-format.md` carries the frozen JSON envelope `{status, message, files}`. Keep it and keep embedding it for every phase.


<!-- Open questions for the reviewer — decisions taken here, each with the alternative:

     1. The spec pins that the scaffold's BuildInstructions length/name tests are replaced
        by per-phase assertions, but it does not pin the builder's name or signature. This
        prompt keeps the name `BuildInstructions` and gives it a `domain.TaskPhase`
        parameter, so there is exactly one builder and no dead zero-arg one. Alternative: a
        new name (`BuildPhaseInstructions`) alongside the old, which leaves dead surface
        until prompt 3 deletes it.

     2. The unknown-phase case returns a one-element Instructions (output-format only)
        instead of an error, because `CreateAgentFromRunner` has no error path and the
        factory only ever asks for the three canonical phases. Alternative: return
        `(claudelib.Instructions, error)` and change the seam's signature, which spreads an
        error path through two factory functions for a case that cannot occur.

     3. This prompt changes the BODY of `CreateAgentFromRunner` even though the spec's
        Suggested Decomposition assigns factory work to prompt 3. Reason: deleting
        `workflow.md` and changing `BuildInstructions`'s signature leaves the repo
        non-compiling unless the single call site is updated in the same prompt, and every
        prompt must leave `make precommit` green. Reviewer: confirm the split — the body
        here, the signature change plus the gate wrapping plus the dispatch table in
        prompt 3.

     4. Planning and ai_review both name `scripts/repo-clone.sh` (ai_review re-clones when
        the clone is absent). Spec 001's three-way consistency test requires every script
        named in `pkg/prompts/*.md` to be allowlisted, and all four names used here are in
        the allowlist, so no extra allowlist change is needed.

     5. AC8 asserts the required strings on "their built instructions". `Instructions.String()`
        wraps each instruction in `<name>…</name>`, so the assertions are written against
        that rendered form — which is also exactly what the model receives through
        `claude.BuildPrompt`. Alternative: assert on the raw embedded `Content`, which would
        not cover the wrapping the model actually sees.

     6. `output-format.md` is kept and embedded for all three phases, so the frozen JSON
        envelope still reaches every phase. AC8's "no phase contains `You are a task
        execution agent`" is satisfied by deleting `workflow.md`, not by editing it. -->

</context>

<requirements>

1. **Create `pkg/prompts/planning.md`.** Plain markdown, no XML wrapper, no frontmatter — it is embedded verbatim as one `claudelib.Instruction`. It must contain the literal strings `scripts/repo-clone.sh` and `service_repo.`, and the literal sentence `Never start a line of your response with #.` It must instruct the model to:

   - act as stage 1 of the defect lane, in its planning phase, and read the `## Observation` section of the task as the operator's report of a misbehaving service — it contains no diagnosis, and the model must not ask for one;
   - resolve the service named in `## Observation` **only** by an exact, case-sensitive match against the `service_repo.<service>` entries listed in the prompt's `## Environment` section. Say plainly that the mapping is operator-supplied, that the match is exact and case-sensitive, and that there is no fuzzy match, no fallback, and no repo from the model's own knowledge;
   - on a match, run `scripts/repo-clone.sh clone <repo>` and record in the `## Plan` section the four keys `service`, `repo`, `clone_path` and `head_sha`, taking `clone_path` and `head_sha` from the script's stdout;
   - on no match, or when the clone fails, return the JSON envelope with `"status":"needs_input"` naming the service it could not resolve or the clone error it saw, and **never** substitute a repo of its own choosing;
   - never start a line of its response with `#`, because the step supplies the `## Plan` heading;
   - end its response with the JSON envelope defined by `<output-format>`, and put nothing after it.

2. **Create `pkg/prompts/execution.md`.** It must contain the literal strings `scripts/vault-read.sh`, `scripts/vault-list.sh` and `scripts/prometheus-query.sh`, all eight verdict field names (`defect_id`, `service`, `repo`, `file:line`, `root_cause`, `recommended_fix`, `evidence`, `confidence`), and the literal sentence `Never start a line of your response with #.` It must instruct the model to:

   - observe the running system in this order: first the task files, through `scripts/vault-read.sh` and `scripts/vault-list.sh`; then metrics, through `scripts/prometheus-query.sh`. Say that these four read-only scripts are its only eyes on the running system, that they cannot write, and that it must not reach a cluster address directly;
   - localise the cause in the clone that planning left, using `Read`, `Grep` and `Glob`;
   - re-clone with `scripts/repo-clone.sh clone <repo>` first if the clone is absent (for example after a pod restart);
   - emit a fenced ```` ```yaml ```` block inside the `## Verdict` section, with the eight keys in the contract's order. Copy the template from `docs/verdict-gate.md` § The verdict block into the prompt verbatim, including the `file:line: <path>:<line>` line;
   - copy `defect_id` from the task's frontmatter, never invent one;
   - put the exact script call and its output into `evidence` — a verdict whose evidence does not name a live read is rejected by the binary;
   - give a cause that is supported only by reading source a `confidence` below `High`, and reserve `High` for a cause the live reads show;
   - when no line can be cited, return the JSON envelope with `"status":"needs_input"`, write no `## Verdict` section at all, and make the message list each script call it tried and what that call returned;
   - never start a line of its response with `#`, because the step supplies the `## Verdict` heading;
   - end its response with the JSON envelope defined by `<output-format>`, and put nothing after it.

3. **Create `pkg/prompts/ai_review.md`.** It must contain the literal strings `## Observation` and `file:line`, and the literal sentence `Never start a line of your response with #.` It must instruct the model to:

   - re-check the `## Verdict` the execution phase wrote against the task's `## Observation`, and to fail the check rather than wave it through;
   - check four things, naming each: the verdict's `service` matches the service named in `## Observation`; `root_cause` explains the observed symptom; `evidence` quotes a script call and its output; the content of the line cited by `file:line` supports `root_cause`;
   - re-clone with `scripts/repo-clone.sh clone <repo>` if the clone is absent, and record in the `## Review` section any `head_sha` difference from the `## Plan` section;
   - on a failed check, return the JSON envelope with `"status":"needs_input"` naming the check that failed;
   - on a passing check, write the `## Review` section and return `"status":"done"`;
   - never start a line of its response with `#`, because the step supplies the `## Review` heading;
   - end its response with the JSON envelope defined by `<output-format>`, and put nothing after it.

4. **Rewrite `pkg/prompts/prompts.go`** so the instructions are selected by phase:

   ```go
   //go:embed planning.md
   var planning string

   //go:embed execution.md
   var execution string

   //go:embed ai_review.md
   var aiReview string

   //go:embed output-format.md
   var outputFormat string

   // BuildInstructions assembles the prompt for one diagnoser phase: that phase's
   // instructions followed by the shared output-format envelope.
   func BuildInstructions(phase domain.TaskPhase) claudelib.Instructions
   ```

   `domain` is `github.com/bborbe/vault-cli/pkg/domain`; the factory already imports it. Switch on `domain.TaskPhasePlanning`, `domain.TaskPhaseExecution` and `domain.TaskPhaseAIReview`. Each case returns a two-element `claudelib.Instructions`: the phase's own instruction first, named `"planning"` / `"execution"` / `"ai_review"` respectively, then the `"output-format"` instruction. For any other phase return just the `"output-format"` instruction — the factory only ever asks for the three, and a silent one-element fallback keeps the call site free of an error path.

5. **Delete `pkg/prompts/workflow.md`.** It carries the scaffold sentence AC8 forbids, and nothing else embeds it once requirement 4 lands. Delete the file; do not leave it behind unreferenced.

6. **Update `CreateAgentFromRunner` in `pkg/factory/factory.go`** so it builds three distinct steps from the three prompts. **Do not change its signature in this prompt** — the last prompt of this spec extends it. Today it builds one `claude.NewAgentStep` and registers that same step under all three phases; replace the body so each phase gets its own step:

   ```go
   planningStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{
       Name:          "planning",
       Runner:        runner,
       Instructions:  prompts.BuildInstructions(domain.TaskPhasePlanning),
       EnvContext:    envContext,
       OutputSection: "## Plan",
       NextPhase:     string(domain.TaskPhaseExecution),
   })
   ```

   and likewise `executionStep` with `OutputSection: "## Verdict"` and `NextPhase: string(domain.TaskPhaseAIReview)`, and `aiReviewStep` with `OutputSection: "## Review"` and `NextPhase: "done"`. Then return

   ```go
   agentlib.NewAgent(
       agentlib.NewPhase(domain.TaskPhasePlanning, planningStep),
       agentlib.NewPhase(domain.TaskPhaseExecution, executionStep),
       agentlib.NewPhase(domain.TaskPhaseAIReview, aiReviewStep),
   )
   ```

   Keep the `EnvContext: envContext` on every step — that is how the `service_repo.<service>` mapping reaches the planning prompt. Keep `CreateAgent` and `CreateAgentProvider` calling `CreateAgentFromRunner`; only its body changes. Do **not** add the verdict gate, do **not** change the dispatch table, and do **not** add a parameter — those are the last prompt's work.

7. **Rewrite `pkg/prompts/prompts_test.go`.** Replace the three scaffold specs (two instructions, first named `workflow`, second named `output-format`) with per-phase assertions. Keep `package prompts_test` and the existing suite file `pkg/prompts/prompts_suite_test.go` untouched.

   Build the instructions with `prompts.BuildInstructions(phase)` and render them with `.String()`, then assert, as a `DescribeTable` with one `Entry` per phase (`domain.TaskPhasePlanning`, `domain.TaskPhaseExecution`, `domain.TaskPhaseAIReview`):

   - `len(instrs) == 2`, `instrs[0].Name` equals the phase name, `instrs[1].Name == "output-format"`, and neither `Content` is empty;
   - the rendered instructions contain the literal sentence `Never start a line of your response with #.`;
   - the rendered instructions do **not** contain `You are a task execution agent`;
   - per-phase required strings: planning contains `scripts/repo-clone.sh` and `service_repo.`; execution contains `scripts/vault-read.sh`, `scripts/vault-list.sh`, `scripts/prometheus-query.sh` and all eight verdict field names; ai_review contains `## Observation` and `file:line`.

   Then add a `DescribeTable` over `prompts.BuildInstructions(domain.TaskPhase("bogus"))` asserting the one-element fallback returns only the `output-format` instruction. Keep every `It`/`Entry` body short by passing the required strings as `Entry` arguments rather than writing one spec per phase.

8. **Coverage and self-check.** `pkg/prompts/prompts.go` is new code and must reach at least 80 % statement coverage: check with `go test -mod=mod -coverprofile=/tmp/prompts-cover.out ./pkg/prompts/...` then `go tool cover -func=/tmp/prompts-cover.out`. Before finishing, re-run `<verification>` and confirm every line passes, then walk AC8's bullet list against the three files and confirm each required string is present character for character.

</requirements>

<constraints>

- **Contract** ([[Defect Lane Task Chain]], amended 2026-09-27; copied into `docs/verdict-gate.md` because the container cannot read the vault). Fixed: the `## Verdict` field names and their order; the `confidence` values `High | Medium | Low`; and the escalation split (no line → no Verdict; line + Medium/Low → Verdict kept; stage 2 only from High). `## Observation` is read and never rewritten — no prompt may tell the model to edit it.
- **Frozen envelope:** the final-response JSON `{status, message, files}` that the delivery layer parses is unchanged. `pkg/prompts/output-format.md` keeps its current wording; do not rewrite it.
- **Frozen tool names.** The four `Bash(scripts/…)` allowlist entries keep their exact names, resolved relative to the agent working directory. A prompt may only name one of the four allowlisted scripts, spelled exactly as the allowlist spells it.
- **Do NOT add any `service_repo.` mapping entry** to this repo's Config, code, tests or docs. The prompts describe the operator-supplied **form** `service_repo.<service>`; no key/value pair may be hard-coded anywhere.
- **Do NOT add a confidence-threshold knob and do NOT add a gate opt-out flag.** `High`-only is the contract, and the spec's Non-goals forbid both outright.
- **Do NOT add a `pod-logs.sh` or any pod-log / event read.** That waits on the Config CRD gaining a service-account field and is an explicit spec Non-goal. No phase prompt may mention pod logs or events.
- Do NOT change the `github.com/bborbe/agent` module or its version. No `go get`, no version bump, and never `-mod=vendor`.
- **Scope.** This prompt touches only `pkg/prompts/*.md`, `pkg/prompts/prompts.go`, `pkg/prompts/prompts_test.go` and the body of `CreateAgentFromRunner` in `pkg/factory/factory.go`. Do NOT create or edit `pkg/verdict/` (a sibling prompt's deliverable), do NOT change `CreateAgentFromRunner`'s signature, do NOT change the dispatch table in `CreateAgentProvider`, and do NOT touch `main.go`, `cmd/run-task/main.go`, `agent/scripts/`, `agent/.claude/CLAUDE.md`, `k8s/`, `README.md` or `CHANGELOG.md`.
- **The CHANGELOG is not this prompt's job.** The last prompt of this spec owns the single `## Unreleased` entry; AC9 greps for it. Do not add one here.
- Do NOT create or edit the four read-only scripts. They are spec `read-only-observation-toolkit`'s deliverable, and **that spec must be merged before this one starts**.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass. The only specs this prompt replaces are the three scaffold assertions in `pkg/prompts/prompts_test.go`; the healthcheck / oauth-probe routing and every other existing test are untouched.

</constraints>

<verification>

Run every line from the repo root. All must pass.

```bash
make test          # the rewritten pkg/prompts suite runs here
make precommit     # AC1 — must exit 0

# the three phase prompts exist, the scaffold prompt is gone
test -f pkg/prompts/planning.md
test -f pkg/prompts/execution.md
test -f pkg/prompts/ai_review.md
test -f pkg/prompts/output-format.md
test ! -f pkg/prompts/workflow.md

# AC8 — per-phase required strings
grep -qF 'scripts/repo-clone.sh' pkg/prompts/planning.md
grep -qF 'service_repo.' pkg/prompts/planning.md
grep -qF 'scripts/vault-read.sh' pkg/prompts/execution.md
grep -qF 'scripts/vault-list.sh' pkg/prompts/execution.md
grep -qF 'scripts/prometheus-query.sh' pkg/prompts/execution.md
for f in defect_id service repo 'file:line' root_cause recommended_fix evidence confidence; do grep -qF "$f" pkg/prompts/execution.md || exit 1; done
grep -qF '## Observation' pkg/prompts/ai_review.md
grep -qF 'file:line' pkg/prompts/ai_review.md

# AC8 — the heading sentence is in every phase prompt
for f in planning execution ai_review; do grep -qF 'Never start a line of your response with #.' pkg/prompts/$f.md || exit 1; done

# AC8 — the scaffold sentence is gone
! grep -qF 'You are a task execution agent' pkg/prompts/planning.md pkg/prompts/execution.md pkg/prompts/ai_review.md pkg/prompts/output-format.md

# AC8 (negative) — no pod-log mention in any embedded prompt; must print nothing
! grep -qiE 'pod[- ]?logs' pkg/prompts/*.md

# every script named in a prompt is one of the four allowlisted names
test "$(grep -ohE 'scripts/[A-Za-z0-9._-]+\.sh' pkg/prompts/*.md | sort -u | wc -l)" -le 4

# the factory now builds three distinct phase steps with their own headings
test "$(grep -c 'claudelib.NewAgentStep' pkg/factory/factory.go)" = "3"
grep -qF 'OutputSection: "## Plan"' pkg/factory/factory.go
grep -qF 'OutputSection: "## Verdict"' pkg/factory/factory.go
grep -qF 'OutputSection: "## Review"' pkg/factory/factory.go

# new code reaches at least 80% statement coverage
go test -mod=mod -coverprofile=/tmp/prompts-cover.out ./pkg/prompts/...
test "$(go tool cover -func=/tmp/prompts-cover.out | tail -1 | sed -E 's/.* ([0-9]+)\.[0-9]+%/\1/')" -ge 80
```

</verification>
