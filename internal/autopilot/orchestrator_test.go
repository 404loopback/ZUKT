package autopilot

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/404loopback/zukt/internal/config"
)

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) error {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	return f.err
}

func TestEnsureReadyWhenAlreadyUpOnlyIndexes(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{
		ZoektAutopilot: true,
		ZoektBackend:   "http",
		ZoektHTTPURL:   "http://127.0.0.1:6070",
		ZoektRepos:     []string{filepath.Join(tmp, "repo")},
		ZoektIndexDir:  filepath.Join(tmp, "index"),
		ProjectRoot:    tmp,
	}

	o := New(cfg, slog.Default())
	r := &fakeRunner{}
	o.SetRunnerForTest(r)
	o.SetProbeForTest(func(context.Context) error { return nil })
	o.SetWaitSettingsForTest(2, 0)

	if err := o.EnsureReady(context.Background()); err != nil {
		t.Fatalf("EnsureReady returned error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected one index command call, got %d", len(r.calls))
	}
	if r.calls[0][0] != "docker" || r.calls[0][1] != "run" {
		t.Fatalf("unexpected command: %#v", r.calls[0])
	}
}

func TestEnsureReadyStartsThenIndexes(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{
		ZoektAutopilot: true,
		ZoektBackend:   "http",
		ZoektHTTPURL:   "http://127.0.0.1:6070",
		ZoektRepos:     []string{filepath.Join(tmp, "repo")},
		ZoektIndexDir:  filepath.Join(tmp, "index"),
		ProjectRoot:    tmp,
	}

	o := New(cfg, slog.Default())
	r := &fakeRunner{}
	o.SetRunnerForTest(r)
	o.SetWaitSettingsForTest(4, 0*time.Millisecond)

	attempt := 0
	o.SetProbeForTest(func(context.Context) error {
		attempt++
		if attempt < 3 {
			return errors.New("down")
		}
		return nil
	})

	if err := o.EnsureReady(context.Background()); err != nil {
		t.Fatalf("EnsureReady returned error: %v", err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("expected two command calls (compose up + index), got %d", len(r.calls))
	}
	if r.calls[0][0] != "docker" || r.calls[0][1] != "compose" {
		t.Fatalf("unexpected first command: %#v", r.calls[0])
	}
	if r.calls[1][0] != "docker" || r.calls[1][1] != "run" {
		t.Fatalf("unexpected second command: %#v", r.calls[1])
	}
}
