package platformapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type recordingTransport struct {
	requests int
}

func (transport *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
		Request:    request,
	}, nil
}

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
		"https://platform.example.test:70000/v1",
		"https://platform.example.test/v1\r\nHost: attacker.invalid",
	} {
		if _, err := NewClient(rawURL, "access", "secret"); err == nil {
			t.Fatalf("expected URL to be rejected: %s", rawURL)
		}
	}
}

func TestPolicyTransportRejectsChangedOriginBeforeNetwork(t *testing.T) {
	approved, err := url.Parse("https://platform.example.test/v2-beta")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := canonicalOrigin(approved)
	if err != nil {
		t.Fatal(err)
	}
	base := &recordingTransport{}
	transport := &policyTransport{base: base, policy: &originPolicy{origin: origin}}
	client := &http.Client{Transport: transport}

	for _, denied := range []string{
		"http://platform.example.test/v2-beta/externalDnsEvents",
		"https://platform.example.test:444/v2-beta/externalDnsEvents",
		"https://platform.example.test.attacker.invalid/v2-beta/externalDnsEvents",
		"http://169.254.169.254/latest/meta-data",
	} {
		if _, err := client.Get(denied); err == nil {
			t.Fatalf("changed origin %q was accepted", denied)
		}
	}
	if base.requests != 0 {
		t.Fatalf("unauthorized requests reached the network %d times", base.requests)
	}

	response, err := client.Get("https://PLATFORM.EXAMPLE.TEST:443/v2-beta/externalDnsEvents?limit=1")
	if err != nil {
		t.Fatalf("approved origin was rejected: %v", err)
	}
	response.Body.Close()
	if base.requests != 1 {
		t.Fatalf("approved request reached the network %d times", base.requests)
	}
}

func TestClientResourceBuilderCannotChangeOrigin(t *testing.T) {
	client, err := NewClient("https://platform.example.test/v1", "access", "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, resourcePath := range []string{
		"https://attacker.invalid/steal",
		"//attacker.invalid/steal",
		"/../steal",
		"/resource?next=https://attacker.invalid",
		"/resource\\..\\steal",
	} {
		if err := client.request(context.Background(), http.MethodGet, resourcePath, nil, nil); err == nil {
			t.Fatalf("unsafe resource path %q was accepted", resourcePath)
		}
	}
}

func TestValidatedRequestURLIsTheValueSentToTheNetwork(t *testing.T) {
	approved, err := url.Parse("https://platform.example.test/v2-beta")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := canonicalOrigin(approved)
	if err != nil {
		t.Fatal(err)
	}
	policy := &originPolicy{origin: origin}

	for _, test := range []struct {
		name    string
		rawURL  string
		allowed bool
	}{
		{name: "approved path", rawURL: "https://platform.example.test/v2-beta/externalDnsEvents?limit=1", allowed: true},
		{name: "userinfo", rawURL: "https://access:secret@platform.example.test/v2-beta/externalDnsEvents"},
		{name: "changed host", rawURL: "https://attacker.invalid/v2-beta/externalDnsEvents"},
		{name: "suffix confusion", rawURL: "https://platform.example.test.attacker.invalid/v2-beta/externalDnsEvents"},
		{name: "changed scheme", rawURL: "http://platform.example.test/v2-beta/externalDnsEvents"},
		{name: "changed port", rawURL: "https://platform.example.test:444/v2-beta/externalDnsEvents"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isValidRedirectURL(test.rawURL, policy); got != test.allowed {
				t.Fatalf("isValidRedirectURL(%q) = %v, want %v", test.rawURL, got, test.allowed)
			}
		})
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
