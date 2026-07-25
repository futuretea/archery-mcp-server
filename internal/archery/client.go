package archery

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/futuretea/archery-mcp-server/pkg/core/config"
)

type Client struct {
	baseURL       *url.URL
	username      string
	password      string
	http          *http.Client
	mu            sync.Mutex
	access        string
	refresh       string
	accessExpires time.Time
}

func NewClient(cfg config.ArcheryConfig) (*Client, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid archery base_url")
	}
	password := os.Getenv(cfg.PasswordEnv)
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("archery password environment %q is not set", cfg.PasswordEnv)
	}
	return &Client{baseURL: baseURL, username: cfg.Username, password: password, http: &http.Client{Timeout: cfg.RequestTimeout}}, nil
}

func (c *Client) Do(ctx context.Context, operation Operation, pathParams map[string]string, query url.Values, body map[string]any) (json.RawMessage, error) {
	access, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	path := operation.Path
	rawPath := operation.Path
	for key, value := range pathParams {
		placeholder := "{" + key + "}"
		path = strings.ReplaceAll(path, placeholder, value)
		rawPath = strings.ReplaceAll(rawPath, placeholder, url.PathEscape(value))
	}
	endpoint := c.endpoint(path, rawPath)
	endpoint.RawQuery = query.Encode()
	if operation.ID == "service_auth.verify" {
		if body == nil {
			body = map[string]any{}
		}
		body["engineer"] = c.username
		body["password"] = c.password
	}
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		raw, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode request: %w", marshalErr)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, operation.Method, endpoint.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+access)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req)
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && (c.accessExpires.IsZero() || time.Now().Before(c.accessExpires)) {
		return c.access, nil
	}
	if c.refresh != "" && c.refreshToken(ctx) == nil {
		return c.access, nil
	}
	if err := c.acquireToken(ctx); err != nil {
		return "", fmt.Errorf("authenticate with Archery: %w", err)
	}
	return c.access, nil
}

func (c *Client) acquireToken(ctx context.Context) error {
	endpoint := c.endpoint("/api/auth/token/", "/api/auth/token/")
	body, _ := json.Marshal(map[string]string{"username": c.username, "password": c.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, err := c.send(req)
	if err != nil {
		return err
	}
	return c.setTokens(raw)
}

func (c *Client) refreshToken(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"refresh": c.refresh})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/auth/token/refresh/", "/api/auth/token/refresh/").String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, err := c.send(req)
	if err != nil {
		return err
	}
	return c.setTokens(raw)
}

func (c *Client) setTokens(raw json.RawMessage) error {
	var response struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.Access == "" {
		return fmt.Errorf("decode Archery token response")
	}
	c.access = response.Access
	if response.Refresh != "" {
		c.refresh = response.Refresh
	}
	c.accessExpires = tokenExpiry(response.Access)
	return nil
}

func (c *Client) endpoint(path, rawPath string) *url.URL {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	endpoint.RawPath = strings.TrimRight(c.baseURL.EscapedPath(), "/") + rawPath
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return &endpoint
}

func tokenExpiry(access string) time.Time {
	parts := strings.Split(access, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.ExpiresAt == 0 {
		return time.Time{}
	}
	return time.Unix(claims.ExpiresAt, 0)
}

func (c *Client) send(req *http.Request) (json.RawMessage, error) {
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("archery request failed")
	}
	defer func() { _ = response.Body.Close() }()
	var raw json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil && response.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("decode Archery response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("archery request returned %d", response.StatusCode)
	}
	if raw == nil {
		raw = json.RawMessage(`{}`)
	}
	return raw, nil
}
