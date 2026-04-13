package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAndSortUnique(t *testing.T) {
	t.Parallel()

	got := NormalizeAndSortUnique([]string{
		" /tmp/a ",
		"/tmp/b",
		"/tmp/a",
		"",
		"   ",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 values, got %d (%#v)", len(got), got)
	}
	if got[0] != "/tmp/a" || got[1] != "/tmp/b" {
		t.Fatalf("unexpected normalized values: %#v", got)
	}
}

func TestValidateRepoPathWithinAllowedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	got, err := ValidateRepoPath(repo, []string{root})
	if err != nil {
		t.Fatalf("ValidateRepoPath returned error: %v", err)
	}
	if got != repo {
		t.Fatalf("expected %s, got %s", repo, got)
	}
}

func TestValidateRepoPathRejectsOutsideRoot(t *testing.T) {
	t.Parallel()

	allowedRoot := t.TempDir()
	repo := t.TempDir()

	if _, err := ValidateRepoPath(repo, []string{allowedRoot}); err == nil {
		t.Fatalf("expected outside-root validation error")
	}
}

func TestValidateRepoPathFollowsSymlinkToAllowedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realRepo := filepath.Join(root, "real-repo")
	if err := os.MkdirAll(realRepo, 0o755); err != nil {
		t.Fatalf("mkdir real repo: %v", err)
	}

	linkRoot := t.TempDir()
	symlinkPath := filepath.Join(linkRoot, "repo-link")
	if err := os.Symlink(realRepo, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	got, err := ValidateRepoPath(symlinkPath, []string{root})
	if err != nil {
		t.Fatalf("ValidateRepoPath returned error: %v", err)
	}
	if got != realRepo {
		t.Fatalf("expected resolved repo %s, got %s", realRepo, got)
	}
}
