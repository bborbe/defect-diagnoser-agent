---
status: draft
created: 2026-09-27
---

## Summary

- Give the defect diagnoser its four read-only "eyes" on the running system: two scripts that read task files from the vault, one that queries Prometheus, and one that makes a read-only clone of a source repo.
- Make the vault and Prometheus addresses actually reach the model. Today they sit in the Config as plain pod env, which the runner silently strips; the fix routes them through the existing `CLAUDE_ENV` passthrough instead of new code.
- Line up the Config's tool allowlist, the container image, the guardrails and the README with what really exists, and drop the pod-log tool, which cannot work on the current platform.
- This spec adds no diagnosis logic. It is the foundation that spec `diagnose-observed-defect-with-verdict-gate` builds on.

## Problem

The scaffolded `defect-diagnoser-agent` Config allowlists five scripts, and none of them exists. One (`pod-logs.sh`) cannot work at all, because the Config CRD has no `serviceAccountName` field and an agent Job therefore has no Kubernetes read access.

The Config sets `GIT_REST_URL` and `PROMETHEUS_URL` as plain pod env. The Claude runner passes only a fixed allowlist of pod variables into the model's subprocess, so even correct scripts would fail with "variable is required" while `kubectl describe pod` shows the variable set.

The image does not install `git`, so no clone can work. And the only existing clone script (in `agent-sentry-issue-analyzer`) accepts `..` as an owner or repo name. A crafted reference therefore escapes the clone directory before the script runs `rm -rf` and `chmod -R`.

Without a safe, tested toolkit, the diagnoser either cannot observe anything or observes through tools that can be steered into writing or deleting.

## Goal

The agent image carries four executable read-only scripts in `agent/scripts/`, each behaving exactly as `docs/observation-scripts.md` specifies. No script can send anything but a GET or write outside its own clone directory. Invalid arguments are rejected before any network call, `git` invocation or filesystem change, and HTTP failures surface their status.

The Config delivers `GIT_REST_URL` and `PROMETHEUS_URL` to the Claude subprocess through `CLAUDE_ENV`. The allowlist, the scripts on disk, and the prompt and guardrail prose agree in every direction. A test enforces that agreement, so an allowlisted script that does not exist, or a script that is not allowlisted, fails `make test`.

## Non-goals

- No pod-log / event reads or script. They wait on the Config CRD gaining a service-account field (NEXT-DIRECTIONS v2).
- No diagnosis prompts, task-type dispatch or verdict validation. Those belong to spec `diagnose-observed-defect-with-verdict-gate`, which depends on this one.
- No new Go env plumbing, flags or helper for the two URLs. The existing `CLAUDE_ENV` passthrough carries them.
- No private-repo clone auth (`GIT_CLONE_TOKEN` / GitHub App). v1 clones only repos reachable without credentials.
- No git-rest gateway auth (`GATEWAY_SECRET`).
- No Prometheus range queries, and no Prometheus path other than `/api/v1/query`.
- No `repo-clone.sh log` subcommand. Nothing in v1 consumes regression history.
- No `kubectl apply`, deploy, secret manifest or RBAC. The Config file stays NOT APPLIED.
- No change to the `github.com/bborbe/agent` module.
- Do NOT add a clone-directory, clone-depth, timeout or output-cap env knob. These are invariants; if a future consumer demands variation, that is a separate spec.

## Acceptance Criteria

Container-executable. Every test named below runs under `make test`. Script tests run as a non-root user. On uid 0 they skip the permission-restoration assertion, with a skip message naming that blind spot (root removes read-only trees without restoring permissions). Mode-bit assertions hold under any uid.

- [ ] **AC1** `make precommit` exits 0 at the repo root. Evidence: exit code.
- [ ] **AC2** `prometheus-query.sh`, run against a recording HTTP test server:
  - One PromQL argument sends exactly one `GET` to path `/api/v1/query` whose `query` parameter equals the argument. The response body goes to stdout and the exit code is 0.
  - An argument starting with `/`, or containing `://`, exits 2 and the server receives zero requests.
  - Zero arguments, or two or more, exit non-zero with zero requests.
  - With `PROMETHEUS_URL` unset, it exits non-zero and stderr contains `PROMETHEUS_URL`.
  - A `500` response exits non-zero with `500` on stderr; a `401` response exits non-zero with `401` on stderr.
  - A 100 KiB body produces exactly 65536 bytes on stdout, `truncated` on stderr, and exit 0.

  Evidence: exit codes, recorded requests (method, path, query) and byte counts, asserted in tests.
- [ ] **AC3** `vault-read.sh` and `vault-list.sh`, run against a recording HTTP test server:
  - Every request either script sends is a `GET` under `/api/v1/files/`.
  - `vault-read.sh` rejects each of these with exit 2 and zero requests: `/etc/passwd`, `../b.md`, `a/../b.md`, `a/..`, `a.md?x=1`, `a.md#x`, `http://evil/x`.
  - `vault-read.sh "25 Tasks/x y.md"` requests the path with the space percent-encoded.
  - For **each** script: zero arguments, and two arguments, each exit non-zero with zero requests (contract: exactly one argument).
  - `vault-list.sh "* Tasks/*.md"` sends its argument as the `glob` query parameter.
  - For **each** script: with `GIT_REST_URL` unset, it exits non-zero and stderr contains `GIT_REST_URL`.
  - For **each** script: a `500` response and a `401` response each exit non-zero with the status on stderr.
  - For **each** script: a 100 KiB body produces exactly 65536 bytes on stdout, `truncated` on stderr, and exit 0.

  Evidence: exit codes and recorded requests, asserted in tests.
- [ ] **AC4** Every HTTP script sets a 30 s timeout. `grep -c -- '--max-time 30' agent/scripts/vault-read.sh agent/scripts/vault-list.sh agent/scripts/prometheus-query.sh` prints a count of at least 1 for each file. Evidence: grep output.
- [ ] **AC5** `repo-clone.sh` rejects bad references. Setup: a stub `git` first on `PATH` logs every invocation, and a snapshot of path + mode bits for everything outside `<agent dir>/repos/` is taken before and after each case.
  - Each of these exits 2: `evil`, `a;b/c`, `http://evil.example/x/y`, `file:///etc/x`, `../x`, `x/..`, `x/.`, `./.`, `../..`, `https://github.com/../x`, `git@github.com:x/...git`.
  - So does `clone evil/x` when `<agent dir>/repos/evil` is a pre-planted symlink to a directory outside `repos/` (the defense-in-depth resolve check).
  - Every rejection leaves zero stub-git invocations and identical before/after snapshots.

  Evidence: exit codes, an empty invocation log, and identical snapshots, asserted in tests.
- [ ] **AC6** `repo-clone.sh` success and error paths, with the stub `git`:
  - `clone bborbe/foo` produces exactly one `clone` invocation: `git clone https://github.com/bborbe/foo <agent dir>/repos/bborbe/foo`, with no `--recurse-submodules`. Stdout has an absolute `clone_path=` inside `<agent dir>/repos/`, plus `head_sha=` and `default_branch=` lines. `find <clone> -perm -u+w` prints nothing.
  - `clone bborbe/.github` is accepted, with exactly one `clone` invocation.
  - A second `clone bborbe/foo` exits 0. The stub log then shows 2 `clone` invocations, and the stub's first-call marker file is gone (only the second-call marker remains).
  - A stub `git clone` that exits 128 makes the script exit non-zero, with `git clone failed` on stderr.
  - `log x y` exits 2 with usage on stderr.

  Evidence: exit codes, invocation log, marker files and `find` output, asserted in tests.
- [ ] **AC7** Env forwarding through `CLAUDE_ENV`:
  - `grep -nE '^\s+CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"$' k8s/defect-diagnoser-agent-config.yaml` returns exactly 1 line. This is anchored so a value that exists only in a YAML comment does not pass.
  - `grep -nE 'CLAUDE_ENV: .*GIT_REST_URL=http://vault-obsidian-personal:9090' k8s/defect-diagnoser-agent-config.yaml` returns 1 line.
  - `grep -nE 'CLAUDE_ENV: .*PROMETHEUS_URL=http://prometheus.monitoring:9090' k8s/defect-diagnoser-agent-config.yaml` returns 1 line.
  - `grep -nE '^\s+(GIT_REST_URL|PROMETHEUS_URL):' k8s/defect-diagnoser-agent-config.yaml` returns 0 lines (exit 1): the dead plain-env keys are gone.

  Evidence: grep output and exit codes.
- [ ] **AC8** Allowlist, scripts and prose agree, checked by a test that parses `ALLOWED_TOOLS` from `k8s/defect-diagnoser-agent-config.yaml`:
  - Every `Bash(scripts/<name>:*)` entry names an executable file under `agent/scripts/`.
  - Every executable file under `agent/scripts/` has a `Bash(scripts/<name>:*)` entry.
  - Every `scripts/<name>.sh` mentioned in `pkg/prompts/*.md` or `agent/.claude/CLAUDE.md` has an entry.
  - Exact string: `grep -c 'ALLOWED_TOOLS: "Read,Grep,Glob,Bash(scripts/vault-read.sh:\*),Bash(scripts/vault-list.sh:\*),Bash(scripts/prometheus-query.sh:\*),Bash(scripts/repo-clone.sh:\*)"' k8s/defect-diagnoser-agent-config.yaml` prints `1`.
  - Negative: `grep -ciE 'pod[- ]?logs' k8s/defect-diagnoser-agent-config.yaml agent/.claude/CLAUDE.md README.md pkg/prompts/*.md` prints `:0` for every file and exits 1.

  Evidence: test assertions, grep output and exit code.
- [ ] **AC9** Read-only by construction (negative). Continuation lines are joined first, and only `curl` invocations are inspected:

  ```
  sed -e ':a' -e '/\\$/N; s/\\\n//; ta' agent/scripts/*.sh | grep -E '(^|[^A-Za-z_])curl ' | grep -cE -- '(-X|--request|--upload-file| -T |--data([^-]|$)|--data-(binary|raw|ascii)|--json| -d | -F |--form)'
  ```

  This prints `0`. Separately, `grep -nE 'git (push|commit|remote)' agent/scripts/*.sh` returns 0 lines (exit 1). Evidence: output and exit codes.
- [ ] **AC10** No env knobs (negative):
  - `grep -nE 'REPO_CLONE_DIR|GIT_CLONE_DEPTH|GIT_CLONE_TOKEN|GATEWAY_SECRET' agent/scripts/*.sh` returns 0 lines.
  - `grep -nE '\$\{[A-Z_]+:-' agent/scripts/*.sh` returns 0 lines.
  - Both exit 1.

  Evidence: empty output.
- [ ] **AC11** Image, hygiene and docs:
  - `grep -nE 'apk .*add.* git( |$)' Dockerfile` returns at least 1 line.
  - `grep -n '^/agent/repos/' .gitignore` returns 1 line.
  - `grep -c 'docs/observation-scripts.md' README.md` returns at least 1.
  - `grep -cE 'CLAUDE_ENV.*(GIT_REST_URL|PROMETHEUS_URL)' README.md` returns at least 1. The plain `CLAUDE_ENV` row already in the Env Vars table does not satisfy this.
  - `grep -c 'task files → Prometheus' README.md` returns 1, so the "Observes" line is rewritten, not deleted. This is a preservation guard and passes on today's README by design; the AC8 negative grep and the deferred-wording check force the actual rewrite.
  - `grep -c 'Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2'` returns at least 1 on each of `k8s/defect-diagnoser-agent-config.yaml` and `README.md`.
  - `grep -c 'ClaudeRunnerConfig.Env' README.md` returns 0 (today: 1, README.md:68). This removes the Go-threading advice however it is capitalised.
  - `sed -n '/^### Claude subprocess env allowlist/,/^## /p' README.md | grep -c 'CLAUDE_ENV'` returns at least 1, so the paragraph keeps its "pod env is stripped" warning and names the channel. Deleting the section fails this check. README's "Claude subprocess env allowlist" paragraph names `CLAUDE_ENV` as the channel for custom env, not a Go change.
  - `grep -n '(task files, Prometheus)' agent/.claude/CLAUDE.md` returns 1 line.
  - `sed -n '/^## Unreleased/,/^## v/p' CHANGELOG.md | grep -ci 'observation scripts'` returns at least 1.

  Evidence: grep output.

Operator-executable:

- [ ] **AC12** Env reaches the subprocess. Run from `cmd/run-task`, with the router token exported as `ANTHROPIC_AUTH_TOKEN`:

  ```
  cp dummy-task.md /tmp/toolkit-ac12-task.md
  export CLAUDE_ENV="$(sed -nE 's/^[[:space:]]+CLAUDE_ENV: "(.*)"$/\1/p' ../../k8s/defect-diagnoser-agent-config.yaml)"
  go run . -task-file=/tmp/toolkit-ac12-task.md -branch=dev -agent-dir="$(cd ../.. && pwd)/agent" \
    -claude-config-dir="$HOME/.claude-agent" -allowed-tools=Grep,Read \
    -anthropic-base-url=http://127.0.0.1:8788 -anthropic-model=MiniMax-M2.7-highspeed \
    -v=2 2> /tmp/toolkit-ac12.log
  grep 'cmd.Env =' /tmp/toolkit-ac12.log
  ```

  The `cmd.Env =` line includes both `GIT_REST_URL=http://vault-obsidian-personal:9090` and `PROMETHEUS_URL=http://prometheus.monitoring:9090`. That proves the **committed Config string** parses into both keys, through the same `env:"CLAUDE_ENV"` path the pod uses (not the `-claude-env` flag). The task file is a `/tmp` copy, because the file deliverer writes into it. No endpoint needs to be reachable. Evidence: log line.

**Scenario coverage:** no new scenario. Every behavior here is reachable by script tests with a recording HTTP server and a stub `git`. The live journey is covered by scenario 001 and by the diagnosis spec's rung-1.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` (AC1)
- `make test`: script suites and the consistency test (AC2, AC3, AC5, AC6, AC8)
- The greps of AC4, AC7, AC8 (exact string + negative), AC9, AC10 and AC11, each with the stated count or exit code.

### Operator-executable (host, after merge)

- AC12 via `cmd/run-task` with `CLAUDE_ENV` exported from the Config value, then `grep 'cmd.Env ='` on stderr.

**Doc edits that accompany this spec:** `docs/observation-scripts.md` (the authoritative script contract), `docs/verdict-gate.md` (used by the diagnosis spec) and `docs/dod.md`. The last is the `.dark-factory.yaml` `validationPrompt`, copied from `agent-sentry-issue-analyzer` with its dangling references removed. All three are committed together with the specs before approve.

## Desired Behavior

1. **HTTP read scripts.** `vault-read.sh`, `vault-list.sh` and `prometheus-query.sh` exist in `agent/scripts/`, are executable, and behave as `docs/observation-scripts.md` specifies:
   - GET only, with a 30 s timeout.
   - Stdout capped at 65536 bytes, with a `truncated` stderr marker and exit 0 when cut.
   - On an HTTP error, non-zero exit with the status on stderr.
   - Arguments validated before any request; a missing env var named on stderr.

   That page is authoritative. The upstream scripts it names are informational only.
2. **Clone script.** `repo-clone.sh clone <repo>` behaves as `docs/observation-scripts.md` specifies:
   - `.` and `..` are rejected as owner or name.
   - As defense in depth, the resolved destination (symlinks followed) is asserted strictly inside `<agent dir>/repos/` before any `rm -rf` or `chmod`.
   - The clone root is the fixed `repos/` directory beside `scripts/`.
   - Write permission is restored before an existing clone is replaced, and the tree is non-writable after cloning.
   - No submodules, no credentials, no `log` subcommand and no env knobs.
   - A failed `git clone` reports `git clone failed`.
3. **Env forwarding via `CLAUDE_ENV`.** The Config's `spec.env` gains `CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"` and loses the plain `GIT_REST_URL` / `PROMETHEUS_URL` keys, which nothing reads. Both entry points already parse `CLAUDE_ENV` / `-claude-env` into `ClaudeRunnerConfig.Env` (`main.go` and `cmd/run-task/main.go`, `ClaudeEnvRaw`), so no Go code changes. README documents `CLAUDE_ENV` as the path for script env.
4. **Config / image / prose consistency.**
   - The Config's `ALLOWED_TOOLS` becomes exactly `Read,Grep,Glob,Bash(scripts/vault-read.sh:*),Bash(scripts/vault-list.sh:*),Bash(scripts/prometheus-query.sh:*),Bash(scripts/repo-clone.sh:*)`.
   - The Config's comments describe read path 3 as "Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2".
   - The Dockerfile's `apk add` line gains `git`, and `/agent/repos/` is gitignored.
   - `agent/.claude/CLAUDE.md` reads "(task files, Prometheus)".
   - README's "Observes" line becomes "task files → Prometheus", and path 3 is noted as deferred in the same wording as the Config.
   - README's "Claude subprocess env allowlist" paragraph is rewritten: custom env reaches the subprocess through `CLAUDE_ENV` (`KEY=VAL,…`), set on the Config, with no Go change.
   - README links `docs/observation-scripts.md`.
   - A test keeps allowlist, scripts and prose in agreement in all three directions.

## Constraints

- **Frozen tool names.** The four `Bash(scripts/…)` allowlist entries keep their exact names, resolved relative to the agent working directory. The Dockerfile's existing `COPY agent/ /agent/` ships them.
- **Script contracts** are frozen in `docs/observation-scripts.md`. A behavior change updates that page in the same change.
- **No Go change for env.** `CLAUDE_ENV` / `ClaudeEnvRaw` in both entry points stays exactly as it is.
- No change to the `github.com/bborbe/agent` module.
- `k8s/defect-diagnoser-agent-config.yaml` keeps its "NOT APPLIED" header. Only `ALLOWED_TOOLS`, the env keys and the path-3 comments change.
- `agent/.claude/CLAUDE.md` § Forbidden is unchanged. The scripts remain the sole exception to the internal-network rule.
- Existing tests keep passing, and the healthcheck / oauth-probe routing is untouched.
- **Assumptions:**
  - git-rest's glob listing (`GET /api/v1/files/?glob=`) is verified by the Sentry collector's live use.
  - git-rest's single-file GET (`GET /api/v1/files/<path>`) is documented in the git-rest README but not verified by live use.
  - Whether Prometheus `/api/v1/query` requires auth is unverified (contract page, 2026-09-27).
  - The YOLO container has `bash`, `curl` and a non-root user available for the script tests.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| git-rest or Prometheus down / 5xx (external unavailability) | Script exits non-zero with the status on stderr | Model sees stderr; the diagnosis escalation names it | Operator restores the endpoint and re-assigns: `vault-cli task set "<title>" assignee defect-diagnoser-agent` |
| Prometheus requires auth (401/403) | Script exits non-zero with `401`/`403` on stderr | stderr | A follow-up spec adds auth forwarding |
| `CLAUDE_ENV` missing a key in the applied Config (drift) | Script exits with `<VAR> is required` | AC7 grep before merge; at runtime, the `cmd.Env =` log line at `-v=2` lacks the key | Fix the Config's `CLAUDE_ENV` value |
| Crafted repo reference or planted symlink (abuse) | Exit 2 before git runs or the filesystem changes | AC5; stderr `invalid repo reference` | None needed |
| Re-clone over a read-only tree as non-root | Write permission restored, then the old tree is replaced | AC6 second-clone case | None needed |
| Disk full / private repo during clone (resource exhaustion) | Non-zero exit, `git clone failed for <url>` | stderr | Diagnosis escalates. Private-repo auth is a Non-goal |
| Huge response (resource exhaustion) | Stdout cut at 65536 bytes, `truncated` on stderr, exit 0 | stderr | Model narrows the query |
| Endpoint hangs | curl aborts at 30 s with a non-zero exit | stderr timeout message | Re-assign the task |
| Rate limiting | Not applicable: git-rest and in-cluster Prometheus apply no rate limit to these reads | — | — |
| Clock skew | Not applicable: instant queries send no `time`, so Prometheus evaluates at its own now | — | — |

## Security / Abuse Cases

- **Attacker control:** script arguments come from the model, which reads untrusted task text and repo content. Every argument is treated as hostile.
- **SSRF / path abuse:**
  - `prometheus-query.sh` fixes the path to `/api/v1/query` and sends the argument only as an encoded query parameter.
  - `vault-read.sh` rejects absolute paths, `..` segments, `?`, `#` and `://`.
  - `repo-clone.sh` accepts only `github.com/<owner>/<name>` from a strict charset and rejects `.`/`..` segments. As defense in depth, it verifies that the symlink-resolved destination lies inside `repos/` before any destructive filesystem call. A reference cannot reach another host, `file://`, or a path outside the clone root.
- **Writes:** no `curl` invocation carries a write verb (AC9). The clone tree is non-writable and never pushed.
- **Trust boundary:** `CLAUDE_ENV` gains only `GIT_REST_URL` and `PROMETHEUS_URL`, and neither is a secret. No token reaches the scripts.
- **Hang:** every HTTP call has a 30 s timeout. The clone is bounded by the Job's lifetime.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Three HTTP scripts + recording-server test suite | 1 | 2, 3, 4, 9 (curl part), 10 | — |
| 2 | `repo-clone.sh` with traversal + symlink guard + stub-git test suite | 2 | 5, 6, 9 (git part), 10 | — |
| 3 | Config `ALLOWED_TOOLS` + `CLAUDE_ENV` + comments, Dockerfile `git`, `.gitignore`, CLAUDE.md / README / CHANGELOG, three-way consistency test | 3, 4 | 7, 8, 11 | prompts 1, 2 |
| — | `make precommit` on every prompt; AC12 by the operator after merge | — | 1, 12 | all |

Rationale: the two script prompts are independent leaves. Prompt 3's consistency test needs every allowlisted script to exist and every script to be allowlisted, so it lands last.

## Do-Nothing Option

The diagnoser has no working way to observe the running system, and its allowlist names tools that do not exist. Any diagnosis prompt would fail on its first read, or fall back to guessing from source, which the defect lane forbids. Spec `diagnose-observed-defect-with-verdict-gate` cannot ship without this one. Not acceptable.
