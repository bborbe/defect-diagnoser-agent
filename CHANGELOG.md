# Changelog

## Unreleased

- chore: update github.com/bborbe/agent to v0.103.1, github.com/bborbe/cqrs to v0.6.12, github.com/bborbe/errors to v1.6.3, github.com/bborbe/kafka to v1.28.3, github.com/bborbe/sentry to v1.10.5, github.com/bborbe/service to v1.11.0, github.com/bborbe/time to v1.27.17, github.com/bborbe/vault-cli to v0.174.1, github.com/onsi/ginkgo/v2 to v2.33.1, github.com/onsi/gomega to v1.44.0, github.com/prometheus/client_golang to v1.25.0

## v0.1.0

- fix: **Go 1.27.2 now passes `make precommit`.** The standalone `errcheck:` and `gosec:` targets are both retired and dropped from `check:`, and both linters now run embedded in golangci-lint, which `.golangci.yml` already enabled. `GOLANGCI_LINT_VERSION` moves v2.13.1 → v2.14.0, the `go` directive and `Dockerfile` move to 1.27.2, and `golang.org/x/net` moves v0.58.0 → v0.60.0. ⚠️ **Neither target was repairable, which is why both are removed rather than repointed.** `go run github.com/kisielk/errcheck@v1.20.0` and `go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)` each build in their **own** module context, pinning a `golang.org/x/tools` reader that stops at export-data version 4 while the 1.27.2 toolchain emits version 5, so each dies with `internal error: package "X" without types was imported` at any `go.mod` directive. Dropping the `@$(ERRCHECK_VERSION)` pin does not rescue it either: `precommit` runs `ensure` → `go mod tidy` first, and tidy strips a require nothing imports. ⚠️ **Coverage is preserved, not traded away.** `.golangci.yml` gains `settings.errcheck.exclude-functions` carrying the retired target's `-ignore '(Close|Write|Fprint)'` list, and `settings.gosec.excludes: [G104]` carrying `-exclude=G104`. The `go` directive bump is load-bearing rather than tidy: it is what clears the nine `stdlib@v1.27.1 -> v1.27.2` advisories that `osv-scanner` reports, and it could not move while the standalone targets ran. Also bumps `osv-scanner` v2.3.1 → v2.6.0: the pinned v2.3.1 panics during call-graph analysis in `golang.org/x/tools@v0.38.0/go/ssa`, and v2.6.0 carries a fixed x/tools.

## v0.0.0

- Initial scaffold from bborbe/agent-claude template via /launch-agent on 2026-09-27
