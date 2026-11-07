package registry

import (
	"strings"
	"unicode"
)

// DataUpgradeInput is what decides whether a database needs a warning.
type DataUpgradeInput struct {
	Name        string
	Tag         string
	Running     string // The real running version when known, or empty
	Recommended string
	NewerMajor  string
	Floating    bool
}

// databaseFamily is how one database handles its data when upgraded. The warnings
// come from upgrading each one on a throwaway volume, so check there before
// changing them.
type databaseFamily struct {
	warning string
	// minorMatters is true when a minor jump changes the data too. MySQL 8.0 to 8.4
	// and MariaDB 11.4 to 11.8 both do, with the major unchanged.
	minorMatters bool
}

var (
	postgresFamily = &databaseFamily{warning: "postgres: a new major version won't start on the old data. Back up, then migrate with pg_upgrade or a dump and restore."}
	mariadbFamily  = &databaseFamily{warning: "mariadb: a new major or minor version starts, but its data still needs upgrading. Back up, then run mariadb-upgrade or set MARIADB_AUTO_UPGRADE=1.", minorMatters: true}
	mysqlFamily    = &databaseFamily{warning: "mysql: a new major or minor version upgrades its data on first start, and the old version won't start afterwards. Back up first.", minorMatters: true}
	mongoFamily    = &databaseFamily{warning: "mongo: a new version won't start if it skips a major. Back up, then upgrade one major at a time."}
)

// databaseFamilies is keyed by the last part of the image name, so it also finds
// images like bitnami/postgresql, and doesn't catch tools like postgres-exporter.
var databaseFamilies = map[string]*databaseFamily{
	"postgres":                 postgresFamily,
	"postgresql":               postgresFamily,
	"postgis":                  postgresFamily,
	"pgvector":                 postgresFamily,
	"timescaledb":              postgresFamily,
	"timescaledb-ha":           postgresFamily,
	"mariadb":                  mariadbFamily,
	"mysql":                    mysqlFamily,
	"mongo":                    mongoFamily,
	"mongodb":                  mongoFamily,
	"mongodb-community-server": mongoFamily,
}

// DataUpgradeWarning says what upgrading a database takes, when an upgrade that
// needs care is in play: a higher version is available, or a tag with no version
// in it could move to one by itself. It returns "" for anything else, including
// apps, because the scanner can't know how each app handles its data.
func DataUpgradeWarning(in DataUpgradeInput) string {
	family := databaseFamilies[in.Name[strings.LastIndex(in.Name, "/")+1:]]
	if family == nil {
		return ""
	}

	// A tag like pg16 is flagged as floating too, but it stays on 16.
	if in.Floating && !strings.ContainsFunc(in.Tag, unicode.IsDigit) {
		return "This tag can move to a new version on the next pull, so pin one. " + family.warning
	}
	if in.NewerMajor != "" {
		return family.warning
	}

	from := in.Running
	if major, _, _ := SemverParts(from); major < 0 {
		from = in.Tag
	}
	if crossesVersion(from, in.Recommended, family.minorMatters) {
		return family.warning
	}
	return ""
}

// crossesVersion reports whether moving between two tags is an upgrade that needs care.
func crossesVersion(from, to string, minorMatters bool) bool {
	fromMajor, fromMinor, _ := SemverParts(from)
	toMajor, toMinor, _ := SemverParts(to)
	if fromMajor < 0 || toMajor < 0 {
		return false
	}
	if toMajor != fromMajor {
		return toMajor > fromMajor
	}
	return minorMatters && fromMinor >= 0 && toMinor > fromMinor
}
