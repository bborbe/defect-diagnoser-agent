// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bborbe/agent/envparse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// configPath is the Config CRD instance that carries the allowlist and the env channel.
const configPath = "k8s/defect-diagnoser-agent-config.yaml"

// guardrailsPath is the agent's own guardrail prose.
const guardrailsPath = "agent/.claude/CLAUDE.md"

// readmePath is the repo README.
const readmePath = "README.md"

// allowedToolsPattern extracts the ALLOWED_TOOLS value from the Config's env block.
var allowedToolsPattern = regexp.MustCompile(`(?m)^\s*ALLOWED_TOOLS:\s*"(.*)"\s*$`)

// claudeEnvPattern extracts the CLAUDE_ENV value from the Config's env block.
var claudeEnvPattern = regexp.MustCompile(`(?m)^\s*CLAUDE_ENV:\s*"(.*)"\s*$`)

// allowlistEntryPattern captures the script name of one `Bash(scripts/<name>:*)` entry.
var allowlistEntryPattern = regexp.MustCompile(`^Bash\(scripts/(.+):\*\)$`)

// scriptMentionPattern finds a `scripts/<name>.sh` mention in prose.
var scriptMentionPattern = regexp.MustCompile(`scripts/([A-Za-z0-9._-]+\.sh)`)

// podLogsPattern matches the dropped pod-log tool in any casing or separator.
var podLogsPattern = regexp.MustCompile(`(?i)pod[- ]?logs`)

// expectedScripts is the exact set the allowlist must name.
var expectedScripts = []string{
	"prometheus-query.sh",
	"repo-clone.sh",
	"vault-list.sh",
	"vault-read.sh",
}

// readRepoPath reads a repo-root-relative file.
func readRepoPath(rel string) string {
	raw, err := os.ReadFile(filepath.Join(repoRoot(), filepath.FromSlash(rel)))
	Expect(err).NotTo(HaveOccurred(), "read %s", rel)
	return string(raw)
}

// promptMarkdownPaths returns the embedded prompt files, repo-root-relative.
func promptMarkdownPaths() []string {
	matches, err := filepath.Glob(filepath.Join(repoRoot(), "pkg", "prompts", "*.md"))
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).NotTo(BeEmpty(), "no embedded prompt markdown found")
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		rel, err := filepath.Rel(repoRoot(), match)
		Expect(err).NotTo(HaveOccurred())
		paths = append(paths, filepath.ToSlash(rel))
	}
	return paths
}

// prosePaths returns every file whose prose names observation scripts.
func prosePaths() []string {
	return append(promptMarkdownPaths(), guardrailsPath)
}

// indentOf returns the number of leading spaces of a line.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// specEnvBlock returns the lines of the Config's `spec.env` block: the lines after
// `  env:` whose indent is at least 4, up to the next line indented 2 or less.
//
// Scoping the value regexes to this block is what makes a CLAUDE_ENV placed at
// `spec` level (or anywhere else) fail instead of passing silently.
func specEnvBlock(config string) string {
	var block []string
	inEnv := false
	for _, line := range strings.Split(config, "\n") {
		if !inEnv {
			inEnv = strings.TrimRight(line, " \t") == "  env:"
			continue
		}
		if indentOf(line) <= 2 {
			break
		}
		block = append(block, line)
	}
	return strings.Join(block, "\n")
}

// envValue extracts the quoted value matched by pattern from the Config's spec.env
// block, failing loudly when the block or the key is absent.
func envValue(pattern *regexp.Regexp, key string) string {
	block := specEnvBlock(readRepoPath(configPath))
	Expect(block).NotTo(BeEmpty(), "spec.env block not found in %s", configPath)
	match := pattern.FindStringSubmatch(block)
	Expect(match).NotTo(BeNil(), "%s not found in the Config's spec.env block", key)
	return match[1]
}

// allowlistedScripts returns the script names named by the Config's ALLOWED_TOOLS,
// as plain strings — never by re-grepping the file, because a shell pattern
// containing `:*` is a BRE quantifier and would silently match nothing.
func allowlistedScripts() []string {
	names := make([]string, 0, len(expectedScripts))
	for _, entry := range strings.Split(envValue(allowedToolsPattern, "ALLOWED_TOOLS"), ",") {
		captured := allowlistEntryPattern.FindStringSubmatch(strings.TrimSpace(entry))
		if captured != nil {
			names = append(names, captured[1])
		}
	}
	sort.Strings(names)
	return names
}

// containsString reports whether haystack holds needle.
func containsString(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

var _ = Describe("Observation script consistency", func() {
	It("allowlists exactly the four observation scripts", func() {
		Expect(allowlistedScripts()).To(Equal(expectedScripts))
	})

	It("has every allowlisted script on disk and executable", func() {
		names := allowlistedScripts()
		Expect(names).NotTo(BeEmpty())
		for _, name := range names {
			path := filepath.Join(repoRoot(), "agent", "scripts", name)
			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred(), "allowlisted script %s is missing", name)
			Expect(
				info.Mode().Perm()&0o111,
			).NotTo(BeZero(), "allowlisted script %s is not executable", name)
		}
	})

	It("allowlists every executable script on disk", func() {
		names := allowlistedScripts()
		entries, err := os.ReadDir(filepath.Join(repoRoot(), "agent", "scripts"))
		Expect(err).NotTo(HaveOccurred())
		var unlisted []string
		for _, entry := range entries {
			info, err := entry.Info()
			Expect(err).NotTo(HaveOccurred())
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
				continue
			}
			if !containsString(names, entry.Name()) {
				unlisted = append(unlisted, entry.Name())
			}
		}
		Expect(
			unlisted,
		).To(BeEmpty(), "executable scripts missing from ALLOWED_TOOLS: %v", unlisted)
	})

	It("allowlists every script the prose mentions", func() {
		names := allowlistedScripts()
		var unlisted []string
		for _, path := range prosePaths() {
			mentions := scriptMentionPattern.FindAllStringSubmatch(readRepoPath(path), -1)
			for _, mention := range mentions {
				if !containsString(names, mention[1]) {
					unlisted = append(unlisted, path+": "+mention[1])
				}
			}
		}
		Expect(unlisted).To(BeEmpty(), "scripts named in prose but not allowlisted: %v", unlisted)
	})

	It("has no pod-log reference left in the Config or the prose", func() {
		paths := append(promptMarkdownPaths(), configPath, guardrailsPath, readmePath)
		var offenders []string
		for _, path := range paths {
			if podLogsPattern.MatchString(readRepoPath(path)) {
				offenders = append(offenders, path)
			}
		}
		Expect(offenders).To(BeEmpty(), "dropped pod-log tool still referenced in: %v", offenders)
	})

	It("forwards both endpoint URLs through CLAUDE_ENV", func() {
		parsed := envparse.KeyValuePairs(envValue(claudeEnvPattern, "CLAUDE_ENV"))
		Expect(parsed).To(HaveKeyWithValue("GIT_REST_URL", "http://vault-obsidian-personal:9090"))
		Expect(parsed).To(HaveKeyWithValue("PROMETHEUS_URL", "http://prometheus.monitoring:9090"))
	})
})
