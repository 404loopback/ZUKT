package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/404loopback/zukt/internal/config"
)

type healthBackendStub struct {
	err error
}

func (s healthBackendStub) ListRepos(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []string{"repo-a"}, nil
}

func TestEnsureSearchBackendHealthySuccess(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ZoektHTTPURL: "http://127.0.0.1:6070",
		ZoektTimeout: 50 * time.Millisecond,
	}

	err := ensureSearchBackendHealthy(context.Background(), cfg, healthBackendStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnsureSearchBackendHealthyFailureIncludesOperableContext(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ZoektHTTPURL: "http://127.0.0.1:6070",
		ZoektTimeout: 50 * time.Millisecond,
	}

	err := ensureSearchBackendHealthy(context.Background(), cfg, healthBackendStub{err: errors.New("connection refused")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if !(strings.Contains(msg, "startup aborted") && strings.Contains(msg, "curl -fsS")) {
		t.Fatalf("expected operable failure message, got: %s", msg)
	}
}
