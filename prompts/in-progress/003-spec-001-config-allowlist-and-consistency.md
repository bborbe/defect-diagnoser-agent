---
status: approved
spec: [001-read-only-observation-toolkit]
created: "2026-09-27T22:45:00Z"
queued: "2026-09-27T23:02:59Z"
branch: dark-factory/read-only-observation-toolkit
---

# Align the Config allowlist, image and prose, and enforce the agreement with a test

<summary>

- The Config's tool allowlist stops naming a script that cannot work and matches the four scripts that exist.
- The vault and Prometheus addresses now reach the model through the channel the runner actually forwards, instead of sitting in pod env where they were silently dropped.
- The dead plain-environment keys for those two addresses are gone.
- The Config's comments describe the deferred Kubernetes read path in the same words the README uses.
- The container image ships `git`, so the clone script can run at all.
- A local clone directory cannot be committed by accident.
- The guardrails and the README describe the two read paths that actually exist, and the README no longer advises a code change for something the Config already handles.
- A test fails the build if the allowlist, the scripts on disk and the prose ever disagree in any direction.
- The changelog records the change.

</summary>

<objective>

Close the gap between what the Config claims and what the image actually contains. Today the Config allowlists five scripts, none of which exists, one of which (`pod-logs.sh`) cannot work on the current platform; it also sets the two endpoint URLs as plain pod env, which the Claude runner strips before spawning the model — so even a correct script would fail with "variable is required" while `kubectl describe pod` shows the variable set. After this change the allowlist, the scripts on disk and the prompt/guardrail prose agree in every direction, and a test enforces that agreement so the next drift fails `make test` instead of failing at runtime.

</objective>

<context>

Read these before writing anything:

- `docs/observation-scripts.md` — the **authoritative contract** for the scripts. Read the "How the scripts get their env" section in full; it names the exact Config value this prompt commits.
- `k8s/defect-diagnoser-agent-config.yaml` — the Config being edited.
- `agent/.claude/CLAUDE.md` — the guardrails prose being edited.
- `README.md` — the Env Vars table and the "Claude subprocess env allowlist" section.
- `CHANGELOG.md` — the file to append an `## Unreleased` section to.
- `Dockerfile` — the `apk --no-cache add` line that must gain `git`. Note the final stage is `FROM alpine`, which resolves to the earlier stage **named** `alpine`, so packages installed in that stage do ship in the image.
- `.gitignore` — the file to extend.
- `main.go` and `cmd/run-task/main.go` — read them to confirm the claim that no Go change is needed: both already parse `CLAUDE_ENV` (`env:"CLAUDE_ENV"`, arg `-claude-env`) through `envparse.KeyValuePairs` into the runner's env map. **Do not edit either file.**
- `pkg/prompts/workflow.md`, `pkg/prompts/output-format.md` — the embedded prompts the consistency test scans for script mentions.
- `pkg/factory/factory_suite_test.go` and `pkg/factory/factory_test.go` — the repo's Ginkgo/Gomega test idiom.
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format and the required `feat:`/`fix:`/… prefix.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100).
- `docs/dod.md` — the Definition of Done for this repo (it explicitly requires the CHANGELOG entry and the docs update).

This prompt depends on prompts 1 and 2 of spec 001 (`1-spec-001-http-observation-scripts`, `2-spec-001-repo-clone-script`). Precondition: `agent/scripts/vault-read.sh`, `agent/scripts/vault-list.sh`, `agent/scripts/prometheus-query.sh`, `agent/scripts/repo-clone.sh` and `pkg/scripts/scripts_suite_test.go` all exist. If any is missing, stop and report `failed` naming the missing file — do not create it. `pkg/scripts/scripts_suite_test.go` already exists from a sibling prompt in this spec and defines `repoRoot()`. **Leave it untouched**, do not create a second suite file (two `RunSpecs` calls in one package fail the build), and do not redefine `repoRoot()`.

<!-- Two deliberate deviations from the spec's literal AC text, both preserving the AC's intent:

     1. AC7/AC11 write the leading whitespace as `\s`. `[[:space:]]` is used here instead:
        portable across grep builds and asserts the identical thing.

     2. AC8's exact-string check is written in the spec as `grep -c '<allowlist>'`. Plain
        `grep` is BRE, where `*` is a quantifier — the pattern `Bash(scripts/vault-read.sh:*)`
        compiles to "…vault-read.sh" + zero-or-more `:` + ")", which does NOT match the file
        and returns 0, so the AC's stated `1` is unreachable with that exact command.
        `grep -cF` (fixed string) returns the intended 1 and is verified to do so. -->

</context>

<requirements>

1. **`k8s/defect-diagnoser-agent-config.yaml`** — three edits, nothing else in the file changes. The `NOT APPLIED` header, the `metadata`/`spec` values, `taskTypes`, `resources`, `secretName` and the `ANTHROPIC_*` keys all stay as they are.

   a. Replace the `ALLOWED_TOOLS` value. Old → new:

   ```yaml
   # old
   ALLOWED_TOOLS: "Read,Grep,Glob,Bash(scripts/vault-read.sh:*),Bash(scripts/vault-list.sh:*),Bash(scripts/prometheus-query.sh:*),Bash(scripts/pod-logs.sh:*),Bash(scripts/repo-clone.sh:*)"
   # new — the `pod-logs.sh` entry is dropped; the four surviving names are unchanged
   ALLOWED_TOOLS: "Read,Grep,Glob,Bash(scripts/vault-read.sh:*),Bash(scripts/vault-list.sh:*),Bash(scripts/prometheus-query.sh:*),Bash(scripts/repo-clone.sh:*)"
   ```

   b. Delete the plain `GIT_REST_URL:` and `PROMETHEUS_URL:` keys from `spec.env` and add `CLAUDE_ENV` in their place. The `CLAUDE_ENV` line must be indented, end immediately after the closing quote (no trailing comment on that line), and hold exactly:

   ```yaml
   CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"
   ```

   Replace the surrounding comments so they explain the mechanism rather than the old state — the current comment still says the scripts are "not yet written — domain work", which is no longer true, and the current endpoint comments sit on the two keys being deleted. Say that the Claude runner strips pod env down to a fixed allowlist, so the endpoint URLs reach the scripts only through `CLAUDE_ENV`, a comma-separated `KEY=VALUE` list parsed by both entry points.

   c. Rewrite the trailing comment block about read path 3. It must contain the exact phrase `Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2`, must keep the substance about the missing `serviceAccount` field and the gated RBAC, and must not contain any case-insensitive match of `pod[- ]?logs` (e.g. `Pod logs`, `pod-logs`) (the platform cannot read pod logs, which is why the script is being dropped). Keep `pods/log` out of the phrase "Pod logs" — describing the ServiceAccount's future permissions is fine.

2. **`Dockerfile`** — add `git` to the existing `apk --no-cache add` line, keeping the rest of the package list and the trailing backslash intact. No other Dockerfile change.

3. **`.gitignore`** — ensure it contains exactly one line `/agent/repos/` (leading slash, so it anchors to the repo root and does not shadow anything else). A sibling prompt may already have added it because its tests create the directory: if the line is present, leave it alone; if it is absent, add it; never duplicate it. Leave the other entries as they are.

4. **`agent/.claude/CLAUDE.md`** — one edit: the domain bullet that reads "(task files, Prometheus, pod logs/events)" becomes "(task files, Prometheus)". Nothing else in the file changes; the `## Forbidden` section stays exactly as it is, and the scripts remain the sole exception to the internal-network rule.

5. **`README.md`** — three edits.

   a. The "Observes" bullet (currently "task files → Prometheus → pod logs/events via its own read-only service account") becomes:

   ```markdown
   - **Observes (read-only, in order):** task files → Prometheus (Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2)
   ```

   The phrase `task files → Prometheus` must survive verbatim, and the deferred wording must be the same phrase used in the Config.

   b. Rewrite the `### Claude subprocess env allowlist` section. Keep the heading verbatim. Keep the warning that the runner strips pod env down to a safe allowlist (`HOME,PATH,USER,TZ,...`) so a variable visible in `kubectl describe pod` can still be absent from the subprocess. Replace the advice to thread variables through `ClaudeRunnerConfig.Env` in `main.go`, and drop the "See `docs/` for precedent (trade-analysis commit …)" sentence — the string `ClaudeRunnerConfig.Env` must not appear anywhere in the file afterwards. Name `CLAUDE_ENV` as the channel: a comma-separated `KEY=VALUE` list set on the Config's `spec.env` (or `-claude-env` for a local run) and parsed by both entry points. Include one line that contains `CLAUDE_ENV` and both endpoints, for example:

   ```markdown
   `CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"`
   ```

   c. Link `docs/observation-scripts.md` from the README — at minimum from the rewritten section, and add it to the `## Related` list.

   d. In `### Config CRD env pattern`, the yaml block is labelled "Example from `k8s/defect-diagnoser-agent-config.yaml`" but does not match that file: replace its `ALLOWED_TOOLS` line with the Config's real value, or relabel it as a generic example.

   Leave the generic `Tune ALLOWED_TOOLS per task shape` example table alone: it illustrates other agents' tool sets (`vault-write.sh`, `trading-api-read.sh`) and is not a claim about this agent.

6. **`CHANGELOG.md`** — add an `## Unreleased` section above `## v0.0.0` with one bullet describing what was implemented. Follow `changelog-guide.md`: the entry needs a `feat:` prefix and must mention "observation scripts". Do not describe verification steps, and do not use the prompt filename.

7. **`pkg/scripts/consistency_test.go`** (`package scripts_test`, using the existing suite file and `repoRoot()`) — the three-way agreement test. It must parse the Config rather than hard-coding a copy of the allowlist:

   a. Read `k8s/defect-diagnoser-agent-config.yaml` and extract the `ALLOWED_TOOLS` value with a regex equivalent to `(?m)^\s*ALLOWED_TOOLS:\s*"(.*)"\s*$`. Fail loudly if it does not match — a silently empty allowlist would make every check below vacuous.

   b. Split the value on `,` and collect the entries matching `^Bash\(scripts/(.+):\*\)$`, keeping the captured script name. Compare the captured names as plain strings — never by re-grepping the file, because a shell `grep` pattern containing `:*` is a BRE quantifier and would silently match nothing.

   c. Assert that set equals exactly `{vault-read.sh, vault-list.sh, prometheus-query.sh, repo-clone.sh}`, so an emptied or renamed allowlist cannot pass vacuously.

   d. For every allowlisted script name, assert `agent/scripts/<name>` exists and is executable.

   e. Walk `agent/scripts/`; for every regular file with any executable bit set, assert an allowlist entry names it. This is the direction that catches a new script nobody allowlisted.

   f. Read `pkg/prompts/*.md` and `agent/.claude/CLAUDE.md`; find every `scripts/<name>.sh` mention with a regex equivalent to `scripts/([A-Za-z0-9._-]+\.sh)`; assert each mentioned name has an allowlist entry. This is the direction that catches prose pointing at a tool that does not exist.

   g. Assert no case-insensitive match of `pod[- ]?logs` in any of: `k8s/defect-diagnoser-agent-config.yaml`, `agent/.claude/CLAUDE.md`, `README.md`, and every `pkg/prompts/*.md`. This is the negative half of the agreement: the dropped tool must not linger in prose or comments.

   h. Assert the Config's `CLAUDE_ENV` value — extracted with a regex equivalent to `(?m)^\s*CLAUDE_ENV:\s*"(.*)"\s*$` — parses into both keys. Mirror the call in `main.go` (`claudeEnv := envparse.KeyValuePairs(a.ClaudeEnvRaw)`): read that line first to confirm the import path, the call form and the result type, then use the same call here and assert the map contains `GIT_REST_URL` and `PROMETHEUS_URL` with the values `http://vault-obsidian-personal:9090` and `http://prometheus.monitoring:9090`. This is the container-side guard for the seam that the operator-side acceptance check proves on the host: a Config value that greps correctly but parses into one key would otherwise ship. Before applying this regex and the ALLOWED_TOOLS regex, extract the `spec.env` block (the lines after `^  env:$` whose indent is at least 4, up to the next line with indent 2 or less) and match only inside it, so a `CLAUDE_ENV` placed at `spec` level or elsewhere fails. Do not add a YAML library (no `go get`).

   Note that `README.md` is deliberately **not** part of check (f): its example allowlist table names scripts belonging to other agents.

8. Keep every `It` body under ~60 lines; factor shared reading/parsing into helpers so `funlen` (80 lines / 50 statements) and `gocognit` (20) pass.

9. Before finishing, re-run `<verification>` and confirm every line passes. Walk requirements 1–7 against the files you changed and confirm each exact string is present (or absent, where the requirement is a removal).

</requirements>

<constraints>

- **Frozen tool names.** The four `Bash(scripts/…)` allowlist entries keep their exact names, resolved relative to the agent working directory. The Dockerfile's existing `COPY agent/ /agent/` ships them.
- **Script contracts are frozen in `docs/observation-scripts.md`.** A behaviour change updates that page in the same change. Do not change that page in this prompt.
- **No Go change for env.** `CLAUDE_ENV` / `ClaudeEnvRaw` in `main.go` and `cmd/run-task/main.go` stays exactly as it is. No new flag, field or helper for the two URLs.
- `k8s/defect-diagnoser-agent-config.yaml` keeps its `NOT APPLIED` header. Only `ALLOWED_TOOLS`, the env keys and the path-3 comments change.
- `agent/.claude/CLAUDE.md` § Forbidden is unchanged. The scripts remain the sole exception to the internal-network rule.
- Do NOT add a `pod-logs.sh`, a pod-log/event read, or any replacement for it. It waits on the Config CRD gaining a service-account field and is an explicit non-goal.
- No `kubectl apply`, deploy, secret manifest or RBAC. The Config file stays NOT APPLIED.
- Do NOT change the `github.com/bborbe/agent` module. No `go get`, never `-mod=vendor`.
- Do NOT create or modify the scripts under `agent/scripts/` — they are sibling prompts' deliverables, and this prompt only reads them. Do not edit `pkg/scripts/scripts_suite_test.go` or the sibling test files.
- AC12 (proving the env reaches the Claude subprocess) is operator-verified on the host after merge, against the router token and the committed Config value; it is not reproducible in this container. Do not attempt it and do not add it to `<verification>`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass, and the healthcheck / oauth-probe routing is untouched.

</constraints>

<verification>

Run each command from the repo root. Every line must pass.

```bash
make test          # the consistency test runs here
make precommit     # AC1 — must exit 0
```

**AC7 — env forwarding (all four must pass):**

```bash
test "$(grep -cE '^[[:space:]]+CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"$' k8s/defect-diagnoser-agent-config.yaml)" = "1"
test "$(grep -cE 'CLAUDE_ENV: .*GIT_REST_URL=http://vault-obsidian-personal:9090' k8s/defect-diagnoser-agent-config.yaml)" = "1"
test "$(grep -cE 'CLAUDE_ENV: .*PROMETHEUS_URL=http://prometheus.monitoring:9090' k8s/defect-diagnoser-agent-config.yaml)" = "1"
! grep -qE '^[[:space:]]+(GIT_REST_URL|PROMETHEUS_URL):' k8s/defect-diagnoser-agent-config.yaml
```

**AC8 — allowlist, scripts and prose agree:**

```bash
# `-F` because the allowlist contains `(`, `)` and `*`, which plain grep reads as BRE metacharacters
test "$(grep -cF 'ALLOWED_TOOLS: "Read,Grep,Glob,Bash(scripts/vault-read.sh:*),Bash(scripts/vault-list.sh:*),Bash(scripts/prometheus-query.sh:*),Bash(scripts/repo-clone.sh:*)"' k8s/defect-diagnoser-agent-config.yaml)" = "1"
! grep -qiE 'pod[- ]?logs' k8s/defect-diagnoser-agent-config.yaml agent/.claude/CLAUDE.md README.md pkg/prompts/*.md
```

**AC11 — image, hygiene and docs:**

```bash
grep -qE 'apk .*add.* git( |$)' Dockerfile
test "$(grep -c '^/agent/repos/' .gitignore)" = "1"
test "$(grep -c 'docs/observation-scripts.md' README.md)" -ge 1
test "$(grep -cE 'CLAUDE_ENV.*(GIT_REST_URL|PROMETHEUS_URL)' README.md)" -ge 1
test "$(grep -c 'task files → Prometheus' README.md)" = "1"
test "$(grep -c 'Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2' k8s/defect-diagnoser-agent-config.yaml)" -ge 1
test "$(grep -c 'Kubernetes API reads, deferred — see NEXT-DIRECTIONS v2' README.md)" -ge 1
! grep -q 'ClaudeRunnerConfig.Env' README.md
test "$(sed -n '/^### Claude subprocess env allowlist/,/^## /p' README.md | grep -c 'CLAUDE_ENV')" -ge 1
test "$(grep -c '(task files, Prometheus)' agent/.claude/CLAUDE.md)" = "1"
test "$(sed -n '/^## Unreleased/,/^## v/p' CHANGELOG.md | grep -ci 'observation scripts')" -ge 1
```

</verification>
