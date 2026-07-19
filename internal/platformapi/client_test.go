package platformapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientUsesNeutralEventEndpointAndBasicAuth(t *testing.T) {
	var got ExternalDNSEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "access" || password != "secret" {
			t.Fatalf("unexpected basic auth: %q %q %v", user, password, ok)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v2-beta/externalDnsEvents" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/v1", "access", "secret")
	if err != nil {
		t.Fatal(err)
	}
	event := &ExternalDNSEvent{EventType: "dns.update", ExternalID: "api.example.test."}
	if err := client.CreateExternalDNSEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if got != *event {
		t.Fatalf("event = %#v, want %#v", got, *event)
	}
}

func TestConnectionUsesBoundedListRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2-beta/externalDnsEvents" || r.URL.Query().Get("limit") != "1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "access", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsRelativeURL(t *testing.T) {
	if _, err := NewClient("/v1", "access", "secret"); err == nil {
		t.Fatal("expected a relative URL error")
	}
}

func TestClientRequiresScopedCredentials(t *testing.T) {
	for _, test := range []struct {
		access string
		secret string
	}{
		{access: "", secret: "secret"},
		{access: "access", secret: ""},
	} {
		if _, err := NewClient("https://platform.example.test/v1", test.access, test.secret); err == nil {
			t.Fatalf("expected empty scoped credential to be rejected: %#v", test)
		}
	}
}

func TestClientRejectsURLCredentialsQueryAndFragment(t *testing.T) {
	for _, rawURL := range []string{
		"https://user:password@platform.example.test/v1",
		"https://platform.example.test/v1?token=secret",
		"https://platform.example.test/v1#fragment",
	} {
		if _, err := NewClient(rawURL, "access", "secret"); err == nil {
			t.Fatalf("expected URL to be rejected: %s", rawURL)
		}
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	client, err := NewClient(redirector.URL+"/v1", "access", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.TestConnection(context.Background()); err == nil {
		t.Fatal("expected redirect response to be rejected")
	}
	if redirected.Load() {
		t.Fatal("scoped request followed a redirect")
	}
}

func TestClientDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret-value", http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "access", "secret")
	if err != nil {
		t.Fatal(err)
	}
	err = client.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected a failed request")
	}
	if got := err.Error(); got != "platform API returned 403 Forbidden" {
		t.Fatalf("unexpected redacted error: %q", got)
	}
}
