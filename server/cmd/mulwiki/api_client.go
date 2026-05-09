package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// apiClient is a minimal HTTP client for CLI commands that talk to the Mulwiki server.
type apiClient struct {
	baseURL     string
	sessionID   string
	bearerToken string
	http        *http.Client
}

func newAPIClient(serverURL string) *apiClient {
	return &apiClient{
		baseURL: serverURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *apiClient) setSessionID(sessionID string) {
	c.sessionID = sessionID
}

func (c *apiClient) setBearerToken(token string) {
	c.bearerToken = token
}

func (c *apiClient) get(path string, target any) error {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *apiClient) post(path string, payload any, target any) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := c.newRequest(http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = resp.Status
		}
		return resp, fmt.Errorf("server returned %d: %s", resp.StatusCode, errBody.Error)
	}
	if target != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func (c *apiClient) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.sessionID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: c.sessionID})
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	return req, nil
}
