// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verdict

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bborbe/errors"
)

// CheckLineGuard reports whether fileLine can be shown to exist inside the
// read-only clone rooted at cloneRoot. nil means the line resolves.
func CheckLineGuard(ctx context.Context, cloneRoot, repo, fileLine string) error {
	path, line, ok := parseFileLine(fileLine)
	if !ok {
		return errors.Errorf(ctx, "%q not found in clone", fileLine)
	}
	repoRoot := filepath.Join(cloneRoot, repo)
	resolvedRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		// The clone or the repo directory is absent: nothing can be cited inside it.
		return errors.Errorf(ctx, "%q not found in clone", path)
	}
	target := filepath.Join(resolvedRepoRoot, path)
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return resolveMissingAncestor(ctx, resolvedRepoRoot, target, path)
	}
	if !within(resolvedRepoRoot, resolved) {
		return errors.Errorf(ctx, "%q resolves outside the clone", path)
	}
	data, readErr := os.ReadFile(resolved)
	if readErr != nil {
		return errors.Wrapf(ctx, readErr, "read %q", resolved)
	}
	count := lineCount(data)
	if count < line {
		return errors.Errorf(ctx, "%q beyond end of file (%d lines)", fileLine, count)
	}
	return nil
}

// resolveMissingAncestor handles a leaf that does not exist: it resolves the
// deepest existing ancestor so a symlinked directory escaping the clone is
// still reported as an escape rather than a missing file.
func resolveMissingAncestor(
	ctx context.Context,
	resolvedRepoRoot string,
	target string,
	path string,
) error {
	ancestor := deepestExisting(target)
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		resolvedAncestor = filepath.Clean(ancestor)
	}
	if !within(resolvedRepoRoot, resolvedAncestor) {
		return errors.Errorf(ctx, "%q resolves outside the clone", path)
	}
	return errors.Errorf(ctx, "%q not found in clone", path)
}

// deepestExisting returns target itself if it exists, else the closest
// ancestor that does.
func deepestExisting(target string) string {
	current := target
	for {
		if _, err := os.Lstat(current); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current
		}
		current = parent
	}
}

// within reports whether target is root or lies below it.
func within(root, target string) bool {
	return target == root || strings.HasPrefix(target, root+string(os.PathSeparator))
}

// lineCount counts \n-separated lines, not counting a single trailing newline
// as an extra line.
func lineCount(data []byte) int {
	return len(strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"))
}

// parseFileLine parses "<repo-relative path>:<positive integer>".
func parseFileLine(fileLine string) (string, int, bool) {
	index := strings.LastIndex(fileLine, ":")
	if index < 0 {
		return "", 0, false
	}
	path := fileLine[:index]
	lineText := fileLine[index+1:]
	if !validRepoRelativePath(path) {
		return "", 0, false
	}
	line, err := strconv.Atoi(lineText)
	if err != nil || line <= 0 {
		return "", 0, false
	}
	return path, line, true
}

// validRepoRelativePath reports whether path is non-empty, relative and free of
// any ".." segment.
func validRepoRelativePath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}
