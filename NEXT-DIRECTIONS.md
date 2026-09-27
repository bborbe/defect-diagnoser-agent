# NEXT-DIRECTIONS — defect-diagnoser-agent

Deferred-not-cut work captured during `/launch-agent` on 2026-09-27. The contract lives in [[Defect Lane Task Chain]]; this file tracks what the code still lacks.

## v0 (shipped — initial scaffold)

- Scaffolded from `bborbe/agent-claude` template
- Config CRD rendered for dev (`k8s/defect-diagnoser-agent-config.yaml`) — **not applied**
- `taskTypes: [defect-diagnose, healthcheck]`
- Phases: planning → execution → ai_review (template runtime, prompts still generic)
- Smoke test: scenario 001

## v1 (next — the actual diagnoser)

- **Domain prompts** — planning (service → repo, read-only clone), execution (observe, localise, write `## Verdict`), ai_review (verdict vs observation)
  - **How**: `pkg/prompts/`, ship via this repo's dark-factory spec → prompts pipeline, never a direct edit
- **Read-only scripts** named in `ALLOWED_TOOLS`: `vault-read.sh`, `vault-list.sh` (git-rest), `prometheus-query.sh`, `repo-clone.sh`
  - **How**: port `repo-clone.sh` and `vault-*.sh` from `agent-sentry-issue-analyzer/scripts/`; write `prometheus-query.sh` read-only
- **Env forwarding** — `GIT_REST_URL`, `PROMETHEUS_URL` must be threaded via `ClaudeRunnerConfig.Env`; the runner strips pod env to an allowlist
- **Secret** `k8s/defect-diagnoser-agent-secret.yaml` — `ANTHROPIC_AUTH_TOKEN` (router token) via teamvault

## v2 — read path 3: pod logs and events

- **Why deferred**: the Config CRD has **no `serviceAccountName` field** (read live 2026-09-27), so the Job cannot run as a dedicated read-only SA
- **How**: platform change in `agent-task-executor` + CRD to accept a service account; then RBAC `defect-diagnoser` (get/list pods, pods/log, events, jobs, deployments — no secrets/configmaps/exec) and a `pod-logs.sh` script. RBAC is production-touching — operator-gated
- **Also**: create the stage-2 `defect-spec` task on completion (carrying `## Observation` verbatim) — shape waits on the page's open question 1

## Notes

- A general service → repo mapping is unowned (page open question 4); v1 uses what exists and escalates when a service does not resolve
