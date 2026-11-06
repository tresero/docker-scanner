package registry

import (
	"docker-scanner/pkg/models"
	"fmt"
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

// caddyWithOlderTags is caddyTooNew plus older plain tags that sit beyond the
// first 10 once sorted.
func caddyWithOlderTags() []DockerHubTag {
	return append(caddyTooNew(),
		hubTag("2.11.7", daysAgo(20)),
		hubTag("2.11.7-builder", daysAgo(20)),
		hubTag("2.11.6", daysAgo(40)),
	)
}

// mariadbTags has a long-term-support line sitting below two newer majors.
func mariadbTags() []DockerHubTag {
	return []DockerHubTag{
		hubTag("13.0.3", hoursAgo(5)), // newest, but too new to report
		hubTag("13.0.2", daysAgo(4)),
		hubTag("13.0.2-ubi10", daysAgo(4)),
		hubTag("12.3.2", daysAgo(4)),
		hubTag("12.3.2-ubi10", daysAgo(4)),
		hubTag("12.3.1", daysAgo(20)),
		hubTag("12.2.5", daysAgo(30)),
		hubTag("12.1.4", daysAgo(40)),
		hubTag("12.0.6", daysAgo(50)),
		hubTag("11.8.5", daysAgo(10)),
		hubTag("11.8.4", daysAgo(60)),
		hubTag("11.4.9", daysAgo(10)),
		hubTag("11.4.8", daysAgo(70)),
		hubTag("11.4", daysAgo(10)),
		hubTag("10.11.15", daysAgo(10)),
	}
}

// TestExpectedResults covers the scan results the fixes must produce. Each
// case runs raw registry tags through the same filter and pick as a real scan.
func TestExpectedResults(t *testing.T) {
	tests := []struct {
		name        string
		tags        []DockerHubTag
		pin         string
		want        string   // expected recommendation; empty means none
		wantMajor   string   // expected NewerMajor
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
		{
			name: "caddy plain tag older than safe-days beyond the top 10 is picked",
			tags: caddyWithOlderTags(),
			pin:  "2.11.7",
			want: "2.11.7",
		},
		{
			name: "caddy older pin upgrades to the plain tag beyond the top 10",
			tags: caddyWithOlderTags(),
			pin:  "2.11.5",
			want: "2.11.7",
		},
		{
			name:        "mariadb 11.4 stays on its line and reports the newer major separately",
			tags:        mariadbTags(),
			pin:         "11.4",
			want:        "11.4.9",
			wantMajor:   "13.0.2",
			neverPicked: []string{"13.0.2", "12.3.2-ubi10", "11.8.5"},
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
			if got.NewerMajor != tt.wantMajor {
				t.Errorf("NewerMajor = %q, want %q", got.NewerMajor, tt.wantMajor)
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

// TestPick_PinLine covers how a major.minor pin limits the candidates.
func TestPick_PinLine(t *testing.T) {
	tests := []struct {
		name      string
		pin       string
		versions  []models.RegistryVersion
		want      string
		wantMajor string
		wantJump  bool
		wantNote  string // substring; empty means the note must be empty
	}{
		{
			name: "line tags all too new recommends nothing but still reports the newer major",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2", ReleasedAt: daysAgo(10)},
				{Tag: "11.4.9", ReleasedAt: hoursAgo(1)},
			},
			wantMajor: "13.0.2",
			wantNote:  "newest is 11.4.9",
		},
		{
			name: "no tags on the line says so",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2", ReleasedAt: daysAgo(10)},
				{Tag: "12.3.2", ReleasedAt: daysAgo(10)},
			},
			wantMajor: "13.0.2",
			wantNote:  "no tags found on the 11.4 line",
		},
		{
			name: "newer major too new is not reported",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2", ReleasedAt: hoursAgo(1)},
				{Tag: "11.4.9", ReleasedAt: daysAgo(10)},
			},
			want: "11.4.9",
		},
		{
			name: "lower majors are not reported as newer",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "11.4.9", ReleasedAt: daysAgo(10)},
				{Tag: "10.11.15", ReleasedAt: daysAgo(10)},
			},
			want: "11.4.9",
		},
		{
			name: "missing dates use position fallback within the line",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2"},
				{Tag: "11.4.9"},
				{Tag: "11.4.8"},
			},
			want:      "11.4.8",
			wantMajor: "13.0.2",
		},
		{
			name: "only pre-release tags on the line",
			pin:  "11.4",
			versions: []models.RegistryVersion{
				{Tag: "11.4.9-rc1", ReleasedAt: daysAgo(10)},
			},
			wantNote: "only pre-release tags",
		},
		{
			name: "a full version pin can still jump major and is flagged",
			pin:  "11.4.8",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2", ReleasedAt: daysAgo(10)},
				{Tag: "11.4.9", ReleasedAt: daysAgo(10)},
			},
			want:     "13.0.2",
			wantJump: true,
		},
		{
			name: "a floating pin is not limited to a line",
			pin:  "latest",
			versions: []models.RegistryVersion{
				{Tag: "13.0.2", ReleasedAt: daysAgo(10)},
				{Tag: "11.4.9", ReleasedAt: daysAgo(10)},
			},
			want: "13.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{Versions: tt.versions, CurrentTag: tt.pin, SafeDays: 3, Now: fixedNow})
			if got.Version != tt.want {
				t.Errorf("Version = %q, want %q (note %q)", got.Version, tt.want, got.Note)
			}
			if got.NewerMajor != tt.wantMajor {
				t.Errorf("NewerMajor = %q, want %q", got.NewerMajor, tt.wantMajor)
			}
			if got.MajorJump != tt.wantJump {
				t.Errorf("MajorJump = %v, want %v", got.MajorJump, tt.wantJump)
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

// numberedTags returns n plain tags, newest first: 1.<n-1>.0 down to 1.0.0.
func numberedTags(n int) []string {
	tags := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		tags = append(tags, fmt.Sprintf("1.%d.0", i))
	}
	return tags
}

func TestFilters_KeepTheWholeListForPicking(t *testing.T) {
	var hub []DockerHubTag
	for _, tag := range numberedTags(25) {
		hub = append(hub, hubTag(tag, daysAgo(10)))
	}
	if got := len(FilterAndSortDockerHubTags(hub)); got != 25 {
		t.Errorf("FilterAndSortDockerHubTags kept %d tags, want 25", got)
	}
	if got := len(FilterAndSortTags(numberedTags(25))); got != 25 {
		t.Errorf("FilterAndSortTags kept %d tags, want 25", got)
	}
}

func TestDisplayVersions(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  int
	}{
		{"empty list", 0, 0},
		{"under the limit is unchanged", 3, 3},
		{"at the limit is unchanged", 10, 10},
		{"over the limit shows the newest 10", 25, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayVersions(FilterAndSortTags(numberedTags(tt.count)))
			if len(got) != tt.want {
				t.Fatalf("got %d tags, want %d", len(got), tt.want)
			}
			if tt.want > 0 && got[0] != fmt.Sprintf("1.%d.0", tt.count-1) {
				t.Errorf("first tag = %q, want the newest", got[0])
			}
		})
	}
}

// TestFillReleaseDates guards the request count: each date costs 3 HTTP
// requests on GHCR, so keeping the whole list must not fetch a date per tag.
func TestFillReleaseDates(t *testing.T) {
	tests := []struct {
		name      string
		pin       string
		tags      []string
		wantCalls []string // the tags dated, in order; nil means check wantCount only
		wantCount int
	}{
		{
			name:      "thousands of tags still fetch at most 10 dates",
			pin:       "latest",
			tags:      numberedTags(3000),
			wantCount: 10,
		},
		{
			name:      "no tags fetch nothing",
			pin:       "latest",
			tags:      nil,
			wantCount: 0,
		},
		{
			name:      "a major.minor pin only dates tags on its line",
			pin:       "11.4",
			tags:      []string{"13.0.2", "12.3.2", "11.4.9", "11.4.8"},
			wantCalls: []string{"11.4.9", "11.4.8"},
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versions := FilterAndSortTags(tt.tags)
			var calls []string
			fillReleaseDates(versions, tt.pin, func(tag string) time.Time {
				calls = append(calls, tag)
				return daysAgo(10)
			})
			if len(calls) != tt.wantCount {
				t.Fatalf("fetched %d dates, want %d", len(calls), tt.wantCount)
			}
			for i, want := range tt.wantCalls {
				if calls[i] != want {
					t.Errorf("call %d = %q, want %q", i, calls[i], want)
				}
			}
			dated := 0
			for _, v := range versions {
				if !v.ReleasedAt.IsZero() {
					dated++
				}
			}
			if dated != tt.wantCount {
				t.Errorf("%d tags have dates, want %d", dated, tt.wantCount)
			}
		})
	}
}
