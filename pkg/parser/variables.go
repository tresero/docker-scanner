package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LookupEnv finds a variable, reporting whether it's set. os.LookupEnv fits.
type LookupEnv func(name string) (string, bool)

var (
	bareVariable   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	bracedVariable = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?+])(.*))?$`)
)

// shellMark wraps values taken from the shell in a second parse of a reference,
// so the parsed image shows which parts came from the scanner's own environment.
const shellMark = "\x00"

// markShell returns shell with each non-empty value wrapped in shellMark, and
// records the names it answers in used. shell can be nil.
func markShell(shell LookupEnv, used *[]string) LookupEnv {
	if shell == nil {
		return nil
	}
	return func(name string) (string, bool) {
		value, ok := shell(name)
		if !ok || value == "" {
			return value, ok
		}
		if !containsString(*used, name) {
			*used = append(*used, name)
		}
		return shellMark + value + shellMark, true
	}
}

// projectLookup finds variables the way Compose does: the shell first, then the
// .env file beside the compose file. shell can be nil.
func projectLookup(composePath string, shell LookupEnv) LookupEnv {
	dotEnv := readDotEnv(filepath.Join(filepath.Dir(composePath), ".env"))
	return func(name string) (string, bool) {
		if shell != nil {
			if value, ok := shell(name); ok {
				return value, true
			}
		}
		value, ok := dotEnv[name]
		return value, ok
	}
}

// readDotEnv parses a .env file. A missing or unreadable file has no variables.
func readDotEnv(path string) map[string]string {
	vars := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return vars
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if found {
			vars[strings.TrimSpace(name)] = dotEnvValue(strings.TrimSpace(value))
		}
	}
	return vars
}

// dotEnvValue removes surrounding quotes, or a trailing comment from an unquoted value.
func dotEnvValue(value string) string {
	if len(value) > 0 && (value[0] == '"' || value[0] == '\'') {
		if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
			return value[1 : end+1]
		}
	}
	if comment := strings.Index(value, " #"); comment >= 0 {
		value = value[:comment]
	}
	return strings.TrimSpace(value)
}

// expandVariables replaces Compose variables in an image reference. It returns
// the names it couldn't resolve, each left in the result as ${NAME}, so the rest
// of the reference still parses. Each name is listed once.
func expandVariables(ref string, lookup LookupEnv) (string, []string) {
	var out strings.Builder
	var unresolved []string
	addUnresolved := func(names ...string) {
		for _, name := range names {
			if !containsString(unresolved, name) {
				unresolved = append(unresolved, name)
			}
		}
	}

	for i := 0; i < len(ref); {
		rest := ref[i:]
		switch {
		case rest[0] != '$':
			out.WriteByte(rest[0])
			i++
		case strings.HasPrefix(rest, "$$"):
			out.WriteByte('$')
			i += 2
		case strings.HasPrefix(rest, "${"):
			end := closingBrace(rest)
			if end < 0 {
				out.WriteString(rest)
				addUnresolved(rest)
				return out.String(), unresolved
			}
			value, missing := expandBraced(rest[2:end], lookup)
			out.WriteString(value)
			addUnresolved(missing...)
			i += end + 1
		default:
			name := bareVariable.FindString(rest[1:])
			if name == "" {
				out.WriteByte('$')
				i++
				continue
			}
			value, missing := expandBraced(name, lookup)
			out.WriteString(value)
			addUnresolved(missing...)
			i += len(name) + 1
		}
	}
	return out.String(), unresolved
}

// expandBraced resolves what's inside ${...}.
func expandBraced(expr string, lookup LookupEnv) (string, []string) {
	match := bracedVariable.FindStringSubmatch(expr)
	if match == nil {
		invalid := "${" + expr + "}"
		return invalid, []string{invalid}
	}
	name, operator, argument := match[1], match[2], match[3]
	placeholder := "${" + name + "}"

	value, set := lookup(name)
	usable := set
	if strings.HasPrefix(operator, ":") {
		usable = set && value != ""
	}

	switch strings.TrimPrefix(operator, ":") {
	case "-":
		if usable {
			return value, nil
		}
		return expandVariables(argument, lookup)
	case "+":
		if usable {
			return expandVariables(argument, lookup)
		}
		return "", nil
	case "?":
		if usable {
			return value, nil
		}
		return placeholder, []string{name}
	}

	if value == "" {
		return placeholder, []string{name}
	}
	return value, nil
}

// closingBrace returns the index of the } matching the ${ that starts s, or -1.
func closingBrace(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
