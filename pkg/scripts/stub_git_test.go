// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scripts_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
)

// repoCloneStubGitScript stands in for git on the child's PATH.
//
// It appends its full argv to $STUB_GIT_LOG, so a spec can assert both what was
// invoked and that nothing was. For `clone` it creates the destination (the last
// argument) plus a marker named after the call index, so a spec can tell a first
// clone from a second one. Any other subcommand succeeds silently, which is what
// the script's rev-parse calls see.
const repoCloneStubGitScript = `#!/usr/bin/env bash
echo "$*" >> "$STUB_GIT_LOG"
if [ "$1" = "clone" ]; then
  for last in "$@"; do :; done
  mkdir -p "$last"
  index="$(grep -c '^clone ' "$STUB_GIT_LOG")"
  echo marker > "$last/stub-clone-marker-$index"
  if [ -n "$STUB_GIT_LINK_TARGET" ]; then
    ln -s "$STUB_GIT_LINK_TARGET" "$last/link"
  fi
  exit "${STUB_GIT_CLONE_EXIT:-0}"
fi
exit 0
`

// repoCloneWriteStubGit installs the stub as `git` inside stubDir and returns the
// path of the log every invocation is appended to. The log is created empty, so a
// spec that expects no invocation reads an empty file rather than a missing one.
func repoCloneWriteStubGit(stubDir string) string {
	logPath := filepath.Join(stubDir, "git.log")
	Expect(os.WriteFile(logPath, nil, 0o600)).To(Succeed())
	Expect(os.WriteFile(
		filepath.Join(stubDir, "git"),
		[]byte(repoCloneStubGitScript),
		0o755,
	)).To(Succeed())
	return logPath
}
