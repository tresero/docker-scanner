package security

import (
    "docker-scanner/internal/config"
    "docker-scanner/pkg/models"
    "docker-scanner/pkg/parser"
    "os"
    "path/filepath"
)

// EnvFileChecker verifies that a .env file exists when
// environment variables reference ${VAR} syntax
type EnvFileChecker struct{}

func (c *EnvFileChecker) Name() string {
    return "env_file_check"
}

func (c *EnvFileChecker) Check(filePath string) ([]models.SecurityIssue, error) {
    var issues []models.SecurityIssue

    composeDir := parser.GetComposeDir(filePath)

    // Check if any .env file exists
    envFound := false
    for _, envName := range config.EnvFileNames {
	envPath := filepath.Join(composeDir, envName)
	if _, err := os.Stat(envPath); err == nil {
	    envFound = true
	    break
	}
    }

    // Get environment variables from compose file
    envVars, err := parser.GetRawEnvironment(filePath)
    if err != nil {
	return nil, err
    }

    // If services use ${VAR} syntax but no .env file exists, flag it
    if !envFound && usesEnvVarSyntax(envVars) {
	issues = append(issues, models.SecurityIssue{
	    Type:        "missing_envfile",
	    Severity:    "high",
	    Description: "Compose file references ${VAR} syntax but no .env file found",
	    Location:    composeDir,
	    Suggestion:  "Create a .env file with the required variables",
	})
    }

    return issues, nil
}

func usesEnvVarSyntax(envVars map[string][]string) bool {
    for _, entries := range envVars {
	for _, entry := range entries {
	    if containsVarRef(entry) {
		return true
	    }
	}
    }
    return false
}

func containsVarRef(s string) bool {
    for i := 0; i < len(s)-1; i++ {
	if s[i] == '$' && s[i+1] == '{' {
	    return true
	}
    }
    return false
}