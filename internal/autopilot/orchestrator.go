package autopilot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/404loopback/zukt/internal/config"
)

type commandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

type execRunner struct{}

func (r execRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type Orchestrator struct {
	cfg      config.Config
	logger   *slog.Logger
	runner   commandRunner
	prober   func(ctx context.Context) error
	attempts int
	sleep    time.Duration
}

func New(cfg config.Config, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		cfg:      cfg,
		logger:   logger,
		runner:   execRunner{},
		prober:   nil,
		attempts: 20,
		sleep:    750 * time.Millisecond,
	}
}

func (o *Orchestrator) EnsureReady(ctx context.Context) error {
	if !o.cfg.ZoektAutopilot {
		o.logger.Info("autopilot disabled")
		return nil
	}
	if o.cfg.ZoektBackend != "http" {
		return fmt.Errorf("autopilot requires ZOEKT_BACKEND=http")
	}

	if err := o.runProbe(ctx); err == nil {
		o.logger.Info("zoekt webserver already available", "base_url", o.cfg.ZoektHTTPURL)
		return o.indexAll(ctx)
	}

	o.logger.Warn("zoekt webserver unavailable, starting with docker compose", "base_url", o.cfg.ZoektHTTPURL)
	if err := o.startZoekt(ctx); err != nil {
		return fmt.Errorf("start zoekt webserver: %w", err)
	}

	if err := o.waitForZoekt(ctx, o.attempts, o.sleep); err != nil {
		return fmt.Errorf("wait zoekt webserver: %w", err)
	}

	return o.indexAll(ctx)
}

func (o *Orchestrator) startZoekt(ctx context.Context) error {
	return o.runner.Run(ctx, o.cfg.ProjectRoot, "docker", "compose", "up", "-d", "zoekt-web")
}

func (o *Orchestrator) indexAll(ctx context.Context) error {
	return o.IndexRepos(ctx, o.cfg.ZoektRepos, o.cfg.ZoektForceReindex)
}

func (o *Orchestrator) probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.cfg.ZoektHTTPURL, "/")+"/search?q=.&num=1&format=json", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (o *Orchestrator) waitForZoekt(ctx context.Context, attempts int, sleep time.Duration) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := o.runProbe(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("probe failed")
	}
	return lastErr
}

// SetRunnerForTest allows command runner injection for tests.
func (o *Orchestrator) SetRunnerForTest(r commandRunner) {
	o.runner = r
}

// SetProbeForTest allows probe injection for tests.
func (o *Orchestrator) SetProbeForTest(prober func(ctx context.Context) error) {
	o.prober = prober
}

// SetWaitSettingsForTest allows shorter retry loops in tests.
func (o *Orchestrator) SetWaitSettingsForTest(attempts int, sleep time.Duration) {
	if attempts > 0 {
		o.attempts = attempts
	}
	if sleep >= 0 {
		o.sleep = sleep
	}
}

func (o *Orchestrator) IndexRepos(ctx context.Context, repos []string, force bool) error {
	if err := os.MkdirAll(o.cfg.ZoektIndexDir, 0o755); err != nil {
		return fmt.Errorf("create index directory %s: %w", o.cfg.ZoektIndexDir, err)
	}
	if !force {
		shards, err := filepath.Glob(filepath.Join(o.cfg.ZoektIndexDir, "*.zoekt"))
		if err == nil && len(shards) > 0 {
			o.logger.Info("existing index shards detected, skipping reindex", "shards", len(shards))
			return nil
		}
	}

	for _, repo := range repos {
		repo = filepath.Clean(repo)
		parent := filepath.Dir(repo)
		base := filepath.Base(repo)

		o.logger.Info("indexing repository", "repo", repo)
		err := o.runner.Run(
			ctx,
			o.cfg.ProjectRoot,
			"docker",
			"run", "--rm",
			"-v", fmt.Sprintf("%s:/data/index", o.cfg.ZoektIndexDir),
			"-v", fmt.Sprintf("%s:/data/srcroot:ro", parent),
			"zukt-zoekt-web",
			"zoekt-index", "-index", "/data/index", filepath.ToSlash(filepath.Join("/data/srcroot", base)),
		)
		if err != nil {
			return fmt.Errorf("index repo %s: %w", repo, err)
		}
	}
	return nil
}

func (o *Orchestrator) runProbe(ctx context.Context) error {
	if o.prober != nil {
		return o.prober(ctx)
	}
	return o.probe(ctx)
}
