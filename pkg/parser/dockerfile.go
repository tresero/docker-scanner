package parser

import (
	"docker-scanner/pkg/models"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// scratchImage is the empty image a Dockerfile can start from.
const scratchImage = "scratch"

// dockerfileSummary is what a Dockerfile's final stage says about the image.
type dockerfileSummary struct {
	base       string // The image it's built from, or "" if there's no FROM
	unresolved []string
	ownLayers  bool // It adds layers, so the built image has its own date
}

// dockerfileBase returns the image a Dockerfile's final stage is built from.
func dockerfileBase(content string, buildArgs map[string]string) (string, []string) {
	summary := summarizeDockerfile(content, buildArgs)
	return summary.base, summary.unresolved
}

// summarizeDockerfile reads a Dockerfile's final stage, following it back through
// any earlier stages it builds on. Only ARGs declared before the first FROM can be
// used in a FROM line, as in Docker, and buildArgs override their defaults.
func summarizeDockerfile(content string, buildArgs map[string]string) dockerfileSummary {
	declared := make(map[string]string)
	lookup := func(name string) (string, bool) {
		value, ok := declared[name]
		return value, ok
	}

	stages := make(map[string]dockerfileSummary)
	currentAlias := ""
	seenFrom := false
	var current dockerfileSummary

	saveStage := func() {
		if currentAlias != "" {
			stages[currentAlias] = current
		}
	}

	for _, line := range dockerfileLines(content) {
		fields := strings.Fields(line)
		switch strings.ToUpper(fields[0]) {
		case "ARG":
			if !seenFrom {
				declareArgs(fields[1:], declared, buildArgs, lookup)
			}
		case "RUN", "COPY", "ADD":
			current.ownLayers = true
		case "FROM":
			image, alias := parseFromLine(fields[1:])
			if image == "" {
				continue
			}
			saveStage()
			seenFrom = true
			base, unresolved := expandVariables(image, lookup)
			current = dockerfileSummary{base: base, unresolved: unresolved}
			if earlier, ok := stages[strings.ToLower(base)]; ok {
				current = earlier
			}
			currentAlias = strings.ToLower(alias)
		}
	}
	return current
}

// parseFromLine returns the image and stage name from what follows FROM.
func parseFromLine(fields []string) (image, alias string) {
	for i, field := range fields {
		switch {
		case strings.HasPrefix(field, "--"):
		case image == "":
			image = field
		case strings.EqualFold(field, "AS") && i+1 < len(fields):
			return image, fields[i+1]
		}
	}
	return image, ""
}

// declareArgs records each NAME or NAME=default from an ARG line.
func declareArgs(args []string, declared, buildArgs map[string]string, lookup LookupEnv) {
	for _, arg := range args {
		name, value, hasDefault := strings.Cut(arg, "=")
		if override, ok := buildArgs[name]; ok {
			declared[name] = override
			continue
		}
		if hasDefault {
			expanded, _ := expandVariables(strings.Trim(value, `"'`), lookup)
			declared[name] = expanded
		}
	}
}

// dockerfileLines returns a Dockerfile's instructions, with comments removed and
// continuation lines joined.
func dockerfileLines(content string) []string {
	var lines []string
	var current strings.Builder
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, `\`) {
			current.WriteString(strings.TrimSuffix(line, `\`) + " ")
			continue
		}
		current.WriteString(line)
		if joined := strings.TrimSpace(current.String()); joined != "" {
			lines = append(lines, joined)
		}
		current.Reset()
	}
	if joined := strings.TrimSpace(current.String()); joined != "" {
		lines = append(lines, joined)
	}
	return lines
}

// buildConfig is a compose service's build: section.
type buildConfig struct {
	dockerfile string // Path to the Dockerfile, empty for a remote context
	inline     string // dockerfile_inline content
	hasInline  bool
	args       map[string]string
}

// parseBuild reads the string and map forms of build:.
func parseBuild(build interface{}, composePath string, lookup LookupEnv) buildConfig {
	context, dockerfile := ".", "Dockerfile"
	config := buildConfig{args: make(map[string]string)}

	switch value := build.(type) {
	case string:
		context = value
	case map[string]interface{}:
		if s, ok := value["context"].(string); ok {
			context = s
		}
		if s, ok := value["dockerfile"].(string); ok {
			dockerfile = s
		}
		if s, ok := value["dockerfile_inline"].(string); ok {
			config.inline, config.hasInline = s, true
		}
		config.args = parseBuildArgs(value["args"], lookup)
	}

	context, contextUnresolved := expandVariables(context, lookup)
	dockerfile, fileUnresolved := expandVariables(dockerfile, lookup)
	if len(contextUnresolved) > 0 || len(fileUnresolved) > 0 || isRemoteContext(context) {
		return config
	}

	if !filepath.IsAbs(context) {
		context = filepath.Join(filepath.Dir(composePath), context)
	}
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(context, dockerfile)
	}
	config.dockerfile = filepath.Clean(dockerfile)
	return config
}

// parseBuildArgs reads args: as a map or a KEY=value list. An arg with no value
// comes from the environment. One that can't be resolved is left out, so the
// Dockerfile's own default still applies.
func parseBuildArgs(args interface{}, lookup LookupEnv) map[string]string {
	result := make(map[string]string)
	set := func(name, value string) {
		if expanded, unresolved := expandVariables(value, lookup); len(unresolved) == 0 {
			result[name] = expanded
		}
	}
	fromEnvironment := func(name string) {
		if value, ok := lookup(name); ok {
			result[name] = value
		}
	}

	switch value := args.(type) {
	case map[string]interface{}:
		for name, arg := range value {
			if arg == nil {
				fromEnvironment(name)
			} else {
				set(name, fmt.Sprint(arg))
			}
		}
	case []interface{}:
		for _, item := range value {
			entry, ok := item.(string)
			if !ok {
				continue
			}
			if name, arg, hasValue := strings.Cut(entry, "="); hasValue {
				set(name, arg)
			} else {
				fromEnvironment(name)
			}
		}
	}
	return result
}

func isRemoteContext(context string) bool {
	return strings.Contains(context, "://") || strings.HasPrefix(context, "git@") || strings.HasPrefix(context, "github.com/")
}

// parseBuildService describes a service that's built rather than pulled. What can
// go out of date is its base image, so that's what the row reports.
func parseBuildService(service Service, composePath string, lookup LookupEnv) models.ImageInfo {
	localImage, _ := expandVariables(service.Image, lookup)
	config := parseBuild(service.Build, composePath, lookup)

	content, dockerfile, readErr := config.inline, composePath+" (inline)", error(nil)
	if !config.hasInline {
		dockerfile = config.dockerfile
		if dockerfile == "" {
			readErr = fmt.Errorf("the build context isn't a local folder")
		} else {
			var data []byte
			data, readErr = os.ReadFile(dockerfile)
			content = string(data)
		}
	}

	var info models.ImageInfo
	if readErr != nil {
		info = localRow(localImage, "couldn't read the Dockerfile: "+readErrText(readErr), true)
	} else {
		info = baseImageInfo(summarizeDockerfile(content, config.args), localImage)
	}
	info.Dockerfile = dockerfile
	info.LocalImage = localImage
	return info
}

// parseStandaloneDockerfile describes a Dockerfile in a folder with no compose file.
func parseStandaloneDockerfile(path string) models.ImageInfo {
	var info models.ImageInfo
	data, err := os.ReadFile(path)
	if err != nil {
		info = localRow("", "couldn't read the Dockerfile: "+readErrText(err), true)
	} else {
		info = baseImageInfo(summarizeDockerfile(string(data), nil), "")
	}
	info.Dockerfile = path
	info.File = path
	return info
}

func baseImageInfo(summary dockerfileSummary, localImage string) models.ImageInfo {
	switch summary.base {
	case "":
		return localRow(localImage, "the Dockerfile has no FROM line", true)
	case scratchImage:
		return localRow(scratchImage, "built FROM scratch, so there's no base image to check", false)
	}
	info := imageRefInfo(summary.base, summary.unresolved)
	info.NoOwnLayers = !summary.ownLayers
	return info
}

// localRow is a row with nothing a registry could answer. Name is filled from the
// service by the caller when name is empty.
func localRow(name, note string, unverified bool) models.ImageInfo {
	return models.ImageInfo{
		Image:              models.Image{Registry: "local", Name: name},
		RecommendationNote: note,
		Unverified:         unverified,
		SkipLookup:         true,
	}
}

// readErrText gives the reason without repeating the full path, which the row shows.
func readErrText(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
}

// DropReferencedDockerfiles removes standalone Dockerfiles that a compose file
// builds, so they aren't reported twice. Compose's build: can point anywhere,
// including into another project's folder.
func DropReferencedDockerfiles(projects []models.Project, lookupEnv LookupEnv) []models.Project {
	referenced := make(map[string]bool)
	for _, project := range projects {
		for _, file := range project.ComposeFiles {
			for _, dockerfile := range composeDockerfiles(file, lookupEnv) {
				referenced[dockerfile] = true
			}
		}
	}

	kept := make([]models.Project, 0, len(projects))
	for _, project := range projects {
		var dockerfiles []string
		for _, dockerfile := range project.Dockerfiles {
			if !referenced[filepath.Clean(dockerfile)] {
				dockerfiles = append(dockerfiles, dockerfile)
			}
		}
		if len(project.ComposeFiles) == 0 && len(dockerfiles) == 0 {
			continue
		}
		project.Dockerfiles = dockerfiles
		kept = append(kept, project)
	}
	return kept
}

func composeDockerfiles(composePath string, lookupEnv LookupEnv) []string {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil
	}
	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil
	}

	lookup := projectLookup(composePath, lookupEnv)
	var dockerfiles []string
	for _, service := range compose.Services {
		if service.Build == nil {
			continue
		}
		// A dockerfile_inline build reports its inline content, not the
		// Dockerfile in its context, so that file still needs its own row.
		if config := parseBuild(service.Build, composePath, lookup); config.dockerfile != "" && !config.hasInline {
			dockerfiles = append(dockerfiles, config.dockerfile)
		}
	}
	return dockerfiles
}
