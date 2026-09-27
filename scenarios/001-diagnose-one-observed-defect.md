# Scenario 001: diagnose one observed defect

**Purpose**: prove the agent turns one operator observation into a `## Verdict` with a `file:line` that resolves, verified against the running system. Run before any deploy; run again after any non-trivial change.

**Setup**:
- Agent `bborbe/defect-diagnoser-agent` deployed to dev
- Config CRD applied: `kubectlnukedev -n dev get config.agent.benjamin-borbe.de defect-diagnoser-agent` shows it
- The applied Config's `ENV_CONTEXT` holds one `service_repo.<service>=<owner>/<repo>` entry for the defect's service (exact, case-sensitive name). This mapping is operator-supplied at apply time and is never committed to this repo's `k8s/` (see `docs/verdict-gate.md` § mapping input)
- One real, currently reproducible defect in a dev service (read paths 1–2 only: task files, Prometheus — pod logs wait on the platform change, see NEXT-DIRECTIONS)

**Acceptance**: the task ends `completed` with a `## Verdict` whose `file:line` exists at the cloned ref and whose `evidence` quotes a live read that shows the misbehaviour.

## Steps

1. **Create the task** (operator writes the task file directly — `vault-cli` has no task-create command; the intake-surface task will later replace this step with the one-gesture command):
   ```bash
   SLUG="<short-slug>"
   DEFECT_ID="defect-$(date +%Y%m%d)-${SLUG}"
   cat > ~/Documents/Obsidian/Personal/"25 Tasks/Defect ${SLUG}.md" <<EOF
   ---
   task_type: defect-diagnose
   assignee: defect-diagnoser-agent
   status: in_progress
   phase: planning
   defect_id: ${DEFECT_ID}
   ---

   ## Observation

   <what misbehaved, in the operator's own words, naming the running service>
   EOF
   ```
   (Write the heredoc lines without the list indentation.) The `## Observation` is the operator's own words. **No root cause, no suspected component, no `file:line`.** The service it names must match the `service_repo.` entry from Setup exactly.

2. **Observe controller pickup**:
   ```bash
   kubectlnukedev -n dev logs agent-task-controller-personal-0 --tail=200 | grep "<task_identifier>"
   ```
   Expected: the task is published within one poll cycle.

3. **Observe executor spawn**:
   ```bash
   kubectlnukedev -n dev get pods | grep defect-diagnoser-agent
   ```
   Expected: a Job pod appears within 30s of the publish.

4. **Verify the task file**:
   Expected:
   - Frontmatter `status: completed`. An escalation (`assignee: ""` with a `## Failure`) is a legitimate outcome of the diagnoser's escalation path, but it fails this happy-path scenario.
   - Body has `## Verdict` with a yaml block carrying `defect_id`, `service`, `repo`, `file:line`, `root_cause`, `recommended_fix`, `evidence`, `confidence: High`

## Pass criteria

- [ ] Task reaches `status: completed` with no `## Failure` section
- [ ] `file:line` resolves: `git -C <clone> show <ref>:<path> | sed -n '<line>p'` prints a non-empty line (the pod's clone is ephemeral, so `<clone>` is a local clone of the verdict's `repo`, and `<ref>` is the `head_sha` recorded in the task's `## Plan`)
- [ ] `evidence` quotes a script call **and its output**, and that output shows the observed misbehaviour
- [ ] The `## Observation` still contains no root cause, component, or `file:line` (operator supplied no diagnosis)
- [ ] No agent-pipeline alerts fired in the 10 min after completion

## Fail recovery

If the task fails (`## Failure`, or `assignee: ""`):
1. Read the `## Failure` / escalation reason for the error class
2. Common classes: transient infra (retry), missing script or env (preflight gap — see [[Fail-Fast Preflight for Tool-Dependent LLM Agents]]), service did not resolve to a repo (mapping gap — escalate, do not invent), defect not reproducible (expected `needs_input`)
3. Re-trigger: `vault-cli task set "<title>" assignee defect-diagnoser-agent`

## Related

- [[Defect Diagnoser Agent]] — agent knowledge page
- [[Defect Lane Task Chain]] — the contract this agent implements
- [[Agent Pipeline Debug Guide]]
