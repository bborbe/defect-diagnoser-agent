# Observation scripts — contracts

The diagnoser observes the running system only through these read-only scripts. **This page is the authoritative contract** for them; the upstream scripts they were ported from (named at the end) are informational only.

The Config's `ALLOWED_TOOLS` grants each script as `Bash(scripts/<name>.sh:*)`, and Claude invokes them relative to its working directory (the agent dir, `/agent` in the image). They live in `agent/scripts/` and ship through the Dockerfile's `COPY agent/ /agent/`.

Contract source: [[Defect Lane Task Chain]] § Live observation — read path 1 (task files) and read path 2 (metrics). Read path 3 (Kubernetes API) waits on a platform change and has no script.

## How the scripts get their env

The Claude runner passes only `HOME,PATH,USER,TZ,ZONEINFO,TMPDIR,LANG,LC_ALL` from the pod into the Claude subprocess. `GIT_REST_URL` and `PROMETHEUS_URL` reach the scripts through the existing `CLAUDE_ENV` channel (comma-separated `KEY=VALUE`, merged into `ClaudeRunnerConfig.Env` by both entry points):

- in-cluster: Config `spec.env.CLAUDE_ENV: "GIT_REST_URL=http://vault-obsidian-personal:9090,PROMETHEUS_URL=http://prometheus.monitoring:9090"`
- local CLI: `-claude-env='GIT_REST_URL=…,PROMETHEUS_URL=…'`

At `-v=2` the runner logs `cmd.Env = [...]`; both keys appear there when forwarding works.

## Common rules (all three HTTP scripts)

| Rule | Value |
|---|---|
| Method | `GET` only — no `-X`, `--request`, `-d`, `--data`, `--data-raw`, `--data-binary`, `--json`, `-F`, `--form`, `-T`, `--upload-file`; query parameters go through `-G --data-urlencode` |
| Timeout | `--max-time 30` |
| HTTP error | non-zero exit; stderr carries the HTTP status (e.g. `500`, `401`) — curl `-sS -f` or an explicit status check |
| Missing env var | non-zero exit; stderr names the variable (`<VAR> is required`) |
| Output cap | stdout capped at 65536 bytes; when cut, stderr contains `truncated` and the exit code is **0** (the cut must not surface as a pipefail / SIGPIPE failure) |
| Env | only the variable named below; no defaulted (`${VAR:-…}`) env knobs, no gateway secret, no tokens |

## `vault-read.sh <path>`

- One argument: a vault-relative file path.
- Rejected with exit 2 and no request: absolute path (leading `/`), any `..` segment (`../b.md`, `a/../b.md`, `a/..`), any `?`, `#` or `://`.
- Spaces are percent-encoded.
- Request: `GET ${GIT_REST_URL}/api/v1/files/<encoded path>`.
- Env: `GIT_REST_URL`.

## `vault-list.sh <glob>`

- One argument: a git-rest glob (`filepath.Match` semantics; `*` does not cross `/`; `**` unsupported).
- Request: `GET ${GIT_REST_URL}/api/v1/files/` with query parameter `glob=<glob>` (URL-encoded).
- stdout: one path per line; empty when nothing matches.
- Env: `GIT_REST_URL`.

## `prometheus-query.sh <promql>`

- Exactly one argument: a PromQL instant-query expression.
- Rejected with exit 2 and no request: an argument starting with `/` or containing `://`.
- Request: `GET ${PROMETHEUS_URL}/api/v1/query` with query parameter `query=<promql>` (URL-encoded). The path is fixed; no other Prometheus path is reachable. No `time` parameter — Prometheus evaluates at its own now.
- Env: `PROMETHEUS_URL`.

## `repo-clone.sh clone <repo>`

- The only subcommand is `clone`. Anything else (including `log`) → exit 2 with usage on stderr.
- `<repo>` accepts `https://github.com/<owner>/<name>[.git]`, `git@github.com:<owner>/<name>.git`, or `<owner>/<name>`. It is normalized to `https://github.com/<owner>/<name>`.
- Validation, before git runs or the filesystem is touched — exit 2 on failure:
  - `owner` and `name` each match `^[A-Za-z0-9._-]+$` (so `bborbe/.github` is valid)
  - neither `owner` nor `name` is `.` or `..` (so `../x`, `x/..`, `x/.`, `./.`, `../..` are invalid)
  - **defense in depth:** the destination `<agent dir>/repos/<owner>/<name>`, resolved to an absolute path with symlinks followed, lies strictly inside `<agent dir>/repos/`. This is checked before any `rm -rf` or `chmod`, and catches a pre-existing symlink under `repos/` pointing outside it.
- Clone root is fixed: the `repos/` directory beside `scripts/` (`/agent/repos` in the image; `agent/repos/` in a local checkout, gitignored). No env override.
- Clone: full history, no `--recurse-submodules`, no credentials (public repos only in v1).
- Replacing an existing clone first restores owner write permission on the old tree, then removes it — so re-cloning over a read-only tree succeeds for a non-root user.
- After cloning, the whole tree including `.git` is made non-writable (`chmod -R a-w`).
- stdout (flat `key=value`): `clone_path=<absolute path>`, `head_sha=<sha>`, `default_branch=<branch>`.
- `git clone` failure (any non-zero exit, e.g. 128) → non-zero exit, stderr `git clone failed for <url>`.

Cleanup after a local run: `chmod -R u+w agent/repos && rm -rf agent/repos`.

## Upstream (informational only)

Ported from `agent-sentry-issue-analyzer/scripts/vault-list.sh`, `agent-sentry-issue-analyzer/scripts/repo-clone.sh` (whose `..` traversal hole this contract closes), and `trading/agent/trade-analysis/agent/scripts/vault-read.sh`. `prometheus-query.sh` is new. Where this page and an upstream script disagree, this page wins.
