package registry

import (
	"docker-scanner/pkg/models"
	"fmt"
	"reflect"
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
		running     string
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
			name:        "caddy plain tag older than safe-days beyond the top 10 is picked",
			tags:        caddyWithOlderTags(),
			pin:         "2.11.7",
			want:        "2.11.7",
			wantInNote:  []string{"newest is 2.11.9", "safe-days 3"},
			neverPicked: []string{"2.11.6", "2.11.9-builder"},
		},
		{
			name: "caddy older pin upgrades to the plain tag beyond the top 10",
			tags: caddyWithOlderTags(),
			pin:  "2.11.5",
			want: "2.11.7",
		},
		{
			name: "redis keeps the same variant string for the same version",
			tags: []DockerHubTag{
				hubTag("8.10.2-alpine3.23", daysAgo(10)),
				hubTag("8.10.2-alpine", daysAgo(10)),
				hubTag("8.10.2", daysAgo(10)),
			},
			pin:         "8.10.2-alpine3.23",
			want:        "8.10.2-alpine3.23",
			neverPicked: []string{"8.10.2-alpine", "8.10.2"},
		},
		{
			name: "valkey 8-alpine stays on 8 and flags 9.2 as a newer major",
			tags: []DockerHubTag{
				hubTag("9.2.1-alpine", daysAgo(10)),
				hubTag("9.2.1", daysAgo(10)),
				hubTag("9.2-alpine", daysAgo(10)),
				hubTag("9-alpine", daysAgo(10)),
				hubTag("8.1.4-alpine", daysAgo(10)),
				hubTag("8.1.4", daysAgo(10)),
				hubTag("8-alpine", daysAgo(10)),
			},
			pin:         "8-alpine",
			want:        "8.1.4-alpine",
			wantMajor:   "9.2.1-alpine",
			neverPicked: []string{"9.2.1", "8.1.4", "8-alpine"},
		},
		{
			name:        "mariadb 11.4 stays on its line and reports the newer major separately",
			tags:        mariadbTags(),
			pin:         "11.4",
			want:        "11.4.9",
			wantMajor:   "13.0.2",
			neverPicked: []string{"13.0.2", "12.3.2-ubi10", "11.8.5"},
		},
		{
			name: "phpmyadmin running version re-pushed yesterday is up to date with no move to the older tag",
			tags: []DockerHubTag{
				hubTag("5.2.3", hoursAgo(24)),
				hubTag("5.2.2", daysAgo(365)),
			},
			pin:         "5.2.3",
			running:     "5.2.3",
			want:        "",
			wantInNote:  []string{"already on the newest version", "5.2.3"},
			neverPicked: []string{"5.2.2"},
		},
		{
			name: "redis running version pushed today gets no recommendation to go down",
			tags: []DockerHubTag{
				hubTag("8.10.3-alpine", hoursAgo(2)),
				hubTag("8.10.2-alpine", daysAgo(30)),
			},
			pin:         "8.10.3-alpine",
			running:     "8.10.3-alpine",
			want:        "",
			wantInNote:  []string{"already on the newest version"},
			neverPicked: []string{"8.10.2-alpine"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{
				Versions:       FilterAndSortDockerHubTags(tt.tags),
				CurrentTag:     tt.pin,
				RunningVersion: tt.running,
				SafeDays:       3,
				Now:            fixedNow,
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

// TestPick_KeepsVariant covers variant preservation, including the variants
// that are easy to get wrong: build numbers, database versions in the suffix,
// and multi-part suffixes.
func TestPick_KeepsVariant(t *testing.T) {
	old := daysAgo(10)
	tests := []struct {
		name      string
		pin       string
		tags      []string
		want      string
		wantMajor string
		wantJump  bool
		wantNote  string
	}{
		{
			name: "a plain pin never gets a build-only variant",
			pin:  "2.11.7",
			tags: []string{"2.11.9-builder", "2.11.9-windowsservercore", "2.11.9-nanoserver", "2.11.8"},
			want: "2.11.8",
		},
		{
			name: "a plain pin never gets another variant at the same version",
			pin:  "8.6.2",
			tags: []string{"8.6.3-alpine", "8.6.3"},
			want: "8.6.3",
		},
		{
			name: "alpine pin keeps alpine",
			pin:  "2.11.2-alpine",
			tags: []string{"2.11.3", "2.11.3-alpine", "2.11.3-builder"},
			want: "2.11.3-alpine",
		},
		{
			name:      "postgres 16-alpine stays on 16-alpine and reports 18 separately",
			pin:       "16-alpine",
			tags:      []string{"18.3", "18.3-alpine", "16.4", "16.4-alpine"},
			want:      "16.4-alpine",
			wantMajor: "18.3-alpine",
		},
		{
			name:      "a major.minor variant pin stays on its line",
			pin:       "16.2-alpine",
			tags:      []string{"18.3.1-alpine", "16.2.1-alpine"},
			want:      "16.2.1-alpine",
			wantMajor: "18.3.1-alpine",
		},
		{
			name:     "a full version variant pin can jump major and is flagged",
			pin:      "16.2.1-alpine",
			tags:     []string{"18.3.1", "18.3.1-alpine", "16.2.1-alpine"},
			want:     "18.3.1-alpine",
			wantJump: true,
		},
		{
			name: "word-only pin keeps its variant",
			pin:  "apache",
			tags: []string{"6.9.4", "6.9.4-php8.5", "6.9.4-fpm", "6.9.4-apache"},
			want: "6.9.4-apache",
		},
		{
			name: "named variant keeps its exact suffix",
			pin:  "5.1.3-libtorrentv2",
			tags: []string{"5.1.4-libtorrentv1", "5.1.4-libtorrentv2"},
			want: "5.1.4-libtorrentv2",
		},
		{
			name: "multi-part variant is kept",
			pin:  "8.3.1-fpm-alpine",
			tags: []string{"8.3.2", "8.3.2-fpm", "8.3.2-alpine", "8.3.2-fpm-alpine"},
			want: "8.3.2-fpm-alpine",
		},
		{
			name: "ubuntu variant pin is kept",
			pin:  "13.0.1-ubuntu",
			tags: []string{"13.0.2", "13.0.2-ubuntu"},
			want: "13.0.2-ubuntu",
		},
		{
			name: "distroless variant pin is kept",
			pin:  "v3.11.1-distroless",
			tags: []string{"v3.11.2", "v3.11.2-distroless"},
			want: "v3.11.2-distroless",
		},
		{
			name: "a database version in the suffix never moves",
			pin:  "0.8.0-pg16",
			tags: []string{"0.8.1-pg17", "0.8.0-pg17", "0.8.0-pg16"},
			want: "0.8.0-pg16",
		},
		{
			name: "alpine release may move at a higher version",
			pin:  "8.10.2-alpine3.23",
			tags: []string{"8.12.0-alpine3.24", "8.12.0-alpine", "8.10.2-alpine3.23"},
			want: "8.12.0-alpine3.24",
		},
		{
			name: "an exact alpine match wins at the same version",
			pin:  "8.10.2-alpine",
			tags: []string{"8.12.0-alpine3.24", "8.12.0-alpine"},
			want: "8.12.0-alpine",
		},
		{
			name: "linuxserver build number is not a variant",
			pin:  "4.0.17.2952-ls309",
			tags: []string{"4.0.18.3000-ls310", "4.0.17.2952-ls309"},
			want: "4.0.18.3000-ls310",
		},
		{
			name: "latest only gets plain tags",
			pin:  "latest",
			tags: []string{"2.11.9-builder", "2.11.9-alpine", "2.11.8"},
			want: "2.11.8",
		},
		{
			name: "stable only gets plain tags",
			pin:  "stable",
			tags: []string{"2.11.9-alpine", "2.11.8"},
			want: "2.11.8",
		},
		{
			name: "a date pin only gets plain tags",
			pin:  "2024.05.01",
			tags: []string{"2024.06.01-alpine", "2024.06.01"},
			want: "2024.06.01",
		},
		{
			name: "a git hash pin only gets plain tags",
			pin:  "a1b2c3d",
			tags: []string{"1.2.3-alpine", "1.2.3"},
			want: "1.2.3",
		},
		{
			name:     "no tag with the pinned variant says so",
			pin:      "2.11.2-alpine",
			tags:     []string{"2.11.3", "2.11.3-builder"},
			wantNote: `no tags found with variant "alpine"`,
		},
		{
			name:     "a plain pin with only variant tags says so",
			pin:      "latest",
			tags:     []string{"2.11.3-alpine"},
			wantNote: "no plain version tags found",
		},
		{
			name:      "major-only pin with nothing on its major says so and reports the newer major",
			pin:       "8-alpine",
			tags:      []string{"9.2.1-alpine"},
			wantMajor: "9.2.1-alpine",
			wantNote:  "no tags found on the 8.x line",
		},
		{
			name: "a partial variant tag stays when only a plain full version exists",
			pin:  "9.1-alpine",
			tags: []string{"9.2.1", "9.1-alpine", "9.1.1"},
			want: "9.1-alpine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hub []DockerHubTag
			for _, tag := range tt.tags {
				hub = append(hub, hubTag(tag, old))
			}
			got := Pick(PickInput{Versions: FilterAndSortDockerHubTags(hub), CurrentTag: tt.pin, SafeDays: 3, Now: fixedNow})
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

func TestTagVariant(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"1.2.3", ""},
		{"v1.2.3", ""},
		{"16", ""},
		{"11.4", ""},
		{"latest", ""},
		{"main", ""},
		{"stable", ""},
		{"a1b2c3d", ""},
		{"2024.05.01", ""},
		{"2024-05-01", ""},
		{"20240501", ""},
		{"", ""},
		{"4.0.17.2952-ls309", ""},
		{"v1.5.6-ls345", ""},
		{"8-alpine", "alpine"},
		{"16-alpine3.20", "alpine3.20"},
		{"8.10.2-alpine3.23", "alpine3.23"},
		{"8.3-fpm-alpine", "fpm-alpine"},
		{"apache", "apache"},
		{"Bookworm", "bookworm"},
		{"fpm-alpine", "fpm-alpine"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := tagVariant(tt.tag); got != tt.want {
				t.Errorf("tagVariant(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

// TestIsMajorJump covers major-only tags, which the old parse couldn't read.
func TestIsMajorJump(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		recommended string
		want        bool
	}{
		{"valkey major-only variant to 9.2", "8-alpine", "9.2", true},
		{"postgres major-only variant to 18", "16-alpine", "18.3-alpine", true},
		{"bare major to a higher major", "8", "9.2.1", true},
		{"same major from a major-only tag", "8-alpine", "8.1.4-alpine", false},
		{"full version to a higher major", "2.5.0", "3.0.0", true},
		{"same major", "2.5.0", "2.6.0", false},
		{"latest is never a jump", "latest", "3.0.0", false},
		{"empty current tag is never a jump", "", "3.0.0", false},
		{"word tag is never a jump", "apache", "6.9.4-apache", false},
		{"long number is a build id, not a major", "2026041305", "1.2.3", false},
		{"date is not a major", "2024-05-01", "1.2.3", false},
		{"unparseable recommendation is never a jump", "8-alpine", "stable", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMajorJump(tt.current, tt.recommended); got != tt.want {
				t.Errorf("isMajorJump(%q, %q) = %v, want %v", tt.current, tt.recommended, got, tt.want)
			}
		})
	}
}

// TestDisplayVersions_ContentsUnchanged protects available_versions: tags
// newly usable for variant pins must not appear in the list.
func TestDisplayVersions_ContentsUnchanged(t *testing.T) {
	tags := []string{
		"9.2.1", "9.2.1-alpine", "9.2-alpine", "9.2", "9.2.1-fpm-alpine",
		"9.2.1-ubuntu", "9.2.1-distroless", "9.2.1-busybox",
	}
	got := DisplayVersions(FilterAndSortTags(tags))
	want := []string{"9.2.1", "9.2.1-alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DisplayVersions = %v, want %v", got, want)
	}
}

// TestSemverParts_MajorOnly covers the parse used for pins like 8-alpine.
func TestSemverParts_MajorOnly(t *testing.T) {
	tests := []struct {
		tag                 string
		major, minor, patch int
	}{
		{"8-alpine", 8, -1, -1},
		{"16", 16, -1, -1},
		{"v8", 8, -1, -1},
		{"8.3-fpm-alpine", 8, 3, -1},
		{"8.10.2-alpine3.23", 8, 10, 2},
		{"2026041305", -1, -1, -1},
		{"20240501", -1, -1, -1},
		{"2024-05-01", -1, -1, -1},
		{"apache", -1, -1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			major, minor, patch := SemverParts(tt.tag)
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Errorf("SemverParts(%q) = (%d, %d, %d), want (%d, %d, %d)", tt.tag, major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}

func TestMatchOverride(t *testing.T) {
	versions := FilterAndSortTags([]string{"13.0.2", "13.0.2-alpine", "11.4.9", "11.4.9-alpine", "2.0.0", "2.0.0-alpine"})
	tests := []struct {
		name     string
		upstream string
		pin      string
		running  string
		want     string
		wantJump bool
	}{
		{"plain pin gets the plain tag", "v2.0.0", "1.0.0", "", "2.0.0", true},
		{"variant pin keeps its variant", "v2.0.0", "1.0.0-alpine", "", "2.0.0-alpine", true},
		{"same major is not a jump", "v13.0.2", "13.0.1", "", "13.0.2", false},
		{"major.minor pin refuses a version off its line", "v13.0.2", "11.4", "", "", false},
		{"major.minor pin accepts a version on its line", "v11.4.9", "11.4", "", "11.4.9", false},
		{"major-only pin refuses a newer major", "v13.0.2", "11-alpine", "", "", false},
		{"no matching tag", "v99.0.0", "1.0.0", "", "", false},
		{"refuses a version below the pin", "v2.0.0", "13.0.2", "", "", false},
		{"refuses a version below the running one", "v2.0.0", "1.0.0", "11.4.9", "", false},
		{"accepts the running version itself", "v2.0.0", "1.0.0", "2.0.0", "2.0.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, jump := MatchOverride(versions, tt.upstream, tt.pin, tt.running)
			if got != tt.want {
				t.Errorf("tag = %q, want %q", got, tt.want)
			}
			if jump != tt.wantJump {
				t.Errorf("majorJump = %v, want %v", jump, tt.wantJump)
			}
		})
	}
}

// TestPick_NeverBelowRunning covers the lower limit: the higher of the running
// version and the pin.
func TestPick_NeverBelowRunning(t *testing.T) {
	tests := []struct {
		name      string
		pin       string
		running   string
		versions  []models.RegistryVersion
		want      string
		wantMajor string
		wantNote  string // substring; empty means the note must be empty
	}{
		{
			name:    "a newer old-enough version is still recommended",
			pin:     "5.2.3",
			running: "5.2.3",
			versions: []models.RegistryVersion{
				{Tag: "5.2.4", ReleasedAt: daysAgo(10)},
				{Tag: "5.2.3", ReleasedAt: daysAgo(90)},
			},
			want: "5.2.4",
		},
		{
			name:    "the running version is confirmed when it is old enough",
			pin:     "5.2.3",
			running: "5.2.3",
			versions: []models.RegistryVersion{
				{Tag: "5.2.3", ReleasedAt: daysAgo(90)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(365)},
			},
			want: "5.2.3",
		},
		{
			name:    "a newer tag that is too new waits and does not go down",
			pin:     "5.2.3",
			running: "5.2.3",
			versions: []models.RegistryVersion{
				{Tag: "5.2.4", ReleasedAt: hoursAgo(1)},
				{Tag: "5.2.3", ReleasedAt: hoursAgo(5)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(365)},
			},
			wantNote: "newest is 5.2.4, pushed 1 hour ago",
		},
		{
			name:    "running ahead of the pin is the limit",
			pin:     "5.2.1",
			running: "5.2.3",
			versions: []models.RegistryVersion{
				{Tag: "5.2.3", ReleasedAt: daysAgo(30)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(40)},
			},
			want: "5.2.3",
		},
		{
			name:    "pin ahead of running is the limit",
			pin:     "5.2.3",
			running: "5.2.1",
			versions: []models.RegistryVersion{
				{Tag: "5.2.3", ReleasedAt: hoursAgo(1)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(40)},
			},
			wantNote: "already on the newest version",
		},
		{
			name: "a pin that isn't running is still the limit",
			pin:  "5.2.3",
			versions: []models.RegistryVersion{
				{Tag: "5.2.3", ReleasedAt: hoursAgo(1)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(365)},
			},
			wantNote: "already on the newest version",
		},
		{
			name:    "an unknown running version falls back to the pin",
			pin:     "5.2.3",
			running: "unknown",
			versions: []models.RegistryVersion{
				{Tag: "5.2.3", ReleasedAt: hoursAgo(1)},
				{Tag: "5.2.2", ReleasedAt: daysAgo(365)},
			},
			wantNote: "already on the newest version",
		},
		{
			name:    "a floating pin uses the running version as the limit",
			pin:     "latest",
			running: "1.5.0",
			versions: []models.RegistryVersion{
				{Tag: "1.6.0", ReleasedAt: hoursAgo(1)},
				{Tag: "1.4.0", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newest is 1.6.0",
		},
		{
			name:    "a floating pin with a multi-part running version uses it as the limit",
			pin:     "latest",
			running: "7.0.4-260620-apache",
			versions: []models.RegistryVersion{
				{Tag: "6.9.0", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newer than any matching tag",
		},
		{
			name:    "a floating pin with an unreadable running version has no limit",
			pin:     "latest",
			running: "unknown",
			versions: []models.RegistryVersion{
				{Tag: "1.4.0", ReleasedAt: daysAgo(30)},
			},
			want: "1.4.0",
		},
		{
			name:    "a word pin's running string is no limit",
			pin:     "apache",
			running: "apache",
			versions: []models.RegistryVersion{
				{Tag: "6.9.4-apache", ReleasedAt: daysAgo(30)},
			},
			want: "6.9.4-apache",
		},
		{
			name:    "running newer than every tag says so",
			pin:     "9.0.0",
			running: "9.0.0",
			versions: []models.RegistryVersion{
				{Tag: "8.0.0", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newer than any matching tag",
		},
		{
			name:    "missing dates never go below running",
			pin:     "2.0.0",
			running: "2.0.0",
			versions: []models.RegistryVersion{
				{Tag: "2.0.0"},
				{Tag: "1.9.0"},
				{Tag: "1.8.0"},
			},
			want: "2.0.0",
		},
		{
			name:     "empty tag list",
			pin:      "2.0.0",
			running:  "2.0.0",
			wantNote: "no usable version tags",
		},
		{
			name:    "only pre-release tags",
			pin:     "2.0.0",
			running: "2.0.0",
			versions: []models.RegistryVersion{
				{Tag: "3.0.0-rc1", ReleasedAt: daysAgo(30)},
			},
			wantNote: "only pre-release tags",
		},
		{
			name:    "a lower 4th version part is below running",
			pin:     "6.1.1.10360-ls300",
			running: "6.1.1.10360-ls300",
			versions: []models.RegistryVersion{
				{Tag: "6.1.1.10000-ls290", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newer than any matching tag",
		},
		{
			name:    "a higher 4th version part is an upgrade",
			pin:     "6.1.1.10360-ls300",
			running: "6.1.1.10360-ls300",
			versions: []models.RegistryVersion{
				{Tag: "6.1.1.10400-ls305", ReleasedAt: daysAgo(30)},
				{Tag: "6.1.1.10360-ls300", ReleasedAt: daysAgo(60)},
			},
			want: "6.1.1.10400-ls305",
		},
		{
			name:    "a lower linuxserver build is below running",
			pin:     "v1.5.6-ls345",
			running: "v1.5.6-ls345",
			versions: []models.RegistryVersion{
				{Tag: "v1.5.6-ls340", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newer than any matching tag",
		},
		{
			name:    "a higher linuxserver build is an upgrade",
			pin:     "v1.5.6-ls345",
			running: "v1.5.6-ls345",
			versions: []models.RegistryVersion{
				{Tag: "v1.5.6-ls346", ReleasedAt: daysAgo(30)},
				{Tag: "v1.5.6-ls345", ReleasedAt: daysAgo(40)},
			},
			want: "v1.5.6-ls346",
		},
		{
			name:    "same version with a different variant isn't a downgrade",
			pin:     "8.10.2-alpine",
			running: "8.10.2",
			versions: []models.RegistryVersion{
				{Tag: "8.10.2-alpine", ReleasedAt: daysAgo(30)},
			},
			want: "8.10.2-alpine",
		},
		{
			name:    "a newer major below running isn't reported",
			pin:     "11.4",
			running: "13.0.2",
			versions: []models.RegistryVersion{
				{Tag: "12.3.2", ReleasedAt: daysAgo(30)},
				{Tag: "11.4.9", ReleasedAt: daysAgo(30)},
			},
			wantNote: "newer than any matching tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{Versions: tt.versions, CurrentTag: tt.pin, RunningVersion: tt.running, SafeDays: 3, Now: fixedNow})
			if got.Version != tt.want {
				t.Errorf("Version = %q, want %q (note %q)", got.Version, tt.want, got.Note)
			}
			if got.NewerMajor != tt.wantMajor {
				t.Errorf("NewerMajor = %q, want %q", got.NewerMajor, tt.wantMajor)
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

// TestPick_LinuxServerBuildCounterReset comes from a live sonarr scan: the build
// counter restarted, so -ls2 is newer than -ls329. Registry order must decide.
func TestPick_LinuxServerBuildCounterReset(t *testing.T) {
	versions := FilterAndSortDockerHubTags([]DockerHubTag{
		hubTag("4.0.20.3014-ls2", daysAgo(6)),
		hubTag("4.0.20.3014-ls1", daysAgo(10)),
		hubTag("4.0.20.3014-ls329", daysAgo(21)),
		hubTag("4.0.20.3014-ls328", daysAgo(28)),
	})
	got := Pick(PickInput{Versions: versions, CurrentTag: "4.0.17.2952-ls309", SafeDays: 3, Now: fixedNow})
	if got.Version != "4.0.20.3014-ls2" {
		t.Errorf("Version = %q, want the most recently pushed build 4.0.20.3014-ls2", got.Version)
	}
}

func TestNotBelow(t *testing.T) {
	tests := []struct {
		tag, floor string
		want       bool
	}{
		{"1.2.3", "", true},
		{"1.2.3", "1.2.3", true},
		{"1.2.4", "1.2.3", true},
		{"1.2.2", "1.2.3", false},
		{"8.1.4-alpine", "8-alpine", true},
		{"7.9.9-alpine", "8-alpine", false},
		{"11.4.9", "11.4", true},
		{"6.1.1.10000-ls290", "6.1.1.10360-ls300", false},
		{"v1.5.6-ls346", "v1.5.6-ls345", true},
		{"v1.5.6-ls9", "v1.5.6-ls10", false},
		{"8.10.2-alpine", "8.10.2", true},
	}
	for _, tt := range tests {
		t.Run(tt.tag+"_vs_"+tt.floor, func(t *testing.T) {
			if got := notBelow(tt.tag, tt.floor); got != tt.want {
				t.Errorf("notBelow(%q, %q) = %v, want %v", tt.tag, tt.floor, got, tt.want)
			}
		})
	}
}

func TestVersionFloor(t *testing.T) {
	tests := []struct {
		name         string
		pin, running string
		want         string
	}{
		{"pin only", "5.2.3", "", "5.2.3"},
		{"running ahead of pin", "5.2.1", "5.2.3", "5.2.3"},
		{"pin ahead of running", "5.2.3", "5.2.1", "5.2.3"},
		{"floating pin uses running", "latest", "5.2.3", "5.2.3"},
		{"unknown running uses pin", "5.2.3", "unknown", "5.2.3"},
		{"neither is readable", "apache", "apache", ""},
		{"neither is set", "", "", ""},
		{"later linuxserver build is the limit", "v1.5.6-ls345", "v1.5.6-ls346", "v1.5.6-ls346"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionFloor(tt.pin, tt.running); got != tt.want {
				t.Errorf("versionFloor(%q, %q) = %q, want %q", tt.pin, tt.running, got, tt.want)
			}
		})
	}
}

func TestCompareSemver_BuildParts(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"6.1.1.10400", "6.1.1.10360", 1},
		{"6.1.1.10360-ls300", "6.1.1.10400-ls305", -1},
		{"1.2.3", "1.2.3.0", 0},
		{"8.10.2-alpine", "8.10.2", -1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareSemver(tt.a, tt.b)
			if (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) || (tt.want == 0 && got != 0) {
				t.Errorf("CompareSemver(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
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
		{
			name:      "tags below the pin are not dated",
			pin:       "2.0.0",
			tags:      []string{"3.0.0", "2.0.0", "1.9.0", "1.8.0"},
			wantCalls: []string{"3.0.0", "2.0.0"},
			wantCount: 2,
		},
		{
			name:      "a variant pin only dates tags with its variant",
			pin:       "8-alpine",
			tags:      []string{"9.2.1-alpine", "8.1.4", "8.1.4-alpine", "8.1.3-alpine"},
			wantCalls: []string{"8.1.4-alpine", "8.1.3-alpine"},
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
