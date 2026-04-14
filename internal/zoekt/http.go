package zoekt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

var searchEndpointCandidates = []string{
	"/api/search",
	"/search",
}

var reposEndpointCandidates = []string{
	"/api/repos",
	"/api/list",
}

type RetryStrategy interface {
	MaxAttempts() int
	NextDelay(attempt int) time.Duration
	RetryableRequestError(err error) bool
	RetryableStatus(status int) bool
}

type ExponentialRetryStrategy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

func DefaultRetryStrategy() RetryStrategy {
	return ExponentialRetryStrategy{
		maxAttempts: 3,
		baseDelay:   100 * time.Millisecond,
		maxDelay:    800 * time.Millisecond,
	}
}

func (s ExponentialRetryStrategy) MaxAttempts() int {
	if s.maxAttempts <= 0 {
		return 1
	}
	return s.maxAttempts
}

func (s ExponentialRetryStrategy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	base := s.baseDelay
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	max := s.maxDelay
	if max <= 0 {
		max = 800 * time.Millisecond
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

func (s ExponentialRetryStrategy) RetryableRequestError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable")
}

func (s ExponentialRetryStrategy) RetryableStatus(status int) bool {
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status <= 599
}

type HTTPSearcher struct {
	baseURL *url.URL
	client  *http.Client
	retry   RetryStrategy
}

func NewHTTPSearcher(baseURL string, timeout time.Duration) (*HTTPSearcher, error) {
	return NewHTTPSearcherWithRetryStrategy(baseURL, timeout, DefaultRetryStrategy())
}

func NewHTTPSearcherWithRetryStrategy(baseURL string, timeout time.Duration, retry RetryStrategy) (*HTTPSearcher, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q", baseURL)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if retry == nil {
		retry = DefaultRetryStrategy()
	}

	return &HTTPSearcher{
		baseURL: parsed,
		client:  &http.Client{Timeout: timeout},
		retry:   retry,
	}, nil
}

func (h *HTTPSearcher) Search(ctx context.Context, query, repo string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 10
	}

	candidates := []string{query}
	escaped := regexp.QuoteMeta(query)
	if escaped != query {
		candidates = append(candidates, escaped)
	}

	var lastErr error
	for _, candidateQuery := range candidates {
		results, err := h.searchAcrossEndpoints(ctx, candidateQuery, repo, limit)
		if err != nil {
			lastErr = err
			continue
		}

		if len(results) > 0 || !containsSymbolFilter(candidateQuery) {
			return results, nil
		}

		// Some Zoekt indexes are built without symbol data; keep DSL support but
		// transparently retry as plain text to avoid false negatives.
		fallbackQuery := symbolFallbackQuery(candidateQuery)
		if fallbackQuery == candidateQuery {
			return results, nil
		}
		fallbackResults, fallbackErr := h.searchAcrossEndpoints(ctx, fallbackQuery, repo, limit)
		if fallbackErr == nil {
			return fallbackResults, nil
		}
		// Original query was valid and executed, so preserve empty-success
		// semantics instead of converting this into an error.
		return results, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no search endpoint tried")
	}
	return nil, fmt.Errorf("zoekt search failed: %w", lastErr)
}

func (h *HTTPSearcher) searchAcrossEndpoints(ctx context.Context, query, repo string, limit int) ([]SearchResult, error) {
	var lastErr error
	emptySuccess := false
	for _, endpoint := range searchEndpointCandidates {
		results, err := h.searchOnce(ctx, endpoint, query, repo, limit)
		if err != nil {
			lastErr = err
			continue
		}
		if len(results) > 0 {
			return results, nil
		}
		emptySuccess = true
	}
	if emptySuccess {
		return []SearchResult{}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no search endpoint tried")
	}
	return nil, lastErr
}

func (h *HTTPSearcher) ListRepos(ctx context.Context) ([]string, error) {
	var lastErr error
	for _, endpoint := range reposEndpointCandidates {
		repos, err := h.listReposOnce(ctx, endpoint)
		if err == nil {
			return repos, nil
		}
		lastErr = err
	}

	// Fallback: infer repositories from a small search request when dedicated endpoint is unavailable.
	results, err := h.Search(ctx, ".", "", 20)
	if err != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("list repos failed (%v), fallback search failed: %w", lastErr, err)
		}
		return nil, fmt.Errorf("list repos fallback search failed: %w", err)
	}

	seen := make(map[string]struct{})
	for _, r := range results {
		if r.Repo == "" {
			continue
		}
		seen[r.Repo] = struct{}{}
	}

	repos := make([]string, 0, len(seen))
	for repo := range seen {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos, nil
}

func (h *HTTPSearcher) searchOnce(ctx context.Context, endpoint, query, repo string, limit int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("q", buildQuery(query, repo))
	params.Set("num", fmt.Sprintf("%d", limit))
	params.Set("format", "json")

	body, status, err := h.get(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found: %s", endpoint)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("unexpected status %d on %s", status, endpoint)
	}

	results, err := ParseSearchResults(body)
	if err != nil {
		return nil, fmt.Errorf("parse search response from %s: %w", endpoint, err)
	}

	if repo != "" {
		filtered := make([]SearchResult, 0, len(results))
		for _, r := range results {
			if r.Repo == repo {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (h *HTTPSearcher) listReposOnce(ctx context.Context, endpoint string) ([]string, error) {
	body, status, err := h.get(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found: %s", endpoint)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("unexpected status %d on %s", status, endpoint)
	}

	repos, err := ParseRepos(body)
	if err != nil {
		return nil, fmt.Errorf("parse repos response from %s: %w", endpoint, err)
	}
	sort.Strings(repos)
	return repos, nil
}

func (h *HTTPSearcher) get(ctx context.Context, endpoint string, params url.Values) ([]byte, int, error) {
	u := *h.baseURL
	u.Path = path.Join(h.baseURL.Path, endpoint)
	if params != nil {
		u.RawQuery = params.Encode()
	}

	var lastErr error
	var lastStatus int
	maxAttempts := h.retry.MaxAttempts()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}

		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			if !h.retry.RetryableRequestError(err) || attempt == maxAttempts {
				return nil, 0, lastErr
			}
			if sleepErr := sleepWithContext(ctx, h.retry.NextDelay(attempt)); sleepErr != nil {
				return nil, 0, sleepErr
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response body: %w", readErr)
			if attempt == maxAttempts {
				return nil, resp.StatusCode, lastErr
			}
			if sleepErr := sleepWithContext(ctx, h.retry.NextDelay(attempt)); sleepErr != nil {
				return nil, resp.StatusCode, sleepErr
			}
			continue
		}

		lastStatus = resp.StatusCode
		if h.retry.RetryableStatus(resp.StatusCode) && attempt < maxAttempts {
			if sleepErr := sleepWithContext(ctx, h.retry.NextDelay(attempt)); sleepErr != nil {
				return nil, resp.StatusCode, sleepErr
			}
			continue
		}
		return body, resp.StatusCode, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed with status %d", lastStatus)
	}
	return nil, lastStatus, lastErr
}

func buildQuery(query, repo string) string {
	query = strings.TrimSpace(query)
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return query
	}
	return fmt.Sprintf("r:^%s$ %s", regexp.QuoteMeta(repo), query)
}

func containsSymbolFilter(query string) bool {
	return strings.Contains(query, "sym:")
}

var symbolQueryPattern = regexp.MustCompile(`\bsym:("([^"\\]|\\.)*"|'([^'\\]|\\.)*'|[^\s]+)`)

func symbolFallbackQuery(query string) string {
	normalized := symbolQueryPattern.ReplaceAllStringFunc(query, func(match string) string {
		return strings.TrimPrefix(match, "sym:")
	})
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" {
		return query
	}
	return normalized
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
