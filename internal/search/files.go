package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/404loopback/zukt/internal/paths"
)

const (
	defaultContextRadius = 20
	maxContextRadius     = 200
	maxReturnedLines     = 2000
)

type FileSlice struct {
	Repo       string `json:"repo,omitempty"`
	Path       string `json:"path"`
	AbsPath    string `json:"abs_path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
	Content    string `json:"content"`
}

type FileContext struct {
	Repo       string `json:"repo,omitempty"`
	Path       string `json:"path"`
	AbsPath    string `json:"abs_path"`
	Line       int    `json:"line"`
	Before     int    `json:"before"`
	After      int    `json:"after"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
	Content    string `json:"content"`
}

func (s *Service) GetFile(_ context.Context, repo, filePath string, startLine, endLine int) (FileSlice, error) {
	absPath, resolvedRepo, displayPath, err := s.resolveFilePath(repo, filePath)
	if err != nil {
		return FileSlice{}, err
	}

	start, end, err := normalizeLineBounds(startLine, endLine)
	if err != nil {
		return FileSlice{}, err
	}

	content, totalLines, resolvedStart, resolvedEnd, truncated, err := readFileRange(absPath, start, end)
	if err != nil {
		return FileSlice{}, err
	}

	return FileSlice{
		Repo:       resolvedRepo,
		Path:       displayPath,
		AbsPath:    absPath,
		StartLine:  resolvedStart,
		EndLine:    resolvedEnd,
		TotalLines: totalLines,
		Truncated:  truncated,
		Content:    content,
	}, nil
}

func (s *Service) GetContext(_ context.Context, repo, filePath string, line, before, after int) (FileContext, error) {
	if line <= 0 {
		return FileContext{}, fmt.Errorf("line must be greater than 0")
	}

	if before < 0 || after < 0 {
		return FileContext{}, fmt.Errorf("before/after cannot be negative")
	}
	if before == 0 && after == 0 {
		before = defaultContextRadius
		after = defaultContextRadius
	}
	if before > maxContextRadius || after > maxContextRadius {
		return FileContext{}, fmt.Errorf("before/after cannot exceed %d", maxContextRadius)
	}

	start := line - before
	if start < 1 {
		start = 1
	}
	end := line + after

	absPath, resolvedRepo, displayPath, err := s.resolveFilePath(repo, filePath)
	if err != nil {
		return FileContext{}, err
	}

	content, totalLines, resolvedStart, resolvedEnd, truncated, err := readFileRange(absPath, start, end)
	if err != nil {
		return FileContext{}, err
	}

	if line > totalLines {
		return FileContext{}, fmt.Errorf("line %d is outside file (total lines: %d)", line, totalLines)
	}

	return FileContext{
		Repo:       resolvedRepo,
		Path:       displayPath,
		AbsPath:    absPath,
		Line:       line,
		Before:     before,
		After:      after,
		StartLine:  resolvedStart,
		EndLine:    resolvedEnd,
		TotalLines: totalLines,
		Truncated:  truncated,
		Content:    content,
	}, nil
}

func (s *Service) resolveFilePath(repo, filePath string) (absPath, resolvedRepo, displayPath string, err error) {
	filePath = strings.TrimSpace(filePath)
	repo = strings.TrimSpace(repo)
	if filePath == "" {
		return "", "", "", fmt.Errorf("path is required")
	}

	if filepath.IsAbs(filePath) {
		absPath, err = filepath.Abs(filePath)
		if err != nil {
			return "", "", "", fmt.Errorf("resolve absolute file path: %w", err)
		}
		absPath = filepath.Clean(absPath)
		if !s.isWithinAllowedRoots(absPath) {
			return "", "", "", fmt.Errorf("path %q is outside allowed roots", absPath)
		}
		if repo != "" {
			repoRoot, resolved, resolveErr := s.resolveRepoRoot(repo)
			if resolveErr != nil {
				return "", "", "", resolveErr
			}
			withinRepo, relErr := paths.IsWithinRoot(absPath, repoRoot)
			if relErr != nil || !withinRepo {
				return "", "", "", fmt.Errorf("path %q is outside repo %q", absPath, repo)
			}
			relPath, relErr := filepath.Rel(repoRoot, absPath)
			if relErr != nil {
				return "", "", "", fmt.Errorf("resolve repo-relative path: %w", relErr)
			}
			return absPath, resolved, filepath.ToSlash(filepath.Clean(relPath)), nil
		}
		return absPath, "", filepath.ToSlash(absPath), nil
	}

	if repo == "" {
		return "", "", "", fmt.Errorf("repo is required when path is relative")
	}

	repoRoot, resolvedRepo, err := s.resolveRepoRoot(repo)
	if err != nil {
		return "", "", "", err
	}

	joined := filepath.Join(repoRoot, filePath)
	absPath, err = filepath.Abs(joined)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve file path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	withinRepo, relErr := paths.IsWithinRoot(absPath, repoRoot)
	if relErr != nil || !withinRepo {
		return "", "", "", fmt.Errorf("path %q escapes repo root", filePath)
	}
	if !s.isWithinAllowedRoots(absPath) {
		return "", "", "", fmt.Errorf("path %q is outside allowed roots", absPath)
	}

	displayPath = filepath.ToSlash(filepath.Clean(filePath))
	return absPath, resolvedRepo, displayPath, nil
}

func (s *Service) resolveRepoRoot(repo string) (string, string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", fmt.Errorf("repo is required")
	}

	if filepath.IsAbs(repo) {
		abs, err := filepath.Abs(repo)
		if err != nil {
			return "", "", fmt.Errorf("resolve repo path: %w", err)
		}
		abs = filepath.Clean(abs)
		if !dirExists(abs) {
			return "", "", fmt.Errorf("repo path not found: %s", abs)
		}
		if !s.isWithinAllowedRoots(abs) {
			return "", "", fmt.Errorf("repo %q is outside allowed roots", repo)
		}
		return abs, repo, nil
	}

	candidates := s.repoCandidates(repo)
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("repo %q cannot be resolved under allowed roots", repo)
	}
	if len(candidates) > 1 {
		return "", "", fmt.Errorf("repo %q is ambiguous (%s)", repo, strings.Join(candidates, ", "))
	}
	return candidates[0], repo, nil
}

func (s *Service) repoCandidates(repo string) []string {
	if len(s.allowedList) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	candidates := make([]string, 0, 8)
	add := func(candidate string) {
		candidate = filepath.Clean(candidate)
		if !dirExists(candidate) || !s.isWithinAllowedRoots(candidate) {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	repo = filepath.Clean(repo)
	for _, root := range s.allowedList {
		add(filepath.Join(root, repo))
	}
	if len(candidates) > 0 {
		slices.Sort(candidates)
		return candidates
	}

	for _, root := range s.allowedList {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == repo && dirExists(filepath.Join(path, ".git")) {
				add(path)
			}
			if _, excluded := s.excludeDirs[name]; excluded && path != root {
				return filepath.SkipDir
			}
			return nil
		})
	}

	slices.Sort(candidates)
	return candidates
}

func normalizeLineBounds(startLine, endLine int) (int, int, error) {
	if startLine < 0 {
		return 0, 0, fmt.Errorf("start_line cannot be negative")
	}
	if endLine < 0 {
		return 0, 0, fmt.Errorf("end_line cannot be negative")
	}
	if startLine == 0 {
		startLine = 1
	}
	if endLine > 0 && endLine < startLine {
		return 0, 0, fmt.Errorf("end_line must be greater than or equal to start_line")
	}
	if endLine > 0 && endLine-startLine+1 > maxReturnedLines {
		return 0, 0, fmt.Errorf("requested range exceeds maximum of %d lines", maxReturnedLines)
	}
	return startLine, endLine, nil
}

func readFileRange(path string, startLine, endLine int) (string, int, int, int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, 0, 0, false, fmt.Errorf("read file %q: %w", path, err)
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	totalLines := len(lines)
	if totalLines == 0 {
		return "", 0, 0, 0, false, nil
	}

	if startLine > totalLines {
		return "", totalLines, totalLines + 1, totalLines, false, nil
	}
	if endLine == 0 || endLine > totalLines {
		endLine = totalLines
	}
	truncated := false
	if endLine-startLine+1 > maxReturnedLines {
		endLine = startLine + maxReturnedLines - 1
		if endLine > totalLines {
			endLine = totalLines
		}
		truncated = true
	}

	selected := lines[startLine-1 : endLine]
	return strings.Join(selected, "\n"), totalLines, startLine, endLine, truncated, nil
}
