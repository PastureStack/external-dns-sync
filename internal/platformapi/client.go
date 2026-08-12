package platformapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second
const maxResponseSize = 4 << 20

type Client struct {
	baseURL   *url.URL
	policy    *originPolicy
	accessKey string
	secretKey string
	http      *http.Client
}

// originPolicy fixes the control-plane destination at client construction.
// Paths and queries may vary, but they can never change the approved scheme,
// hostname, or effective port.
type originPolicy struct {
	origin string
}

// policyTransport rechecks every request at the final network boundary. This
// remains effective if a future caller accidentally constructs a request from
// an untrusted absolute URL.
type policyTransport struct {
	base   http.RoundTripper
	policy *originPolicy
}

func (transport *policyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("platform API request URL is missing")
	}
	requestURL := request.URL.String()
	if transport == nil || transport.policy == nil || !isValidRedirectURL(requestURL, transport.policy) {
		return nil, fmt.Errorf("platform API request origin is not authorized")
	}
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

type ExternalDNSEvent struct {
	EventType   string `json:"eventType,omitempty"`
	ExternalID  string `json:"externalId,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	StackName   string `json:"stackName,omitempty"`
	FQDN        string `json:"fqdn,omitempty"`
}

func NewClient(rawURL, accessKey, secretKey string) (*Client, error) {
	if strings.TrimSpace(accessKey) == "" {
		return nil, fmt.Errorf("platform access key is required")
	}
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("platform secret key is required")
	}
	u, err := parsePlatformURL(rawURL)
	if err != nil {
		return nil, err
	}

	switch {
	case u.Path == "" || u.Path == "/":
		u.Path = "/v2-beta"
	case u.Path == "/v1":
		u.Path = "/v2-beta"
	case strings.HasPrefix(u.Path, "/v1/"):
		u.Path = "/v2-beta/" + strings.TrimPrefix(u.Path, "/v1/")
	}
	u.RawPath = ""

	origin, err := canonicalOrigin(u)
	if err != nil {
		return nil, err
	}
	policy := &originPolicy{origin: origin}
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	// Control-plane credentials must not be forwarded through an ambient proxy.
	baseTransport.Proxy = nil
	if baseTransport.TLSClientConfig == nil {
		baseTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		baseTransport.TLSClientConfig = baseTransport.TLSClientConfig.Clone()
		baseTransport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}

	return &Client{
		baseURL:   u,
		policy:    policy,
		accessKey: accessKey,
		secretKey: secretKey,
		http: &http.Client{
			Timeout:   defaultTimeout,
			Transport: &policyTransport{base: baseTransport, policy: policy},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) CreateExternalDNSEvent(ctx context.Context, event *ExternalDNSEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode external DNS event: %w", err)
	}
	return c.request(ctx, http.MethodPost, "/externalDnsEvents", nil, bytes.NewReader(body))
}

func (c *Client) TestConnection(ctx context.Context) error {
	query := url.Values{}
	query.Set("limit", "1")
	return c.request(ctx, http.MethodGet, "/externalDnsEvents", query, nil)
}

func (c *Client) request(ctx context.Context, method, resourcePath string, query url.Values, body io.Reader) error {
	if c == nil || c.baseURL == nil || c.policy == nil || c.http == nil {
		return fmt.Errorf("platform API client is not configured")
	}
	if !strings.HasPrefix(resourcePath, "/") || strings.HasPrefix(resourcePath, "//") ||
		strings.ContainsAny(resourcePath, "\\?#\r\n\t") || strings.Contains(resourcePath, "..") {
		return fmt.Errorf("platform API resource path is invalid")
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + resourcePath
	endpoint.RawPath = ""
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = ""
	requestURL := endpoint.String()
	if !isValidRedirectURL(requestURL, c.policy) {
		return fmt.Errorf("platform API request origin is not authorized")
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create platform API request: %w", err)
	}
	req.SetBasicAuth(c.accessKey, c.secretKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("platform API request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if readErr != nil {
		return fmt.Errorf("read platform API response: %w", readErr)
	}
	if len(responseBody) > maxResponseSize {
		return fmt.Errorf("platform API response exceeds %d bytes", maxResponseSize)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("platform API returned %s", resp.Status)
	}
	return nil
}

func parsePlatformURL(rawURL string) (*url.URL, error) {
	if rawURL == "" || rawURL != strings.TrimSpace(rawURL) || strings.ContainsAny(rawURL, "\r\n\t") {
		return nil, fmt.Errorf("platform URL must not be empty or contain whitespace")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse platform URL: %w", err)
	}
	if strings.ToLower(u.Scheme) != "http" && strings.ToLower(u.Scheme) != "https" {
		return nil, fmt.Errorf("platform URL must use http or https")
	}
	if u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("platform URL must include a host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("platform URL must not contain user information")
	}
	if u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("platform URL must not contain a query or fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if _, err := canonicalOrigin(u); err != nil {
		return nil, err
	}
	return u, nil
}

// isValidRedirectURL deliberately validates both constructed and final
// requests against the origin fixed when the client was created. Keeping the
// checked URL in one value ensures that the value which passes authorization
// is exactly the value handed to the HTTP stack.
func isValidRedirectURL(rawURL string, policy *originPolicy) bool {
	if policy == nil {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	origin, err := canonicalOrigin(parsed)
	return err == nil && origin == policy.origin
}

func canonicalOrigin(parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("platform URL is missing")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("platform URL must use http or https")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" {
		return "", fmt.Errorf("platform URL must include a host")
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("platform URL contains an invalid port")
	}
	return scheme + "://" + net.JoinHostPort(hostname, port), nil
}
