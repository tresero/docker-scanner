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

    // Check against all secret patterns
    for patternName, pattern := range config.SecretPatterns {
	if pattern.MatchString(entry) {
	    varName := extractVarName(entry)
	    issues = append(issues, models.SecurityIssue{
		Type:     "hardcoded_secret",
		Severity: severity(patternName),
		Description: fmt.Sprintf(
		    "Service '%s' has hardcoded %s value",
		    serviceName, patternName,
		),
		Location:   fmt.Sprintf("%s -> %s", filePath, varName),
		Suggestion: fmt.Sprintf("Use ${%s} and add to .env file", strings.ToUpper(varName)),
	    })
	}
    }

    return issues
}

func extractVarName(entry string) string {
    parts := strings.SplitN(entry, "=", 2)
    if len(parts) > 0 {
	return strings.TrimSpace(parts[0])
    }
    return entry
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