package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunRepoCheckout_UsesPreferredWorkDirWhenValidGitRepo(t *testing.T) {
	var gotWorkDir string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotWorkDir = body["workdir"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"/tmp/checkout","branch_name":"main"}`))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	preferred := t.TempDir()
	cmd := exec.Command("git", "-C", preferred, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, string(out))
	}

	cwd := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	t.Setenv("MULTICA_DAEMON_PORT", port)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_AGENT_NAME", "agent")
	t.Setenv("MULTICA_TASK_ID", "task-1")
	t.Setenv("MULTICA_PREFERRED_WORKDIR", preferred)

	if err := runRepoCheckout(testCmd(), []string{"https://github.com/example/repo"}); err != nil {
		t.Fatalf("runRepoCheckout: %v", err)
	}
	if gotWorkDir != filepath.Clean(preferred) {
		t.Fatalf("workdir=%q, want preferred=%q", gotWorkDir, filepath.Clean(preferred))
	}
}

func TestRunRepoCheckout_FallsBackToCWDWhenPreferredInvalid(t *testing.T) {
	var gotWorkDir string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotWorkDir = body["workdir"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"/tmp/checkout","branch_name":"main"}`))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	cwd := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	invalidPreferred := filepath.Join(t.TempDir(), "missing-repo")
	t.Setenv("MULTICA_DAEMON_PORT", port)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_AGENT_NAME", "agent")
	t.Setenv("MULTICA_TASK_ID", "task-1")
	t.Setenv("MULTICA_PREFERRED_WORKDIR", invalidPreferred)

	if err := runRepoCheckout(testCmd(), []string{"https://github.com/example/repo"}); err != nil {
		t.Fatalf("runRepoCheckout: %v", err)
	}
	gotResolved, _ := filepath.EvalSymlinks(gotWorkDir)
	cwdResolved, _ := filepath.EvalSymlinks(cwd)
	if gotResolved != cwdResolved {
		t.Fatalf("workdir=%q (resolved=%q), want cwd fallback=%q (resolved=%q)", gotWorkDir, gotResolved, cwd, cwdResolved)
	}
}
