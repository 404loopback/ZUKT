package backend

import (
	"testing"
	"time"

	"github.com/404loopback/zukt/internal/config"
)

func TestNewSearcherHTTP(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ZoektHTTPURL: "http://127.0.0.1:6070",
		ZoektTimeout: time.Second,
	}
	searcher, err := NewSearcher(cfg)
	if err != nil {
		t.Fatalf("NewSearcher returned error: %v", err)
	}
	if searcher == nil {
		t.Fatalf("expected non-nil searcher")
	}
}

func TestNewSearcherRejectsInvalidHTTPURL(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ZoektHTTPURL: "://invalid-url",
		ZoektTimeout: time.Second,
	}
	if _, err := NewSearcher(cfg); err == nil {
		t.Fatalf("expected invalid URL error")
	}
}
