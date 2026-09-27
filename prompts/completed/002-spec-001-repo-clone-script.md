---
status: completed
spec: [001-read-only-observation-toolkit]
summary: Added agent/scripts/repo-clone.sh, a read-only GitHub clone script with owner/name normalization, pre-git traversal and symlink containment guards, plus a stub-git Ginkgo suite, the /agent/repos/ gitignore line and a CHANGELOG entry
execution_id: defect-diagnoser-agent-diagnose-v1-exec-002-spec-001-repo-clone-script
dark-factory-version: dev
created: "2026-09-27T22:45:00Z"
queued: "2026-09-27T23:02:59Z"
started: "2026-09-27T23:12:31Z"
completed: "2026-09-27T23:24:40Z"
branch: dark-factory/read-only-observation-toolkit
---

# Add the read-only repo-clone script with a traversal and symlink guard

<summary>

- The agent image gains one script that makes a read-only local clone of a public source repo.
- Only `owner/name`-shaped references to GitHub are accepted; every other shape is refused.
- A crafted reference containing shell metacharacters, a scheme, or a `..` segment is refused before anything runs.
- A directory pre-planted as a symlink inside the clone root cannot be used to escape it.
- Every refusal happens before `git` runs and before any file is created, removed or changed.
- Re-cloning over a previous read-only tree works for a non-root user.
- A finished clone is non-writable, so the model cannot modify or push what it read.
- The clone reports the local path, the commit it landed on, and the default branch.
- The local clone directory is ignored by git, so a test run cannot commit a nested clone.
- The behaviour is locked down by tests that use a stub `git` and compare before/after filesystem snapshots.

</summary>

<objective>

Give the defect diagnoser its fourth read-only eye: `agent/scripts/repo-clone.sh`, which clones a public GitHub repo into a fixed `repos/` directory beside the scripts so a later diagnosis prompt can read real source at a real commit. The script is the security boundary — its argument comes from a model that has read untrusted task text and untrusted repo content, so a crafted reference must never reach another host, another path, or a destructive filesystem call.

</objective>

<context>

Read these before writing anything:

- `docs/observation-scripts.md` — the **authoritative contract**, section `repo-clone.sh clone <repo>`. Read it in full. Where it and any other source disagree, it wins.
- `agent/.claude/CLAUDE.md` — the guardrails prose that tells the model these scripts are its API.
- `k8s/defect-diagnoser-agent-config.yaml` — the Config that will allowlist this script as `Bash(scripts/repo-clone.sh:*)`. **Do not edit it in this prompt.**
- `.gitignore` — you add one line to it (requirement 1).
- `pkg/factory/factory_suite_test.go` and `pkg/factory/factory_test.go` — the repo's Ginkgo/Gomega test idiom.
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite and spec conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100).
- `docs/dod.md` — the Definition of Done for this repo.

**The upstream reference is not available.** The spec says this contract closes a `..` traversal hole in `agent-sentry-issue-analyzer/scripts/repo-clone.sh`. That repo is **not** reachable from this container and is not vendored. Do not guess at its contents and do not copy from it — implement only what `docs/observation-scripts.md` specifies.

Three facts about the environment, all verified while writing this prompt:

1. `pkg/scripts/` will contain **only** `_test.go` files in `package scripts_test`. That is a valid Go package: `go test ./...` compiles and runs it. Do **not** add a non-test `.go` file.
2. `pkg/scripts/scripts_suite_test.go` is shared with a sibling prompt that adds the HTTP-script tests. **If it already exists, leave it untouched.** If it does not exist, create it with exactly this content — and never create a second suite file, because two `RunSpecs` calls in one package fail the build:

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

`repoRoot()` is defined there. Use it; do **not** redefine it in your test file.

3. The clone root is fixed beside the script, so the tests run a copy of the script inside a temp tree (requirement 13) and never write into `<repo>/agent/repos/`. Requirement 1 still adds the ignore line, for manual local runs: dark-factory commits whatever is in the worktree, and a leftover clone must never be committed.

</context>

<requirements>

1. Create `agent/scripts/repo-clone.sh`: `#!/usr/bin/env bash` plus a one-line comment, marked executable (`chmod +x`). Do **not** use `set -e` or `set -u`, and do **not** write `${VAR:-…}` or introduce any environment knob (no clone dir, depth, timeout, token or gateway secret — the spec forbids env knobs outright).

   Also add the single line `/agent/repos/` to `.gitignore` (leading slash, anchored to the repo root), leaving the existing entries untouched. This is deliberately part of this prompt rather than the later Config/prose prompt: this prompt's tests are what create the directory, and a clone tree left behind by a killed test run would otherwise be committed.

2. Subcommand handling, before anything else:

   - `[ "$#" -ne 2 ]` → print the usage line (`usage: repo-clone.sh clone <owner>/<name>`) to stderr, `exit 2`.
   - `[ "$1" != "clone" ]` → the same usage line to stderr, `exit 2`. There is no `log` subcommand and no other subcommand.

3. Normalise the reference to `https://github.com/<owner>/<name>`:

   - `https://github.com/<rest>` → slug `<rest>`.
   - `git@github.com:<rest>` → slug `<rest>`.
   - any other argument containing `://` → `invalid repo reference`, `exit 2` (this catches `http://evil.example/x/y` and `file:///etc/x`). Only argument-count and unknown-subcommand errors print usage; every reference-shape failure prints `invalid repo reference`.
   - anything else → slug is the argument unchanged.
   - strip one trailing `.git` from the slug.
   - split the slug on the **first** `/`: a slug with no `/` is invalid (`exit 2`), and a name that still contains a `/` is invalid (`exit 2`).

4. Validation — all of it before `git` runs and before the filesystem is touched. Every failure prints `invalid repo reference` to stderr and exits 2:

   - `owner` and `name` are each non-empty and each contain only characters from `A-Za-z0-9._-`.
   - neither `owner` nor `name` is `.` or `..`.
   - **Defence in depth:** the destination `<agent dir>/repos/<owner>/<name>`, resolved with symlinks followed, lies strictly inside the resolved `<agent dir>/repos/`.
     - The base is `$repos_dir` from requirement 5 (already physical, because `agent_dir` comes from `pwd -P`), whether or not it exists yet. If `$repos_dir` is itself a symlink (`[ -L "$repos_dir" ]`), reject.
     - Resolve the parent: if `$repos_dir/$owner` exists or is a symlink, `parent="$(cd "$repos_dir/$owner" 2>/dev/null && pwd -P)"`, and reject if that yields an empty string (dangling symlink, not a directory). Otherwise `parent="$repos_dir/$owner"`.
     - Resolve the destination itself the same way: if `$parent/$name` exists or is a symlink, `dest_real="$(cd "$parent/$name" 2>/dev/null && pwd -P)"`, and reject if empty (dangling symlink, symlink to a file, plain file). Otherwise `dest_real="$parent/$name"`.
     - Require both `parent` and `dest_real` to match the `case` pattern `"$repos_dir"/*` (the trailing slash stops `repos-evil/` from passing). Every rejection prints `invalid repo reference` and exits 2.
     - This check must **not create anything** — no `mkdir`, no `rm -rf`, no `chmod` before it. Creating the owner directory would follow a planted symlink and write outside `repos/`, which the snapshot assertions catch.

5. The clone root is fixed: the `repos/` directory beside `scripts/`. Derive it from the script's own location with symlinks followed:

   ```bash
   script_dir="$(cd "$(dirname "$0")" && pwd -P)"
   agent_dir="$(cd "$script_dir/.." && pwd -P)"
   repos_dir="$agent_dir/repos"
   ```

   No env override, no argument, no default.

6. Replacing an existing clone: when the destination exists or is a symlink, first `chmod -R u+w "$dest"` (tolerate failure), then `rm -rf "$dest"`, then `mkdir -p` the owner directory. Restoring write permission first is what makes a re-clone over the previous read-only tree succeed for a non-root user.

7. Clone with exactly `git clone "$url" "$dest"` where `url` is `https://github.com/<owner>/<name>`. No `--recurse-submodules`, no other flags, no credentials, no submodules.

8. Any non-zero `git clone` exit → print `git clone failed for <url>` to stderr and exit non-zero.

9. On success:

   - `head_sha` from `git -C "$dest" rev-parse HEAD` and `default_branch` from `git -C "$dest" rev-parse --abbrev-ref HEAD`, each tolerated to fail (suppress stderr, do not abort) so an unusual tree cannot fail the script.
   - Make the whole tree, `.git` included, non-writable: `chmod -R a-w "$dest"` (only when the destination exists).
   - Write to stdout, flat `key=value`, all three lines unconditionally: `clone_path=<absolute path>`, `head_sha=<sha>`, `default_branch=<branch>`.

10. Create `pkg/scripts/stub_git_test.go` (the stub helper) and `pkg/scripts/repo_clone_test.go` (the specs), both in `package scripts_test`. Use the shared suite file from `<context>`; do not create a second suite file and do not redefine `repoRoot()`. Sibling prompt 1 already declares `runScript`, `newRecordingServer` and `recorder` in this package; prefix every helper here with `repoClone` (e.g. `runRepoClone`, `repoCloneSnapshot`, `writeRepoCloneStubGit`).

11. Stub `git` contract — the test writes an executable shell script named `git` into a temp directory and prepends that directory to the child's `PATH`:

    - it appends its full argv to the log file named by `STUB_GIT_LOG`;
    - when `$1` is `clone`, it `mkdir -p`s the **last** argument (the destination), after appending, counts the `clone ` lines in the log to get the call index (so the first call gets index 1), writes `<dest>/stub-clone-marker-<index>`, and exits with `$STUB_GIT_CLONE_EXIT` (default 0);
    - for any other subcommand it exits 0 printing nothing.

    Consequences the tests rely on: the `clone ` lines in the log are the assertion target (the script's later `rev-parse` calls also get logged, so filter on the `clone ` prefix); and a second clone removes the first call's marker because the script `rm -rf`s the destination before cloning again.

12. Build the child environment explicitly for every invocation: `PATH` = stub directory + the parent's `PATH`, `HOME` = a temp directory, plus the `STUB_GIT_*` variables. Never pass `os.Environ()` — the container sets `http_proxy` / `HTTPS_PROXY`, and passing them through makes `curl`/`git` route loopback traffic to the proxy. Run the script directly with `exec.CommandContext(ctx, scriptPath, args...)` under a `context.WithTimeout(ctx, 60*time.Second)`.

13. Sandbox: never execute the script in place. In a `BeforeEach`, create a temp root, copy `agent/scripts/repo-clone.sh` (read from `repoRoot()`) to `<tmp>/agent/scripts/repo-clone.sh` with mode `0755`, and run that copy. Its `<agent dir>` is `<tmp>/agent`, so a broken guard can at worst damage the temp root, never the mounted worktree. Snapshot helper: walk `<tmp>` and the outside-target directory with `filepath.WalkDir` + `os.Lstat`, record `path mode` per entry, sort, and join. For rejection cases, include `<tmp>/agent/repos` in the snapshot too, so a stray `mkdir`/`rm` under `repos/` is caught. For success cases, exclude it. Compare before and after each case.

14. Resolve the expected destination with symlinks followed, so the comparison is stable: `agentDir, _ := filepath.EvalSymlinks(filepath.Join(tmpRoot, "agent"))` then `filepath.Join(agentDir, "repos", "bborbe", "foo")`. The script resolves its own path with `pwd -P`, and macOS temp dirs sit behind the `/var` → `/private/var` symlink.

15. Test cases — all of them, mirroring the spec's AC5 and AC6:

    **Rejections** — for each of `evil`, `a;b/c`, `http://evil.example/x/y`, `file:///etc/x`, `../x`, `x/..`, `x/.`, `./.`, `../..`, `https://github.com/../x`, `git@github.com:x/...git`: exit 2, **zero** stub-git invocations, and identical before/after snapshots.

    **Symlink escape** — create a temp directory outside `repos/` holding a sentinel file; plant `<agent dir>/repos/evil` as a symlink to it; run `clone evil/x`. Assert exit 2, zero stub-git invocations, stderr containing `invalid repo reference`, the sentinel file still present, `agent/repos/evil` still a symlink, and identical before/after snapshots. This is the case that fails if validation creates the owner directory before resolving it.

    **Destination symlink escape** — make `<agent dir>/repos/bborbe` a real directory and plant `<agent dir>/repos/bborbe/foo` as a symlink to the outside temp directory, whose mode is set to `0555` and which holds a sentinel file; run `clone bborbe/foo`. Assert exit 2, zero stub-git invocations, `invalid repo reference` on stderr, the outside directory's mode still `0555`, the sentinel present, the link still a symlink, and identical before/after snapshots. This case fails if only the owner component is resolved, because `chmod -R u+w` follows a command-line symlink.

    **Success** — `clone bborbe/foo`: exactly one `clone` invocation, whose logged argv is `clone https://github.com/bborbe/foo <expected dest>`; no `--recurse-submodules` anywhere in the log; stdout carries an absolute `clone_path=` inside `<agent dir>/repos/`, plus `head_sha=` and `default_branch=` lines; `find <clone> -perm -u+w` prints nothing. Also have the stub create `<dest>/link -> <outside dir>` (outside dir mode `0755`); after the clone the outside dir's mode is unchanged — `chmod -R` must not follow symlinks found while recursing. Assert non-writability with a Go walk (`info.Mode().Perm()&0o200 == 0` for every entry), not GNU `find -perm`.

    **Dotted owner** — `clone bborbe/.github`: accepted, exactly one `clone` invocation.

    **Second clone** — run `clone bborbe/foo` twice against the same log: the second exits 0, the log shows two `clone` invocations, `stub-clone-marker-1` is gone and `stub-clone-marker-2` is present. **Skip this spec when `os.Geteuid() == 0`** with a message naming the blind spot: root removes a read-only tree without needing the permission restoration, so on uid 0 the spec proves nothing about the restoration path. The mode-bit assertions (the `find -perm -u+w` check) hold under any uid and must not be skipped.

    **Clone failure** — `STUB_GIT_CLONE_EXIT=128`: non-zero exit and `git clone failed` on stderr.

    **Bad subcommand** — `log x y`: exit 2 with the usage line on stderr.

16. Cleanup: an `AfterEach` must restore write permission before removing the temp root — `chmod -R u+w <tmp>`, then `os.RemoveAll(<tmp>)`. Without the `chmod`, the read-only clone tree survives the run. The tests must never create `agent/repos` in the repository itself; after `make test` it must be absent.

17. Keep every `It` body under ~60 lines by using helpers for the snapshot, the stub setup and the script invocation; `funlen` (80 lines / 50 statements) and `gocognit` (20) apply to test files too.

18. Before finishing, re-run `<verification>` and confirm every line passes. Walk each case in requirement 15 against the code and confirm it is actually asserted — a rejection case that does not assert both the exit code and the invocation count does not count as covering the case.

</requirements>

<constraints>

- **Frozen tool names.** The four `Bash(scripts/…)` allowlist entries keep their exact names, resolved relative to the agent working directory. The Dockerfile's existing `COPY agent/ /agent/` ships them.
- **Script contracts are frozen in `docs/observation-scripts.md`.** A behaviour change updates that page in the same change. Do not change that page in this prompt.
- The clone root is the fixed `repos/` directory beside `scripts/` (`/agent/repos` in the image, `agent/repos/` in a local checkout). No env override — that is an explicit non-goal.
- No `repo-clone.sh log` subcommand. Nothing in v1 consumes regression history.
- No private-repo clone auth (`GIT_CLONE_TOKEN` / GitHub App) and no git-rest gateway auth (`GATEWAY_SECRET`). v1 clones only repos reachable without credentials.
- **No Go change for env.** `CLAUDE_ENV` / `ClaudeEnvRaw` in `main.go` and `cmd/run-task/main.go` stays exactly as it is.
- The only files outside `agent/scripts/repo-clone.sh` and `pkg/scripts/` that this prompt touches are `.gitignore` (the single `/agent/repos/` line) and nothing else. Do NOT modify `k8s/defect-diagnoser-agent-config.yaml`, `agent/.claude/CLAUDE.md`, `README.md`, `CHANGELOG.md` or the `Dockerfile` — the allowlist, the prose and the three-way consistency test land in a later prompt. The DoD's CHANGELOG/README/scenario items are satisfied by prompt 3 and the spec ("no new scenario") per the spec's Suggested Decomposition — report them as deferred-by-spec, not as blockers.
- Do NOT create the three HTTP scripts (`vault-read.sh`, `vault-list.sh`, `prometheus-query.sh`) in this prompt; they are a sibling prompt's deliverable. The suite file in `<context>` is the only shared file, and it must not be duplicated.
- Do NOT add a `pod-logs.sh` or any pod-log/event read. That waits on the Config CRD gaining a service-account field and is an explicit non-goal.
- Do NOT change the `github.com/bborbe/agent` module. No `go get`, never `-mod=vendor`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass, and the healthcheck / oauth-probe routing is untouched.

</constraints>

<verification>

Run each command from the repo root. All must pass.

```bash
make test          # the new pkg/scripts suite runs here
make precommit     # AC1 — must exit 0

# AC9 (git half) — no push / commit / remote invocation anywhere in the scripts
! grep -nE 'git (push|commit|remote)' agent/scripts/*.sh

# AC9 (curl half) — no curl invocation carries a write verb. The pipeline must print 0.
test "$(sed -e ':a' -e '/\\$/N; s/\\\n//; ta' agent/scripts/*.sh | grep -E '(^|[^A-Za-z_])curl ' | grep -cE -- '(-X|--request|--upload-file| -T |--data([^-]|$)|--data-(binary|raw|ascii)|--json| -d | -F |--form)')" = "0"

# AC10 — no env knobs, no defaulted env vars. Both must find nothing.
! grep -qE 'REPO_CLONE_DIR|GIT_CLONE_DEPTH|GIT_CLONE_TOKEN|GATEWAY_SECRET' agent/scripts/*.sh
! grep -qE '\$\{[A-Z_]+:-' agent/scripts/*.sh

# the script exists and is executable
test -x agent/scripts/repo-clone.sh

# the clone directory is ignored, and the test suite left no tree behind
test "$(grep -c '^/agent/repos/' .gitignore)" = "1"
test -z "$(find agent/repos -mindepth 1 2>/dev/null)"
```

</verification>
