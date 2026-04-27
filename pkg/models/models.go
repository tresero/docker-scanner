package models

import "time"

// Image represents a Docker image reference
type Image struct {
	Registry string // e.g., "docker.io", "ghcr.io"
	Name     string // e.g., "serfriz/caddy-cloudflare"
	Tag      string // e.g., "latest", "1.2.3"
	Service  string // e.g., "caddy"
	Project  string // e.g., "caddy"
}

// ImageInfo contains image details and metadata
type ImageInfo struct {
	Image               Image
	File                string   // Path to compose file
	UsesLatest          bool
	RunningVersion      string   // Actual version running in Docker
	AvailableVersions   []string
	RecommendedVersion  string
	RecommendedAge      string // e.g., "5 days", "3 weeks"
	MajorVersionJump    bool   // true if recommended is a different major version
	IsDowngrade         bool   // true if recommended is older than running version
	SecurityIssues      []SecurityIssue
}

// SecurityIssue represents a security finding
type SecurityIssue struct {
	Type        string // e.g., "hardcoded_secret", "missing_envfile"
	Severity    string // "high", "medium", "low"
	Description string
	Location    string // e.g., "CLOUDFLARE_API_TOKEN"
	Suggestion  string
}

// Project represents a docker project directory
type Project struct {
	Name      string   // e.g., "caddy"
	Path      string   // Full path to project directory
	ComposeFiles []string // Paths to compose files in this project
}

// RegistryVersion represents available version info
type RegistryVersion struct {
	Tag      string
	Released string    // raw timestamp string from registry
	ReleasedAt time.Time // parsed timestamp
}