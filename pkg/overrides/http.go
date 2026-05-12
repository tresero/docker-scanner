package overrides

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// DefaultGetter returns a Getter that fetches URLs via net/http with a
// reasonable timeout. If GITHUB_TOKEN is set in the environment, it's
// sent as a Bearer token — GitHub's anonymous rate limit is 60/hour,
// authenticated is 5000/hour. Larger fleets need the token.
func DefaultGetter() Getter {
	client := &http.Client{Timeout: 10 * time.Second}
	token := os.Getenv("GITHUB_TOKEN")

	return func(url string) ([]byte, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
		}

		return io.ReadAll(resp.Body)
	}
}