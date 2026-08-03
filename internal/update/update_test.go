package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want             bool
	}{
		{"0.6.0", "0.5.0", true},
		{"0.5.0", "0.5.0", false},
		{"0.5.0", "0.6.0", false},
		{"1.0.0", "0.9.9", true},
		{"0.5.1", "0.5.0", true},
		{"v0.6.0", "0.5.0", true}, // leading "v" tolerated on either side
		{"0.6.0", "dev", false},
		{"garbage", "0.5.0", false}, // unparseable components compare as 0
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestCheck_UpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.6.0","html_url":"https://github.com/adishM98/local-action/releases/tag/v0.6.0"}`))
	}))
	defer srv.Close()
	releasesURL = srv.URL
	defer func() { releasesURL = "https://api.github.com/repos/adishM98/local-action/releases/latest" }()

	info := Check(context.Background(), "0.5.0")
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable true")
	}
	if info.LatestVersion != "0.6.0" {
		t.Errorf("LatestVersion = %q, want 0.6.0", info.LatestVersion)
	}
	if info.ReleaseURL == "" {
		t.Error("expected non-empty ReleaseURL")
	}
	if info.CurrentVersion != "0.5.0" {
		t.Errorf("CurrentVersion = %q, want 0.5.0", info.CurrentVersion)
	}
}

func TestCheck_UpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.5.0","html_url":"https://example.com"}`))
	}))
	defer srv.Close()
	releasesURL = srv.URL
	defer func() { releasesURL = "https://api.github.com/repos/adishM98/local-action/releases/latest" }()

	info := Check(context.Background(), "0.5.0")
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable false when already on the latest version")
	}
}

func TestCheck_NetworkFailureIsSilent(t *testing.T) {
	releasesURL = "http://127.0.0.1:1" // nothing listens here
	defer func() { releasesURL = "https://api.github.com/repos/adishM98/local-action/releases/latest" }()

	info := Check(context.Background(), "0.5.0")
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable false on network failure, not an error")
	}
}

func TestCheck_NonOKStatusIsSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	releasesURL = srv.URL
	defer func() { releasesURL = "https://api.github.com/repos/adishM98/local-action/releases/latest" }()

	info := Check(context.Background(), "0.5.0")
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable false on a non-200 response")
	}
}
