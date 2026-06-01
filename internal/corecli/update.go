package corecli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Update-related helpers: discover the latest GitHub release, compare it
// to the running version, and throttle how often the CLI checks. The repo
// is public, so the GitHub API needs no auth.
const (
	updateRepoOwner = "dakasa-yggdrasil"
	updateRepoName  = "yggdrasil"
	// GitHubAPIBase is overridable in tests.
	GitHubAPIBase = "https://api.github.com"
	// UpdateCheckInterval is the throttle window for automatic checks.
	UpdateCheckInterval = 24 * time.Hour
)

// LatestReleaseTag returns the tag_name of the newest GitHub release (e.g.
// "v0.2.1"). apiBase defaults to api.github.com; httpClient defaults to a
// 5s-timeout client. Both are injectable for tests.
func LatestReleaseTag(ctx context.Context, httpClient *http.Client, apiBase string) (string, error) {
	if apiBase == "" {
		apiBase = GitHubAPIBase
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	url := apiBase + "/repos/" + updateRepoOwner + "/" + updateRepoName + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("github releases/latest: HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("github returned no tag_name")
	}
	return rel.TagName, nil
}

// IsNewerVersion reports whether latest is a strictly newer semver than
// current. A leading 'v' on either side is tolerated. A non-semver current
// (e.g. "dev" source builds) or an unparseable latest returns false — the
// CLI never nags when it cannot be certain.
func IsNewerVersion(current, latest string) bool {
	cur, okC := parseSemver(current)
	lat, okL := parseSemver(latest)
	if !okC || !okL {
		return false
	}
	for i := 0; i < 3; i++ {
		if lat[i] != cur[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// ShouldCheckForUpdate returns true when the last check is older than
// interval, or never happened.
func ShouldCheckForUpdate(lastCheckedUnix int64, now time.Time, interval time.Duration) bool {
	if lastCheckedUnix <= 0 {
		return true
	}
	return now.Sub(time.Unix(lastCheckedUnix, 0)) >= interval
}
