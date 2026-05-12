package overrides

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// defaultsYAML is the curated list of overrides for common home-server
// images that ship with the binary. Users get sensible defaults out of
// the box; they can replace them entirely by writing their own file at
// one of the lookup locations.
//
//go:embed defaults.yml
var defaultsYAML []byte

// LoadOptions controls where Load looks for an override file.
//
// Precedence (highest wins):
//  1. ExplicitPath — from a --overrides CLI flag
//  2. $XDGConfigHome/docker-scanner/overrides.yml
//  3. $HomeDir/.config/docker-scanner/overrides.yml
//  4. Bundled defaults embedded in the binary
//
// Env vars are NOT read here — the caller in main.go is responsible for
// translating $XDG_CONFIG_HOME and $HOME into these fields. Keeping Load
// free of env access makes it deterministic and testable.
type LoadOptions struct {
	ExplicitPath  string
	XDGConfigHome string
	HomeDir       string
}

// Load resolves the override file according to LoadOptions precedence
// and returns the parsed config. Returns an error only if:
//   - ExplicitPath was set and the file is missing or unparseable
//   - A located fallback file exists but fails to parse
//
// A missing fallback file (XDG or home) is not an error — it just means
// "not configured at this location, try the next one."
func Load(opts LoadOptions) (*Config, error) {
	if opts.ExplicitPath != "" {
		return loadFile(opts.ExplicitPath, true)
	}

	if opts.XDGConfigHome != "" {
		path := filepath.Join(opts.XDGConfigHome, "docker-scanner", "overrides.yml")
		if cfg, err := loadFile(path, false); err != nil {
			return nil, err
		} else if cfg != nil {
			return cfg, nil
		}
	}

	if opts.HomeDir != "" {
		path := filepath.Join(opts.HomeDir, ".config", "docker-scanner", "overrides.yml")
		if cfg, err := loadFile(path, false); err != nil {
			return nil, err
		} else if cfg != nil {
			return cfg, nil
		}
	}

	// Fall back to bundled defaults.
	cfg, err := Parse(defaultsYAML)
	if err != nil {
		return nil, fmt.Errorf("bundled defaults are invalid: %w", err)
	}
	return cfg, nil
}

// loadFile reads and parses a file. The required flag changes the
// semantics when the file is missing:
//   - required=true: missing file is an error
//   - required=false: missing file returns (nil, nil) for fallback to try next
//
// Parse errors are always errors regardless of required.
func loadFile(path string, required bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil, nil
		}
		return nil, fmt.Errorf("read override file %s: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse override file %s: %w", path, err)
	}
	return cfg, nil
}