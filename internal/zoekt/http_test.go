package zoekt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
