package bine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func checkRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if isGitHubRetryAfterResponse(resp) {
		return true, nil
	}

	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

func retryBackoff(min, max time.Duration, attempt int, resp *http.Response) time.Duration {
	if isGitHubRetryAfterResponse(resp) {
		if delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			return delay
		}
	}

	return retryablehttp.DefaultBackoff(min, max, attempt, resp)
}

func isGitHubRetryAfterResponse(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	if !strings.EqualFold(resp.Request.URL.Hostname(), "api.github.com") {
		return false
	}

	_, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())

	return ok
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		delay := time.Duration(seconds) * time.Second
		if delay/time.Second != time.Duration(seconds) {
			return 0, false
		}

		return delay, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if delay := retryAt.Sub(now); delay > 0 {
		return delay, true
	}

	return 0, true
}

func githubAPIStatusError(resp *http.Response) error {
	status := fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return errors.New(status)
	}

	remaining, remainingErr := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if remainingErr == nil && remaining == 0 {
		if reset, ok := rateLimitReset(resp.Header.Get("X-RateLimit-Reset")); ok {
			return fmt.Errorf("%s: rate limit exhausted; resets at %s", status, reset.Format(time.RFC3339))
		}

		return fmt.Errorf("%s: rate limit exhausted", status)
	}

	if delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
		return fmt.Errorf("%s: rate limited; retry after %s", status, delay.Round(time.Second))
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%s: rate limited", status)
	}

	return errors.New(status)
}

func rateLimitReset(value string) (time.Time, bool) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}

	return time.Unix(seconds, 0).UTC(), true
}
