package corecli

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.2.0", "0.2.1", true},
		{"0.2.0", "v0.2.1", true}, // tolerate a leading v on either side
		{"v0.2.1", "v0.2.1", false},
		{"0.2.1", "0.2.0", false}, // current already newer
		{"0.3.0", "0.2.9", false},
		{"1.0.0", "0.9.9", false},
		{"0.9.9", "1.0.0", true},
		{"dev", "0.2.1", false},   // source build: don't nag
		{"0.2.0", "garbage", false}, // unparseable latest: stay quiet
	}
	for _, c := range cases {
		if got := IsNewerVersion(c.current, c.latest); got != c.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestShouldCheckForUpdate(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	day := 24 * time.Hour
	// never checked → yes
	if !ShouldCheckForUpdate(0, now, day) {
		t.Error("expected check when never checked before")
	}
	// just checked → no
	if ShouldCheckForUpdate(now.Unix(), now, day) {
		t.Error("expected no check immediately after a check")
	}
	// checked 25h ago → yes
	if !ShouldCheckForUpdate(now.Add(-25*time.Hour).Unix(), now, day) {
		t.Error("expected check after the interval elapsed")
	}
}

func TestLatestReleaseTag(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"tag_name": "v0.2.1"})
	})
	tag, err := LatestReleaseTag(context.Background(), fs.server.Client(), fs.server.URL)
	if err != nil {
		t.Fatalf("LatestReleaseTag: %v", err)
	}
	if tag != "v0.2.1" {
		t.Errorf("tag = %q, want v0.2.1", tag)
	}
	if !strings.HasSuffix(fs.last.path, "/repos/dakasa-yggdrasil/yggdrasil/releases/latest") {
		t.Errorf("wrong path: %s", fs.last.path)
	}
}
