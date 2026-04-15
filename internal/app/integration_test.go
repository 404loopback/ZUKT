package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// TestRunConfigLoadingAndHealthCheck tests that app properly loads config and fails gracefully
// when backend is unavailable (ensuring health check is invoked).
func TestRunConfigLoadingAndHealthCheck(t *testing.T) {

	// Setup: configure for unavailable backend
	t.Setenv("MCP_SERVER_NAME", "test-zukt")
	t.Setenv("MCP_SERVER_VERSION", "0.1.0")
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:29999") // Port unlikely to be in use
	// Set very short timeout to fail fast
	t.Setenv("ZOEKT_HTTP_TIMEOUT", "100ms")

	inputBuf := bytes.NewBufferString("")
	outputBuf := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// App should fail at startup due to health check failure
	err := Run(ctx, inputBuf, outputBuf)
	if err == nil {
		t.Fatal("expected error when backend health check fails")
	}

	// Verify error message contains startup abort indication
	errMsg := err.Error()
	if !strings.Contains(errMsg, "startup aborted") {
		t.Fatalf("expected 'startup aborted' in error, got: %s", errMsg)
	}
}

// TestRunBackendHealthCheckFailure verifies startup fails gracefully when backend is down.
func TestRunBackendHealthCheckFailure(t *testing.T) {

	t.Setenv("MCP_SERVER_NAME", "test-zukt")
	t.Setenv("MCP_SERVER_VERSION", "0.1.0")
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:19999") // Port unlikely to be in use

	inputBuf := bytes.NewBufferString("")
	outputBuf := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Run(ctx, inputBuf, outputBuf)
	if err == nil {
		t.Fatal("expected error when backend is unreachable")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "startup aborted") {
		t.Fatalf("expected startup aborted error, got: %s", errMsg)
	}
}

// TestRunConfigValidation verifies config validation during startup.
func TestRunConfigValidation(t *testing.T) {

	testCases := []struct {
		name   string
		env    map[string]string
		expect string
	}{
		{
			name: "invalid backend",
			env: map[string]string{
				"ZOEKT_BACKEND": "nosql",
			},
			expect: "unsupported ZOEKT_BACKEND",
		},
		{
			name: "invalid http url",
			env: map[string]string{
				"ZOEKT_BACKEND":       "http",
				"ZOEKT_HTTP_BASE_URL": "http://example.com",
			},
			expect: "must be localhost or loopback IP",
		},
		{
			name: "invalid timeout duration",
			env: map[string]string{
				"ZOEKT_BACKEND":       "http",
				"ZOEKT_HTTP_BASE_URL": "http://127.0.0.1:6070",
				"ZOEKT_HTTP_TIMEOUT":  "invalid",
			},
			expect: "invalid ZOEKT_HTTP_TIMEOUT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear existing env and set test case env
			for _, key := range []string{"ZOEKT_BACKEND", "ZOEKT_HTTP_BASE_URL", "ZOEKT_HTTP_TIMEOUT"} {
				t.Setenv(key, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			inputBuf := bytes.NewBufferString("")
			outputBuf := &bytes.Buffer{}
			ctx := context.Background()

			err := Run(ctx, inputBuf, outputBuf)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.expect)
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Fatalf("expected error containing %q, got: %v", tc.expect, err)
			}
		})
	}
}
