package bine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCheckRetryHonorsGitHubRetryAfter(t *testing.T) {
	resp := retryResponse(http.StatusForbidden, "api.github.com", http.Header{"Retry-After": {"2"}})

	retry, err := checkRetry(context.Background(), resp, nil)
	assert.NilError(t, err)
	assert.Assert(t, retry)
	assert.Equal(t, retryBackoff(time.Second, 30*time.Second, 0, resp), 2*time.Second)
}

func TestCheckRetryDoesNotRetryOtherForbiddenResponses(t *testing.T) {
	resp := retryResponse(http.StatusForbidden, "example.com", http.Header{"Retry-After": {"2"}})

	retry, err := checkRetry(context.Background(), resp, nil)
	assert.NilError(t, err)
	assert.Assert(t, !retry)
}

func TestRetryClientPreservesFinalGitHubRateLimitResponse(t *testing.T) {
	client := newRetryClient()
	client.RetryMax = 1
	attempts := 0
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Retry-After": {"0"}},
			Body:       io.NopCloser(strings.NewReader("rate limited")),
			Request:    req,
		}, nil
	})

	_, err := ghLatestVersion(context.Background(), client.StandardClient(), &bin{}, "", "foo", "bar")
	assert.Error(t, err, "GitHub API returned status 403: rate limited; retry after 0s")
	assert.Equal(t, attempts, 2)
}

func TestRetryClientHonorsOuterRedirectPolicy(t *testing.T) {
	retry := newRetryClient()
	requests := 0
	retry.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {"http://example.com/insecure"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})
	client := retry.StandardClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/start", nil)
	assert.NilError(t, err)
	_, err = client.Do(req)
	assert.ErrorContains(t, err, "redirect refused")
	assert.Equal(t, requests, 1)
}

func TestGitHubAPIStatusErrorReportsPrimaryLimitReset(t *testing.T) {
	reset := time.Date(2026, time.August, 20, 14, 30, 0, 0, time.UTC)
	resp := retryResponse(http.StatusForbidden, "api.github.com", make(http.Header))
	resp.Header.Set("X-RateLimit-Remaining", "0")
	resp.Header.Set("X-RateLimit-Reset", "1787236200")

	err := githubAPIStatusError(resp)
	assert.Error(t, err, "GitHub API returned status 403: rate limit exhausted; resets at "+reset.Format(time.RFC3339))
}

func TestGitHubAPIStatusErrorReportsSecondaryLimitDelay(t *testing.T) {
	resp := retryResponse(http.StatusTooManyRequests, "api.github.com", http.Header{"Retry-After": {"30"}})

	err := githubAPIStatusError(resp)
	assert.Error(t, err, "GitHub API returned status 429: rate limited; retry after 30s")
}

func retryResponse(status int, host string, header http.Header) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Request: &http.Request{
			URL: &url.URL{Scheme: "https", Host: host},
		},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
