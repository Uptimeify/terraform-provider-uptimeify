// Package client is a small JSON client for the Uptimeify REST API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries     = 3
	maxRetryAfter  = 60 * time.Second
	baseRetryDelay = time.Second
)

// Client talks to one Uptimeify endpoint with one API token
// (organization token `wsm_...` or customer token).
type Client struct {
	endpoint  string
	token     string
	userAgent string
	http      *http.Client
	sleep     func(context.Context, time.Duration) error
}

// New builds a client. endpoint is the base URL, e.g. https://uptimeify.io.
func New(endpoint, token, version string) *Client {
	return &Client{
		endpoint:  strings.TrimRight(endpoint, "/"),
		token:     token,
		userAgent: "terraform-provider-uptimeify/" + version,
		http:      &http.Client{Timeout: 60 * time.Second},
		sleep:     sleepCtx,
	}
}

// APIError is the h3 error form of the API:
// { statusCode, statusMessage, data: { code, requestId } }.
type APIError struct {
	StatusCode    int
	StatusMessage string
	Code          string
	RequestID     string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("uptimeify API: HTTP %d", e.StatusCode)
	if e.StatusMessage != "" {
		msg += " " + e.StatusMessage
	}
	if e.Code != "" {
		msg += " (code " + e.Code + ")"
	}
	if e.RequestID != "" {
		msg += " [requestId " + e.RequestID + "]"
	}
	return msg
}

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// Do sends method path with in as JSON body (nil = no body) and decodes the answer into out (nil = discard).
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	idempotent := method != http.MethodPost && method != http.MethodPatch

	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, method, path, body)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode < 300 {
			if out == nil || len(respBody) == 0 {
				return nil
			}
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("uptimeify API: decode %s %s: %w", method, path, err)
			}
			return nil
		}

		// 429 means the request was not processed, so every method may retry it.
		retryable := resp.StatusCode == http.StatusTooManyRequests ||
			(idempotent && (resp.StatusCode == http.StatusBadGateway ||
				resp.StatusCode == http.StatusServiceUnavailable ||
				resp.StatusCode == http.StatusGatewayTimeout))
		if !retryable || attempt >= maxRetries {
			return parseError(resp, respBody)
		}
		delay := baseRetryDelay << attempt
		if d, ok := retryAfter(resp.Header.Get("Retry-After")); ok {
			delay = d
		}
		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func (c *Client) send(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func parseError(resp *http.Response, body []byte) error {
	e := &APIError{StatusCode: resp.StatusCode, RequestID: resp.Header.Get("X-Request-ID")}
	var h3 struct {
		StatusMessage string `json:"statusMessage"`
		Message       string `json:"message"`
		Data          struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &h3) == nil {
		e.StatusMessage = h3.StatusMessage
		if e.StatusMessage == "" {
			e.StatusMessage = h3.Message
		}
		e.Code = h3.Data.Code
		if h3.Data.RequestID != "" {
			e.RequestID = h3.Data.RequestID
		}
	}
	if e.StatusMessage == "" {
		e.StatusMessage = http.StatusText(resp.StatusCode)
	}
	return e
}

// retryAfter parses seconds or an HTTP date, capped at maxRetryAfter.
func retryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	var d time.Duration
	if s, err := strconv.Atoi(v); err == nil {
		d = time.Duration(s) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	} else {
		return 0, false
	}
	return min(max(d, 0), maxRetryAfter), true
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
