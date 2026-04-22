package config

import "regexp"

// ComposeFileNames lists the compose filenames to scan for
var ComposeFileNames = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

// EnvFileNames lists the env filenames to check for
var EnvFileNames = []string{
	".env",
	".env.local",
	".env.production",
}

// SecretPatterns defines regex patterns that indicate hardcoded secrets
// Key: pattern name, Value: compiled regex
var SecretPatterns = map[string]*regexp.Regexp{
	"password":   regexp.MustCompile(`(?i)(password|passwd|db_pass)\s*[:=]\s*[^$\s{][^\s]*`),
	"api_key":    regexp.MustCompile(`(?i)(api_key|apikey)\s*[:=]\s*[^$\s{][^\s]*`),
	"api_token":  regexp.MustCompile(`(?i)(api_token|token)\s*[:=]\s*[^$\s{][^\s]*`),
	"secret":     regexp.MustCompile(`(?i)(secret|secret_key)\s*[:=]\s*[^$\s{][^\s]*`),
	"credential": regexp.MustCompile(`(?i)(credential|auth)\s*[:=]\s*[^$\s{][^\s]*`),
	"url_creds":  regexp.MustCompile(`://[^$\s{][^:]+:[^$\s{][^@]+@`),
}

// UnsafeTagPatterns defines tags considered unsafe
var UnsafeTagPatterns = []string{
	"latest",
	"main",
	"master",
	"dev",
	"nightly",
	"edge",
}

// VersionFetchLimit is the max number of versions to retrieve
var VersionFetchLimit = 10

// HTTPTimeoutSeconds is the default timeout for registry queries
var HTTPTimeoutSeconds = 10

// IgnoredDirs lists directory names to skip during scanning
var IgnoredDirs = []string{
	"node_modules",
	"vendor",
	".git",
	".svn",
	"__pycache__",
	".cache",
	".npm",
	".yarn",
	"bower_components",
}