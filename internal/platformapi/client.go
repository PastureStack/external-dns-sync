package platformapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second
const maxResponseSize = 4 << 20

type Client struct {
	baseURL   string
	accessKey string
	secretKey string
	http      *http.Client
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
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse platform URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("platform URL must use http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("platform URL must include a host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("platform URL must not contain user information")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("platform URL must not contain a query or fragment")
	}

	switch {
	case u.Path == "" || u.Path == "/":
		u.Path = "/v2-beta"
	case u.Path == "/v1":
		u.Path = "/v2-beta"
	case strings.HasPrefix(u.Path, "/v1/"):
		u.Path = "/v2-beta/" + strings.TrimPrefix(u.Path, "/v1/")
	}

	return &Client{
		baseURL:   strings.TrimRight(u.String(), "/"),
		accessKey: accessKey,
		secretKey: secretKey,
		http: &http.Client{
			Timeout: defaultTimeout,
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
	return c.request(ctx, http.MethodPost, c.baseURL+"/externalDnsEvents", bytes.NewReader(body))
}

func (c *Client) TestConnection(ctx context.Context) error {
	return c.request(ctx, http.MethodGet, c.baseURL+"/externalDnsEvents?limit=1", nil)
}

func (c *Client) request(ctx context.Context, method, endpoint string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
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
