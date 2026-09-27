# Changelog

## Unreleased

- feat: Add `agent/scripts/repo-clone.sh`, the fourth read-only observation script, cloning a public GitHub repo into the fixed `repos/` directory beside `scripts/` and reporting `clone_path`, `head_sha` and `default_branch` [rejects any reference that is not `owner/name`, resolves the destination with symlinks followed and refuses anything outside `repos/` before git runs or the filesystem is touched, then makes the clone non-writable]

## v0.0.0

- Initial scaffold from bborbe/agent-claude template via /launch-agent on 2026-09-27
