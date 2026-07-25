package archery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futuretea/archery-mcp-server/pkg/core/config"
)

func TestClientExecutesEveryCatalogOperation(t *testing.T) {
	t.Setenv("ARCHERY_CLIENT_CONTRACT_PASSWORD", "synthetic-password")
	operations := Operations()
	seen := make(map[string]int, len(operations))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prefix/api/auth/token/" {
			_, _ = w.Write([]byte(`{"access":"issued-access","refresh":"issued-refresh"}`))
			return
		}
		for _, operation := range operations {
			path := "/prefix" + strings.ReplaceAll(operation.Path, "{id}", "a/b")
			escapedPath := "/prefix" + strings.ReplaceAll(operation.Path, "{id}", "a%2Fb")
			if r.URL.Path == path && r.Method == operation.Method {
				if r.URL.EscapedPath() != escapedPath {
					t.Fatalf("%s escaped path = %q, want %q", operation.ID, r.URL.EscapedPath(), escapedPath)
				}
				if r.Header.Get("Authorization") != "Bearer issued-access" {
					t.Fatalf("%s authorization = %q", operation.ID, r.Header.Get("Authorization"))
				}
				if r.URL.Query().Get("page") != "2" {
					t.Fatalf("%s page query = %q, want 2", operation.ID, r.URL.Query().Get("page"))
				}
				if operation.Method == http.MethodGet {
					if r.Header.Get("Content-Type") != "" {
						t.Fatalf("%s GET content type = %q, want empty", operation.ID, r.Header.Get("Content-Type"))
					}
				} else {
					if r.Header.Get("Content-Type") != "application/json" {
						t.Fatalf("%s content type = %q, want application/json", operation.ID, r.Header.Get("Content-Type"))
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatalf("%s decode request body: %v", operation.ID, err)
					}
					if body["marker"] != operation.ID {
						t.Fatalf("%s marker = %v, want %q", operation.ID, body["marker"], operation.ID)
					}
					if operation.ID == "service_auth.verify" {
						assertServiceAuthBody(t, body)
					}
				}
				seen[operation.ID]++
				_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[]}`))
				return
			}
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client := newContractClient(t, server.URL+"/prefix")
	for _, operation := range operations {
		params := map[string]string{}
		if strings.Contains(operation.Path, "{id}") {
			params["id"] = "a/b"
		}
		query := url.Values{"page": {"2"}}
		body := map[string]any{"marker": operation.ID}
		if operation.Method == http.MethodGet {
			body = nil
		}
		if _, err := client.Do(context.Background(), operation, params, query, body); err != nil {
			t.Fatalf("%s: Client.Do() error = %v", operation.ID, err)
		}
	}
	for _, operation := range operations {
		if seen[operation.ID] != 1 {
			t.Fatalf("%s requests = %d, want 1", operation.ID, seen[operation.ID])
		}
	}
}

func assertServiceAuthBody(t *testing.T, body map[string]any) {
	t.Helper()
	if body["engineer"] != "mcp-service" || body["password"] != "synthetic-password" {
		t.Fatal("service_auth.verify does not use configured service credentials")
	}
}

func TestClientRefreshesExpiredOrMissingAccess(t *testing.T) {
	for _, initial := range []struct {
		name   string
		access string
		expiry time.Time
	}{
		{name: "expired access", access: testJWT(time.Now().Add(-time.Minute)), expiry: time.Now().Add(-time.Minute)},
		{name: "missing access"},
	} {
		t.Run(initial.name, func(t *testing.T) {
			var refreshes, tokens int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/prefix/api/auth/token/refresh/":
					refreshes++
					_, _ = w.Write([]byte(`{"access":"refreshed-access"}`))
				case "/prefix/api/auth/token/":
					tokens++
					_, _ = w.Write([]byte(`{"access":"new-access","refresh":"new-refresh"}`))
				case "/prefix/api/v1/user/":
					if r.Header.Get("Authorization") != "Bearer refreshed-access" {
						t.Fatalf("Authorization = %q, want refreshed access", r.Header.Get("Authorization"))
					}
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			defer server.Close()

			client := newContractClient(t, server.URL+"/prefix")
			client.access = initial.access
			client.accessExpires = initial.expiry
			client.refresh = "stored-refresh"
			if _, err := client.Do(context.Background(), operationByID(t, "users.list"), nil, nil, nil); err != nil {
				t.Fatalf("Client.Do() error = %v", err)
			}
			if refreshes != 1 || tokens != 0 {
				t.Fatalf("refresh/token requests = %d/%d, want 1/0", refreshes, tokens)
			}
		})
	}
}

func TestClientRefreshFailureObtainsFreshToken(t *testing.T) {
	var refreshes, tokens, requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/auth/token/refresh/":
			refreshes++
			http.Error(w, "expired refresh", http.StatusUnauthorized)
		case "/prefix/api/auth/token/":
			tokens++
			_, _ = w.Write([]byte(`{"access":"fresh-access","refresh":"fresh-refresh"}`))
		case "/prefix/api/v1/user/":
			requests++
			if r.Header.Get("Authorization") != "Bearer fresh-access" {
				t.Fatalf("Authorization = %q, want fresh access", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newContractClient(t, server.URL+"/prefix")
	client.access = testJWT(time.Now().Add(-time.Minute))
	client.accessExpires = time.Now().Add(-time.Minute)
	client.refresh = "expired-refresh"
	if _, err := client.Do(context.Background(), operationByID(t, "users.list"), nil, nil, nil); err != nil {
		t.Fatalf("Client.Do() error = %v", err)
	}
	if refreshes != 1 || tokens != 1 || requests != 1 {
		t.Fatalf("refresh/token/business requests = %d/%d/%d, want 1/1/1", refreshes, tokens, requests)
	}
}

func TestClientReturnsTokenAndResponseErrorsWithoutBusinessReplay(t *testing.T) {
	tests := []struct {
		name       string
		tokenBody  string
		tokenCode  int
		response   string
		statusCode int
		business   int
		secret     string
	}{
		{name: "token non-2xx", tokenBody: `{"detail":"synthetic-token-secret"}`, tokenCode: http.StatusUnauthorized, secret: "synthetic-token-secret"},
		{name: "invalid token JSON", tokenBody: "synthetic-token-secret", tokenCode: http.StatusOK, secret: "synthetic-token-secret"},
		{name: "business non-2xx", tokenBody: `{"access":"issued"}`, tokenCode: http.StatusOK, response: `{"detail":"synthetic-business-secret"}`, statusCode: http.StatusBadGateway, business: 1, secret: "synthetic-business-secret"},
		{name: "business invalid JSON", tokenBody: `{"access":"issued"}`, tokenCode: http.StatusOK, response: "synthetic-business-secret", statusCode: http.StatusOK, business: 1, secret: "synthetic-business-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var business int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/prefix/api/auth/token/" {
					w.WriteHeader(tt.tokenCode)
					_, _ = w.Write([]byte(tt.tokenBody))
					return
				}
				business++
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			_, err := newContractClient(t, server.URL+"/prefix").Do(context.Background(), operationByID(t, "users.create"), nil, nil, map[string]any{"username": "user"})
			if err == nil {
				t.Fatal("Client.Do() error = nil, want error")
			}
			if strings.Contains(err.Error(), tt.secret) {
				t.Fatalf("Client.Do() leaked upstream response content: %v", err)
			}
			if business != tt.business {
				t.Fatalf("business requests = %d, want %d", business, tt.business)
			}
		})
	}
}

func TestClientReturnsTimeoutWithoutBusinessReplay(t *testing.T) {
	var business int
	release := make(chan struct{})
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prefix/api/auth/token/" {
			_, _ = w.Write([]byte(`{"access":"issued"}`))
			return
		}
		business++
		<-release
		close(done)
	}))
	defer server.Close()

	client := newContractClientWithTimeout(t, server.URL+"/prefix", 20*time.Millisecond)
	_, err := client.Do(context.Background(), operationByID(t, "users.create"), nil, nil, map[string]any{"username": "user"})
	close(release)
	<-done
	if err == nil {
		t.Fatal("Client.Do() error = nil, want timeout error")
	}
	if business != 1 {
		t.Fatalf("business requests = %d, want 1", business)
	}
}

func TestClientPreservesPaginationAndSerializesRefresh(t *testing.T) {
	var mu sync.Mutex
	var refreshes, requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/auth/token/refresh/":
			mu.Lock()
			refreshes++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"access":"refreshed-access"}`))
		case "/prefix/api/v1/user/":
			if r.Header.Get("Authorization") != "Bearer refreshed-access" {
				t.Errorf("Authorization = %q, want refreshed access", r.Header.Get("Authorization"))
			}
			mu.Lock()
			requests++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"count":3,"next":"next-page","previous":"previous-page","results":[{"id":1}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newContractClient(t, server.URL+"/prefix")
	client.access = testJWT(time.Now().Add(-time.Minute))
	client.accessExpires = time.Now().Add(-time.Minute)
	client.refresh = "stored-refresh"
	operation := operationByID(t, "users.list")
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			raw, err := client.Do(context.Background(), operation, nil, nil, nil)
			if err == nil {
				var page struct {
					Count    int              `json:"count"`
					Next     string           `json:"next"`
					Previous string           `json:"previous"`
					Results  []map[string]int `json:"results"`
				}
				if decodeErr := json.Unmarshal(raw, &page); decodeErr != nil {
					err = decodeErr
				} else if page.Count != 3 || page.Next != "next-page" || page.Previous != "previous-page" || len(page.Results) != 1 || page.Results[0]["id"] != 1 {
					err = fmt.Errorf("pagination envelope not preserved: %s", raw)
				}
			}
			errors <- err
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Client.Do() error = %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if refreshes != 1 {
		t.Fatalf("refresh requests = %d, want 1", refreshes)
	}
	if requests != 8 {
		t.Fatalf("business requests = %d, want 8", requests)
	}
}

func newContractClient(t *testing.T, baseURL string) *Client {
	return newContractClientWithTimeout(t, baseURL, time.Second)
}

func newContractClientWithTimeout(t *testing.T, baseURL string, timeout time.Duration) *Client {
	t.Helper()
	t.Setenv("ARCHERY_CLIENT_CONTRACT_PASSWORD", "synthetic-password")
	client, err := NewClient(config.ArcheryConfig{BaseURL: baseURL, Username: "mcp-service", PasswordEnv: "ARCHERY_CLIENT_CONTRACT_PASSWORD", RequestTimeout: timeout})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func operationByID(t *testing.T, id string) Operation {
	t.Helper()
	for _, operation := range Operations() {
		if operation.ID == id {
			return operation
		}
	}
	t.Fatalf("Operations() missing %q", id)
	return Operation{}
}

func testJWT(expiry time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, expiry.Unix())))
	return header + "." + payload + ".signature"
}
