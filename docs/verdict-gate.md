# Verdict gate — deterministic checks the binary applies

The model writes the `## Verdict`; the binary decides whether it stands. This page is the authoritative rule set for the diagnoser (the prompt-generation container cannot read the vault, so everything the gate needs from the contract is copied here). Contract source: [[Defect Lane Task Chain]] § The contract and § Live observation → "Not reproducible → escalate, never guess" (amended 2026-09-27).

## The verdict block (copied from the contract)

A fenced yaml block inside the `## Verdict` section, keys in this order:

```yaml
defect_id: <copied from the task's frontmatter defect_id>
service: <the running service observed>
repo: <owner>/<repo>            # resolved in planning
file:line: <path>:<line>        # must resolve in the repo at the cloned ref
root_cause: <one paragraph>
recommended_fix: <one paragraph>
evidence: <the live observation — script call + its output>
confidence: High | Medium | Low
```

Escalation split (contract, amended 2026-09-27):

- no line can be cited → `needs_input`, **no** `## Verdict`
- a line is cited but `confidence` is Medium or Low → `## Verdict` written, `needs_input`
- stage 2 is only ever created from a `confidence: High` verdict

## Service → repo mapping input

The diagnoser owns no mapping data. The operator supplies entries through the existing `ENV_CONTEXT` channel, one per service:

```
ENV_CONTEXT=service_repo.<service>=<owner>/<repo>,service_repo.<other>=<owner>/<repo>
```

They render into the prompt's `## Environment` section as `service_repo.<service>: <owner>/<repo>`. A service resolves only on an **exact, case-sensitive** key match. No fuzzy match, no fallback, no repo from the model's own knowledge.

## Model-output heading rule — applied in every phase, first

After each model step reports `done`, and before any other check in that phase, the binary inspects the section the step just wrote:

| Phase | Section checked |
|---|---|
| planning | `## Plan` |
| execution | `## Verdict` |
| ai_review | `## Review` |

If the section body contains **any column-0 line starting with `#`**, the phase yields `failed`. The message names the section (e.g. `heading rule: section "## Plan" contains a column-0 heading line`) and **never echoes the offending line**. No message the binary generates ever has a line starting with `#` (see § Failure messages). The offending section is removed from the task's parsed sections before delivery, so the written task never carries it.

This one rule covers every model-written section. It replaces the earlier per-section checks (verdict body, plan, review).

**Why, and what is out of scope.** The root cause is a bborbe/agent framework defect: `Markdown.Marshal` writes model output verbatim, and the section splitter treats any column-0 `#` line as a heading while ignoring code fences. A model response containing a column-0 `## Verdict` line would otherwise become a second, forged section on the next parse. The defect is reported upstream, and fixing it is out of scope here. Inside this repo, heading injection is contained by four measures together:

- this heading rule
- § Failure messages (every `## Failure` line prefixed `> `, once)
- § Echoed values
- the read-only script allowlist

Each phase's instructions also tell the model never to start a line of its response with `#`, because the step supplies the heading.

## Verdict gate — rules a–k after execution, rules a–j again at ai_review

The gate parses the **last** fenced ```` ```yaml ```` block in `## Verdict`. Rules are checked in this order; the **first** rule that fires decides the outcome and is the only one reported. "Removed" means the gate deletes the `## Verdict` section from the task's parsed sections before delivery, so the delivered task file does not contain it.

| # | Rule | Outcome | `## Verdict` in delivered body |
|---|---|---|---|
| a | `## Verdict` section missing; no fenced yaml block; yaml does not parse; or any of `defect_id`, `service`, `repo`, `file:line`, `confidence` contains a newline | `failed` — message names the defect or the field, and **never echoes the offending value** | removed |
| b | `confidence` missing, empty, or not exactly `High`, `Medium` or `Low` (case-sensitive) | `failed` — message names `confidence` and the value, `%q`-quoted | removed |
| c | `confidence` is `Medium` or `Low` **and** `file:line` is empty or missing | `needs_input` — message `no line cited; evidence:` followed by the verdict's `evidence` as a quote block (see § Quoting evidence). This is the contract's "no line can be cited" case; the evidence survives in `## Failure` | removed |
| d | any other of the eight fields (`defect_id`, `service`, `repo`, `file:line`, `root_cause`, `recommended_fix`, `evidence`) missing or empty | `failed` — message names the field | removed |
| e | `file:line` not `<path>:<positive integer>` with a repo-relative path (no leading `/`, no `..` segment) | `failed` — message contains `file:line` | removed |
| f | `evidence` contains neither `scripts/vault-` nor `scripts/prometheus-query.sh` | `needs_input` — `evidence cites no live read` | removed |
| g | task frontmatter has no `defect_id`, or verdict `defect_id` ≠ frontmatter `defect_id` | `needs_input` — message names both values, each `%q`-quoted | removed |
| h | no `service_repo.<service>` entry for the verdict's `service` (exact, case-sensitive) | `needs_input` — `service %q does not resolve` | removed |
| i | verdict `repo` ≠ the mapped repo | `needs_input` — message names both repos, each `%q`-quoted | removed |
| j | `confidence` is `Medium` or `Low` (line cited, all else valid). The gate first runs the **line guard** (§ below) against the clone planning left in the pod | line guard fails → `needs_input` with the line-guard message (reason + quoted evidence); line guard passes → `needs_input` — `confidence <c> below High` | line guard fails → removed; passes → **kept** |
| k | otherwise (`confidence: High`, all valid) | `done`, advance to `ai_review` | kept |

**At ai_review.** First the heading rule runs on `## Review`. Then rules a–j run on the verdict, before the line guard, so a Job entering at `phase: ai_review` can never complete an ungated verdict. The first rule that fires decides, exactly as after execution (for example, a Medium verdict yields `below High`). Rule k at ai_review means "continue to the line guard".

**Intentional asymmetry:** `High` with an empty `file:line` is `failed` (rule d), not `needs_input`. A High-confidence verdict without a line is schema drift — the model contradicted itself — so it takes the retry path, unlike rule c, where a Medium/Low verdict with no line is the contract's honest "no line can be cited".

So a kept Medium/Low verdict always cites a line that exists in the clone. An absent clone (for example a pod that died after planning, with execution re-dispatched to a new pod before re-cloning) fails the guard as "file absent".

## Echoed values — security rule

Any value the gate echoes into a message (rules b, g, h, i and the line-guard `<path>`) is rendered on one line with Go `%q` quoting. Values of the five scalar fields that contain a newline never reach an echo: rule a rejects them first and names only the field. So no echoed value can start a column-0 line in the written task.

## Quoting evidence

Every gate or line-guard message that quotes the verdict's `evidence` (rule c, rule j's line-guard failure, the ai_review line guard) includes the evidence **raw**, after `evidence:`. It is not prefixed here: § Failure messages prefixes every message line exactly once, so quoted evidence arrives in the written task as `> <line>`, never `> > <line>`.

A `needs_input` or `failed` envelope returned by the model itself passes through with the same status and the same message content, and no `## Verdict` (the framework does not write the section for such envelopes). Its message is still prefixed per § Failure messages, because it can carry script output.

## Failure messages — security rule

**Every line of every message the agent delivers into `## Failure`** is prefixed with `> `. That covers messages the binary generates (gate rules, line guard, heading rule) and model-returned `needs_input` / `failed` envelopes it passes through. It is applied once, at the point the result leaves the agent, so no message line reaches the framework's fence at column 0.

Why: the framework writes the message verbatim inside a ```` ``` ```` fence, and its splitter ignores fences. Any message line starting with `#` would become a forged section, and a line of three backticks would close the fence early. This is the only prefixing step. It covers the whole family, including quoted evidence (§ Quoting evidence emits it raw) and model-authored text such as a `needs_input` message that lists what each script returned. Assertions that a message *contains* a phrase still hold, because the prefix keeps the text intact.

## Line guard — at ai_review after rules a–j pass, and inside rule j

The line guard only ever sees a verdict that already parsed and passed rules a–i. A missing or unparseable verdict is caught earlier, by rule a, as `failed`. Every failure below yields `needs_input` with `## Verdict` removed, and a message of the form `<reason>; evidence:` followed by the verdict's `evidence` as a quote block (§ Quoting evidence).

1. Resolve `<agent dir>/repos/<repo>/<path>` from the verdict's `repo` and `file:line`, following symlinks on every path component.
2. Resolved path outside `<agent dir>/repos/<repo>/` (symlinked file or symlinked directory escaping the clone) → reason `%q resolves outside the clone` (the cited path).
3. File absent → reason `%q not found in clone`.
4. File has fewer than `<line>` lines → reason `%q beyond end of file (<n> lines)` (the cited `path:line`).
5. Otherwise → pass. After ai_review: `done`, phase `done`, status `completed`. Inside rule j: continue to the `below High` escalation with the verdict kept.

A line-guard failure means the cited line cannot be shown to exist, which the contract treats as "no line can be cited" — so the verdict is removed, as in rule c, and its evidence survives in `## Failure`.

## Stale failure marker

The framework writes `## Failure` on `failed` / `needs_input` and never removes it. When the ai_review line guard passes (the task completes), the diagnoser removes any `## Failure` section from the delivered body, so a completed task never carries a failure from an earlier attempt. Git history of the task file keeps the earlier failure.
