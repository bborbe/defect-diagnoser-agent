// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scripts_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// maxBodyBytes is the stdout cap every observation script applies.
const maxBodyBytes = 65536

// scriptTimeout bounds one script invocation.
const scriptTimeout = 10 * time.Second

// recordedRequest is one request the recording server observed.
type recordedRequest struct {
	method     string
	requestURI string
	path       string
	rawQuery   string
	query      url.Values
}

// recorder collects the requests a recording server observed. Safe for concurrent use.
type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

// record appends one observed request.
func (r *recorder) record(request recordedRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
}

// all returns a copy of every observed request.
func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

// count returns how many requests the server observed.
func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// newRecordingServer starts a server answering every request with the given status
// and body, recording what it received. The server is closed at spec cleanup.
func newRecordingServer(status int, body string) (*httptest.Server, *recorder) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(recordedRequest{
			method:     r.Method,
			requestURI: r.RequestURI,
			path:       rawPath(r.RequestURI),
			rawQuery:   r.URL.RawQuery,
			query:      r.URL.Query(),
		})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	DeferCleanup(server.Close)
	return server, rec
}

// rawPath returns the escaped path of a request URI, without its query string.
func rawPath(requestURI string) string {
	if index := strings.Index(requestURI, "?"); index >= 0 {
		return requestURI[:index]
	}
	return requestURI
}

// scriptPath resolves a script name to its absolute path in the repository.
func scriptPath(name string) string {
	return filepath.Join(repoRoot(), "agent", "scripts", name)
}

// runScript executes the real script through the real subprocess boundary and
// returns its stdout, stderr and exit code.
//
// The child environment is built explicitly rather than inherited: the ambient
// proxy variables would otherwise route a loopback request to the proxy, the
// recording server would observe nothing, and every "no request" assertion
// would silently invert. HOME points at a fresh directory so a stray ~/.curlrc
// cannot inject a proxy either.
func runScript(script string, env map[string]string, args ...string) (string, string, int) {
	home, err := os.MkdirTemp("", "observation-scripts-home-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(home) })

	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath(script), args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home}
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
		Fail("run " + script + ": " + runErr.Error())
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

// expectRefused asserts the script exited 2 with a reason on stderr and sent nothing.
func expectRefused(script string, env map[string]string, rec *recorder, args ...string) {
	stdout, stderr, exitCode := runScript(script, env, args...)
	Expect(exitCode).To(Equal(2))
	Expect(stdout).To(BeEmpty())
	Expect(stderr).NotTo(BeEmpty())
	Expect(rec.count()).To(Equal(0))
}

// expectRefusedWithServer asserts the script refused the arguments without any request.
func expectRefusedWithServer(script string, endpointVar string, args ...string) {
	server, rec := newRecordingServer(http.StatusOK, "ignored")
	expectRefused(script, map[string]string{endpointVar: server.URL}, rec, args...)
}

// expectMissingEndpoint asserts the script names the unset endpoint variable on stderr.
func expectMissingEndpoint(script string, endpointVar string, args ...string) {
	_, stderr, exitCode := runScript(script, map[string]string{}, args...)
	Expect(exitCode).NotTo(Equal(0))
	Expect(stderr).To(ContainSubstring(endpointVar))
}

// expectHTTPError asserts a non-200 response fails with the status on stderr.
func expectHTTPError(script string, endpointVar string, status int, args ...string) {
	server, rec := newRecordingServer(status, "ignored")
	stdout, stderr, exitCode := runScript(
		script,
		map[string]string{endpointVar: server.URL},
		args...,
	)
	Expect(exitCode).NotTo(Equal(0))
	Expect(stdout).To(BeEmpty())
	Expect(stderr).To(ContainSubstring(strconv.Itoa(status)))
	Expect(rec.count()).To(Equal(1))
}

// expectTruncated asserts a body over the cap is cut to exactly maxBodyBytes and
// that the cut is not reported as a failure.
func expectTruncated(script string, endpointVar string, args ...string) {
	server, _ := newRecordingServer(http.StatusOK, strings.Repeat("a", 100*1024))
	stdout, stderr, exitCode := runScript(
		script,
		map[string]string{endpointVar: server.URL},
		args...,
	)
	Expect(exitCode).To(Equal(0))
	Expect(stdout).To(HaveLen(maxBodyBytes))
	Expect(stderr).To(ContainSubstring("truncated"))
}

// assertReadOnly asserts every observed request is a GET under the given raw path prefix.
func assertReadOnly(rec *recorder, pathPrefix string) {
	requests := rec.all()
	Expect(requests).NotTo(BeEmpty())
	for _, request := range requests {
		Expect(request.method).To(Equal(http.MethodGet))
		Expect(request.path).To(HavePrefix(pathPrefix))
	}
}

var _ = Describe("prometheus-query.sh", func() {
	const script = "prometheus-query.sh"

	It("sends exactly one GET to the fixed query endpoint", func() {
		server, rec := newRecordingServer(http.StatusOK, `{"status":"success"}`)
		stdout, stderr, exitCode := runScript(
			script,
			map[string]string{"PROMETHEUS_URL": server.URL},
			"up",
		)
		Expect(exitCode).To(Equal(0))
		Expect(stderr).To(BeEmpty())
		Expect(stdout).To(Equal(`{"status":"success"}`))

		requests := rec.all()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].path).To(Equal("/api/v1/query"))
		Expect(requests[0].query.Get("query")).To(Equal("up"))
		Expect(requests[0].query.Has("time")).To(BeFalse())
		assertReadOnly(rec, "/api/v1/query")
	})

	DescribeTable("refuses an argument that could escape the fixed endpoint",
		func(arg string) {
			expectRefusedWithServer(script, "PROMETHEUS_URL", arg)
		},
		Entry("absolute path", "/etc/passwd"),
		Entry("scheme separator", "http://evil/x"),
	)

	DescribeTable("refuses a wrong argument count",
		func(args []string) {
			expectRefusedWithServer(script, "PROMETHEUS_URL", args...)
		},
		Entry("no argument", []string{}),
		Entry("two arguments", []string{"up", "down"}),
	)

	It("names the missing endpoint variable", func() {
		expectMissingEndpoint(script, "PROMETHEUS_URL", "up")
	})

	DescribeTable("surfaces the HTTP status on stderr",
		func(status int) {
			expectHTTPError(script, "PROMETHEUS_URL", status, "up")
		},
		Entry("server error", http.StatusInternalServerError),
		Entry("unauthorized", http.StatusUnauthorized),
	)

	It("truncates an oversized body without failing", func() {
		expectTruncated(script, "PROMETHEUS_URL", "up")
	})
})

var _ = Describe("vault-read.sh", func() {
	const script = "vault-read.sh"

	It("reads one file and percent-encodes spaces in its path", func() {
		server, rec := newRecordingServer(http.StatusOK, "content")
		stdout, stderr, exitCode := runScript(
			script,
			map[string]string{"GIT_REST_URL": server.URL},
			"25 Tasks/x y.md",
		)
		Expect(exitCode).To(Equal(0))
		Expect(stderr).To(BeEmpty())
		Expect(stdout).To(Equal("content"))

		requests := rec.all()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].path).To(Equal("/api/v1/files/25%20Tasks/x%20y.md"))
		Expect(requests[0].rawQuery).To(BeEmpty())
		assertReadOnly(rec, "/api/v1/files/")
	})

	DescribeTable("refuses a path that could escape the fixed endpoint",
		func(arg string) {
			expectRefusedWithServer(script, "GIT_REST_URL", arg)
		},
		Entry("absolute path", "/etc/passwd"),
		Entry("leading parent segment", "../b.md"),
		Entry("inner parent segment", "a/../b.md"),
		Entry("trailing parent segment", "a/.."),
		Entry("query string", "a.md?x=1"),
		Entry("fragment", "a.md#x"),
		Entry("absolute url", "http://evil/x"),
	)

	DescribeTable("sends the argument literally with url globbing disabled",
		func(arg string) {
			server, rec := newRecordingServer(http.StatusOK, "content")
			_, _, exitCode := runScript(
				script,
				map[string]string{"GIT_REST_URL": server.URL},
				arg,
			)
			Expect(exitCode).To(Equal(0))

			requests := rec.all()
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].path).To(HavePrefix("/api/v1/files/"))
		},
		Entry("brace expansion", "{a,..}"),
		Entry("bracket range", "x[1-2].md"),
	)

	DescribeTable("refuses a wrong argument count",
		func(args []string) {
			expectRefusedWithServer(script, "GIT_REST_URL", args...)
		},
		Entry("no argument", []string{}),
		Entry("two arguments", []string{"a.md", "b.md"}),
	)

	It("names the missing endpoint variable", func() {
		expectMissingEndpoint(script, "GIT_REST_URL", "a.md")
	})

	DescribeTable("surfaces the HTTP status on stderr",
		func(status int) {
			expectHTTPError(script, "GIT_REST_URL", status, "a.md")
		},
		Entry("server error", http.StatusInternalServerError),
		Entry("unauthorized", http.StatusUnauthorized),
	)

	It("truncates an oversized body without failing", func() {
		expectTruncated(script, "GIT_REST_URL", "a.md")
	})
})

var _ = Describe("vault-list.sh", func() {
	const script = "vault-list.sh"

	It("lists files through the glob query parameter", func() {
		server, rec := newRecordingServer(http.StatusOK, "Tasks/a.md\n")
		stdout, stderr, exitCode := runScript(
			script,
			map[string]string{"GIT_REST_URL": server.URL},
			"* Tasks/*.md",
		)
		Expect(exitCode).To(Equal(0))
		Expect(stderr).To(BeEmpty())
		Expect(stdout).To(Equal("Tasks/a.md\n"))

		requests := rec.all()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].path).To(Equal("/api/v1/files/"))
		Expect(requests[0].query.Get("glob")).To(Equal("* Tasks/*.md"))
		assertReadOnly(rec, "/api/v1/files/")
	})

	DescribeTable("refuses a wrong argument count",
		func(args []string) {
			expectRefusedWithServer(script, "GIT_REST_URL", args...)
		},
		Entry("no argument", []string{}),
		Entry("two arguments", []string{"*.md", "*.txt"}),
	)

	It("names the missing endpoint variable", func() {
		expectMissingEndpoint(script, "GIT_REST_URL", "*.md")
	})

	DescribeTable("surfaces the HTTP status on stderr",
		func(status int) {
			expectHTTPError(script, "GIT_REST_URL", status, "*.md")
		},
		Entry("server error", http.StatusInternalServerError),
		Entry("unauthorized", http.StatusUnauthorized),
	)

	It("truncates an oversized body without failing", func() {
		expectTruncated(script, "GIT_REST_URL", "*.md")
	})
})

var _ = Describe("observation script contracts", func() {
	DescribeTable("declares the 30 second request timeout",
		func(script string) {
			content, err := os.ReadFile(scriptPath(script))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("--max-time 30"))
		},
		Entry("vault-read.sh", "vault-read.sh"),
		Entry("vault-list.sh", "vault-list.sh"),
		Entry("prometheus-query.sh", "prometheus-query.sh"),
	)
})
