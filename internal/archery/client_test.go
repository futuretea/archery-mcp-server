package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/futuretea/archery-mcp-server/pkg/core/config"
)

// Seam: Client.Do obtains a JWT internally and sends fixed catalog requests through the configured base path.
func TestClientDoUsesBearerTokenAndPreservesBasePath(t *testing.T) {
	t.Setenv("ARCHERY_CLIENT_TEST_PASSWORD", "synthetic-test-value")
	var tokenRequests, businessRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/auth/token/":
			tokenRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s, want POST", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("token content type = %q, want application/json", r.Header.Get("Content-Type"))
			}
			var credentials struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
				t.Fatalf("decode token request: %v", err)
			}
			if credentials.Username != "mcp-service" || credentials.Password != "synthetic-test-value" {
				t.Fatalf("token credentials do not use configured service identity")
			}
			_, _ = w.Write([]byte(`{"access":"issued-for-test","refresh":"refresh-for-test"}`))
		case "/prefix/api/v1/user/":
			businessRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("business method = %s, want GET", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer issued-for-test" {
				t.Fatalf("Authorization = %q, want bearer token", got)
			}
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[]}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(config.ArcheryConfig{
		BaseURL:        server.URL + "/prefix",
		Username:       "mcp-service",
		PasswordEnv:    "ARCHERY_CLIENT_TEST_PASSWORD",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var list Operation
	for _, operation := range Operations() {
		if operation.ID == "users.list" {
			list = operation
			break
		}
	}
	if list.ID == "" {
		t.Fatal("Operations() missing users.list")
	}
	if _, err := client.Do(context.Background(), list, nil, nil, nil); err != nil {
		t.Fatalf("Client.Do() error = %v", err)
	}
	if tokenRequests != 1 || businessRequests != 1 {
		t.Fatalf("token/business requests = %d/%d, want 1/1", tokenRequests, businessRequests)
	}
}
