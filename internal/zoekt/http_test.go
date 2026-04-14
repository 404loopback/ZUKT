package zoekt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type immediateRetryStrategy struct {
	attempts int
}

func (s immediateRetryStrategy) MaxAttempts() int {
	if s.attempts <= 0 {
		return 1
	}
	return s.attempts
}

func (s immediateRetryStrategy) NextDelay(int) time.Duration {
	return 0
}

func (s immediateRetryStrategy) RetryableRequestError(error) bool {
	return true
}

func (s immediateRetryStrategy) RetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func TestHTTPSearcherSearch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Result": {
				"FileMatches": [
					{
						"Repository": "local/repo",
						"FileName": "main.go",
						"LineMatches": [{"Line":"func main() {}", "LineNumber": 10}]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Repo != "local/repo" || results[0].File != "main.go" || results[0].Line != 10 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestHTTPSearcherDoesNotRetryOnBadRequest(t *testing.T) {
	t.Parallel()

	apiSearchCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			apiSearchCalls++
			http.Error(w, "bad request", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcherWithRetryStrategy(srv.URL, time.Second, immediateRetryStrategy{attempts: 4})
	if err != nil {
		t.Fatalf("NewHTTPSearcherWithRetryStrategy error: %v", err)
	}

	if _, err := searcher.Search(context.Background(), "main", "", 10); err == nil {
		t.Fatalf("expected search error")
	}
	if apiSearchCalls != 1 {
		t.Fatalf("expected exactly one /api/search call for 400 response, got %d", apiSearchCalls)
	}
}

func TestHTTPSearcherRetriesOnTooManyRequests(t *testing.T) {
	t.Parallel()

	apiSearchCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			apiSearchCalls++
			if apiSearchCalls == 1 {
				http.Error(w, "rate limit", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"Result": {
					"FileMatches": [
						{
							"Repository": "local/repo",
							"FileName": "main.go",
							"LineMatches": [{"Line":"func main() {}", "LineNumber": 11}]
						}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcherWithRetryStrategy(srv.URL, time.Second, immediateRetryStrategy{attempts: 3})
	if err != nil {
		t.Fatalf("NewHTTPSearcherWithRetryStrategy error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("Search should succeed after retry, got: %v", err)
	}
	if len(results) != 1 || results[0].Line != 11 {
		t.Fatalf("unexpected results: %#v", results)
	}
	if apiSearchCalls != 2 {
		t.Fatalf("expected two /api/search calls, got %d", apiSearchCalls)
	}
}

func TestHTTPSearcherListRepos(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repos":["repo/b","repo/a","repo/a"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	repos, err := searcher.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0] != "repo/a" || repos[1] != "repo/b" {
		t.Fatalf("unexpected repos: %#v", repos)
	}
}

func TestHTTPSearcherSearchRetryOnServerError(t *testing.T) {
	t.Parallel()

	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		attempt++
		if attempt == 1 {
			http.Error(w, "temporary error", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{
			"Result": {
				"FileMatches": [
					{
						"Repository": "local/repo",
						"FileName": "main.go",
						"LineMatches": [{"Line":"func main() {}", "LineNumber": 7}]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("Search should succeed after retry, got error: %v", err)
	}
	if len(results) != 1 || results[0].Line != 7 {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestHTTPSearcherSearchMalformedPayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"Result":`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	_, err = searcher.Search(context.Background(), "main", "", 10)
	if err == nil {
		t.Fatalf("expected parse error for malformed payload")
	}
}

func TestHTTPSearcherSearchParsesChunkMatches(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Result": {
				"FileMatches": [
					{
						"Repository": "local/repo",
						"FileName": "pkg/service.go",
						"ChunkMatches": [
							{
								"Content": "first line\nsecond line",
								"Ranges": [{"Start": {"LineNumber": 42}}]
							}
						]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "service", "", 10)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Line != 42 {
		t.Fatalf("expected line 42, got %d", results[0].Line)
	}
	if results[0].Snippet != "first line" {
		t.Fatalf("unexpected snippet: %q", results[0].Snippet)
	}
}

func TestHTTPSearcherSearchParsesLowercaseResultWithFragments(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {
				"FileMatches": [
					{
						"Repo": "ZUKT",
						"FileName": "cmd/zukt/main.go",
						"Matches": [
							{
								"LineNum": 13,
								"Fragments": [
									{"Pre":"func ","Match":"main","Post":"() {"}
								]
							}
						]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Repo != "ZUKT" || results[0].File != "cmd/zukt/main.go" {
		t.Fatalf("unexpected file result: %+v", results[0])
	}
	if results[0].Line != 13 {
		t.Fatalf("expected line 13, got %d", results[0].Line)
	}
	if results[0].Snippet != "func main() {" {
		t.Fatalf("unexpected snippet: %q", results[0].Snippet)
	}
}

func TestHTTPSearcherSearchEscapesInvalidQueryWhenNeeded(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "toolResult(") {
			http.Error(w, "invalid regex", http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Result": {
				"FileMatches": [
					{
						"Repository": "local/repo",
						"FileName": "internal/mcp/server.go",
						"LineMatches": [{"Line":"func toolResult(payload any)", "LineNumber": 10}]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "toolResult(", "", 10)
	if err != nil {
		t.Fatalf("Search should succeed with escaped fallback, got error: %v", err)
	}
	if len(results) != 1 || results[0].Line != 10 {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestHTTPSearcherListReposGenericPayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"items": [
					{"repository": {"name": "PITANCE"}},
					{"repository": {"name": "ZUKT", "branches": [{"name": "main"}]}}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	repos, err := searcher.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d (%#v)", len(repos), repos)
	}
	if repos[0] != "PITANCE" || repos[1] != "ZUKT" {
		t.Fatalf("unexpected repos: %#v", repos)
	}
}

func TestHTTPSearcherSearchFallsBackWhenSymbolQueryReturnsEmpty(t *testing.T) {
	t.Parallel()

	queries := make([]string, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("q")
		queries = append(queries, q)

		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(q, "sym:ParseSearchResults") {
			_, _ = w.Write([]byte(`{"Result":{"FileMatches":[]}}`))
			return
		}
		if strings.Contains(q, "ParseSearchResults") {
			_, _ = w.Write([]byte(`{
				"Result": {
					"FileMatches": [
						{
							"Repository": "ZUKT",
							"FileName": "internal/zoekt/parser.go",
							"LineMatches": [{"Line":"func ParseSearchResults(body []byte) ([]SearchResult, error) {", "LineNumber": 11}]
						}
					]
				}
			}`))
			return
		}
		http.Error(w, "unexpected query", http.StatusBadRequest)
	}))
	defer srv.Close()

	searcher, err := NewHTTPSearcher(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSearcher error: %v", err)
	}

	results, err := searcher.Search(context.Background(), "sym:ParseSearchResults", "", 10)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after symbol fallback, got %d (%#v)", len(results), results)
	}
	if results[0].File != "internal/zoekt/parser.go" || results[0].Line != 11 {
		t.Fatalf("unexpected fallback result: %#v", results[0])
	}
	if len(queries) < 2 {
		t.Fatalf("expected symbol query + fallback query, got %#v", queries)
	}
	if !strings.Contains(queries[0], "sym:ParseSearchResults") {
		t.Fatalf("expected first query to keep symbol DSL, got %q", queries[0])
	}
	if strings.Contains(queries[1], "sym:") {
		t.Fatalf("expected second query without symbol filter, got %q", queries[1])
	}
}

func TestSymbolFallbackQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		match bool
	}{
		{
			name:  "plain symbol",
			in:    `sym:ParseSearchResults`,
			want:  `ParseSearchResults`,
			match: true,
		},
		{
			name:  "mixed DSL",
			in:    `sym:ParseSearchResults r:ZUKT file:parser.go`,
			want:  `ParseSearchResults r:ZUKT file:parser.go`,
			match: true,
		},
		{
			name:  "quoted symbol",
			in:    `sym:"Parse Search Results" r:ZUKT`,
			want:  `"Parse Search Results" r:ZUKT`,
			match: true,
		},
		{
			name:  "no symbol filter",
			in:    `file:parser.go ParseSearchResults`,
			want:  `file:parser.go ParseSearchResults`,
			match: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := symbolFallbackQuery(tc.in)
			if got != tc.want {
				t.Fatalf("symbolFallbackQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if containsSymbolFilter(tc.in) != tc.match {
				t.Fatalf("containsSymbolFilter(%q) mismatch", tc.in)
			}
		})
	}
}
