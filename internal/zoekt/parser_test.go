package zoekt

import "testing"

func TestParseSearchResultsNestedResult(t *testing.T) {
	body := []byte(`{
		"Result": {
			"FileMatches": [
				{
					"Repository": "test-repo",
					"FileName": "main.go",
					"LineMatches": [
						{
							"Line": "func main() {",
							"LineNumber": 42
						}
					]
				}
			]
		}
	}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Repo != "test-repo" || results[0].File != "main.go" || results[0].Line != 42 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestParseSearchResultsLowercaseSearchResult(t *testing.T) {
	body := []byte(`{
		"searchResult": {
			"fileMatches": [
				{
					"repository": "myrepo",
					"fileName": "app.go",
					"lineMatches": [
						{
							"line": "package main",
							"lineNumber": 1
						}
					]
				}
			]
		}
	}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Repo != "myrepo" || results[0].Line != 1 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestParseSearchResultsChunkMatches(t *testing.T) {
	body := []byte(`{
		"Result": {
			"FileMatches": [
				{
					"Repo": "test-repo",
					"File": "test.go",
					"ChunkMatches": [
						{
							"Content": "func test() {\n  t.Error(\"fail\")\n}",
							"Ranges": [
								{
									"Start": {
										"LineNumber": 10
									}
								}
							]
						}
					]
				}
			]
		}
	}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Line != 10 {
		t.Fatalf("expected line 10, got %d", results[0].Line)
	}
	if results[0].Snippet != "func test() {" {
		t.Fatalf("expected first line snippet, got %q", results[0].Snippet)
	}
}

func TestParseSearchResultsFragments(t *testing.T) {
	body := []byte(`{
		"Result": {
			"FileMatches": [
				{
					"Repository": "test-repo",
					"FileName": "code.py",
					"LineMatches": [
						{
							"LineNumber": 5,
							"Fragments": [
								{"Pre": "x = ", "Match": "42", "Post": " # answer"}
							]
						}
					]
				}
			]
		}
	}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Snippet != "x = 42 # answer" {
		t.Fatalf("expected reconstructed line, got %q", results[0].Snippet)
	}
}

func TestParseSearchResultsEmpty(t *testing.T) {
	body := []byte(`{"Result": {"FileMatches": []}}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestParseReposSimpleArray(t *testing.T) {
	body := []byte(`{"repos": ["repo-a", "repo-b", "repo-c"]}`)

	results, err := ParseRepos(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(results))
	}
}

func TestParseReposObjectArray(t *testing.T) {
	body := []byte(`{
		"Repositories": [
			{"name": "repo-x"},
			{"Name": "repo-y"},
			{"repo": "repo-z"}
		]
	}`)

	results, err := ParseRepos(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(results))
	}
	for _, r := range results {
		if r == "" {
			t.Fatalf("got empty repo name")
		}
	}
}

func TestParseReposLowercaseRepositories(t *testing.T) {
	body := []byte(`{
		"repositories": ["proj-1", "proj-2"]
	}`)

	results, err := ParseRepos(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(results))
	}
}

func TestParseReposGenericWalk(t *testing.T) {
	body := []byte(`{
		"data": {
			"repositories": [
				{
					"name": "deep-repo"
				}
			]
		}
	}`)

	results, err := ParseRepos(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(results))
	}
	if results[0] != "deep-repo" {
		t.Fatalf("expected deep-repo, got %q", results[0])
	}
}

func TestParseReposNoDuplicate(t *testing.T) {
	body := []byte(`{
		"repos": ["dup", "dup", "unique"],
		"repositories": ["dup"]
	}`)

	results, err := ParseRepos(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 unique repos, got %d: %v", len(results), results)
	}
}

func TestParseReposEmpty(t *testing.T) {
	body := []byte(`{"Result": {"FileMatches": []}}`)

	_, err := ParseRepos(body)
	if err == nil {
		t.Fatalf("expected error for no repos found")
	}
}

func TestParseSearchResultsRealWorldFormat(t *testing.T) {
	// Simulates actual Zoekt response with mixed casing
	body := []byte(`{
		"Result": {
			"FileMatches": [
				{
					"Repository": "github.com/user/project",
					"FileName": "src/utils.js",
					"LineMatches": [
						{
							"Line": "export function helper() {",
							"LineNumber": 42
						},
						{
							"Line": "  return 'done';",
							"LineNumber": 43
						}
					]
				}
			]
		}
	}`)

	results, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func BenchmarkParseSearchResults(b *testing.B) {
	body := []byte(`{
		"Result": {
			"FileMatches": [
				{
					"Repository": "test-repo",
					"FileName": "main.go",
					"LineMatches": [
						{
							"Line": "func main() {",
							"LineNumber": 42
						}
					]
				}
			]
		}
	}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseSearchResults(body)
	}
}

func BenchmarkParseRepos(b *testing.B) {
	body := []byte(`{"repos": ["repo-a", "repo-b", "repo-c", "repo-d", "repo-e"]}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseRepos(body)
	}
}
