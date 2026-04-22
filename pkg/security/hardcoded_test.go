package security

import (
    "os"
    "path/filepath"
    "testing"
)

func writeTestCompose(t *testing.T, dir, content string) string {
    t.Helper()
    path := filepath.Join(dir, "docker-compose.yml")
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
	t.Fatal(err)
    }
    return path
}

func TestHardcodedSecrets_DetectsPlainPassword(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=mysecretpassword
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) == 0 {
	t.Error("expected hardcoded password to be detected")
    }

    found := false
    for _, issue := range issues {
	if issue.Type == "hardcoded_secret" && issue.Severity == "high" {
	    found = true
	}
    }
    if !found {
	t.Error("expected high severity hardcoded_secret issue")
    }
}

func TestHardcodedSecrets_AllowsEnvVar(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) != 0 {
	t.Errorf("expected no issues for ${VAR} syntax, got %d", len(issues))
    }
}

func TestHardcodedSecrets_DetectsAPIKey(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  app:
    image: myapp:latest
    environment:
      - API_KEY=sk-1234567890abcdef
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) == 0 {
	t.Error("expected hardcoded API key to be detected")
    }
}

func TestHardcodedSecrets_DetectsSecret(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  app:
    image: directus/directus:latest
    environment:
      SECRET: my-super-secret-value
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) == 0 {
	t.Error("expected hardcoded secret to be detected")
    }
}

func TestHardcodedSecrets_MapFormat(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: rootpass123
      MYSQL_DATABASE: mydb
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) == 0 {
	t.Error("expected hardcoded password in map format to be detected")
    }
}

func TestHardcodedSecrets_NoEnvironment(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`)

    checker := &HardcodedSecretsChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) != 0 {
	t.Errorf("expected no issues for service without environment, got %d", len(issues))
    }
}

func TestEnvFile_MissingEnvFile(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  app:
    image: myapp:latest
    environment:
      - DB_HOST=${DB_HOST}
      - DB_PASS=${DB_PASS}
`)

    checker := &EnvFileChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) == 0 {
	t.Error("expected missing .env file to be flagged")
    }
}

func TestEnvFile_EnvFileExists(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  app:
    image: myapp:latest
    environment:
      - DB_HOST=${DB_HOST}
`)

    // Create .env file
    envPath := filepath.Join(dir, ".env")
    os.WriteFile(envPath, []byte("DB_HOST=localhost\n"), 0644)

    checker := &EnvFileChecker{}
    issues, err := checker.Check(path)
    if err != nil {
	t.Fatal(err)
    }

    if len(issues) != 0 {
	t.Errorf("expected no issues when .env exists, got %d", len(issues))
    }
}

func TestRunAll(t *testing.T) {
    dir := t.TempDir()
    path := writeTestCompose(t, dir, `
services:
  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=hardcoded
      - DB_HOST=${DB_HOST}
`)

    checkers := DefaultCheckers()
    issues := RunAll(checkers, path)

    // Should find hardcoded password + missing .env
    if len(issues) < 2 {
	t.Errorf("expected at least 2 issues, got %d", len(issues))
    }
}