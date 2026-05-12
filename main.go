package main

import (
	"docker-scanner/pkg/models"
	"docker-scanner/pkg/overrides"
	"docker-scanner/pkg/parser"
	"docker-scanner/pkg/registry"
	"docker-scanner/pkg/report"
	"docker-scanner/pkg/scanner"
	"docker-scanner/pkg/security"
	"flag"
	"fmt"
	"os"
)

var (
	scanDir       = flag.String("dir", ".", "Root directory to scan for compose files")
	outputFile    = flag.String("output", "", "Output file for report (default: stdout)")
	format        = flag.String("format", "text", "Report format: text, md, or html")
	safeDays      = flag.Int("safe-days", 3, "Only recommend versions older than N days (72-hour rule)")
	verbose       = flag.Bool("verbose", false, "Show detailed scan progress")
	skipRemote    = flag.Bool("skip-remote", false, "Skip registry version lookups")
	overridesPath = flag.String("overrides", "", "Path to overrides YAML (default: $XDG_CONFIG_HOME/docker-scanner/overrides.yml, then ~/.config/docker-scanner/overrides.yml, then bundled defaults)")
)

func main() {
	flag.Parse()

	if *scanDir == "." && flag.NFlag() == 0 {
		printHelp()
		return
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Scanning %s for compose files...\n", *scanDir)
	}

	projects, err := scanner.Scan(*scanDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning: %v\n", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Fprintf(os.Stderr, "No compose files found in %s\n", *scanDir)
		os.Exit(0)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Found %d project(s)\n", len(projects))
	}

	overrideCfg, err := loadOverrides()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading overrides: %v\n", err)
		os.Exit(1)
	}
	if *verbose && overrideCfg != nil {
		fmt.Fprintf(os.Stderr, "Loaded %d override(s)\n", len(overrideCfg.Overrides))
	}

	var allResults []models.ImageInfo

	for _, project := range projects {
		if *verbose {
			fmt.Fprintf(os.Stderr, "  Parsing project: %s\n", project.Name)
		}

		images, err := parser.Parse(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error parsing %s: %v\n", project.Name, err)
			continue
		}

		checkers := security.DefaultCheckers()
		for i := range images {
			issues := security.RunAll(checkers, images[i].File)
			images[i].SecurityIssues = append(images[i].SecurityIssues, issues...)

			name := images[i].Image.ContainerName
			if name == "" {
				name = images[i].Image.Service
			}
			version := parser.GetRunningVersion(name)
			if version == "" {
				composeName := images[i].Image.Project + "-" + images[i].Image.Service + "-1"
				version = parser.GetRunningVersion(composeName)
			}
			images[i].RunningVersion = version
		}

		if !*skipRemote {
			registries := registry.DefaultRegistries()
			getter := overrides.DefaultGetter()

			for i := range images {
				imageName := images[i].Image.Registry + "/" + images[i].Image.Name

				// Override path: trust the upstream source for this image,
				// but still apply the safeDays cutoff.
				if override := overrideCfg.Find(imageName); override != nil {
					if *verbose {
						fmt.Fprintf(os.Stderr, "    Using override (%s → %s) for %s\n",
							override.Source, override.Repo, imageName)
					}
					rec, err := overrides.Resolve(override, getter, *safeDays)
					if err != nil {
						if *verbose {
							fmt.Fprintf(os.Stderr, "    Override failed for %s, falling back to registry: %v\n", imageName, err)
						}
						// Fall through to registry path below.
					} else if rec == nil {
						if *verbose {
							fmt.Fprintf(os.Stderr, "    Override returned no release older than %d days for %s, falling back to registry\n", *safeDays, imageName)
						}
						// Fall through to registry path below.
					} else {
						images[i].RecommendedVersion = rec.Version
						images[i].RecommendedAge = rec.Age
						// MajorJump and IsDowngrade left at zero values —
						// the override version may not be semver-parseable
						// (build numbers etc.), so we don't attempt the
						// comparison. Future work: add when sensible.
						continue
					}
				}

				// Registry path.
				if *verbose {
					fmt.Fprintf(os.Stderr, "    Fetching versions for %s\n", imageName)
				}

				versions, err := registry.Lookup(registries, images[i].Image)
				if err != nil {
					if *verbose {
						fmt.Fprintf(os.Stderr, "    Warning: %v\n", err)
					}
					continue
				}

				var tags []string
				for _, v := range versions {
					tags = append(tags, v.Tag)
				}
				images[i].AvailableVersions = tags

				pick := registry.PickSafeVersion(versions, images[i].Image.Tag, *safeDays)
				images[i].RecommendedVersion = pick.Version
				images[i].RecommendedAge = pick.Age
				images[i].MajorVersionJump = pick.MajorJump

				if images[i].RunningVersion != "" && pick.Version != "" {
					images[i].IsDowngrade = registry.CompareSemver(pick.Version, images[i].RunningVersion) < 0
				}
			}
		}

		allResults = append(allResults, images...)
	}

	var output string
	switch *format {
	case "md", "markdown":
		output = report.GenerateMarkdown(allResults)
	case "html":
		output = report.GenerateHTML(allResults)
	default:
		output = report.Generate(allResults)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", *outputFile)
	} else {
		fmt.Print(output)
	}
}

// loadOverrides resolves the override config using env vars for the
// fallback paths. Kept in main.go so pkg/overrides stays free of env access.
func loadOverrides() (*overrides.Config, error) {
	return overrides.Load(overrides.LoadOptions{
		ExplicitPath:  *overridesPath,
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		HomeDir:       os.Getenv("HOME"),
	})
}

func printHelp() {
	fmt.Println("docker-scanner - Security and version auditing for Docker Compose projects")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  docker-scanner -dir <path> [flags]")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  docker-scanner -dir ~/docker-projects")
	fmt.Println("  docker-scanner -dir ~/docker-projects -format html -output report.html")
	fmt.Println("  docker-scanner -dir ~/docker-projects -safe-days 7")
	fmt.Println("  docker-scanner -dir ~/docker-projects -overrides ~/my-overrides.yml")
	fmt.Println()
	fmt.Println("Flags:")
	flag.PrintDefaults()
}