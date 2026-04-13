package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Normalize(value string) string {
	return filepath.Clean(strings.TrimSpace(value))
}

func NormalizeAndSortUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = Normalize(value)
		if value == "" || value == "." {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func NormalizeAbsolute(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func ValidateRepoPath(repo string, allowedRoots []string) (string, error) {
	repoAbs, err := NormalizeAbsolute(repo)
	if err != nil {
		return "", fmt.Errorf("repo path must be absolute: %s", strings.TrimSpace(repo))
	}
	info, err := os.Stat(repoAbs)
	if err != nil {
		return "", fmt.Errorf("repo path not accessible %s: %w", repoAbs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo path is not a directory: %s", repoAbs)
	}
	if len(allowedRoots) == 0 {
		return "", fmt.Errorf("ZOEKT_ALLOWED_ROOTS cannot be empty")
	}

	repoEval, err := filepath.EvalSymlinks(repoAbs)
	if err != nil {
		return "", fmt.Errorf("resolve repo symlinks %s: %w", repoAbs, err)
	}

	for _, root := range NormalizeAndSortUnique(allowedRoots) {
		rootAbs, absErr := NormalizeAbsolute(root)
		if absErr != nil {
			continue
		}
		rootEval := rootAbs
		if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
			rootEval = resolved
		}

		if within, err := IsWithinRoot(repoEval, rootEval); err == nil && within {
			return repoEval, nil
		}
	}
	return "", fmt.Errorf("repo path %s is outside ZOEKT_ALLOWED_ROOTS", repoAbs)
}

func IsWithinRoot(target, root string) (bool, error) {
	target = filepath.Clean(target)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." {
		return false, nil
	}
	prefix := ".." + string(filepath.Separator)
	return !strings.HasPrefix(rel, prefix), nil
}

func ContainerSourcePath(baseName string) string {
	return filepath.ToSlash(filepath.Join("/data/srcroot", strings.TrimSpace(baseName)))
}
