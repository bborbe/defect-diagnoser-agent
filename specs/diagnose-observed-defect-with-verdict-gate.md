---
status: draft
created: 2026-09-27
---

## Summary

- Turn the scaffolded agent into the real stage-1 defect diagnoser. A `defect-diagnose` task goes through planning, execution and ai_review, and ends with either a `## Verdict` that names a root cause and a `file:line`, or an honest escalation to the operator.
- Planning looks the observed service up in a service → repo mapping that the operator supplies, then clones that repo read-only. If the service is not in the mapping, the task escalates instead of guessing.
- Execution observes the live system through the read-only toolkit and quotes each read and its output as evidence.
- The binary, not the model, decides whether a verdict stands. A malformed, unmapped or unevidenced verdict never reaches the task file, and anything below `High` escalates. After review, the binary confirms that the cited line really exists in the clone.
- Depends on spec `read-only-observation-toolkit`, which provides the scripts, the env forwarding and the allowlist these prompts call.

## Problem

The scaffold's dispatch rejects `defect-diagnose`, which is the only work task type in its own Config. Every phase runs the same generic prompt and jumps straight to `done`. Nothing turns an operator's observation into a diagnosis, so the operator still diagnoses every defect by hand. That is the largest single interaction cost the defect lane exists to remove (goal SC2 of "An Observed Defect Reaches human_review Without Me").

A model left to itself can also invent a repo, a `defect_id` or a `file:line`, or claim a cause it never observed. A confident-looking guess would then travel to stage 2 as fact. The lane needs a diagnoser whose verdicts are checked by code: well-formed, tied to the task and to the operator's mapping, backed by a live read, confident, and resolvable in the clone.

## Goal

A `defect-diagnose` task file runs through planning, execution and ai_review inside one agent. It ends in exactly one of two states. A `failed` outcome retries under the existing trigger-count budget, and at the cap it too reaches state 2.

1. **Completed.** `status: completed`, `phase: done`, and no `## Failure` section. The `## Verdict` yaml block carries the eight contract fields (`docs/verdict-gate.md`, copied from [[Defect Lane Task Chain]] § The contract), with:
   - `confidence: High`
   - `defect_id` equal to the task's own `defect_id`
   - `repo` equal to the operator's mapping entry for `service`
   - a `file:line` whose line exists in that repo's read-only clone
   - `evidence` that names a live read script (`scripts/vault-` or `scripts/prometheus-query.sh`) and quotes its output; whether that output supports the cause is checked by the ai_review model (DB5)
2. **Escalated.** `assignee: ""`, `previous_assignee` set, and a `## Failure` naming why. This follows the contract as amended 2026-09-27 (§ Live observation → "Not reproducible → escalate, never guess"):
   - **No line can be cited, including a cited line that fails the line guard:** no `## Verdict` in the body.
   - **A line is cited and passes the line guard, but `confidence` is Medium or Low:** the `## Verdict` is kept. The gate runs the line guard before keeping it, so a kept Medium/Low verdict never carries an unresolvable line.
   - **Any other rejected verdict:** the `## Verdict` is removed from the body.

   When the verdict is removed because no line can be cited, the `## Failure` message quotes the evidence with every line prefixed `> `. That covers rule c, rule j's line-guard failure and the ai_review line guard. The operator keeps what was observed, and no quoted line can inject a heading.

The goal's SC2 counts only outcome 1, shown on a real defect by rung-1 (AC10 a) or scenario 001. The escalation outcomes AC10(b) admits make this spec pass, but they do not meet SC2.

## Non-goals

- Creating the stage-2 `defect-spec` task (contract open question 1).
- Pod-log / event reads. These need a platform change first (see spec `read-only-observation-toolkit`).
- A general service → repo mapping, or `service_repo.` entries in any Config or code file of this repo. The mapping is operator-supplied input (contract open question 4).
- Private-repo cloning, replay, deploy, `kubectl apply` and secrets.
- Per-phase tool scoping. One Config allowlist serves all phases.
- The `llm` task-type alias. It is not in the Config's `taskTypes` and is dropped from the dispatch table.
- Any change to the `github.com/bborbe/agent` module, including an upstream fix of the in-process phase cursor (a separate change, reported to the fleet).
- Do NOT add a confidence-threshold knob or a gate opt-out flag. `High`-only is the contract; if a future consumer demands variation, that is a separate spec.

## Acceptance Criteria

Container-executable. Every test runs under `make test`.

"Wired agent" means the agent returned by the provider's `Get(ctx, "defect-diagnose")`, built with a fake Claude runner and the `service_repo.` mapping in its env context. "Written task" means the file that `CreateFileResultDeliverer` writes.

- [ ] **AC1** `make precommit` exits 0. Evidence: exit code.
- [ ] **AC2** Dispatch:
  - `Get("defect-diagnose")` returns the diagnoser: running it at `phase: planning` sends the fake runner a prompt containing `scripts/repo-clone.sh`.
  - `factory.CreateAgent`, the local-CLI path that `cmd/run-task/main.go` calls, builds the diagnoser too. It delegates to a `CreateAgentFromRunner` seam that takes the runner, and the test drives that seam with the fake runner: at `phase: planning` the captured prompt contains `scripts/repo-clone.sh`. `grep -n 'CreateAgentFromRunner' pkg/factory/factory.go` shows `CreateAgent` calling it.
  - `Get("healthcheck")` and `Get("oauth-probe")` return the liveness agent, whose prompt contains none of the diagnoser's instructions.
  - `Get("llm")` returns an error containing `unknown task_type`.

  Evidence: test assertions on the prompts the fake runner captured and on the returned error.
- [ ] **AC3** Wired phase chain, asserted on the written task in each case. Every case except (j) enters at `phase: planning` with no `## Plan` in the input. Cases (b), (d), (e) and (f) pin the execution phase: the fake runner captured exactly two prompts, the second containing `scripts/prometheus-query.sh`, and the written task has `phase: execution`.
  - (a) **Happy path.** The fake runner returns a plan, then a `High` verdict that passes every gate rule and whose line exists in a fixture clone, then a passing review. The input task carries a stale `## Failure` from a prior attempt. The written task ends `status: completed`, `phase: done`, contains `## Plan`, `## Verdict` and `## Review` in that order, and has no `## Failure`.
  - (b) **Medium verdict.** The verdict is valid with `confidence: Medium`, and its cited line exists in the fixture clone. The written task has `assignee: ""`, `phase: execution`, and a `## Failure` containing `below High`. `## Verdict` is present and `## Review` is absent (ai_review never ran).
  - (c) **Missing line.** The verdict is `High`, but its cited line exceeds the fixture file's length; the review passes. The written task has `assignee: ""`, a `## Failure` containing `beyond end of file`, the file's line count, and `evidence:` followed by each line of the fixture's evidence prefixed `> `. It has `phase: ai_review`, **no `## Verdict`**, and is not `status: completed`.
  - (d) **defect_id mismatch.** The verdict's `defect_id` differs from the frontmatter's. The written task has `assignee: ""`, `phase: execution`, and **no `## Verdict`**: the removal happens on the task's parsed sections, not on a string copy.
  - (e) **Malformed verdict (rule a).** The execution response contains no yaml block. The written task has `phase: execution`, the assignee kept (`defect-diagnoser-agent`), a `## Failure`, and **no `## Verdict`**.
  - (f) **Model escalates at execution.** The second fake response is `{"status":"needs_input","message":"…"}`. The written task has `phase: execution` and `assignee: ""`. This pins the phase cursor on entry, not only in the gate's reject branches.
  - (g) **Heading rule, planning.** The planning response contains a column-0 `## Verdict` line. The result is `failed` naming `## Plan`; the fake runner captured exactly one prompt; the written task has `phase: planning`, no `## Plan` section, and `grep -c '^## Verdict'` = 0. The `## Failure` body has no line starting with `#`.
  - (h) **Heading rule, execution.** The execution response carries a valid verdict block plus a column-0 `## Verdict` line. The result is `failed` naming `## Verdict`, and the written task has `phase: execution` and `grep -c '^## Verdict'` = 0.
  - (i) **Heading rule, ai_review.** The review response contains a column-0 `## Verdict` line. The result is `failed` naming `## Review`, and the written task has `phase: ai_review`, no `## Review`, and exactly one `^## Verdict` line (the gated verdict).
  - (j) **Entry at ai_review.** The task enters at `phase: ai_review` with a pre-written, otherwise valid `confidence: Medium` verdict whose line exists in the fixture clone, and the fake runner returns a passing review. The written task is not `status: completed`: it has `assignee: ""`, `phase: ai_review`, and a `## Failure` containing `below High`.

  Evidence: written-task content asserted in tests.
- [ ] **AC4** Gate `failed` rules (rules a, b, d and e in `docs/verdict-gate.md`). The table has one entry each for:
  - a missing section, no yaml block, and unparseable yaml
  - a `## Verdict` body containing a column-0 `## Notes` line: `failed` naming `## Verdict` (the model-output heading rule, which runs before rule a)
  - each of `defect_id`, `service`, `repo`, `file:line` and `confidence` given as a yaml block scalar spanning two lines: `failed` naming the field, and the message does not contain the value. The `confidence` entry's block scalar contains a line `## Review`; the written task has `grep -c '^## Review'` equal to 0
  - each of the seven non-confidence fields missing, including `High` with an empty `file:line`
  - `confidence` of `high`, `Certain` or empty
  - `file:line` of `pkg/x.go`, `pkg/x.go:0`, `pkg/x.go:-3`, `pkg/x.go:abc`, `/abs/x.go:3` and `a/../x.go:3`

  Every entry yields `failed`, with a message naming the field (or `file:line`), and the delivered body has no `## Verdict`. The `/abs/x.go:3` entry also asserts that the string `/abs/x.go:3` is absent from the delivered body. `pkg/x/y.go:42`, with everything else valid, passes rule e. Evidence: test table assertions.
- [ ] **AC5** Gate `needs_input` rules (rules c and f–j):
  - `Low` with an empty `file:line`, and `Medium` with an empty `file:line`: each yields `needs_input` whose message contains `no line cited; evidence:` followed by every line of the fixture's `evidence`, each prefixed `> `. No `## Verdict` in the body.
  - **Heading injection.** Rule c with an `evidence` containing a line `## Verdict` and a line of three backticks. In the written task, `grep -c '^## Verdict'` is 0, the set of column-0 `#` lines is exactly the headings in the input task file plus those the framework wrote (no injected heading), and the column-0 triple-backtick lines are exactly those in the input task file plus the two lines of the framework's `## Failure` fence.
  - `evidence` naming neither `scripts/vault-` nor `scripts/prometheus-query.sh`: `needs_input` containing `evidence cites no live read`. No `## Verdict`.
  - Frontmatter without `defect_id`: `needs_input`, no `## Verdict`. A verdict `defect_id` that differs from the frontmatter's: `needs_input` naming both, no `## Verdict`.
  - Service `Agent-Task-Controller` against mapping key `service_repo.agent-task-controller`: `needs_input` containing `does not resolve` (the match is exact and case-sensitive). No `## Verdict`.
  - A `repo` different from the mapped repo: `needs_input` naming both, no `## Verdict`.
  - `Low` with a valid line that exists in the fixture clone: `needs_input` containing `below High`, with `## Verdict` kept.
  - `Low` with a line past the end of the fixture file (rule j's line guard): `needs_input` containing `beyond end of file` and the quoted evidence. No `## Verdict`.

  Evidence: test table assertions on status, message and body.
- [ ] **AC6** Gate precedence and passthrough:
  - A verdict missing `root_cause` whose `file:line` is also `/abs/x.go:3` yields exactly one reported rule, `root_cause` (rule d fires before rule e).
  - A `Medium` verdict with an empty `file:line` that is also missing `root_cause` yields `needs_input` containing `no line cited`, not `failed` (rule c fires before rule d).
  - A model envelope `{"status":"needs_input",...}` and a model envelope `{"status":"failed",...}` each pass through with their status unchanged, and the delivered message contains each line of the model's message prefixed `> `.
  - A model `needs_input` envelope whose message contains a line `## Verdict` and a line of three backticks: the written task has `grep -c '^## Verdict'` = 0, and every line of the `## Failure` message starts with `> ` (docs/verdict-gate.md § Failure messages).

  Evidence: test assertions.
- [ ] **AC7** ai_review line guard, run against a fixture clone. A `## Verdict` that is missing, or whose yaml does not parse, never reaches the guard: at ai_review it yields `failed` from rule a, with no `## Verdict` in the delivered body. Each failing row below yields `needs_input` with no `## Verdict` in the delivered body, plus `evidence:` followed by the fixture's evidence lines, each prefixed `> `:
  - the path is absent
  - the cited line is greater than the file's line count
  - the cited file is a symlink that resolves outside the clone
  - the cited file sits in a directory that is itself a symlink resolving outside the clone
  - passing row: a line within the file yields `done`, `phase: done`, with `## Verdict` kept

  Evidence: test entries.
- [ ] **AC8** Prompt content. The prompts live in `pkg/prompts/*.md` and are embedded; their built instructions are checked for:
  - planning: contains `scripts/repo-clone.sh` and `service_repo.`
  - execution: contains `scripts/vault-read.sh`, `scripts/vault-list.sh`, `scripts/prometheus-query.sh` and all eight verdict keys
  - ai_review: contains `## Observation` and `file:line`
  - no phase contains `You are a task execution agent`
  - each of the three phases contains the literal sentence `Never start a line of your response with #.`

  Negative: `grep -ciE 'pod[- ]?logs' pkg/prompts/*.md` prints `:0` for every file and exits 1. Evidence: string assertions and grep output.
- [ ] **AC9** Docs and scenario:
  - `grep -c 'service_repo\.' README.md` is at least 1, and `grep -c 'docs/verdict-gate.md' README.md` is at least 1.
  - `grep -rn 'service_repo\.' k8s/` returns 0 lines (exit 1).
  - `sed -n '/^1\. \*\*Create the task/,/^2\. /p' scenarios/001-diagnose-one-observed-defect.md | grep -c defect_id` is at least 1.
  - `sed -n '/^\*\*Setup\*\*/,/^\*\*Acceptance\*\*/p' scenarios/001-diagnose-one-observed-defect.md | grep -c 'service_repo\.'` is at least 1.
  - `sed -n '/^## Unreleased/,/^## v/p' CHANGELOG.md | grep -ci 'diagnos'` is at least 1.
  - `grep -ciE 'pod[- ]?logs' README.md` prints `0`.

  Evidence: grep output and exit codes.

Operator-executable:

- [ ] **AC10** Rung-1 live run (commands under Verification). The fixture task file has:
  - frontmatter `task_type: defect-diagnose`, `phase: planning`, `status: in_progress`, `assignee: defect-diagnoser-agent` and `defect_id: rung1-<date>`
  - an `## Observation` that names one real dev service and contains no root cause, no component and no `file:line`

  **This spec passes on either outcome below. Only outcome (a) contributes to the goal's SC2.**
  - (a) The file ends `status: completed`, `phase: done`, with no `## Failure`. `## Verdict` has `confidence: High` and `defect_id: rung1-<date>`. `sed -n '<line>p' agent/repos/<repo>/<path>` prints a non-empty line. `evidence` contains `scripts/vault-` or `scripts/prometheus-query.sh`, plus quoted output.
  - (b) The file ends `assignee: ""`, and execution was reached:
    - `## Plan` carries a line matching `head_sha[:=]`.
    - `## Failure`, or the retained `## Verdict`'s `evidence`, cites `scripts/vault-` or `scripts/prometheus-query.sh` and what it returned.
    - A `does not resolve` escalation fails this branch, because the service was mapped.
    - A line-guard escalation (`beyond end of file`, `not found in clone`, `resolves outside the clone`) passes this branch: its `## Failure` quotes the verdict's evidence, which carries the script citation.

  In both cases, stderr at `-v=2` has a `cmd.Env =` line that includes `GIT_REST_URL=` and `PROMETHEUS_URL=`. Evidence: task-file content plus the log line.

**Scenario coverage:** no new scenario. Scenario 001 already covers the deployed journey. This spec only changes two parts of it:
- its Setup, which gains a `service_repo.` entry in the applied Config's `ENV_CONTEXT`
- its Step 1, which is rewritten: the operator writes the task file directly (heredoc into `~/Documents/Obsidian/Personal/25 Tasks/Defect <slug>.md`) with `defect_id` in its frontmatter, because `vault-cli task create` does not exist. The intake-surface task will later replace this step with the one-gesture command. Both edits are applied to `scenarios/001-diagnose-one-observed-defect.md` alongside this spec; AC9 guards them.

The scenario's "no `## Failure`" pass criterion still holds after a prior failed attempt, because a completed task has any stale `## Failure` removed (DB5, AC3a).

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` (AC1)
- `make test`: dispatch, wired phase chain, gate tables, precedence, line guard and prompt content (AC2–AC8)
- the greps of AC8 (negative) and AC9, with the stated counts and exit codes

### Operator-executable (host, after both specs merge; rung-1)

git-rest (`vault-obsidian-personal`, namespace `dev`) and Prometheus (`monitoring/prometheus`, ClusterIP 9090, confirmed on nukedev 2026-09-27) are ClusterIP services with no host route. Rung-1 therefore needs two port-forwards. The **operator runs** them and approves them explicitly at run time; agent sessions never run port-forwards:

```
kubectlnukedev -n dev port-forward svc/vault-obsidian-personal 19090:9090
kubectlnukedev -n monitoring port-forward svc/prometheus 19091:9090
```

Then, with the router token exported as `ANTHROPIC_AUTH_TOKEN` (never passed as a flag):

```
go run ./cmd/run-task \
  -task-file=/tmp/defect-diagnose-rung1.md \
  -branch=dev -phase=planning \
  -agent-dir="$(pwd)/agent" \
  -claude-config-dir="$HOME/.claude-agent" \
  -allowed-tools='<ALLOWED_TOOLS copied verbatim from k8s/defect-diagnoser-agent-config.yaml>' \
  -env-context='service_repo.<service>=<owner>/<repo>' \
  -claude-env='GIT_REST_URL=http://127.0.0.1:19090,PROMETHEUS_URL=http://127.0.0.1:19091' \
  -anthropic-base-url=http://127.0.0.1:8788 \
  -anthropic-model=MiniMax-M2.7-highspeed \
  -v=2 2> /tmp/defect-diagnose-rung1.log
grep 'cmd.Env =' /tmp/defect-diagnose-rung1.log
```

Check AC10 (a) or (b) against `/tmp/defect-diagnose-rung1.md`. Then clean up with `chmod -R u+w agent/repos && rm -rf agent/repos`, and stop both port-forwards.

## Desired Behavior

1. **Dispatch and phase chain.** `defect-diagnose` routes to the diagnoser. `healthcheck` and `oauth-probe` still route to the liveness agent. `llm` is no longer accepted. Each diagnoser phase has its own instructions in `pkg/prompts/*.md`, and the phases walk in-process as long as each one succeeds:
   - planning writes `## Plan` and advances to `execution`
   - execution writes `## Verdict` and advances to `ai_review`
   - ai_review writes `## Review` and advances to `done`

   After each model step, before anything else, the binary applies the model-output heading rule (`docs/verdict-gate.md`). If `## Plan`, `## Verdict` or `## Review` contains any column-0 `#` line, the phase yields `failed` naming the section, never echoes the line, and removes that section. Each phase records itself as the task's `phase` on entry, so an escalation leaves `phase` at the phase that escalated. The framework's idempotency is kept: on re-dispatch, a phase whose section exists without a failure marker is skipped.
2. **Planning: resolve and clone.** Planning resolves the service named in `## Observation` only by an exact, case-sensitive match against the prompt's `service_repo.<service>` entries. The operator supplies these through `ENV_CONTEXT` (see `docs/verdict-gate.md` § mapping input).
   - **On a match:** planning runs `scripts/repo-clone.sh clone <repo>`, and `## Plan` records `service`, `repo`, `clone_path` and `head_sha`.
   - **On no match, or when the clone fails:** planning returns `needs_input` naming the service or the clone error. It never substitutes a repo.

   README documents the mapping input and links `docs/verdict-gate.md`. Scenario 001's Setup requires one `service_repo.<service>` entry in the applied Config's `ENV_CONTEXT`, and its Step 1 writes the task file directly with `defect_id` in frontmatter (both applied as a direct doc edit alongside this spec; AC9 guards them).
3. **Execution: observe, localise, emit the verdict.** Execution observes in the contract's order:
   1. task files, via `scripts/vault-read.sh` / `scripts/vault-list.sh`
   2. metrics, via `scripts/prometheus-query.sh`

   It then localises the cause in the clone with Read/Grep/Glob and emits a fenced yaml `## Verdict`, followed by the JSON envelope. The verdict uses the template in `docs/verdict-gate.md`, with `defect_id` copied from frontmatter. `evidence` quotes the exact script call and its output. A cause supported only by source gets `confidence` below `High`. If the clone is absent at execution (a pod restart after planning), execution re-clones before localising. Every phase's instructions (planning, execution, ai_review) include the sentence `Never start a line of your response with #.`, because the step supplies the heading. When no line can be cited, execution returns a `needs_input` envelope with no verdict, and its message lists each script call tried and what it returned.
4. **Deterministic verdict gate.** After execution reports done, the binary applies rules a–k of `docs/verdict-gate.md` in order, and the first rule that fires decides the outcome:
   - malformed or invalid → `failed`
   - no line cited at Medium/Low (message quotes the verdict's `evidence`), evidence naming no live read script, `defect_id` mismatch, unmapped service, or repo mismatch → `needs_input`
   - Medium/Low with a cited line → the line guard runs first; if it fails → `needs_input` with the verdict removed and the evidence quoted; if it passes → `needs_input`, with the verdict kept
   - otherwise → advance to `ai_review`

   Every rejected outcome except "Medium/Low whose line passes the guard" removes `## Verdict` from the task's parsed sections before delivery. In the written task every line of every `## Failure` message, including quoted `evidence`, is prefixed `> ` exactly once (`docs/verdict-gate.md` § Failure messages; § Quoting evidence emits the evidence raw). Model-returned `needs_input` / `failed` envelopes pass through with their status unchanged; their message lines are prefixed the same way.
5. **ai_review: re-check, line guard, clean completion.** The ai_review prompt re-checks the verdict against `## Observation`:
   - the service matches
   - `root_cause` explains the observed symptom
   - `evidence` quotes a script call and its output
   - the cited line's content supports `root_cause`

   ai_review re-clones when the clone is absent, and records in `## Review` any `head_sha` difference from `## Plan`. A failed check returns `needs_input` naming the check.

   After a passing model review, the binary applies the heading rule to `## Review`. It then re-runs gate rules a–j on the verdict, so a Job entering at `phase: ai_review` cannot complete an ungated verdict. Finally it applies the line guard in `docs/verdict-gate.md`: the path must resolve inside the clone (symlinks followed on every component), the file must exist, and the file must have at least `<line>` lines. Any failure yields `needs_input` with `## Verdict` removed and a message of `<reason>; evidence:` plus the quoted evidence. On a pass the task completes, and any stale `## Failure` from an earlier attempt is removed from the delivered body.

## Constraints

- **Contract** ([[Defect Lane Task Chain]], amended 2026-09-27; copied into `docs/verdict-gate.md` because the container cannot read the vault). The following are fixed: the `## Verdict` field names and their order; the `confidence` values `High | Medium | Low`; and the escalation split (no line → no Verdict; line + Medium/Low → Verdict kept; stage 2 only from High). `## Observation` is read and never rewritten.
- **Gate rules** are frozen in `docs/verdict-gate.md`. A rule change updates that page in the same change.
- **Frozen envelope:** the final-response JSON `{status, message, files}` that the delivery layer parses is unchanged.
- **Escalation mechanics** are the framework's. `needs_input` clears `assignee`, sets `previous_assignee` and writes `## Failure`. `failed` keeps the assignee for the trigger-count retry.
- **Phase cursor.** The diagnoser writes no frontmatter fields of its own except `phase`: on entering each phase it sets the parsed frontmatter `phase` to that phase. bborbe/agent v0.89.0 parses the frontmatter once per Job and a needs_input/failed delivery keeps that parsed phase, so without this an escalation after an in-process advance writes back the Job's entry phase.
- `healthcheck` / `oauth-probe` routing and the liveness agent are unchanged.
- There is no change to the `github.com/bborbe/agent` module version or code.
- The scaffold's `BuildInstructions` length/name tests are replaced by per-phase assertions. All other existing tests keep passing.
- **Depends on** spec `read-only-observation-toolkit`, which provides:
  - the four scripts
  - `GIT_REST_URL` / `PROMETHEUS_URL` delivered via `CLAUDE_ENV`
  - the exact `ALLOWED_TOOLS` string
  - the three-way consistency test that covers this spec's prompts
- **Assumptions:**
  - A Job walks all three phases in one pod when each succeeds (the framework's in-process phase walk).
  - The rung-1 target repo clones without credentials.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| No read path shows the misbehaviour | Execution returns `needs_input` with no Verdict, listing each read and its result | `## Failure` text | Operator adds context to the task and re-assigns: `vault-cli task set "<title>" assignee defect-diagnoser-agent` |
| Model renames a key or writes `confidence: high` (schema drift) | Gate returns `failed` naming the field; `## Verdict` removed; retries follow the trigger-count budget, then escalate | `## Failure` names the field | Tighten the execution prompt; the gate stays strict |
| Model invents a repo or `defect_id`, or cites no live read | Gate returns `needs_input` naming the values; Verdict removed | `## Failure` | Operator corrects the mapping entry or the task, then re-assigns |
| Service not in the mapping (or its case differs) | Planning, with the gate as backstop, returns `needs_input` `does not resolve` | `## Failure` | Operator adds an exact `service_repo.<service>` entry to the Config's `ENV_CONTEXT` (a separate, gated change) and re-assigns |
| Cited line fails the line guard (file moved, line beyond EOF, symlink escape), after ai_review or inside rule j | `needs_input`; Verdict removed; evidence quoted in `## Failure`; task stays at `phase: ai_review` (or `execution` for rule j) | `## Failure` names the path and the file's line count | `vault-cli task set "<title>" phase execution`, then `vault-cli task set "<title>" assignee defect-diagnoser-agent` |
| Clone fails (private repo, disk full) (external unavailability / resource exhaustion) | Planning returns `needs_input` with the script's stderr | `## Failure` contains `git clone failed` | Disk full: re-assign to get a fresh pod (`vault-cli task set "<title>" assignee defect-diagnoser-agent`). Private repo: operator diagnoses by hand and records the result in the task |
| git-rest / Prometheus unreachable | Script errors surface in the model's escalation | `## Failure` names the script and status | Restore the endpoint, then re-assign |
| Pod dies between phases (partial progress) | Next Job starts at the persisted `phase`; ai_review re-clones if needed and notes `head_sha` drift; the line guard checks the clone present at that time | `## Review` notes drift | None needed: the guard escalates if the line no longer resolves |
| Stale `## Failure` from a prior attempt survives into a later run (the framework never removes it) | While it is present, `ShouldRun` re-runs every phase. On successful completion the diagnoser removes it, so a `completed` task carries no `## Failure`. On another escalation the framework replaces it with the new reason | AC3a; scenario 001's "no `## Failure`" criterion | None needed; the task file's git history keeps the earlier failure |
| LLM router rate-limits or times out (rate limiting) | Runner error → `failed`, assignee kept, retried by trigger count | `## Failure` `claude run failed` | Automatic; escalates at the cap |
| Clock skew | Not applicable: no time-dependent logic, and Prometheus evaluates instant queries at its own now | — | — |
| Two local rung-1 runs share `agent/repos` | The second clone replaces the first mid-read | Visible to the operator | Run one at a time. In the cluster, `maxConcurrentJobs: 1` and per-pod filesystems prevent it |

## Security / Abuse Cases

- **Untrusted input.** `## Observation` (anyone with vault write access can edit it), cloned repo content and script output all enter the prompt as untrusted text. Injection can at most steer calls among the four read-only scripts plus Read/Grep/Glob (the toolkit spec's allowlist), and none of these can write. Heading injection into the task file is contained by the measures in docs/verdict-gate.md: the model-output heading rule, `> `-prefixing of every `## Failure` message line (binary-generated and model passthrough alike), `%q` echoes, and that read-only allowlist. The root cause is a bborbe/agent framework defect, reported upstream and out of scope here: `Markdown.Marshal` writes model output verbatim, and the section splitter ignores code fences.
- **Clone target anchoring.** The cloned repo is the operator's mapping entry, matched exactly, and the gate enforces `repo` equality. A hostile observation cannot redirect the clone. The clone script itself rejects traversal and symlink escapes (toolkit spec).
- **Path escape in the verdict.** A `file:line` with a leading `/` or a `..` segment is rejected before use. The line guard resolves symlinks on every path component and refuses any path outside the clone, so a verdict cannot make the binary stat arbitrary host files.
- **Fabricated identity and evidence.** `defect_id` must equal the task's frontmatter value, so a verdict cannot relabel itself onto another defect chain. `evidence` must name a live read script; whether its output supports the cause is checked by the ai_review model (DB5), not by the binary.
- **Echoed-value injection.** A `defect_id`, `service`, `repo`, `file:line` or `confidence` value containing a newline is rejected as `failed`, naming the field without echoing the value. Every other value the gate echoes (rules b, g, h, i and the line-guard path) is rendered on one line with Go `%q` quoting, so no echoed value can start a column-0 line. Model-written sections (`## Plan`, `## Verdict`, `## Review`) with any column-0 `#` line are rejected by the one general heading rule (AC3 g–i).
- **Heading injection via quoted evidence.** The framework's section splitter ignores code fences, and `evidence` is model-written text derived from untrusted reads. Every quoted evidence line is prefixed `> `, so a line such as `## Verdict` or a line of three backticks cannot create a fake section or break out of the `## Failure` fence (AC5 heading-injection row).
- **Hang / retry-forever.** `failed` retries are bounded by the existing trigger-count cap, and `needs_input` never auto-retries.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Model-output heading rule; verdict gate (rules a–k after execution, a–j again at ai_review; removing sections from the parsed sections); line guard; stale-Failure removal. All as step wrappers, with table tests | 4, 5 (binary side) | 4, 5, 6, 7 | toolkit spec merged |
| 2 | Per-phase prompts in `pkg/prompts/*.md` that replace the generic scaffold prompt, with content tests | 2, 3, 5 (prompt side) | 8 | toolkit spec merged |
| 3 | Dispatch (`defect-diagnose` in, `llm` out); phase-chain wiring with the gate wrappers; a provider seam for a fake runner; wired phase-chain tests; README mapping section; CHANGELOG; AC9 greps (scenario 001 edits already applied) | 1, 2 (docs) | 2, 3, 9 | prompts 1, 2 |
| — | `make precommit` on every prompt; rung-1 run by the operator after both specs merge | — | 1, 10 | all |

Rationale: the gate (prompt 1) and the prompts (prompt 2) are independent once the toolkit spec's scripts exist. Prompt 3 wires both into the provider and proves the wiring end to end, so it lands last. Nothing in this spec may start before `read-only-observation-toolkit` is merged, because the prompts call its scripts and its consistency test must see them.

## Do-Nothing Option

A deployed agent would fail every `defect-diagnose` task with `unknown task_type`. The operator would keep diagnosing each observed defect by hand, which is exactly the cost goal SC2 exists to remove. The Sentry analyzer is no substitute: its schema, prompts and required Sentry token are Sentry-specific, and it has no live-observation path. Doing nothing is not acceptable while the goal stands.
