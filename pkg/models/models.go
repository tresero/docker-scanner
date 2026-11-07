package models

import "time"

// Image represents a Docker image reference
type Image struct {
	Registry      string `json:"registry"`       // e.g., "docker.io", "ghcr.io"
	Name          string `json:"name"`            // e.g., "serfriz/caddy-cloudflare"
	Tag           string `json:"tag"`             // e.g., "latest", "1.2.3"
	Service       string `json:"service"`         // e.g., "caddy"
	ContainerName string `json:"container_name"`  // e.g., "salsa_blanca" (from container_name in compose)
	Project       string `json:"project"`         // e.g., "caddy"
}

// ImageInfo contains image details and metadata
type ImageInfo struct {
	Image               Image           `json:"image"`
	File                string          `json:"file"`                            // Path to compose file
	UsesLatest          bool            `json:"uses_latest"`
	RunningVersion      string          `json:"running_version,omitempty"`       // Actual version running in Docker
	AvailableVersions   []string        `json:"available_versions,omitempty"`
	RecommendedVersion  string          `json:"recommended_version,omitempty"`
	RecommendedAge      string          `json:"recommended_age,omitempty"`       // e.g., "5 days", "3 weeks"
	MajorVersionJump    bool            `json:"major_version_jump,omitempty"`    // true if recommended is a different major version
	IsDowngrade         bool            `json:"is_downgrade,omitempty"`          // Kept for compatibility: recommendations are no longer below the running version
	SecurityIssues      []SecurityIssue `json:"security_issues,omitempty"`
	RunningDigests         []string `json:"-"`                                 // Digests of the running image, from the Docker daemon
	RunningLabelVersion    string   `json:"-"`                                 // The running image's version label, if it has one
	ResolvedRunningVersion string   `json:"running_resolved_version,omitempty"` // The real version behind a running tag like 16-alpine
	RecommendationNote  string          `json:"recommendation_note,omitempty"`   // Why nothing newer is recommended, e.g., waiting for safe-days
	Unverified          bool            `json:"unverified,omitempty"`            // The scan couldn't confirm this image's status; recommendation_note says why
	NewerMajorVersion   string          `json:"newer_major_version,omitempty"`   // Newest safe tag on a higher major, when the pin's line is kept
	UnresolvedVariables []string        `json:"unresolved_variables,omitempty"`  // Compose variables in image: with no value, so the real image isn't known
	ContainerState      string          `json:"container_state,omitempty"`       // running, unhealthy, restarting, exited, created, paused, dead or "not created"; empty if Docker couldn't be asked
	ExitCode            *int            `json:"exit_code,omitempty"`             // Set when the container has stopped
	Health              string          `json:"health,omitempty"`                // Docker health check result, if the image defines one
}

// SecurityIssue represents a security finding
type SecurityIssue struct {
	Type        string `json:"type"`                  // e.g., "hardcoded_secret", "missing_envfile"
	Severity    string `json:"severity"`              // "high", "medium", "low"
	Description string `json:"description"`
	Location    string `json:"location"`              // e.g., "CLOUDFLARE_API_TOKEN"
	Suggestion  string `json:"suggestion,omitempty"`
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
	Digest   string    // image digest, when the registry provides it
}