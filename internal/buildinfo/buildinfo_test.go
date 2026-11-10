package buildinfo

import (
	"strings"
	"testing"
)

func TestGetDefaults(t *testing.T) {
	info := Get()
	if info.Version != "dev" {
		t.Errorf("Version = %q, want dev", info.Version)
	}
	if info.Commit != "unknown" {
		t.Errorf("Commit = %q, want unknown", info.Commit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("BuildTime = %q, want unknown", info.BuildTime)
	}
	if info.GoVersion == "" {
		t.Error("GoVersion is empty, want the toolchain version")
	}
}

func TestGetReflectsInjectedValues(t *testing.T) {
	original := Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
	Version, Commit, BuildTime = "1.2.3", "abc1234", "2026-01-02T03:04:05Z"
	defer func() { Version, Commit, BuildTime = original.Version, original.Commit, original.BuildTime }()

	info := Get()
	if info.Version != "1.2.3" || info.Commit != "abc1234" || info.BuildTime != "2026-01-02T03:04:05Z" {
		t.Fatalf("Get() = %+v, want the injected values", info)
	}
}

func TestRenderContainsAllFields(t *testing.T) {
	original := Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
	Version, Commit, BuildTime = "9.9.9", "deadbee", "2026-01-02T03:04:05Z"
	defer func() { Version, Commit, BuildTime = original.Version, original.Commit, original.BuildTime }()

	line := Get().Render()
	for _, want := range []string{"media-core", "9.9.9", "deadbee", "2026-01-02T03:04:05Z"} {
		if !strings.Contains(line, want) {
			t.Errorf("Render() = %q, want it to contain %q", line, want)
		}
	}
}
