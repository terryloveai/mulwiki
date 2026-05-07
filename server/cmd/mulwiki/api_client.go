package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// apiClient is a minimal HTTP client for CLI commands that talk to the Mulwiki server.
type apiClient struct {
	baseURL string
	http    *http.Client
}

func newAPIClient(serverURL string) *apiClient {
	return &apiClient{
		baseURL: serverURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *apiClient) get(path string, target any) error {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
