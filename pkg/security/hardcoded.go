package security

import (
    "docker-scanner/internal/config"
    "docker-scanner/pkg/models"
    "docker-scanner/pkg/parser"
    "fmt"
    "strings"
)

// HardcodedSecretsChecker scans environment variables
// for hardcoded secrets that should use ${VAR} syntax
type HardcodedSecretsChecker struct{}

func (c *HardcodedSecretsChecker) Name() string {
    return "hardcoded_secrets_check"
}

func (c *HardcodedSecretsChecker) Check(filePath string) ([]models.SecurityIssue, error) {
    var issues []models.SecurityIssue

    envVars, err := parser.GetRawEnvironment(filePath)
    if err != nil {
	return nil, err
    }

    for serviceName, entries := range envVars {
	for _, entry := range entries {
	    found := checkEntry(entry, serviceName, filePath)
	    issues = append(issues, found...)
	}
    }

    return issues, nil
}

func checkEntry(entry string, serviceName string, filePath string) []models.SecurityIssue {
    var issues []models.SecurityIssue

    // Skip entries that already use ${VAR} syntax
    if containsVarRef(entry) {
	return nil
    }

    // Skip on/off switches such as socket-proxy's AUTH=0: no secret is one of these
    if _, value, ok := strings.Cut(entry, "="); ok && isSwitchValue(value) {
	return nil
    }

    // Check against all secret patterns; report only the first match per entry
    // to avoid duplicates when a variable name matches multiple patterns
    // (e.g. AUTH_TOKEN matches both "api_token" and "credential").
    for patternName, pattern := range config.SecretPatterns {
	if pattern.MatchString(entry) {
	    varName := extractVarName(entry)
	    location := fmt.Sprintf("%s -> %s", filePath, varName)
	    suggestion := fmt.Sprintf("Use ${%s} and add to .env file", strings.ToUpper(varName))
	    if varName == "" {
		location = fmt.Sprintf("%s -> URL with credentials", filePath)
		suggestion = "Move the URL into the .env file and use ${VAR} here"
	    }
	    return []models.SecurityIssue{{
		Type:     "hardcoded_secret",
		Severity: severity(patternName),
		Description: fmt.Sprintf(
		    "Service '%s' has hardcoded %s value",
		    serviceName, patternName,
		),
		Location:   location,
		Suggestion: suggestion,
		Service:    serviceName,
	    }}
	}
    }

    return issues
}

// isSwitchValue reports whether a value can only be an on/off setting. Short
// numbers other than 0 and 1 are left alone, because 1234 can be a real password.
func isSwitchValue(value string) bool {
    switch strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`)) {
    case "0", "1", "true", "false", "yes", "no", "on", "off":
	return true
    }
    return false
}

// extractVarName returns the name an entry sets, never its value. The secret
// patterns match NAME=value, NAME:value and a bare user:pass@ URL, so it cuts
// at '=' or ':', and returns "" for a URL, which has no name to show.
func extractVarName(entry string) string {
    if name, _, ok := strings.Cut(entry, "="); ok {
	return strings.TrimSpace(name)
    }
    if strings.Contains(entry, "://") {
	return ""
    }
    name, _, _ := strings.Cut(entry, ":")
    return strings.TrimSpace(name)
}

func severity(patternName string) string {
    switch patternName {
    case "password", "secret", "url_creds":
	return "high"
    case "api_key", "api_token":
	return "high"
    case "credential":
	return "medium"
    default:
	return "medium"
    }
}