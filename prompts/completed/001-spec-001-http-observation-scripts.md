---
status: completed
spec: [001-read-only-observation-toolkit]
summary: Added three executable read-only HTTP observation scripts (agent/scripts/vault-read.sh, vault-list.sh, prometheus-query.sh) plus a subprocess-driven Ginkgo suite in pkg/scripts that drives them against a local recording server.
execution_id: defect-diagnoser-agent-diagnose-v1-exec-001-spec-001-http-observation-scripts
dark-factory-version: dev
created: "2026-09-27T22:45:00Z"
queued: "2026-09-27T23:01:28Z"
started: "2026-09-27T23:01:29Z"
completed: "2026-09-27T23:12:30Z"
branch: dark-factory/read-only-observation-toolkit
---

# Add the three read-only HTTP observation scripts

<summary>

- The agent image gains three read-only helper scripts the model can call to read task files and metrics.
- Reading one vault file and listing vault files both go through the vault's HTTP API with a GET only.
- Querying Prometheus sends exactly one instant query to the fixed query endpoint and nothing else.
- Each script refuses a malformed argument before it makes any network call.
- Each script names the missing endpoint setting on stderr instead of failing silently.
- An HTTP error surfaces its status code on stderr instead of looking like a successful empty read.
- A huge response is cut off at a fixed size, marked `truncated` on stderr, and still exits 0 so the cut never looks like a failure.
- Every HTTP call gives up after 30 seconds, so a hanging endpoint cannot stall a task.
- The behaviour is locked down by tests that drive the real scripts against a local recording HTTP server.
- Nothing outside the new scripts and their tests changes in this prompt.

</summary>

<objective>

Give the defect diagnoser its first two "eyes" on the running system: three executable, read-only HTTP scripts in `agent/scripts/` — `vault-read.sh`, `vault-list.sh`, `prometheus-query.sh` — that behave exactly as `docs/observation-scripts.md` specifies, so that a later diagnosis prompt can read task files and Prometheus metrics without any tool that can write, delete, or reach an arbitrary host. The scripts are the security boundary: every argument is hostile input, and every failure must be visible rather than silent.

</objective>

<context>

Read these before writing anything:

- `docs/observation-scripts.md` — the **authoritative contract** for all three scripts. Read it in full. Where it and any other source disagree, it wins.
- `agent/.claude/CLAUDE.md` — the guardrails prose that tells the model these scripts are its API.
- `k8s/defect-diagnoser-agent-config.yaml` — the Config that will allowlist these scripts as `Bash(scripts/<name>.sh:*)`. **Do not edit it in this prompt**; the allowlist change and the consistency test land in a later prompt.
- `pkg/factory/factory_suite_test.go` and `pkg/factory/factory_test.go` — the repo's Ginkgo/Gomega test idiom (suite file, external `_test` package, counterfeiter not needed here).
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite and spec conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — gosec expectations (test files are excluded from gosec, so no `#nosec` comments are needed).
- `docs/dod.md` — the Definition of Done for this repo.

Two facts about the environment, both verified while writing this prompt:

1. `pkg/scripts/` will contain **only** `_test.go` files in `package scripts_test`. That is a valid Go package: `go test ./...` compiles and runs it. Do **not** add a non-test `.go` file to "complete" the package.
2. The container sets `http_proxy` / `HTTPS_PROXY` in the ambient environment. `curl` honours them, so a loopback request to the test server would be sent to the proxy instead, the recording server would see zero requests, and every "zero requests" assertion would silently invert. The test harness must therefore construct the child environment explicitly (see requirement 9).

Create `pkg/scripts/scripts_suite_test.go` with exactly this content. All three prompts in this spec share this one suite file — do not create a second one, and if it already exists with this content leave it untouched:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scripts_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

func TestSuite(t *testing.T) {
	time.Local = time.UTC
	format.TruncatedDiff = false
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.Timeout = 60 * time.Second
	RunSpecs(t, "Scripts Suite", suiteConfig, reporterConfig)
}

// repoRoot walks up from the test's working directory until it finds go.mod.
func repoRoot() string {
	dir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			Fail("go.mod not found above " + dir)
		}
		dir = parent
	}
}
```

</context>

<requirements>

1. Create `agent/scripts/vault-read.sh`, `agent/scripts/vault-list.sh` and `agent/scripts/prometheus-query.sh`.

   - Each begins with `#!/usr/bin/env bash` and a one-line comment naming what it reads.
   - Each is marked executable (`chmod +x agent/scripts/*.sh`) — the allowlist entry `Bash(scripts/<name>.sh:*)` resolves to a file the agent executes directly.
   - Do **not** use `set -e` or `set -u`. `set -u` would abort on an unset endpoint variable before the explicit check can print the variable name.
   - Do **not** write `${VAR:-…}` anywhere, and do not introduce any environment knob (no clone dir, depth, timeout, output cap, token or gateway secret). The spec forbids env knobs outright.

2. Argument handling, in exactly this order and all of it **before any network call**:

   a. `[ "$#" -ne 1 ]` → print the usage line to stderr, `exit 2`.
   b. Per-script argument validation (requirements 5, 6, 7) → print the rejection reason to stderr, `exit 2`.
   c. Endpoint presence: `[ -z "${GIT_REST_URL}" ]` (or `PROMETHEUS_URL`) → print `GIT_REST_URL is required` / `PROMETHEUS_URL is required` to stderr, `exit 1`.

   Zero arguments and two-or-more arguments must both exit non-zero with zero requests reaching the server.

3. Request shape. Send the response body to a temp file and keep the status in a variable — do **not** pipe `curl`'s body into `head`, because a pipe turns the cut into SIGPIPE and the truncation case would exit non-zero instead of 0. This is the shape, shown with the Prometheus URL and parameter; each script substitutes its own URL, its own parameter name and its own argument:

   ```bash
   tmp="$(mktemp)"
   trap 'rm -f "$tmp"' EXIT
   status="$(curl -sS --globoff --max-time 30 -o "$tmp" -w '%{http_code}' \
     -G --data-urlencode "query=${expr}" "${PROMETHEUS_URL}/api/v1/query")" || {
     echo "request failed" >&2
     exit 1
   }
   ```

   Rules that follow from it:

   - `GET` only. Never pass `-X`, `--request`, `-d`, `--data`, `--data-raw`, `--data-binary`, `--data-ascii`, `--json`, `-F`, `--form`, `-T`, `--upload-file`. `-G --data-urlencode` is the **only** permitted way to send parameters — it is also the only way the read-only check (AC9) accepts.
   - `--max-time 30` on every `curl` invocation, spelled exactly `--max-time 30` (AC4 greps for that literal).
   - `--globoff` on every `curl` invocation. Without it curl expands `{a,b}` and `[1-3]` in the URL, so `vault-read.sh '{a,..}'` passes the `/../` check yet makes curl send a second request to `/api/v1/` (curl squashes the `..` segment). That escapes `/api/v1/files/` and violates AC3.

4. Response handling, identical in all three scripts:

   - Status other than `200` → write the status alone to stderr (so `500` and `401` each appear) and `exit 1`.
   - Non-zero `curl` exit (transport failure, timeout) → a message on stderr, `exit 1`.
   - Otherwise read the size as `size="$(wc -c < "$tmp")"` and normalise it with `size="$((size))"` (the arithmetic expansion strips the leading whitespace some `wc` builds emit).
   - `size` greater than `65536` → `head -c 65536 "$tmp"` to stdout, `echo truncated >&2`, `exit 0`. The cut must **not** be reported as an error.
   - `size` at or below `65536` → `cat "$tmp"` to stdout, `exit 0`.

5. `vault-read.sh <path>` — read one vault file.

   - Reject with exit 2 when the argument starts with `/`; when `"/$path/"` contains the full segment `"/../"` (this rejects `../b.md`, `a/../b.md`, `a/..` while still allowing a file legitimately named `a..b.md`); or when the argument contains `?`, `#` or `://`.
   - Percent-encode before building the URL: `%` → `%25` first, then space → `%20`. No other encoding is needed because validation has already excluded `?`, `#` and `://`.
   - Request: `GET ${GIT_REST_URL}/api/v1/files/<encoded path>` with no query string.
   - Env: `GIT_REST_URL`.

6. `vault-list.sh <glob>` — list vault files matching a glob.

   - No argument-content validation: the glob travels as a query parameter, not as a path.
   - Request: `GET ${GIT_REST_URL}/api/v1/files/` with the query parameter `glob=<glob>`, sent with `-G --data-urlencode`.
   - Env: `GIT_REST_URL`. The body is written to stdout unchanged (one path per line, empty when nothing matches).

7. `prometheus-query.sh <promql>` — one instant PromQL query.

   - Reject with exit 2 when the argument starts with `/` or contains `://`.
   - Request: `GET ${PROMETHEUS_URL}/api/v1/query` with the query parameter `query=<promql>`, sent with `-G --data-urlencode`. The path is fixed — no other Prometheus path may be constructed, and no `time` parameter is sent.
   - Env: `PROMETHEUS_URL`.

8. Create `pkg/scripts/http_scripts_test.go` in `package scripts_test`, using the suite file from `<context>`. It must drive the **real** scripts through the **real** subprocess boundary. Do not re-implement any script logic in Go.

9. Test harness contract:

   - Run the script directly: `exec.CommandContext(ctx, scriptPath, args...)` — not via `bash <script>`. This is how the agent invokes it, and it exercises both the shebang and the executable bit. Use a `context.WithTimeout(ctx, 10*time.Second)`.
   - Build the child environment explicitly: `PATH` taken from the parent, `HOME` set to a temp directory (so a stray `~/.curlrc` cannot inject a proxy), plus **only** the endpoint variable under test. Never pass `os.Environ()` — the ambient proxy variables would route the loopback request to the proxy and make the recording server record nothing.
   - Recording server: `httptest.NewServer` whose handler records method, raw request path and raw query, and writes a configurable status and body. Guard the recorded slice with a mutex. Assert on the **raw/escaped** path (`r.RequestURI`): `r.URL.Path` is already percent-decoded and would hide an unencoded space.
   - Helpers: `newRecordingServer(status int, body string) (*httptest.Server, *recorder)` (registers `DeferCleanup(server.Close)`) and `runScript(script string, env map[string]string, args ...string) (stdout, stderr string, exitCode int)`. Resolve script paths from `repoRoot()`.
   - Keep every `It` body under ~60 lines by using these helpers; `funlen` (80 lines / 50 statements) and `gocognit` (20) apply to test files too.

10. Test cases — all of them, mirroring the spec's AC2 and AC3:

    **`prometheus-query.sh`**

    - one argument → exactly one request, method `GET`, path `/api/v1/query`, `query` parameter equal to the argument; stdout equals the response body; exit 0.
    - argument starting with `/` → exit 2 and zero requests; argument containing `://` → exit 2 and zero requests.
    - zero arguments → non-zero exit and zero requests; two arguments → non-zero exit and zero requests.
    - `PROMETHEUS_URL` unset → non-zero exit and stderr contains `PROMETHEUS_URL`.
    - `500` response → non-zero exit with `500` on stderr; `401` response → non-zero exit with `401` on stderr.
    - 100 KiB response body → stdout is exactly 65536 bytes, stderr contains `truncated`, exit 0.

    **`vault-read.sh`**

    - each of `/etc/passwd`, `../b.md`, `a/../b.md`, `a/..`, `a.md?x=1`, `a.md#x`, `http://evil/x` → exit 2 and zero requests.
    - `25 Tasks/x y.md` → the recorded raw request path is `/api/v1/files/25%20Tasks/x%20y.md`.
    - `{a,..}` and `x[1-2].md` → exactly one request each, and its raw request path (`r.RequestURI`) starts with `/api/v1/files/` (URL globbing is off).
    - zero arguments → non-zero exit and zero requests; two arguments → non-zero exit and zero requests.
    - `GIT_REST_URL` unset → non-zero exit and stderr contains `GIT_REST_URL`.
    - `500` and `401` responses → non-zero exit with the status on stderr.
    - 100 KiB body → exactly 65536 bytes on stdout, `truncated` on stderr, exit 0.

    **`vault-list.sh`**

    - `* Tasks/*.md` → the recorded `glob` query parameter equals `* Tasks/*.md`, and the request path is `/api/v1/files/`.
    - zero arguments → non-zero exit and zero requests; two arguments → non-zero exit and zero requests.
    - `GIT_REST_URL` unset → non-zero exit and stderr contains `GIT_REST_URL`.
    - `500` and `401` responses → non-zero exit with the status on stderr.
    - 100 KiB body → exactly 65536 bytes on stdout, `truncated` on stderr, exit 0.

    **All three**

    - every recorded request is a `GET`, and every recorded path is under `/api/v1/files/` (vault scripts) or equals `/api/v1/query` (Prometheus script).
    - each script file contains the literal `--max-time 30` (AC4's grep, asserted from the test as well).

11. Before finishing, re-run `<verification>` and confirm every line passes. Walk each test case in requirement 10 against the code and confirm it is actually asserted — a test that never asserts a status or a request count does not count as covering the case.

</requirements>

<constraints>

- **Frozen tool names.** The four `Bash(scripts/…)` allowlist entries keep their exact names, resolved relative to the agent working directory. The Dockerfile's existing `COPY agent/ /agent/` ships them.
- **Script contracts are frozen in `docs/observation-scripts.md`.** A behaviour change updates that page in the same change. Do not change that page in this prompt.
- **No Go change for env.** `CLAUDE_ENV` / `ClaudeEnvRaw` in `main.go` and `cmd/run-task/main.go` stays exactly as it is. No new flag, field or helper for the two URLs.
- Do NOT modify `k8s/defect-diagnoser-agent-config.yaml`, `agent/.claude/CLAUDE.md`, `README.md`, `CHANGELOG.md`, `.gitignore` or the `Dockerfile` in this prompt — the allowlist, the prose and the three-way consistency test land in a later prompt.
- Do NOT create `repo-clone.sh` in this prompt; it is a sibling prompt's deliverable.
- Do NOT add a `pod-logs.sh` or any pod-log/event read. That waits on the Config CRD gaining a service-account field and is an explicit non-goal.
- No env knobs of any kind in the scripts: no clone directory, clone depth, timeout, output cap, `GIT_CLONE_TOKEN` or `GATEWAY_SECRET`. These are invariants, not defaults.
- No private-repo clone auth, no git-rest gateway auth, no Prometheus path other than `/api/v1/query`, no Prometheus range query.
- Do NOT change the `github.com/bborbe/agent` module. No `go get`, no `go mod tidy` beyond what `make precommit` runs, never `-mod=vendor`.
- Do NOT commit — dark-factory handles git.
- `docs/dod.md` (the validationPrompt) asks for a CHANGELOG entry, a README update and a scenario. For this prompt all three are deferred by the spec, not unmet: the CHANGELOG and README edits are prompt 3's deliverable (AC11), and the spec states "Scenario coverage: no new scenario". Report them as deferred-by-spec, never as blockers.
- Create no other files under `agent/scripts/` (no shared helper): prompt 3's consistency test requires every executable there to be allowlisted.
- Existing tests must still pass, and the healthcheck / oauth-probe routing is untouched.

</constraints>

<verification>

Run each command from the repo root. All must pass.

```bash
make test          # the new pkg/scripts suite runs here
make precommit     # AC1 — must exit 0

# AC4 — each of the three scripts sets a 30 s timeout (count of 1 or more per file)
test -z "$(grep -L -- '--max-time 30' agent/scripts/vault-read.sh agent/scripts/vault-list.sh agent/scripts/prometheus-query.sh)"

# AC9 (curl half) — no curl invocation carries a write verb. The pipeline must print 0.
test "$(sed -e ':a' -e '/\\$/N; s/\\\n//; ta' agent/scripts/*.sh | grep -E '(^|[^A-Za-z_])curl ' | grep -cE -- '(-X|--request|--upload-file| -T |--data([^-]|$)|--data-(binary|raw|ascii)|--json| -d | -F |--form)')" = "0"

# AC10 — no env knobs, no defaulted env vars. Both must find nothing.
! grep -qE 'REPO_CLONE_DIR|GIT_CLONE_DEPTH|GIT_CLONE_TOKEN|GATEWAY_SECRET' agent/scripts/*.sh
! grep -qE '\$\{[A-Z_]+:-' agent/scripts/*.sh

# the three scripts exist and are executable
test -x agent/scripts/vault-read.sh && test -x agent/scripts/vault-list.sh && test -x agent/scripts/prometheus-query.sh
```

</verification>
