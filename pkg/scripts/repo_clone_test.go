// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scripts_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// repoCloneTimeout bounds one script invocation.
const repoCloneTimeout = 60 * time.Second

// repoCloneSandbox is the throwaway tree one spec runs the script inside.
//
// The script under test is a copy, so its fixed clone root is <tmp>/agent/repos
// and a broken guard can at worst damage the temp tree, never the worktree.
type repoCloneSandbox struct {
	tmpRoot  string
	agentDir string
	script   string
	outside  string
	stubDir  string
	logPath  string
	home     string
}

// newRepoCloneSandbox copies the script into a fresh temp tree and installs the
// git stub beside it. Every path is resolved with symlinks followed, because the
// script resolves its own location with pwd -P.
func newRepoCloneSandbox() *repoCloneSandbox {
	raw, err := os.MkdirTemp("", "repo-clone-sandbox-")
	Expect(err).NotTo(HaveOccurred())
	resolved, err := filepath.EvalSymlinks(raw)
	Expect(err).NotTo(HaveOccurred())

	sandbox := &repoCloneSandbox{tmpRoot: resolved, agentDir: filepath.Join(resolved, "agent")}

	scriptsDir := filepath.Join(sandbox.agentDir, "scripts")
	Expect(os.MkdirAll(scriptsDir, 0o755)).To(Succeed())
	source, err := os.ReadFile(filepath.Join(repoRoot(), "agent", "scripts", "repo-clone.sh"))
	Expect(err).NotTo(HaveOccurred())
	sandbox.script = filepath.Join(scriptsDir, "repo-clone.sh")
	Expect(os.WriteFile(sandbox.script, source, 0o755)).To(Succeed())

	sandbox.outside = filepath.Join(resolved, "outside")
	Expect(os.MkdirAll(sandbox.outside, 0o755)).To(Succeed())

	sandbox.stubDir, err = os.MkdirTemp("", "repo-clone-stub-")
	Expect(err).NotTo(HaveOccurred())
	sandbox.logPath = repoCloneWriteStubGit(sandbox.stubDir)

	sandbox.home, err = os.MkdirTemp("", "repo-clone-home-")
	Expect(err).NotTo(HaveOccurred())

	return sandbox
}

// repoCloneRemoveSandbox restores write permission before deleting every tree the
// spec created. Without the chmod the read-only clone survives the whole run.
func repoCloneRemoveSandbox(sandbox *repoCloneSandbox) {
	for _, root := range []string{sandbox.tmpRoot, sandbox.stubDir, sandbox.home} {
		_ = exec.Command("chmod", "-R", "u+w", root).Run()
		_ = os.RemoveAll(root)
	}
}

// roots returns the trees a snapshot covers.
func (s *repoCloneSandbox) roots() []string {
	return []string{s.tmpRoot, s.outside}
}

// reposExcludes returns the clone tree, which a successful clone is meant to change.
func (s *repoCloneSandbox) reposExcludes() []string {
	return []string{filepath.Join(s.agentDir, "repos")}
}

// dest returns the destination the script must resolve `bborbe/foo` to.
func (s *repoCloneSandbox) dest() string {
	return filepath.Join(s.agentDir, "repos", "bborbe", "foo")
}

// repoCloneSnapshot records "path mode" for every entry under roots, skipping the
// excluded paths, so a spec can assert that nothing else on disk moved.
func repoCloneSnapshot(roots []string, excludes []string) string {
	seen := map[string]bool{}
	lines := []string{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if repoCloneExcluded(path, excludes) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			if info, statErr := os.Lstat(path); statErr == nil {
				lines = append(lines, path+" "+info.Mode().String())
			}
			return nil
		})
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// repoCloneExcluded reports whether path is one of the snapshot's excluded entries.
func repoCloneExcluded(path string, excludes []string) bool {
	for _, exclude := range excludes {
		if path == exclude {
			return true
		}
	}
	return false
}

// runRepoClone executes the sandboxed script through a real subprocess boundary.
//
// The child environment is built explicitly: the ambient proxy variables would
// otherwise reach git, and HOME points at a scratch directory.
func runRepoClone(
	sandbox *repoCloneSandbox,
	env map[string]string,
	args ...string,
) (string, string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), repoCloneTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sandbox.script, args...)
	cmd.Env = []string{
		"PATH=" + sandbox.stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + sandbox.home,
		"STUB_GIT_LOG=" + sandbox.logPath,
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr == nil {
		return stdout.String(), stderr.String(), 0
	}

	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		Fail("run repo-clone.sh: " + runErr.Error())
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

// repoCloneLog returns everything the git stub recorded.
func repoCloneLog(sandbox *repoCloneSandbox) string {
	content, err := os.ReadFile(sandbox.logPath)
	Expect(err).NotTo(HaveOccurred())
	return string(content)
}

// repoCloneCloneLines returns only the clone invocations the stub recorded. The
// script's later rev-parse calls are logged too, so the prefix matters.
func repoCloneCloneLines(sandbox *repoCloneSandbox) []string {
	all := strings.Split(repoCloneLog(sandbox), "\n")
	lines := make([]string, 0, len(all))
	for _, line := range all {
		if strings.HasPrefix(line, "clone ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// repoCloneExpectNotWritable asserts no entry under root carries an owner write
// bit. Symlinks are skipped: their own mode is meaningless, and chmod -R must not
// have followed one to its target.
func repoCloneExpectNotWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, statErr := os.Lstat(path)
		Expect(statErr).NotTo(HaveOccurred())
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		Expect(info.Mode().Perm()&0o200).To(BeZero(), path)
		return nil
	})
}

// repoCloneMode returns the permission bits of path, without following symlinks.
func repoCloneMode(path string) os.FileMode {
	info, err := os.Lstat(path)
	Expect(err).NotTo(HaveOccurred())
	return info.Mode().Perm()
}

// repoCloneIsSymlink reports whether path is itself a symbolic link.
func repoCloneIsSymlink(path string) bool {
	info, err := os.Lstat(path)
	Expect(err).NotTo(HaveOccurred())
	return info.Mode()&os.ModeSymlink != 0
}

var _ = Describe("repo-clone.sh", func() {
	var sandbox *repoCloneSandbox

	BeforeEach(func() {
		sandbox = newRepoCloneSandbox()
	})

	AfterEach(func() {
		repoCloneRemoveSandbox(sandbox)
	})

	Describe("argument handling", func() {
		DescribeTable("refuses a wrong argument count",
			func(args []string) {
				before := repoCloneSnapshot(sandbox.roots(), nil)
				stdout, stderr, exitCode := runRepoClone(sandbox, nil, args...)

				Expect(exitCode).To(Equal(2))
				Expect(stdout).To(BeEmpty())
				Expect(stderr).To(ContainSubstring("usage: repo-clone.sh clone <owner>/<name>"))
				Expect(repoCloneLog(sandbox)).To(BeEmpty())
				Expect(repoCloneSnapshot(sandbox.roots(), nil)).To(Equal(before))
			},
			Entry("no arguments", []string{}),
			Entry("one argument", []string{"clone"}),
			Entry("three arguments", []string{"clone", "bborbe/foo", "extra"}),
		)

		DescribeTable("refuses a subcommand that is not clone",
			func(args []string) {
				before := repoCloneSnapshot(sandbox.roots(), nil)
				stdout, stderr, exitCode := runRepoClone(sandbox, nil, args...)

				Expect(exitCode).To(Equal(2))
				Expect(stdout).To(BeEmpty())
				Expect(stderr).To(ContainSubstring("usage: repo-clone.sh clone <owner>/<name>"))
				Expect(repoCloneLog(sandbox)).To(BeEmpty())
				Expect(repoCloneSnapshot(sandbox.roots(), nil)).To(Equal(before))
			},
			Entry("log with its full argument count", []string{"log", "x", "y"}),
			Entry("log with a valid argument count", []string{"log", "x"}),
		)
	})

	Describe("reference validation", func() {
		DescribeTable("refuses a reference that is not owner/name",
			func(ref string) {
				before := repoCloneSnapshot(sandbox.roots(), nil)
				stdout, stderr, exitCode := runRepoClone(sandbox, nil, "clone", ref)

				Expect(exitCode).To(Equal(2))
				Expect(stdout).To(BeEmpty())
				Expect(stderr).To(ContainSubstring("invalid repo reference"))
				Expect(repoCloneLog(sandbox)).To(BeEmpty())
				Expect(repoCloneSnapshot(sandbox.roots(), nil)).To(Equal(before))
			},
			Entry("a bare name with no owner", "evil"),
			Entry("a shell metacharacter in the owner", "a;b/c"),
			Entry("an http scheme", "http://evil.example/x/y"),
			Entry("a file scheme", "file:///etc/x"),
			Entry("a leading .. segment", "../x"),
			Entry("a trailing .. segment", "x/.."),
			Entry("a trailing . segment", "x/."),
			Entry("two . segments", "./."),
			Entry("two .. segments", "../.."),
			Entry("a .. owner behind the https prefix", "https://github.com/../x"),
			Entry("a .. name behind the scp prefix", "git@github.com:x/...git"),
		)
	})

	Describe("path containment", func() {
		It("refuses an owner directory that is a symlink out of repos/", func() {
			reposDir := filepath.Join(sandbox.agentDir, "repos")
			Expect(os.MkdirAll(reposDir, 0o755)).To(Succeed())
			sentinel := filepath.Join(sandbox.outside, "sentinel")
			Expect(os.WriteFile(sentinel, []byte("keep"), 0o600)).To(Succeed())
			link := filepath.Join(reposDir, "evil")
			Expect(os.Symlink(sandbox.outside, link)).To(Succeed())

			before := repoCloneSnapshot(sandbox.roots(), nil)
			stdout, stderr, exitCode := runRepoClone(sandbox, nil, "clone", "evil/x")

			Expect(exitCode).To(Equal(2))
			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(ContainSubstring("invalid repo reference"))
			Expect(repoCloneLog(sandbox)).To(BeEmpty())
			Expect(sentinel).To(BeAnExistingFile())
			Expect(repoCloneIsSymlink(link)).To(BeTrue())
			Expect(repoCloneSnapshot(sandbox.roots(), nil)).To(Equal(before))
		})

		It("refuses a destination that is a symlink out of repos/", func() {
			sentinel := filepath.Join(sandbox.outside, "sentinel")
			Expect(os.WriteFile(sentinel, []byte("keep"), 0o600)).To(Succeed())
			Expect(os.Chmod(sandbox.outside, 0o555)).To(Succeed())

			owner := filepath.Join(sandbox.agentDir, "repos", "bborbe")
			Expect(os.MkdirAll(owner, 0o755)).To(Succeed())
			link := filepath.Join(owner, "foo")
			Expect(os.Symlink(sandbox.outside, link)).To(Succeed())

			before := repoCloneSnapshot(sandbox.roots(), nil)
			stdout, stderr, exitCode := runRepoClone(sandbox, nil, "clone", "bborbe/foo")

			Expect(exitCode).To(Equal(2))
			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(ContainSubstring("invalid repo reference"))
			Expect(repoCloneLog(sandbox)).To(BeEmpty())
			Expect(repoCloneMode(sandbox.outside)).To(Equal(os.FileMode(0o555)))
			Expect(sentinel).To(BeAnExistingFile())
			Expect(repoCloneIsSymlink(link)).To(BeTrue())
			Expect(repoCloneSnapshot(sandbox.roots(), nil)).To(Equal(before))
		})
	})

	Describe("cloning into the fixed clone root", func() {
		It("clones into repos/ and makes the whole tree read-only", func() {
			before := repoCloneSnapshot(sandbox.roots(), sandbox.reposExcludes())

			stdout, stderr, exitCode := runRepoClone(
				sandbox,
				map[string]string{"STUB_GIT_LINK_TARGET": sandbox.outside},
				"clone",
				"bborbe/foo",
			)

			Expect(exitCode).To(Equal(0))
			Expect(stderr).To(BeEmpty())
			Expect(repoCloneCloneLines(sandbox)).To(Equal([]string{
				"clone https://github.com/bborbe/foo " + sandbox.dest(),
			}))
			Expect(repoCloneLog(sandbox)).NotTo(ContainSubstring("--recurse-submodules"))
			Expect(stdout).To(ContainSubstring("clone_path=" + sandbox.dest()))
			Expect(stdout).To(ContainSubstring("head_sha="))
			Expect(stdout).To(ContainSubstring("default_branch="))

			repoCloneExpectNotWritable(sandbox.dest())
			Expect(repoCloneMode(sandbox.outside)).To(Equal(os.FileMode(0o755)))
			Expect(repoCloneSnapshot(sandbox.roots(), sandbox.reposExcludes())).To(Equal(before))
		})

		It("accepts a dotted owner", func() {
			_, _, exitCode := runRepoClone(sandbox, nil, "clone", "bborbe/.github")

			Expect(exitCode).To(Equal(0))
			Expect(repoCloneCloneLines(sandbox)).To(Equal([]string{
				"clone https://github.com/bborbe/.github " +
					filepath.Join(sandbox.agentDir, "repos", "bborbe", ".github"),
			}))
		})
	})

	Describe("re-cloning and failure", func() {
		It("re-clones over a read-only tree", func() {
			if os.Geteuid() == 0 {
				Skip("uid 0 removes a read-only tree without the permission " +
					"restoration, so this spec proves nothing here")
			}

			_, _, firstExit := runRepoClone(sandbox, nil, "clone", "bborbe/foo")
			Expect(firstExit).To(Equal(0))
			firstMarker := filepath.Join(sandbox.dest(), "stub-clone-marker-1")
			Expect(firstMarker).To(BeAnExistingFile())

			_, _, secondExit := runRepoClone(sandbox, nil, "clone", "bborbe/foo")
			Expect(secondExit).To(Equal(0))
			Expect(repoCloneCloneLines(sandbox)).To(HaveLen(2))
			Expect(firstMarker).NotTo(BeAnExistingFile())
			Expect(
				filepath.Join(sandbox.dest(), "stub-clone-marker-2"),
			).To(BeAnExistingFile())
			repoCloneExpectNotWritable(sandbox.dest())
		})

		It("reports a failing git clone", func() {
			_, stderr, exitCode := runRepoClone(
				sandbox,
				map[string]string{"STUB_GIT_CLONE_EXIT": "128"},
				"clone",
				"bborbe/foo",
			)

			Expect(exitCode).NotTo(BeZero())
			Expect(stderr).To(ContainSubstring("git clone failed"))
		})
	})
})
