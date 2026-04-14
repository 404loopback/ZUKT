package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/404loopback/zukt/internal/zoekt"
)

func TestGetFileRelativePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoRoot := createRepoFixture(t, root, "ZUKT")
	target := filepath.Join(repoRoot, "internal", "search", "sample.txt")
	writeFileFixture(t, target, "line1\nline2\nline3\nline4\n")

	svc := NewService(zoekt.NewMockSearcher(), []string{root}, nil)
	got, err := svc.GetFile(context.Background(), "ZUKT", "internal/search/sample.txt", 2, 3)
	if err != nil {
		t.Fatalf("GetFile returned error: %v", err)
	}

	if got.Repo != "ZUKT" {
		t.Fatalf("unexpected repo: %q", got.Repo)
	}
	if got.Path != "internal/search/sample.txt" {
		t.Fatalf("unexpected path: %q", got.Path)
	}
	if got.StartLine != 2 || got.EndLine != 3 {
		t.Fatalf("unexpected range: %d..%d", got.StartLine, got.EndLine)
	}
	if got.Content != "line2\nline3" {
		t.Fatalf("unexpected content: %q", got.Content)
	}
}

func TestGetContextDefaultsAndBounds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoRoot := createRepoFixture(t, root, "PITANCE")
	target := filepath.Join(repoRoot, "service.go")
	writeFileFixture(t, target, strings.Join([]string{
		"l1", "l2", "l3", "l4", "l5", "l6",
	}, "\n")+"\n")

	svc := NewService(zoekt.NewMockSearcher(), []string{root}, nil)
	got, err := svc.GetContext(context.Background(), "PITANCE", "service.go", 4, 1, 2)
	if err != nil {
		t.Fatalf("GetContext returned error: %v", err)
	}
	if got.StartLine != 3 || got.EndLine != 6 {
		t.Fatalf("unexpected range: %d..%d", got.StartLine, got.EndLine)
	}
	if got.Content != "l3\nl4\nl5\nl6" {
		t.Fatalf("unexpected content: %q", got.Content)
	}
}

func TestGetFileRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	createRepoFixture(t, root, "ZUKT")
	svc := NewService(zoekt.NewMockSearcher(), []string{root}, nil)

	_, err := svc.GetFile(context.Background(), "ZUKT", "../outside.txt", 1, 10)
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.Contains(err.Error(), "escapes repo root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFileRejectsAbsolutePathOutsideAllowedRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	externalFile := filepath.Join(t.TempDir(), "external.txt")
	writeFileFixture(t, externalFile, "secret\n")

	svc := NewService(zoekt.NewMockSearcher(), []string{root}, nil)
	_, err := svc.GetFile(context.Background(), "", externalFile, 1, 10)
	if err == nil {
		t.Fatalf("expected outside root rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed roots") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFileRejectsAmbiguousRepo(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()
	createRepoFixture(t, rootA, "ZUKT")
	createRepoFixture(t, rootB, "ZUKT")

	svc := NewService(zoekt.NewMockSearcher(), []string{rootA, rootB}, nil)
	_, err := svc.GetFile(context.Background(), "ZUKT", "main.go", 1, 10)
	if err == nil {
		t.Fatalf("expected ambiguous repo error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFileMarksTruncated(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoRoot := createRepoFixture(t, root, "ZUKT")
	target := filepath.Join(repoRoot, "big.txt")

	lines := make([]string, 0, 2105)
	for i := 0; i < 2105; i++ {
		lines = append(lines, "line")
	}
	writeFileFixture(t, target, strings.Join(lines, "\n")+"\n")

	svc := NewService(zoekt.NewMockSearcher(), []string{root}, nil)
	got, err := svc.GetFile(context.Background(), "ZUKT", "big.txt", 1, 0)
	if err != nil {
		t.Fatalf("GetFile returned error: %v", err)
	}
	if !got.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if got.EndLine != maxReturnedLines {
		t.Fatalf("unexpected end line: %d", got.EndLine)
	}
}

func createRepoFixture(t *testing.T, root, name string) string {
	t.Helper()
	repo := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("create repo fixture: %v", err)
	}
	return repo
}

func writeFileFixture(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create directory for fixture: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}
