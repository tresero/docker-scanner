package security

import "docker-scanner/pkg/models"

// Checker is the interface for all security checks.
// Implement this to add new checks without modifying existing code.
type Checker interface {
    Name() string
    Check(filePath string) ([]models.SecurityIssue, error)
}

// RunAll executes all provided checkers against a compose file
func RunAll(checkers []Checker, filePath string) []models.SecurityIssue {
    var issues []models.SecurityIssue

    for _, checker := range checkers {
	found, err := checker.Check(filePath)
	if err != nil {
	    issues = append(issues, models.SecurityIssue{
		Type:        "check_error",
		Severity:    "low",
		Description: "Error running " + checker.Name() + ": " + err.Error(),
		Location:    filePath,
	    })
	    continue
	}
	issues = append(issues, found...)
    }

    return issues
}

// DefaultCheckers returns the standard set of security checkers
func DefaultCheckers() []Checker {
    return []Checker{
	&EnvFileChecker{},
	&HardcodedSecretsChecker{},
    }
}