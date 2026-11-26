---
status: completed
spec: [002-diagnose-observed-defect-with-verdict-gate]
summary: Added pkg/verdict with the model-output heading rule, deterministic verdict gate rules a-k, symlink-safe line guard and phase-recording step wrappers, locked down by 66 table-driven specs at 94.6% coverage.
execution_id: defect-diagnoser-agent-diagnose-v1-exec-004-spec-002-verdict-gate
dark-factory-version: v0.198.17
created: "2026-09-28T01:40:00Z"
queued: "2026-11-26T18:43:49Z"
started: "2026-11-26T18:47:06Z"
completed: "2026-11-26T18:57:44Z"
branch: dark-factory/diagnose-observed-defect-with-verdict-gate
---

# Add the deterministic verdict gate, the heading rule and the line guard

<summary>

- A diagnosis only counts when the code, not the model, says it is well formed.
- A verdict that is malformed, that names a field wrongly, or that contradicts the task never reaches the task file.
- A verdict that cites no line, invents a service or a repo, or cites no live read is escalated to the operator instead.
- A verdict below the top confidence level is escalated but kept, so the operator still sees the evidence.
- After review, the cited line is checked against the clone: the file must exist, must hold that line, and must not escape the clone through a symbolic link.
- A phase whose answer starts a line with a hash sign is rejected before anything else, so the model can never forge a section.
- Every reason the operator reads is quoted and prefixed, so a line of evidence can never forge a heading or close the framework's fence.
- A task that completes carries no failure left over from an earlier attempt.
- Each phase records itself, so an escalation leaves the task on the phase that escalated.
- The behaviour is locked down by table tests, one of which builds a real throwaway clone.

</summary>

<objective>

Give the defect diagnoser its deterministic referee. A model writes the `## Verdict`; this change makes the **binary** decide whether that verdict stands. Three binary checks are added as agent step wrappers, so they run inside the phase chain rather than beside it: the model-output heading rule (applied to whichever section the step just wrote), the verdict gate (rules a–j after execution and again at ai_review), and the line guard (the cited line must resolve inside the read-only clone, following symlinks on every path component). Every rejection removes the `## Verdict` section from the task's parsed sections and prefixes every line of the delivered message with `> `, so a hostile read can never forge a heading or close the framework's fence.

</objective>

<context>

Read these before writing anything:

- `specs/in-progress/002-diagnose-observed-defect-with-verdict-gate.md` — the spec this prompt implements. Read `## Desired Behavior` 1 and 4, `## Constraints`, `## Failure Modes`, `## Security / Abuse Cases` and `## Acceptance Criteria` AC4–AC7.
- `docs/verdict-gate.md` — the **authoritative rule set**. Read it in full. Where it and any other source disagree, it wins. Its rules, reason strings and quoting rules are reproduced verbatim in `<requirements>` below so you do not have to trust a grep.
- `specs/in-progress/001-read-only-observation-toolkit.md` — the dependency spec. It provides the four read-only scripts, the `GIT_REST_URL` / `PROMETHEUS_URL` forwarding and the tool allowlist. **Spec 001 must be merged before this prompt runs**; this prompt re-implements none of it.
- `prompts/completed/001-spec-001-http-observation-scripts.md` — house style for this repo: how verbatim contracts are inlined, how the shared Ginkgo suite file is written, how test harnesses are described.
- `pkg/factory/factory.go`, `pkg/factory/factory_test.go`, `pkg/factory/factory_suite_test.go` — the repo's Ginkgo/Gomega idiom, the external `_test` package convention, and the existing step construction you must stay compatible with.
- `pkg/prompts/prompts.go` — the current embedded-prompt builder (a later prompt in this spec changes it; do not change it here).
- `agent/.claude/CLAUDE.md` — the guardrails prose the model reads.
- `docs/dod.md` — the Definition of Done for this repo.
- `/home/node/.claude/CLAUDE.md` — the container's execution rules.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite and spec conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` API; never `fmt.Errorf`, never `context.Background()` inside `pkg/`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — linter limits that apply to test files too (`funlen` 80 lines / 50 statements, `gocognit` 20, `nestif` 4, golines max-len 100, `dupl`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — interface → constructor → struct, doc comments, error wrapping.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — gosec expectations.

**Framework facts, all verified while writing this prompt against `github.com/bborbe/agent v0.141.1` at `$(go env GOMODCACHE)/github.com/bborbe/agent@v0.141.1`.** Read these files before writing code:

- `agent_step.go` — `type Step interface { Name() string; ShouldRun(ctx, *Markdown) (bool, error); Run(ctx, *Markdown) (*Result, error) }`. A `Result` carries `Status AgentStatus`, `NextPhase string`, `Message string`. The framework marshals the mutated `*Markdown` after each step's `Run` and publishes it through the `ResultDeliverer`, so the `*Markdown` **is** the delivered body.
- `agent_markdown.go` — `type Markdown struct { Frontmatter TaskFrontmatter; Preamble string; Sections []Section }`, `type Section struct { Heading string; Body string }`, and `FindSection(heading string) (*Section, bool)`. `Sections` is an exported slice, so removal is a filter over it. There is **no** `RemoveSection` method.
- `agent_task-frontmatter.go` — `type TaskFrontmatter map[string]interface{}` with `func (f TaskFrontmatter) String(key string) (string, bool)`.
- `claude/agent-step.go` — `claude.NewAgentStep(claude.AgentStepConfig{Name, Runner, Instructions, EnvContext, OutputSection, NextPhase}) agentlib.Step`. On a `done` runner body it writes the section through `md.ReplaceSection(Section{Heading: cfg.OutputSection, Body: …})`, where the body is the envelope's `output` field passed through `stripLeadingSectionHeading`; when the response is not an envelope, or the envelope carries no `output`, the raw runner text is written unchanged (`claude/agent-step.go:277-278`). It returns `Result{Status: AgentStatusDone, NextPhase: cfg.NextPhase}`. On a runner body that parses to a `needs_input` / `failed` envelope it returns that status **without writing the section**. On a runner error it returns `Result{Status: AgentStatusFailed, Message: "<name> claude run failed: ..."}`.
- `agent_agent.go` — `Agent.Run` walks phases in-process: after a step returns `Done` with a `NextPhase` that names another phase on the same `Agent`, the next phase runs in the same process. It exits on any non-`Done` status.
- `delivery/content-generator.go` — `NewPassthroughContentGenerator()` writes `## Failure` from `result.Message` for `failed` / `needs_input`, inside a ```` ``` ```` fence, and applies `status` / `phase` frontmatter. `agentlib.AgentResultInfo` is `{Status, Output, Message, NextPhase, ContinueToNext, AgentTurns, InteractionCount}`.
- `mocks/claude-claude-runner.go` — the counterfeiter fake is importable as `agentmocks "github.com/bborbe/agent/mocks"` and offers `RunReturns(result *claude.ClaudeResult, err error)`, `RunReturnsOnCall(i int, result *claude.ClaudeResult, err error)`, `RunCallCount() int` and `RunArgsForCall(i int) (context.Context, string)`. Use it for every step test.
- `github.com/bborbe/errors` — `errors.Errorf(ctx, format, args...)` produces an error whose `Error()` is exactly `fmt.Sprintf(format, args...)`; `errors.Wrapf(ctx, err, format, args...)` wraps. Import as `"github.com/bborbe/errors"`.

**Two facts verified empirically for this prompt:**

1. `gopkg.in/yaml.v3` parses `file:line: pkg/x/y.go:42` with the key `file:line` and the value `pkg/x/y.go:42`, and parses a block scalar `confidence: |` + two indented lines into a value containing `\n`. A block scalar value ends with a trailing newline, so the newline check must be `strings.Contains(v, "\n")`.
2. `agentlib.ParseMarkdown` splits a body at every column-0 `## ` line. A `## Verdict` section whose body contains a column-0 `## Verdict` line therefore round-trips into two sections. This is exactly the forgery the heading rule must catch **before** the markdown is re-parsed, which is why the rule runs inside the step's `Run`.


<!-- Open questions for the reviewer — decisions taken here, each with the alternative:

     1. AC4 says every gate-failed row's message "names the field (or file:line)". The
        section-missing / no-yaml-block / unparseable-yaml rows have no field to name, so
        this prompt pins three explicit messages for them (`verdict section missing`,
        `verdict yaml block missing`, `verdict yaml does not parse`). Alternative: one
        generic message. Pinned messages are used so the table test can assert exactly one.

     2. Rule e's message deliberately does NOT echo the offending `file:line` value: the
        docs' echoed-value rule lists rules b, g, h, i and the line-guard path, not e. That
        is what makes AC4's "/abs/x.go:3 is absent from the delivered body" hold. The pinned
        message contains the literal `file:line` and nothing else from the value.

     3. docs/verdict-gate.md § Line guard describes reason 2 and 3 as quoting "the cited
        path" and reason 4 as quoting "the cited `path:line`". This prompt follows that
        literally: `resolves outside the clone` and `not found in clone` quote the path;
        `beyond end of file (<n> lines)` quotes the path:line. If the operator wants
        path:line everywhere, the two formats change together with the docs page.

     4. AC5 says the needs_input message's evidence lines are "each prefixed `> `", while
        docs § Failure messages says the prefix is "applied once, at the point the result
        leaves the agent". This prompt keeps `Decision.Message` raw and prefixes exactly once
        in the step wrapper, so the `> ` form is asserted on the delivered message. The gate
        table asserts the raw `evidence:` form; the step test asserts the prefixed form.

     5. AC7's "cited file sits in a directory that is itself a symlink resolving outside the
        clone" is only detectable when the target file exists outside the clone. The guard
        therefore also resolves the deepest existing ancestor, so a dangling escape still
        reports `resolves outside the clone` rather than `not found in clone`.

     6. The spec's Non-goal forbids "`service_repo.` entries in any Config or code file of
        this repo", while AC9 only greps `README.md` (>= 1) and `k8s/` (0 lines). The literal
        prefix `"service_repo."` is used in `pkg/verdict` as the lookup prefix for the keys
        the OPERATOR supplies — it is the mechanism, not a mapping entry, and no key/value
        pair is hard-coded. Reviewer: confirm this reading; the alternative is deriving the
        prefix from the env context, which cannot distinguish the mapping keys from any
        other ENV_CONTEXT key.

     7. `Gate` is a concrete exported struct with a `New*` constructor rather than a public
        interface. The gate is deterministic and cheap, so every test drives the real
        implementation and an interface would have no fake and no second implementation.
        Reviewer: confirm this is acceptable against the repo's interface-per-collaborator
        convention; if not, add `//counterfeiter:generate` and a fake with a consumer. -->

</context>

<requirements>

Create a new package `pkg/verdict`. It owns every binary check this prompt adds. Nothing else in the repo changes: `pkg/factory` is wired in the last prompt of this spec, and `pkg/prompts` is a sibling prompt's deliverable.

**The contract, verbatim from `docs/verdict-gate.md`.** Do not paraphrase these strings — the tests assert them character for character.

The `## Verdict` block is a fenced yaml block inside the `## Verdict` section, keys in this order:

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

The gate parses the **last** fenced ```` ```yaml ```` block in `## Verdict`. Rules are checked in this order; the **first** rule that fires decides the outcome and is the only one reported. "Removed" means the gate deletes the `## Verdict` section from the task's parsed sections before delivery, so the delivered task file does not contain it.

| # | Rule | Outcome | `## Verdict` in delivered body |
|---|---|---|---|
| a | `## Verdict` section missing; no fenced yaml block; yaml does not parse; or any of `defect_id`, `service`, `repo`, `file:line`, `confidence` contains a newline | `failed` — message names the defect or the field, and **never echoes the offending value** | removed |
| b | `confidence` missing, empty, or not exactly `High`, `Medium` or `Low` (case-sensitive) | `failed` — message names `confidence` and the value, `%q`-quoted | removed |
| c | `confidence` is `Medium` or `Low` **and** `file:line` is empty or missing | `needs_input` — message `no line cited; evidence:` followed by the verdict's `evidence` as a quote block | removed |
| d | any other of the eight fields (`defect_id`, `service`, `repo`, `file:line`, `root_cause`, `recommended_fix`, `evidence`) missing or empty | `failed` — message names the field | removed |
| e | `file:line` not `<path>:<positive integer>` with a repo-relative path (no leading `/`, no `..` segment) | `failed` — message contains `file:line` | removed |
| f | `evidence` contains neither `scripts/vault-` nor `scripts/prometheus-query.sh` | `needs_input` — `evidence cites no live read` | removed |
| g | task frontmatter has no `defect_id`, or verdict `defect_id` ≠ frontmatter `defect_id` | `needs_input` — message names both values, each `%q`-quoted | removed |
| h | no `service_repo.<service>` entry for the verdict's `service` (exact, case-sensitive) | `needs_input` — `service %q does not resolve` | removed |
| i | verdict `repo` ≠ the mapped repo | `needs_input` — message names both repos, each `%q`-quoted | removed |
| j | `confidence` is `Medium` or `Low` (line cited, all else valid). The gate first runs the **line guard** against the clone planning left in the pod | line guard fails → `needs_input` with the line-guard message (reason + quoted evidence); line guard passes → `needs_input` — `confidence <c> below High` | line guard fails → removed; passes → **kept** |
| k | otherwise (`confidence: High`, all valid) | `done`, advance to `ai_review` | kept |

**Model-output heading rule — applied in every phase, first.** After each model step reports `done`, and before any other check in that phase, the binary inspects the section the step just wrote: planning → `## Plan`, execution → `## Verdict`, ai_review → `## Review`. If the section body contains **any column-0 line starting with `#`** — not only `# ` and `## `, but any `#` — the phase yields `failed`. The message names the section and **never echoes the offending line**; use exactly:

```
heading rule: section %q contains a column-0 heading line
```

so the message reads e.g. `heading rule: section "## Plan" contains a column-0 heading line`. The offending section is removed from the task's parsed sections before delivery.

**Echoed values — security rule.** Any value the gate echoes into a message (rules b, g, h, i and the line-guard path) is rendered on one line with Go `%q` quoting. Values of the five scalar fields that contain a newline never reach an echo: rule a rejects them first and names only the field. No echoed value can therefore start a column-0 line in the written task.

**Quoting evidence.** Every gate or line-guard message that quotes the verdict's `evidence` (rule c, rule j's line-guard failure, the ai_review line guard) includes the evidence **raw**, after `evidence:`. It is **not** prefixed at that point: the failure-message rule below prefixes every message line exactly once, so quoted evidence arrives in the written task as `> <line>`, never `> > <line>`.

**Failure messages — security rule.** **Every line of every message the agent delivers into `## Failure`** is prefixed with `> `. That covers messages the binary generates (gate rules, line guard, heading rule) and model-returned `needs_input` / `failed` envelopes it passes through. It is applied once, at the point the result leaves the agent. Assertions that a message *contains* a phrase still hold, because the prefix keeps the text intact.

**Line guard.** The line guard only ever sees a verdict that already parsed and passed rules a–i. A missing or unparseable verdict is caught earlier, by rule a, as `failed`. Every failure yields `needs_input` with `## Verdict` removed, and a message of the form `<reason>; evidence:` followed by the verdict's `evidence` as a quote block. Steps:

1. Resolve `<agent dir>/repos/<repo>/<path>` from the verdict's `repo` and `file:line`, following symlinks on every path component.
2. Resolved path outside `<agent dir>/repos/<repo>/` (symlinked file or symlinked directory escaping the clone) → reason `%q resolves outside the clone` (the cited path).
3. File absent → reason `%q not found in clone`.
4. File has fewer than `<line>` lines → reason `%q beyond end of file (<n> lines)` (the cited `path:line`).
5. Otherwise → pass. After ai_review: `done`, phase `done`, status `completed`. Inside rule j: continue to the `below High` escalation with the verdict kept.

**Stale failure marker.** The framework writes `## Failure` on `failed` / `needs_input` and never removes it. When the ai_review line guard passes (the task completes), the diagnoser removes any `## Failure` section from the delivered body, so a completed task never carries a failure from an earlier attempt.

**Pinned reason strings.** Use these exact formats, so the tests can assert them:

| Rule | Message format |
|---|---|
| a, section missing | `verdict section missing` |
| a, no fenced yaml block | `verdict yaml block missing` |
| a, yaml does not parse | `verdict yaml does not parse` |
| a, newline in a scalar field | `field %q contains a newline` (the field name, `%q`-quoted) |
| b | `confidence %q is not one of High, Medium, Low` (the value, `%q`-quoted) |
| c | `no line cited; evidence:` + `"\n"` + the verdict's `evidence`, raw |
| d | `field %q is missing or empty` (the field name, `%q`-quoted) |
| e | `file:line is not a repo-relative <path>:<positive line>` |
| f | `evidence cites no live read` |
| g | `defect_id mismatch: verdict %q, frontmatter %q` |
| h | `service %q does not resolve` |
| i | `repo %q does not match mapped repo %q` |
| j, guard fails | `<reason>; evidence:` + `"\n"` + the verdict's `evidence`, raw |
| j, guard passes | `confidence Medium below High` (the verdict's own confidence value, unquoted) |

For rule g, when the frontmatter has no `defect_id`, render the frontmatter value as `""` — the same message shape covers both the absent and the mismatched case.

1. **Package layout.** Create exactly these files:

   - `pkg/verdict/verdict.go` — the contract's field names and confidence values, the `Verdict` type, and the `## Verdict` yaml-block extraction.
   - `pkg/verdict/gate.go` — `Decision`, `Gate`, `NewGate`, `Evaluate`, `Review`, and one small helper per rule.
   - `pkg/verdict/lineguard.go` — `CheckLineGuard` and the `file:line` parser.
   - `pkg/verdict/step.go` — `PrefixMessage` and the three step constructors.
   - `pkg/verdict/verdict_suite_test.go` — a Ginkgo suite file, `package verdict_test`, with exactly the shape of `pkg/factory/factory_suite_test.go` (`time.Local = time.UTC`, `format.TruncatedDiff = false`, `RegisterFailHandler(Fail)`, `GinkgoConfiguration()` with a 60 s timeout, `RunSpecs(t, "Verdict Suite", …)`).
   - `pkg/verdict/verdict_test.go`, `pkg/verdict/gate_test.go`, `pkg/verdict/lineguard_test.go`, `pkg/verdict/step_test.go` — all `package verdict_test`.

   Do **not** create a second suite file, and do **not** add a non-test `.go` file to a package that would then contain only tests.

2. **Pinned API.** These names and signatures are what the last prompt of this spec wires into `pkg/factory`. Do not rename them.

   ```go
   // Field names, in the contract's order.
   const (
       FieldDefectID       = "defect_id"
       FieldService        = "service"
       FieldRepo           = "repo"
       FieldFileLine       = "file:line"
       FieldRootCause      = "root_cause"
       FieldRecommendedFix = "recommended_fix"
       FieldEvidence       = "evidence"
       FieldConfidence     = "confidence"
   )

   // Confidence values, case-sensitive.
   const (
       ConfidenceHigh   = "High"
       ConfidenceMedium = "Medium"
       ConfidenceLow    = "Low"
   )

   // VerdictFieldOrder is the eight field names in the contract's order.
   var VerdictFieldOrder = []string{FieldDefectID, FieldService, FieldRepo, FieldFileLine, FieldRootCause, FieldRecommendedFix, FieldEvidence, FieldConfidence}

   // Verdict is the parsed ## Verdict contract block.
   type Verdict struct {
       DefectID       string
       Service        string
       Repo           string
       FileLine       string
       RootCause      string
       RecommendedFix string
       Evidence       string
       Confidence     string
   }

   // Decision is the outcome of one gate evaluation.
   type Decision struct {
       Status  agentlib.AgentStatus // Failed, NeedsInput, or Done
       Message string               // binary-generated; NOT yet "> "-prefixed
   }

   // NewGate constructs the gate. envContext carries the operator-supplied
   // service_repo.<service> entries. cloneRoot is <agent dir>/repos.
   func NewGate(envContext map[string]string, cloneRoot string) *Gate

   // Evaluate applies gate rules a–j to md after the execution step, in order.
   // Done means rules a–j passed (rule k): the caller advances to ai_review.
   func (g *Gate) Evaluate(ctx context.Context, md *agentlib.Markdown) Decision

   // Review applies gate rules a–j to md again — so a Job entering at phase:
   // ai_review can never complete an ungated verdict — and, when they pass, the
   // line guard. Done means the cited line resolves: the caller completes.
   func (g *Gate) Review(ctx context.Context, md *agentlib.Markdown) Decision

   // CheckLineGuard reports whether fileLine can be shown to exist inside the
   // read-only clone rooted at cloneRoot. nil means the line resolves.
   func CheckLineGuard(ctx context.Context, cloneRoot, repo, fileLine string) error

   // PrefixMessage prefixes every line of message with "> ".
   func PrefixMessage(message string) string

   // Step constructors. Each records its phase on entry, delegates ShouldRun to
   // inner, and prefixes its result message per the failure-message rule.
   func NewPlanningStep(inner agentlib.Step) agentlib.Step
   func NewExecutionStep(inner agentlib.Step, gate *Gate) agentlib.Step
   func NewAIReviewStep(inner agentlib.Step, gate *Gate) agentlib.Step
   ```

   `Gate` is a concrete exported struct with a `New*` constructor. **Introduce no new interface in this package** — the gate and the line guard are deterministic and cheap, so every test drives the real implementation, and an interface with no fake and no second implementation would be dead surface. (No `//counterfeiter:generate` directive is needed; `make generate` must keep working untouched.)

3. **Yaml-block extraction and parsing** (`pkg/verdict/verdict.go`). Keep the parse outcome explicit so rule a can name which failure it hit:

   ```go
   // parseOutcome distinguishes rule a's four failures.
   type parseOutcome int

   const (
       parseOK parseOutcome = iota
       parseNoSection
       parseNoBlock
       parseInvalidYAML
   )

   // fields holds the ## Verdict yaml block's values keyed by the contract's field
   // names. A key is absent when the field is missing from the block; a nil yaml
   // value is normalised to the empty string (never the literal "<nil>").
   type fields map[string]string

   // extractFields reads the last fenced ```yaml block of the ## Verdict section.
   func extractFields(ctx context.Context, md *agentlib.Markdown) (fields, parseOutcome)
   ```

   - The fenced block opens on a line whose trimmed content is exactly ```` ```yaml ```` and closes on the next line whose trimmed content is ```` ``` ````. Take the **last** such block. A block that is opened and never closed counts as no block.
   - Unmarshal the block with `gopkg.in/yaml.v3` into `map[string]any`. A yaml error means `parseInvalidYAML`.
   - Normalise: for each of the eight field names, convert a present value to a string with `fmt.Sprintf("%v", v)`, but treat a `nil` value as the empty string. A key that is absent from the map stays absent from `fields`.
   - `VerdictFieldOrder` is the single source of the field order; use it wherever the eight names are needed.

4. **The gate** (`pkg/verdict/gate.go`). `NewGate` stores the env context and the clone root. `Evaluate` and `Review` share one ordered rule walk; `Review` is `Evaluate` plus the line guard on the rule-k path. Implement rules a–j in the documented order and return the first that fires.

   - **Rule a.** `parseNoSection` → `verdict section missing`. `parseNoBlock` → `verdict yaml block missing`. `parseInvalidYAML` → `verdict yaml does not parse`. Then, in `VerdictFieldOrder` order, check only `defect_id`, `service`, `repo`, `file:line`, `confidence` for `strings.Contains(v, "\n")` → `field %q contains a newline`, naming the field and never the value. All four are `failed` with the verdict removed.
   - **Rule b.** `confidence` missing or not exactly one of `High`, `Medium`, `Low` (case-sensitive) → `failed`, message `confidence %q is not one of High, Medium, Low`, the value `%q`-quoted. This covers missing, empty, `high` and `Certain`.
   - **Rule c.** `confidence` is `Medium` or `Low` **and** `file:line` is missing or empty → `needs_input` with the `no line cited; evidence:` message and the verdict removed.
   - **Rule d.** any of `defect_id`, `service`, `repo`, `file:line`, `root_cause`, `recommended_fix`, `evidence` missing or empty → `failed`, `field %q is missing or empty`, verdict removed. Note the deliberate asymmetry the docs call out: `High` with an empty `file:line` is rule d (`failed`), not rule c (`needs_input`).
   - **Rule e.** `file:line` is not `<path>:<positive integer>` with a repo-relative path → `failed`, message containing `file:line` and **not** echoing the value, verdict removed. Validate: split on the **last** `:`; the path is non-empty, does not start with `/`, and has no `..` segment; the line parses with `strconv.Atoi` and is greater than zero.
   - **Rule f.** `evidence` contains neither `scripts/vault-` nor `scripts/prometheus-query.sh` → `needs_input`, `evidence cites no live read`, verdict removed.
   - **Rule g.** read the frontmatter `defect_id` with `md.Frontmatter.String(FieldDefectID)`. Missing, or different from the verdict's → `needs_input`, `defect_id mismatch: verdict %q, frontmatter %q`, verdict removed.
   - **Rule h.** look up the exact key `"service_repo." + verdict.Service` in the env context (case-sensitive, no fuzzy match, no fallback). Absent → `needs_input`, `service %q does not resolve`, verdict removed.
   - **Rule i.** the mapped repo differs from `verdict.Repo` → `needs_input`, `repo %q does not match mapped repo %q`, verdict removed.
   - **Rule j.** `confidence` is `Medium` or `Low` (the line is present and everything else valid). Run `CheckLineGuard` first. On failure → `needs_input`, `<reason>; evidence:` + the raw evidence, verdict removed. On success → `needs_input`, `confidence Medium below High` (the verdict's own confidence value, unquoted), verdict **kept**.
   - **Rule k.** otherwise (`confidence: High`, everything valid) → `Decision{Status: agentlib.AgentStatusDone}`, verdict kept. In `Review` this means "continue to the line guard": run `CheckLineGuard`; on failure → `needs_input`, `<reason>; evidence:` + the raw evidence, verdict removed; on success → `Decision{Status: agentlib.AgentStatusDone}`.
   - **Removal is a filter over `md.Sections`** — never string surgery on the marshaled document, and never a copy. Remove **every** section whose `Heading` is `## Verdict`. The task file must not contain it after delivery.
   - Build the message strings with `errors.Errorf` / `errors.Wrapf` from `github.com/bborbe/errors`, never `fmt.Errorf`. One helper per rule keeps every function inside `funlen` (80 lines / 50 statements) and `gocognit` (20).

5. **The line guard** (`pkg/verdict/lineguard.go`). `CheckLineGuard(ctx, cloneRoot, repo, fileLine)` returns `nil` when the cited line resolves and otherwise an error whose `Error()` is exactly the documented reason. Resolve symlinks on **every** path component, and make the escape check happen even when the leaf does not exist:

   1. `repoRoot := filepath.Join(cloneRoot, repo)`; resolve it with `filepath.EvalSymlinks`. If that fails (the clone or the repo directory is absent), fall back to `filepath.Clean(repoRoot)`.
   2. `target := filepath.Join(resolvedRepoRoot, path)`, where `path` is the path half of `file:line`. Rule e has already guaranteed no leading `/` and no `..` segment, so this cannot walk out of the repo root by itself.
   3. `resolved, err := filepath.EvalSymlinks(target)`. On success: if `resolved` is not `resolvedRepoRoot` plus a path separator prefix → `errors.Errorf(ctx, "%q resolves outside the clone", path)`. Otherwise count the file's lines.
   4. On an `EvalSymlinks` error, find the deepest existing ancestor of `target`, resolve **it** with `filepath.EvalSymlinks`, and apply the same prefix check. An escape → `%q resolves outside the clone`; otherwise → `errors.Errorf(ctx, "%q not found in clone", path)`.
   5. **Line count.** Read the resolved file and count `\n`-separated lines, with a single trailing newline not counted as an extra line: `len(strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"))`. When the count is less than the cited line number → `errors.Errorf(ctx, "%q beyond end of file (%d lines)", fileLine, n)` — the reason quotes the cited `path:line`, not just the path.
   6. Otherwise return `nil`. A read error on a file that exists is a genuine I/O failure: return it wrapped, not as one of the three reasons.

   Use `filepath.EvalSymlinks` and `os` / `strings` only. Do not `os.Stat` a path before resolving it, and do not follow a symlink with anything that writes.

6. **The step wrappers** (`pkg/verdict/step.go`). Each wrapper embeds the inner `agentlib.Step`, delegates `Name()` and `ShouldRun()` to it, and implements `Run` as:

   1. **Record the phase first.** Set `md.Frontmatter["phase"]` to this phase's canonical string before calling the inner step, so an escalation — including one the inner step raises — leaves the task on the phase that escalated. Use the `domain.TaskPhasePlanning` / `domain.TaskPhaseExecution` / `domain.TaskPhaseAIReview` constants from `github.com/bborbe/vault-cli/pkg/domain`.
   2. Call `inner.Run(ctx, md)`.
   3. If the inner result's `Status` is **not** `agentlib.AgentStatusDone`, pass it through unchanged except for the message prefixing in step 4 below. No heading rule, no gate.
   4. On `Done`, apply the heading rule to this phase's section first — `## Plan` for planning, `## Verdict` for execution, `## Review` for ai_review. If the section body has any column-0 line starting with `#`, return `Result{Status: agentlib.AgentStatusFailed, Message: <heading-rule message>}` and remove that section from `md.Sections`. Never echo the offending line.
   5. Otherwise, on `Done`:
      - **planning** — return the inner result unchanged.
      - **execution** — call `gate.Evaluate(ctx, md)`. `Done` means advance: return the inner result (which already carries `NextPhase`). `Failed` / `NeedsInput` → return `Result{Status: decision.Status, Message: decision.Message}`. The gate has already removed the section when the rule says so.
      - **ai_review** — call `gate.Review(ctx, md)`. `Done` means the line guard passed: remove **every** `## Failure` section from `md.Sections`, then return `Result{Status: agentlib.AgentStatusDone, NextPhase: "done"}`. `Failed` / `NeedsInput` → return `Result{Status: decision.Status, Message: decision.Message}`.
   6. **Prefix once, at the point the result leaves the step.** When the result being returned has `Status` `agentlib.AgentStatusFailed` or `agentlib.AgentStatusNeedsInput`, replace its `Message` with `PrefixMessage(message)`. Apply it to messages the gate generated **and** to messages the inner step passed through from the model. Do not prefix a `Done` message. Do not prefix twice — a line already starting with `> ` must not become `> > `.
   7. `PrefixMessage` splits on `"\n"`, prefixes every element with `> `, and joins with `"\n"`. It is the only prefixing step in the package.

7. **Shared test fixtures.** Put these in `pkg/verdict/verdict_test.go` and reuse them everywhere; factor them into helpers so no `It` body is a copy of another (`dupl` is enabled and analyses test files).

   A canonical valid verdict, used as the base for every table row that changes one thing:

   ```yaml
   defect_id: defect-1
   service: checkout
   repo: bborbe/foo
   file:line: pkg/x/y.go:3
   root_cause: the handler dereferences a nil pointer
   recommended_fix: guard the nil pointer
   evidence: |
     scripts/prometheus-query.sh 'rate(http_requests_total{code="500"}[5m])'
     {"status":"success"}
   confidence: High
   ```

   A canonical task document whose frontmatter carries `defect_id: defect-1`, and whose body carries `## Observation` and a `## Verdict` section holding the fenced block above. Build it with `agentlib.ParseMarkdown(ctx, content)`; every gate test then mutates the `*agentlib.Markdown` and calls `Evaluate` / `Review`. Assert on `Decision.Status`, on `Decision.Message` substrings, and on the marshaled `md` (`grep -c '^## Verdict'` over the marshaled string, written as a Go helper).

   The gate fixture env context maps one service: `{"service_repo.checkout": "bborbe/foo"}`. `NewGate` takes it plus a clone root; for the gate tables the clone root may be a temp directory that holds a real `bborbe/foo/pkg/x/y.go` with at least three lines, so rule j's guard has something to pass against.

8. **Gate `failed` table** — one `DescribeTable` in `pkg/verdict/gate_test.go` with one entry per row below. Every row asserts `Decision.Status == agentlib.AgentStatusFailed`, a message naming the field (or `file:line`, or the documented defect), and `grep -c '^## Verdict'` equal to 0 on the marshaled document.

   - the `## Verdict` section removed entirely; a `## Verdict` section whose body has no fenced ```` ```yaml ```` block; a `## Verdict` section whose block does not parse.
   - a `## Verdict` body carrying a valid block **plus** a column-0 `## Notes` line → `failed` naming `## Verdict` (the heading rule, which runs before rule a). Assert the message is `heading rule: section "## Verdict" contains a column-0 heading line`.
   - each of `defect_id`, `service`, `repo`, `file:line` and `confidence` given as a yaml block scalar spanning two lines → `failed` naming that field, and the message does **not** contain the value. For the `confidence` row make the block scalar's second line `## Review`, and additionally assert `grep -c '^## Review'` is 0 on the marshaled document.
   - each of the seven non-confidence fields missing → `failed` naming it. Include `confidence: High` with an empty `file:line`, which must yield `failed` (rule d) and **not** `needs_input`.
   - `confidence` of `high`, `Certain`, and empty → `failed` naming `confidence` with the value `%q`-quoted.
   - `file:line` of `pkg/x.go`, `pkg/x.go:0`, `pkg/x.go:-3`, `pkg/x.go:abc`, `/abs/x.go:3` and `a/../x.go:3` → `failed` with a message containing `file:line`. For the `/abs/x.go:3` row additionally assert the string `/abs/x.go:3` is absent from the marshaled document.
   - passing row: `file:line: pkg/x/y.go:42` with everything else valid → `Decision.Status == agentlib.AgentStatusDone` and `## Verdict` kept (rule e did not fire).

9. **Gate `needs_input` table** — a second `DescribeTable` in `pkg/verdict/gate_test.go` covering rules c and f–j:

   - `Low` with an empty `file:line`, and `Medium` with an empty `file:line` → `needs_input` whose message contains `no line cited; evidence:` followed by every line of the fixture's `evidence`; `## Verdict` removed.
   - **Heading injection through evidence.** Rule c with an `evidence` whose value contains a line `## Verdict` and a line of three backticks. The gate's message quotes the evidence raw, and after `PrefixMessage` no line of the message starts with `#` and no line is a bare fence. Assert both.
   - `evidence` naming neither `scripts/vault-` nor `scripts/prometheus-query.sh` → `needs_input` containing `evidence cites no live read`; `## Verdict` removed.
   - frontmatter without `defect_id` → `needs_input`; and a verdict `defect_id` differing from the frontmatter's → `needs_input` naming both. Both remove `## Verdict`.
   - service `Agent-Task-Controller` against the mapping key `service_repo.agent-task-controller` → `needs_input` containing `does not resolve` (the match is exact and case-sensitive); `## Verdict` removed.
   - a `repo` different from the mapped repo → `needs_input` naming both; `## Verdict` removed.
   - `Low` with a valid line that exists in the fixture clone → `needs_input` containing `below High`, with `## Verdict` **kept**.
   - `Low` with a line past the end of the fixture file → `needs_input` containing `beyond end of file` and the quoted evidence; `## Verdict` removed (rule j's own line guard).

10. **Gate precedence and model passthrough** — a third `DescribeTable`:

    - a verdict missing `root_cause` whose `file:line` is also `/abs/x.go:3` → exactly one rule reported, `root_cause` (rule d fires before rule e). Assert the message names `root_cause` and contains neither `file:line` nor `/abs/x.go:3`.
    - a `Medium` verdict with an empty `file:line` that is also missing `root_cause` → `needs_input` containing `no line cited`, **not** `failed` (rule c fires before rule d).
    - a model envelope `{"status":"needs_input","message":"…"}` and a model envelope `{"status":"failed","message":"…"}` each pass through the step with their status unchanged, and the delivered message contains each line of the model's message prefixed `> `. Drive these through a real `claude.NewAgentStep` with an `agentmocks.ClaudeRunner`, wrapped by the matching step constructor.
    - a model `needs_input` envelope whose message contains a line `## Verdict` and a line of three backticks → after the wrapper, every line of the result message starts with `> `.

    For every step test, build the inner step with `claude.NewAgentStep(claude.AgentStepConfig{Name: …, Runner: fake, Instructions: nil, OutputSection: …, NextPhase: …})` and call `step.Run(ctx, md)` directly. Assert `result.Status`, `result.Message` and the marshaled `md`. Where a row is about what the operator finally reads, additionally run the real `delivery.NewPassthroughContentGenerator()` with `agentlib.AgentResultInfo{Status: result.Status, Message: result.Message, Output: marshaled}` and assert on the generated document — that is the delivered body, including the `## Failure` fence.

11. **ai_review line guard, against a real fixture clone** (`pkg/verdict/lineguard_test.go`). Build the clone with `t.TempDir()` / `os.MkdirAll` and `os.WriteFile` — **never** under `agent/repos/`, which a sibling spec's tests own and which two concurrent runs would corrupt. Assert directly on `CheckLineGuard` for the three reasons, and through `NewAIReviewStep` + `agentmocks.ClaudeRunner` for the delivered outcome:

    - a `## Verdict` that is missing, or whose yaml does not parse, never reaches the guard: at ai_review it yields `failed` from rule a, with `## Verdict` removed.
    - the path is absent → `needs_input`, `%q not found in clone`, `## Verdict` removed, and the message ends with `evidence:` followed by the fixture's evidence lines.
    - the cited line is greater than the file's line count → `needs_input`, `beyond end of file` plus the count, `## Verdict` removed, evidence quoted.
    - the cited file is a symlink that resolves outside the clone → `needs_input`, `resolves outside the clone`, `## Verdict` removed, evidence quoted.
    - the cited file sits in a directory that is itself a symlink resolving outside the clone → the same outcome. Create the escaping target under a **second** `t.TempDir()` and assert the sentinel file it holds is untouched.
    - passing row: a line within the file yields `done`, `phase: done` and `## Verdict` kept.

    For each failing row assert `grep -c '^## Verdict'` is 0 on the marshaled document, and assert the reason phrase appears in the prefixed message.

12. **Heading rule and step wiring tests** (`pkg/verdict/step_test.go`), driven by `agentmocks.ClaudeRunner`:

    - planning: a runner response containing a column-0 `## Verdict` line → `failed` naming `## Plan`, no `## Plan` section left, and no line of the message starts with `#`.
    - execution: a runner response carrying a valid verdict block plus a column-0 `## Verdict` line → `failed` naming `## Verdict`, and no `## Verdict` section left.
    - ai_review: a runner response containing a column-0 `## Verdict` line → `failed` naming `## Review`, no `## Review` section left, and exactly one `^## Verdict` line in the marshaled document (the gated verdict the execution phase wrote).
    - each of the three phases, on a `failed` or `needs_input` outcome, has left `md.Frontmatter["phase"]` set to that phase's own name.
    - ai_review on a passing review removes a pre-existing `## Failure` section and returns `NextPhase: "done"`.
    - `PrefixMessage` unit cases: a single line, a multi-line message, a message with a trailing newline, and a message that is empty. Each line of the result starts with `> `, and no line starts with `> > `.

13. **Coverage.** New code must reach at least 80 % statement coverage. Check with `go test -mod=mod -coverprofile=/tmp/verdict-cover.out ./pkg/verdict/...` then `go tool cover -func=/tmp/verdict-cover.out`. Test error paths, not only happy paths — every rule's `failed` and `needs_input` branch, every line-guard reason, and the `PrefixMessage` edge cases.

14. **Self-check before finishing.** Re-run `<verification>` and confirm every line passes. Walk requirements 1–13 against the code you wrote: confirm every reason string in the pinned table appears character for character, confirm every "removed" outcome really filters `md.Sections` rather than a string copy, and confirm no message the binary generates can put a `#` at column 0. Then walk the spec's `## Acceptance Criteria` AC4–AC7 row by row against your test table and confirm each row is actually asserted — a table entry that asserts nothing about status or body does not count.

</requirements>

<constraints>

- **Contract** ([[Defect Lane Task Chain]], amended 2026-09-27; copied into `docs/verdict-gate.md` because the container cannot read the vault). The following are fixed: the `## Verdict` field names and their order; the `confidence` values `High | Medium | Low`; and the escalation split (no line → no Verdict; line + Medium/Low → Verdict kept; stage 2 only from High). `## Observation` is read and never rewritten.
- **Gate rules are frozen in `docs/verdict-gate.md`.** A rule change updates that page in the same change. Do not change that page in this prompt.
- **Frozen envelope:** the final-response JSON `{status, message, files}` that the delivery layer parses is unchanged.
- **Escalation mechanics are the framework's.** `needs_input` clears `assignee`, sets `previous_assignee` and writes `## Failure`. `failed` keeps the assignee for the trigger-count retry. Do not reimplement any of it.
- **Phase cursor.** The diagnoser writes no frontmatter field of its own except `phase`: on entering each phase it sets the parsed frontmatter `phase` to that phase. bborbe/agent v0.141.1 parses the frontmatter once per Job and a needs_input/failed delivery keeps that parsed phase, so without this an escalation after an in-process advance writes back the Job's entry phase.
- **Do NOT add a confidence-threshold knob and do NOT add a gate opt-out flag.** `High`-only is the contract. This is an explicit spec Non-goal; if a future consumer demands variation, that is a separate spec.
- **Do NOT add any `service_repo.` mapping entry** to this repo's Config, code, tests or docs. The mapping is operator-supplied input delivered through `ENV_CONTEXT`. The literal prefix `"service_repo."` exists in `pkg/verdict` only as the lookup prefix for the keys the operator supplies — it is not a mapping entry, and no key/value pair may be hard-coded.
- **Do NOT add a `pod-logs.sh` or any pod-log / event read.** That waits on the Config CRD gaining a service-account field and is an explicit spec Non-goal.
- Do NOT change the `github.com/bborbe/agent` module or its version. No `go get`, no version bump, and never `-mod=vendor`.
- Do NOT change `pkg/factory/`, `pkg/prompts/`, `main.go`, `cmd/run-task/main.go`, `k8s/`, `README.md`, `CHANGELOG.md` or `agent/scripts/` in this prompt. Dispatch, phase-chain wiring, the per-phase prompts, the README mapping section and the CHANGELOG entry belong to the sibling prompts of this spec.
- Do NOT create or edit the four read-only scripts. They are spec `read-only-observation-toolkit`'s deliverable, and **that spec must be merged before this one starts**.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass, and the healthcheck / oauth-probe routing is untouched.

</constraints>

<verification>

Run every line from the repo root. All must pass.

```bash
make test          # the new pkg/verdict suite runs here
make precommit     # AC1 — must exit 0

# the new package exists, with one shared suite file
test -f pkg/verdict/verdict.go
test -f pkg/verdict/gate.go
test -f pkg/verdict/lineguard.go
test -f pkg/verdict/step.go
test -f pkg/verdict/verdict_suite_test.go

# new code reaches at least 80% statement coverage
go test -mod=mod -coverprofile=/tmp/verdict-cover.out ./pkg/verdict/...
test "$(go tool cover -func=/tmp/verdict-cover.out | tail -1 | sed -E 's/.* ([0-9]+)\.[0-9]+%/\1/')" -ge 80

# every reason string the spec pins is present verbatim
grep -qF 'no line cited; evidence:' pkg/verdict/*.go
grep -qF 'evidence cites no live read' pkg/verdict/*.go
grep -qF 'below High' pkg/verdict/*.go
grep -qF 'resolves outside the clone' pkg/verdict/*.go
grep -qF 'not found in clone' pkg/verdict/*.go
grep -qF 'beyond end of file' pkg/verdict/*.go
grep -qF 'does not resolve' pkg/verdict/*.go
grep -qF 'heading rule: section %q contains a column-0 heading line' pkg/verdict/*.go

# the line-guard tests must use their own temp clone, never the shared local one
! grep -rn 'agent/repos' pkg/verdict/

# project convention: no fmt.Errorf in pkg/
! grep -rn 'fmt\.Errorf' pkg/verdict/
```

</verification>
