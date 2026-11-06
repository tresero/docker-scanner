package registry

import (
	"docker-scanner/pkg/models"
	"strings"
	"testing"
	"time"
)

// fixedNow is the clock for every test in this file, so results never depend
// on when the tests run.
var fixedNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func hoursAgo(n int) time.Time { return fixedNow.Add(-time.Duration(n) * time.Hour) }
func daysAgo(n int) time.Time  { return hoursAgo(n * 24) }

// hubTag builds a Docker Hub API tag pushed at the given time.
func hubTag(name string, pushed time.Time) DockerHubTag {
	return DockerHubTag{Name: name, LastUpdated: pushed.Format(time.RFC3339)}
}

// caddyTooNew is the #23/#24 report: five variants each of two versions, all
// pushed in the last 3 days, which fills all 10 slots of the old limit.
func caddyTooNew() []DockerHubTag {
	var tags []DockerHubTag
	for _, version := range []string{"2.11.9", "2.11.8"} {
		for _, suffix := range []string{"", "-alpine", "-builder", "-windowsservercore", "-nanoserver"} {
			tags = append(tags, hubTag(version+suffix, hoursAgo(1)))
		}
	}
	return tags
}

// TestExpectedResults covers the scan results the fixes must produce. Each
// case runs raw registry tags through the same filter and pick as a real scan.
func TestExpectedResults(t *testing.T) {
	tests := []struct {
		name        string
		tags        []DockerHubTag
		pin         string
		want        string   // expected recommendation; empty means none
		wantInNote  []string // substrings the note must contain
		neverPicked []string
	}{
		{
			name:        "caddy all variants too new recommends nothing and says why",
			tags:        caddyTooNew(),
			pin:         "2.11.7",
			want:        "",
			wantInNote:  []string{"2.11.9", "1 hour", "safe-days 3"},
			neverPicked: []string{"2.11.9-builder", "2.11.9-windowsservercore", "2.11.9-nanoserver", "2.11.8-builder"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{
				Versions:   FilterAndSortDockerHubTags(tt.tags),
				CurrentTag: tt.pin,
				SafeDays:   3,
				Now:        fixedNow,
			})
			if got.Version != tt.want {
				t.Errorf("Version = %q, want %q (note %q)", got.Version, tt.want, got.Note)
			}
			for _, bad := range tt.neverPicked {
				if got.Version == bad {
					t.Errorf("picked forbidden tag %q", bad)
				}
			}
			for _, sub := range tt.wantInNote {
				if !strings.Contains(got.Note, sub) {
					t.Errorf("Note = %q, want it to mention %q", got.Note, sub)
				}
			}
		})
	}
}

// TestPick_SafeDays covers the age rule and when position fallback may run.
func TestPick_SafeDays(t *testing.T) {
	tests := []struct {
		name     string
		versions []models.RegistryVersion
		safeDays int
		want     string
		wantAge  string
		wantNote string // substring; empty means the note must be empty
	}{
		{
			name:     "empty tag list",
			versions: nil,
			safeDays: 3,
			wantNote: "no usable version tags",
		},
		{
			name: "only pre-release tags",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0-rc1", ReleasedAt: daysAgo(30)},
				{Tag: "2.0.0-beta", ReleasedAt: daysAgo(40)},
			},
			safeDays: 3,
			wantNote: "only pre-release tags",
		},
		{
			name: "all dated tags too new recommends nothing",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: hoursAgo(1)},
				{Tag: "1.9.0", ReleasedAt: hoursAgo(2)},
			},
			safeDays: 3,
			wantNote: "newest is 2.0.0, pushed 1 hour ago",
		},
		{
			name: "newest stable old enough",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: daysAgo(5)},
				{Tag: "1.9.0", ReleasedAt: daysAgo(50)},
			},
			safeDays: 3,
			want:     "2.0.0",
			wantAge:  "5 days",
		},
		{
			name: "skips too new and picks the older dated tag",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: hoursAgo(1)},
				{Tag: "1.9.0", ReleasedAt: daysAgo(5)},
			},
			safeDays: 3,
			want:     "1.9.0",
			wantAge:  "5 days",
		},
		{
			name: "tag exactly at the cutoff is still too new",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: daysAgo(3)},
			},
			safeDays: 3,
			wantNote: "newest is 2.0.0",
		},
		{
			name: "safe-days 0 accepts a tag pushed an hour ago",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: hoursAgo(1)},
			},
			safeDays: 0,
			want:     "2.0.0",
			wantAge:  "1 hour",
		},
		{
			name: "missing dates still use position fallback",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0"},
				{Tag: "1.9.0"},
			},
			safeDays: 3,
			want:     "1.9.0",
			wantAge:  "age unknown",
		},
		{
			name: "some dates known but too new does not fall back to an undated tag",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0", ReleasedAt: hoursAgo(1)},
				{Tag: "1.9.0"},
			},
			safeDays: 3,
			wantNote: "newest is 2.0.0",
		},
		{
			name: "undated newer tag is skipped for an old enough dated tag",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0"},
				{Tag: "1.9.0", ReleasedAt: daysAgo(5)},
			},
			safeDays: 3,
			want:     "1.9.0",
			wantAge:  "5 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{Versions: tt.versions, CurrentTag: "latest", SafeDays: tt.safeDays, Now: fixedNow})
			if got.Version != tt.want {
				t.Errorf("Version = %q, want %q", got.Version, tt.want)
			}
			if got.Age != tt.wantAge {
				t.Errorf("Age = %q, want %q", got.Age, tt.wantAge)
			}
			if tt.wantNote == "" && got.Note != "" {
				t.Errorf("Note = %q, want empty", got.Note)
			}
			if !strings.Contains(got.Note, tt.wantNote) {
				t.Errorf("Note = %q, want it to mention %q", got.Note, tt.wantNote)
			}
		})
	}
}
