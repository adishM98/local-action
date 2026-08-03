// Package update checks GitHub's releases API for a newer version than the
// one currently running. Check-only, never installs anything — see
// docs/RELEASE.md for why: this isn't code-signed yet, so silently
// downloading and running an update would be a real security hole, not
// just a rough edge.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// var, not const, so tests can point it at an httptest.Server instead of
// hitting the real GitHub API.
var releasesURL = "https://api.github.com/repos/adishM98/local-action/releases/latest"

type Info struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion,omitempty"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// Check compares currentVersion against GitHub's latest release tag. Any
// failure (offline, rate-limited, unexpected response shape, current
// version not a real build) just resolves to no update available — this is
// a nice-to-have notice, never something that should surface as an error.
func Check(ctx context.Context, currentVersion string) Info {
	info := Info{CurrentVersion: currentVersion}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return info
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info
	}

	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return info
	}

	latest := strings.TrimPrefix(body.TagName, "v")
	info.LatestVersion = latest
	info.ReleaseURL = body.HTMLURL
	info.UpdateAvailable = isNewer(latest, currentVersion)
	return info
}

// isNewer reports whether latest is a greater semver than current.
// Malformed/non-numeric components compare as 0, and "dev" (the default
// when a binary is built without the version ldflag, e.g. `go build`
// directly) is always treated as NOT having an update — there's no
// meaningful "newer than dev" to report for a local development build.
func isNewer(latest, current string) bool {
	if current == "" || current == "dev" {
		return false
	}
	lp, cp := parseVersion(latest), parseVersion(current)
	for i := 0; i < 3; i++ {
		if lp[i] != cp[i] {
			return lp[i] > cp[i]
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}
		out[i] = n
	}
	return out
}
