package zoekt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseSearchResults parses the search response body from Zoekt HTTP API.
// Handles multiple response formats to support different Zoekt versions.
func ParseSearchResults(body []byte) ([]SearchResult, error) {
	type rawPosition struct {
		LineNumber int `json:"LineNumber"`
		LineNum    int `json:"lineNumber"`
		Line       int `json:"line"`
	}
	type rawRange struct {
		Start rawPosition `json:"Start"`
		End   rawPosition `json:"End"`
	}
	type rawChunkMatch struct {
		Content        string     `json:"Content"`
		ContentLower   string     `json:"content"`
		Ranges         []rawRange `json:"Ranges"`
		RangesLower    []rawRange `json:"ranges"`
		ContentSnippet string     `json:"ContentSnippet"`
	}
	type rawMatch struct {
		Line       string `json:"Line"`
		LineLower  string `json:"line"`
		LineNumber int    `json:"LineNumber"`
		LineNum    int    `json:"lineNumber"`
		LineNumAlt int    `json:"LineNum"`
		Fragments  []struct {
			Pre   string `json:"Pre"`
			Match string `json:"Match"`
			Post  string `json:"Post"`
		} `json:"Fragments"`
	}
	type rawFileMatch struct {
		Repository      string          `json:"Repository"`
		RepositoryLower string          `json:"repository"`
		Repo            string          `json:"Repo"`
		FileName        string          `json:"FileName"`
		FileNameLower   string          `json:"fileName"`
		File            string          `json:"File"`
		FileLower       string          `json:"file"`
		Path            string          `json:"path"`
		LineMatches     []rawMatch      `json:"LineMatches"`
		LineMatchesLow  []rawMatch      `json:"lineMatches"`
		Matches         []rawMatch      `json:"Matches"`
		ChunkMatches    []rawChunkMatch `json:"ChunkMatches"`
		ChunkMatchesLow []rawChunkMatch `json:"chunkMatches"`
	}
	type searchPayload struct {
		FileMatches []rawFileMatch `json:"FileMatches"`
		Files       []rawFileMatch `json:"Files"`
	}
	type response struct {
		Result       searchPayload `json:"Result"`
		SearchResult searchPayload `json:"SearchResult"`
		ResultLower  searchPayload `json:"result"`
		SearchLower  searchPayload `json:"searchResult"`
	}

	var top response
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}

	all := make([]rawFileMatch, 0, len(top.Result.FileMatches)+len(top.Result.Files)+len(top.SearchResult.FileMatches)+len(top.SearchResult.Files))
	all = append(all, top.Result.FileMatches...)
	all = append(all, top.Result.Files...)
	all = append(all, top.SearchResult.FileMatches...)
	all = append(all, top.SearchResult.Files...)
	all = append(all, top.ResultLower.FileMatches...)
	all = append(all, top.ResultLower.Files...)
	all = append(all, top.SearchLower.FileMatches...)
	all = append(all, top.SearchLower.Files...)

	if len(all) == 0 {
		// Attempt flat shape fallback.
		var flat struct {
			FileMatches []rawFileMatch `json:"FileMatches"`
			Files       []rawFileMatch `json:"Files"`
		}
		if err := json.Unmarshal(body, &flat); err == nil {
			all = append(all, flat.FileMatches...)
			all = append(all, flat.Files...)
		}
	}

	results := make([]SearchResult, 0)
	for _, fm := range all {
		repo := fm.Repository
		if repo == "" {
			repo = fm.RepositoryLower
		}
		if repo == "" {
			repo = fm.Repo
		}
		file := fm.FileName
		if file == "" {
			file = fm.FileNameLower
		}
		if file == "" {
			file = fm.File
		}
		if file == "" {
			file = fm.FileLower
		}
		if file == "" {
			file = fm.Path
		}

		lines := fm.LineMatches
		if len(lines) == 0 {
			lines = fm.LineMatchesLow
		}
		if len(lines) == 0 {
			lines = fm.Matches
		}
		if len(lines) == 0 {
			chunks := fm.ChunkMatches
			if len(chunks) == 0 {
				chunks = fm.ChunkMatchesLow
			}
			for _, c := range chunks {
				snippet := c.Content
				if snippet == "" {
					snippet = c.ContentLower
				}
				if snippet == "" {
					snippet = c.ContentSnippet
				}
				snippet = firstLine(snippet)

				ranges := c.Ranges
				if len(ranges) == 0 {
					ranges = c.RangesLower
				}
				lineNumber := 0
				if len(ranges) > 0 {
					lineNumber = ranges[0].Start.LineNumber
					if lineNumber == 0 {
						lineNumber = ranges[0].Start.LineNum
					}
					if lineNumber == 0 {
						lineNumber = ranges[0].Start.Line
					}
				}
				results = append(results, SearchResult{
					Repo:    repo,
					File:    file,
					Line:    lineNumber,
					Snippet: snippet,
				})
			}
		}
		if len(lines) == 0 && len(fm.ChunkMatches) == 0 && len(fm.ChunkMatchesLow) == 0 {
			results = append(results, SearchResult{
				Repo:    repo,
				File:    file,
				Line:    0,
				Snippet: "",
			})
			continue
		}
		for _, m := range lines {
			line := m.Line
			if line == "" {
				line = m.LineLower
			}
			if line == "" && len(m.Fragments) > 0 {
				var b strings.Builder
				for _, f := range m.Fragments {
					b.WriteString(f.Pre)
					b.WriteString(f.Match)
					b.WriteString(f.Post)
				}
				line = b.String()
			}
			lineNumber := m.LineNumber
			if lineNumber == 0 {
				lineNumber = m.LineNum
			}
			if lineNumber == 0 {
				lineNumber = m.LineNumAlt
			}
			results = append(results, SearchResult{
				Repo:    repo,
				File:    file,
				Line:    lineNumber,
				Snippet: line,
			})
		}
	}
	return results, nil
}

// ParseRepos parses the repos list response from Zoekt HTTP API.
// Handles multiple response formats to support different Zoekt versions.
func ParseRepos(body []byte) ([]string, error) {
	// Common shapes:
	// {\"repos\":[\"a\",\"b\"]} or {\"Repositories\":[{\"name\":\"a\"}]} or {\"RepoURLs\":[\"a\",\"b\"]}
	var shape1 struct {
		Repos        []string `json:"repos"`
		Repositories []string `json:"repositories"`
		RepoURLs     []string `json:"RepoURLs"`
	}
	if err := json.Unmarshal(body, &shape1); err == nil {
		repos := uniqueStrings(append(append(shape1.Repos, shape1.Repositories...), shape1.RepoURLs...))
		if len(repos) > 0 {
			return repos, nil
		}
	}

	var shape2 struct {
		Repositories []struct {
			Name  string `json:"name"`
			NameU string `json:"Name"`
			Repo  string `json:"repo"`
			RepoU string `json:"Repo"`
		} `json:"Repositories"`
		Repos []struct {
			Name  string `json:"name"`
			NameU string `json:"Name"`
			Repo  string `json:"repo"`
			RepoU string `json:"Repo"`
		} `json:"repos"`
	}
	if err := json.Unmarshal(body, &shape2); err != nil {
		// Continue with generic payload traversal below.
	} else {
		repos := make([]string, 0, len(shape2.Repositories)+len(shape2.Repos))
		for _, r := range shape2.Repositories {
			name := firstNonEmpty(r.Name, r.NameU, r.Repo, r.RepoU)
			if name != "" {
				repos = append(repos, name)
			}
		}
		for _, r := range shape2.Repos {
			name := firstNonEmpty(r.Name, r.NameU, r.Repo, r.RepoU)
			if name != "" {
				repos = append(repos, name)
			}
		}

		repos = uniqueStrings(repos)
		if len(repos) > 0 {
			return repos, nil
		}
	}

	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		return nil, err
	}
	repos := make([]string, 0, 16)
	var walk func(parentKey string, v any)
	walk = func(parentKey string, v any) {
		switch vv := v.(type) {
		case map[string]any:
			for k, child := range vv {
				lower := strings.ToLower(strings.TrimSpace(k))
				// Branch metadata often contains names unrelated to repositories.
				if lower == "branches" || lower == "branch" {
					continue
				}
				if lower == "repository" || lower == "repo" {
					if s, ok := child.(string); ok {
						repos = append(repos, s)
					}
				}
				if lower == "name" && (parentKey == "repository" || parentKey == "repo" || parentKey == "repositories" || parentKey == "repos") {
					if s, ok := child.(string); ok {
						repos = append(repos, s)
					}
				}
				walk(lower, child)
			}
		case []any:
			for _, child := range vv {
				walk(parentKey, child)
			}
		}
	}
	walk("", generic)
	repos = uniqueStrings(repos)
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found in payload")
	}
	return repos, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if idx := strings.IndexByte(v, '\n'); idx >= 0 {
		return strings.TrimSpace(v[:idx])
	}
	return v
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
