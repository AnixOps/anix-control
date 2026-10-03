package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client talks to one Control instance.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	mu     sync.Mutex
	tokens map[string]string // e-mail -> bearer token
}

func newClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		tokens:  map[string]string{},
	}
}

// Response is a raw HTTP answer.
type Response struct {
	Status int
	Body   []byte
	Header http.Header
}

// do sends one request. Control rate-limits every client address (admin
// routes 10 requests per second); the staging tool is one address, so a 429
// is waited out and retried instead of becoming part of a result.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, token string) (Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := c.once(ctx, method, path, query, body, token)
		if err != nil || response.Status != http.StatusTooManyRequests || attempt == 20 {
			return response, err
		}
		wait := time.Second
		if seconds, convErr := strconv.Atoi(response.Header.Get("Retry-After")); convErr == nil && seconds > 0 && seconds < 30 {
			wait = time.Duration(seconds) * time.Second
		}
		select {
		case <-ctx.Done():
			return response, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *Client) once(ctx context.Context, method, path string, query url.Values, body []byte, token string) (Response, error) {
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader) // #nosec G704 -- requests go to the staging Control named on the command line
	if err != nil {
		return Response{}, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.HTTP.Do(request) // #nosec G704 -- requests go to the staging Control named on the command line
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return Response{}, err
	}
	return Response{Status: response.StatusCode, Body: data, Header: response.Header}, nil
}

// jsonCall sends a JSON body and decodes {"data": ...} (kernel API) into out.
func (c *Client) kernel(ctx context.Context, method, path string, query url.Values, in, out any, token string) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	response, err := c.do(ctx, method, path, query, body, token)
	if err != nil {
		return err
	}
	if response.Status >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, response.Status, truncate(response.Body, 400))
	}
	if out == nil {
		return nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return fmt.Errorf("%s %s: decode: %w", method, path, err)
	}
	return json.Unmarshal(envelope.Data, out)
}

// Login returns a cached bearer token for the account, logging in when
// needed.
func (c *Client) Login(ctx context.Context, email, password string) (string, error) {
	c.mu.Lock()
	token, ok := c.tokens[email]
	c.mu.Unlock()
	if ok {
		return token, nil
	}
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	response, err := c.do(ctx, http.MethodPost, "/api/v2/login", nil, body, "")
	if err != nil {
		return "", err
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Data.Token == "" {
		return "", fmt.Errorf("login %s: HTTP %d: %s", email, response.Status, truncate(response.Body, 300))
	}
	c.mu.Lock()
	c.tokens[email] = envelope.Data.Token
	c.mu.Unlock()
	return envelope.Data.Token, nil
}

// Forget drops a cached token (after a 401).
func (c *Client) Forget(email string) {
	c.mu.Lock()
	delete(c.tokens, email)
	c.mu.Unlock()
}

// WaitReady polls /readyz until Control answers 200.
func (c *Client) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		response, err := c.do(ctx, http.MethodGet, "/readyz", nil, nil, "")
		if err == nil && response.Status == http.StatusOK {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = fmt.Errorf("HTTP %d", response.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if last == nil {
		last = errors.New("timeout")
	}
	return fmt.Errorf("%s not ready after %s: %w", c.BaseURL, timeout, last)
}

func truncate(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}
	return string(data[:limit]) + "..."
}
