package registry

import (
	"docker-scanner/pkg/models"
	"strings"
	"testing"
	"time"
)

func TestDataUpgradeWarning(t *testing.T) {
	const (
		postgresWarning = "won't start on the old data"
		mariadbWarning  = "mariadb-upgrade"
		mysqlWarning    = "the old version won't start afterwards"
		mysqlSkipping   = "skips a long-term release"
		mongoWarning    = "one major at a time"
		floatingWarning = "can move to a new version on the next pull"
	)
	tests := []struct {
		name string
		in   DataUpgradeInput
		want []string // Empty means no warning
	}{
		// What each database really did when upgraded on a throwaway volume.
		{name: "postgres with a newer major available", in: DataUpgradeInput{Name: "postgres", Tag: "16-alpine", Recommended: "16.14-alpine", NewerMajor: "18.4-alpine"}, want: []string{postgresWarning, "pg_upgrade", "Back up"}},
		{name: "mariadb with a newer major available", in: DataUpgradeInput{Name: "mariadb", Tag: "11.4", Recommended: "11.4.13", NewerMajor: "13.0.2"}, want: []string{mariadbWarning, "MARIADB_AUTO_UPGRADE", "Back up"}},
		{name: "mysql with a newer major available", in: DataUpgradeInput{Name: "mysql", Tag: "8.0", Recommended: "8.0.46", NewerMajor: "9.1.0"}, want: []string{mysqlWarning, mysqlSkipping, "Back up"}},
		{name: "mongo with a newer major available", in: DataUpgradeInput{Name: "mongo", Tag: "6", Recommended: "6.0.20", NewerMajor: "8.0.1"}, want: []string{mongoWarning, "Back up"}},

		{name: "a recommendation that jumps a major", in: DataUpgradeInput{Name: "postgres", Tag: "16.4", Recommended: "18.4"}, want: []string{postgresWarning}},
		{name: "a jump from the real running version", in: DataUpgradeInput{Name: "postgres", Tag: "18.4", Running: "16.4", Recommended: "18.4"}, want: []string{postgresWarning}},

		{name: "a mariadb minor jump changes its data too", in: DataUpgradeInput{Name: "mariadb", Tag: "11.4.8", Recommended: "11.8.2"}, want: []string{mariadbWarning}},
		{name: "a mysql minor jump changes its data too", in: DataUpgradeInput{Name: "mysql", Tag: "8.0.46", Recommended: "8.4.3"}, want: []string{mysqlWarning}},
		{name: "a postgres minor is safe", in: DataUpgradeInput{Name: "postgres", Tag: "16.4", Recommended: "16.14"}},
		{name: "a mariadb patch is safe", in: DataUpgradeInput{Name: "mariadb", Tag: "11.4.8", Recommended: "11.4.13"}},
		{name: "a mysql patch is safe", in: DataUpgradeInput{Name: "mysql", Tag: "8.4.1", Recommended: "8.4.3"}},
		{name: "a mongo minor is safe", in: DataUpgradeInput{Name: "mongo", Tag: "7.0.1", Recommended: "7.0.14"}},

		{name: "already current", in: DataUpgradeInput{Name: "postgres", Tag: "16.14", Recommended: "16.14"}},
		{name: "nothing recommended", in: DataUpgradeInput{Name: "postgres", Tag: "16.14"}},
		{name: "a major-only pin on its line is safe", in: DataUpgradeInput{Name: "postgres", Tag: "16-alpine", Recommended: "16.14-alpine"}},
		{name: "a major.minor pin on its line is safe", in: DataUpgradeInput{Name: "mariadb", Tag: "11.4", Recommended: "11.4.13"}},

		{name: "a floating tag can jump by itself", in: DataUpgradeInput{Name: "postgres", Tag: "latest", Floating: true}, want: []string{floatingWarning, postgresWarning}},
		{name: "a word tag can jump by itself", in: DataUpgradeInput{Name: "mariadb", Tag: "lts", Floating: true}, want: []string{floatingWarning, mariadbWarning}},

		{name: "postgis is postgres", in: DataUpgradeInput{Name: "postgis/postgis", Tag: "16-3.4", NewerMajor: "18-3.5"}, want: []string{postgresWarning}},
		{name: "pgvector is postgres", in: DataUpgradeInput{Name: "pgvector/pgvector", Tag: "latest", Floating: true}, want: []string{floatingWarning, postgresWarning}},
		{name: "timescaledb is postgres", in: DataUpgradeInput{Name: "timescale/timescaledb", Tag: "latest", Floating: true}, want: []string{floatingWarning, postgresWarning}},
		{name: "a floating-looking tag with a version stays on it", in: DataUpgradeInput{Name: "pgvector/pgvector", Tag: "pg16", Floating: true}},
		{name: "latest with a version stays on it", in: DataUpgradeInput{Name: "timescale/timescaledb", Tag: "latest-pg16", Floating: true}},
		{name: "another registry's postgres", in: DataUpgradeInput{Name: "bitnami/postgresql", Tag: "16.4.0", Recommended: "18.0.0"}, want: []string{postgresWarning}},
		{name: "linuxserver's mariadb", in: DataUpgradeInput{Name: "linuxserver/mariadb", Tag: "11.4.8-ls1", NewerMajor: "12.0.1-ls2"}, want: []string{mariadbWarning}},
		{name: "mongodb's own image", in: DataUpgradeInput{Name: "mongodb/mongodb-community-server", Tag: "7.0.1", NewerMajor: "8.0.1"}, want: []string{mongoWarning}},

		{name: "an unknown running version uses the tag", in: DataUpgradeInput{Name: "postgres", Tag: "16.4", Running: "unknown", Recommended: "18.4"}, want: []string{postgresWarning}},
		{name: "a recommendation below running needs no warning", in: DataUpgradeInput{Name: "postgres", Tag: "16.4", Running: "18.1", Recommended: "16.14"}},

		{name: "an app isn't a database", in: DataUpgradeInput{Name: "pretix/standalone", Tag: "2026.6.0", Recommended: "2027.1.0"}},
		{name: "a floating app isn't a database", in: DataUpgradeInput{Name: "wordpress", Tag: "apache", Floating: true}},
		{name: "redis keeps reading its data", in: DataUpgradeInput{Name: "redis", Tag: "7.4.1", Recommended: "8.10.3"}},
		{name: "a database's tool isn't a database", in: DataUpgradeInput{Name: "mongo-express", Tag: "latest", Floating: true}},
		{name: "a postgres exporter isn't a database", in: DataUpgradeInput{Name: "prometheuscommunity/postgres-exporter", Tag: "0.15.0", Recommended: "1.0.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DataUpgradeWarning(tt.in)
			if len(tt.want) == 0 && got != "" {
				t.Errorf("warning = %q, want none", got)
			}
			if len(tt.want) > 0 && got == "" {
				t.Fatal("no warning, want one")
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("warning = %q, want it to mention %q", got, want)
				}
			}
		})
	}
}

// TestPick_DatabasesStayOnMajor comes from real upgrades: mysql 8.0 refuses to start
// as 9.7 or 26.10, and mongo 6 refuses to start as 8. Recommending those would
// suggest a tag that can't start on the data.
func TestPick_DatabasesStayOnMajor(t *testing.T) {
	mysqlTags := []models.RegistryVersion{
		{Tag: "26.10.0", ReleasedAt: daysAgo(10)},
		{Tag: "9.7.4", ReleasedAt: daysAgo(10)},
		{Tag: "8.4.13", ReleasedAt: daysAgo(10)},
		{Tag: "8.0.46", ReleasedAt: daysAgo(20)},
		{Tag: "8.0.30", ReleasedAt: daysAgo(900)},
	}
	alpineTags := []models.RegistryVersion{
		{Tag: "18.4-alpine", ReleasedAt: daysAgo(10)},
		{Tag: "16.14-alpine", ReleasedAt: daysAgo(100)},
		{Tag: "16.14", ReleasedAt: daysAgo(100)},
	}
	tests := []struct {
		name        string
		pin         string
		running     string
		versions    []models.RegistryVersion
		stay        bool
		want        string
		wantNewer   string
		wantJump    bool
		wantNoteHas string
	}{
		{name: "mysql's full pin stays on its major", pin: "8.0.30", versions: mysqlTags, stay: true, want: "8.4.13", wantNewer: "26.10.0"},
		{name: "an app's full pin can still jump", pin: "8.0.30", versions: mysqlTags, want: "26.10.0", wantJump: true},
		{name: "a major pin keeps its variant", pin: "16-alpine", versions: alpineTags, stay: true, want: "16.14-alpine", wantNewer: "18.4-alpine"},
		{name: "a major.minor pin keeps its own line", pin: "16.4-alpine", versions: alpineTags, stay: true, wantNewer: "18.4-alpine", wantNoteHas: "16.4 line"},
		{name: "a full variant pin stays on its major", pin: "16.4.1-alpine", versions: alpineTags, stay: true, want: "16.14-alpine", wantNewer: "18.4-alpine"},
		{name: "latest stays on the running major", pin: "latest", running: "8.0.46", versions: mysqlTags, stay: true, want: "8.4.13", wantNewer: "26.10.0"},
		{name: "latest with nothing running can pick anything", pin: "latest", versions: mysqlTags, stay: true, want: "26.10.0"},
		{name: "latest with an unknown running version can pick anything", pin: "latest", running: "unknown", versions: mysqlTags, stay: true, want: "26.10.0"},
		{name: "a higher running major is the line, not a downgrade", pin: "8.0.30", running: "9.7.4", versions: mysqlTags, stay: true, want: "9.7.4", wantNewer: "26.10.0", wantJump: true},
		{
			name: "too new on the major waits rather than jumping", pin: "8.0.30",
			versions: []models.RegistryVersion{{Tag: "9.7.4", ReleasedAt: daysAgo(10)}, {Tag: "8.4.13", ReleasedAt: fixedNow.Add(-time.Hour)}},
			stay:     true, wantNewer: "9.7.4", wantNoteHas: "newest is 8.4.13",
		},
		{
			name: "no tags on the major says so", pin: "7.0.1",
			versions: []models.RegistryVersion{{Tag: "9.7.4", ReleasedAt: daysAgo(10)}},
			stay:     true, wantNewer: "9.7.4", wantNoteHas: "7.x",
		},
		{name: "empty tag list", pin: "8.0.30", stay: true, wantNoteHas: "no usable"},
		{
			name: "only pre-release tags", pin: "8.0.30",
			versions: []models.RegistryVersion{{Tag: "8.4.0-rc1", ReleasedAt: daysAgo(10)}},
			stay:     true, wantNoteHas: "pre-release",
		},
		{
			name: "missing dates stay on the major", pin: "8.0.30",
			versions: []models.RegistryVersion{{Tag: "9.7.4"}, {Tag: "8.4.13"}, {Tag: "8.4.12"}},
			stay:     true, want: "8.4.12", wantNewer: "9.7.4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(PickInput{Versions: tt.versions, CurrentTag: tt.pin, RunningVersion: tt.running, StayOnMajor: tt.stay, SafeDays: 3, Now: fixedNow})
			if got.Version != tt.want {
				t.Errorf("Version = %q, want %q (note %q)", got.Version, tt.want, got.Note)
			}
			if got.NewerMajor != tt.wantNewer {
				t.Errorf("NewerMajor = %q, want %q", got.NewerMajor, tt.wantNewer)
			}
			if got.MajorJump != tt.wantJump {
				t.Errorf("MajorJump = %v, want %v", got.MajorJump, tt.wantJump)
			}
			if !strings.Contains(got.Note, tt.wantNoteHas) {
				t.Errorf("Note = %q, want it to mention %q", got.Note, tt.wantNoteHas)
			}
		})
	}
}

func TestIsDatabase(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"postgres", true},
		{"mariadb", true},
		{"mysql", true},
		{"mongo", true},
		{"bitnami/postgresql", true},
		{"pgvector/pgvector", true},
		{"linuxserver/mariadb", true},
		{"redis", false},
		{"valkey/valkey", false},
		{"pretix/standalone", false},
		{"mongo-express", false},
		{"prometheuscommunity/postgres-exporter", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDatabase(tt.name); got != tt.want {
				t.Errorf("IsDatabase(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
