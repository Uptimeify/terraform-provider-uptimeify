// Package client is a small JSON client for the Uptimeify REST API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries     = 3
	maxRetryAfter  = 60 * time.Second
	baseRetryDelay = time.Second
	// A website create of the same customer is still running (429 websiteCreationInProgress, e.g.
	// a parallel `for_each`): wait for it up to this long in total instead of three short tries.
	creationRetryBudget = 60 * time.Second
	creationRetryDelay  = 2 * time.Second
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
	// Issues are the per-field validation problems of a 400 invalidRequestBody (data.issues).
	Issues []Issue
	// Data is the full `data` object, e.g. existingPublicId of 409 monitorAlreadyExists.
	Data map[string]any
}

// Issue is one entry of data.issues: the field path (dot separated) and what is wrong with it.
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
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
	for _, is := range e.Issues {
		if is.Path != "" {
			msg += "\n  - " + is.Path + ": " + is.Message
		} else {
			msg += "\n  - " + is.Message
		}
	}
	return msg
}

// ExistingMonitor returns the public id of the monitor a create collided with
// (409 monitorAlreadyExists), or "" for any other error.
func ExistingMonitor(err error) string {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict || apiErr.Code != "monitorAlreadyExists" {
		return ""
	}
	id, _ := apiErr.Data["existingPublicId"].(string)
	return id
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

	var slept time.Duration // only counted for websiteCreationInProgress
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
		inProgress := resp.StatusCode == http.StatusTooManyRequests && errorCode(respBody) == "websiteCreationInProgress"
		if !retryable || (!inProgress && attempt >= maxRetries) || (inProgress && slept >= creationRetryBudget) {
			return parseError(resp, respBody)
		}
		delay := baseRetryDelay << attempt
		if inProgress {
			delay = creationRetryDelay
		}
		if d, ok := retryAfter(resp.Header.Get("Retry-After")); ok {
			delay = d
		}
		if inProgress {
			// Jitter so parallel creates of one customer do not retry in lockstep.
			delay = min(delay+rand.N(time.Second), creationRetryBudget-slept)
			slept += delay
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

func errorCode(body []byte) string {
	var h3 struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &h3)
	return h3.Data.Code
}

func parseError(resp *http.Response, body []byte) error {
	e := &APIError{StatusCode: resp.StatusCode, RequestID: resp.Header.Get("X-Request-ID")}
	var h3 struct {
		StatusMessage string `json:"statusMessage"`
		Message       string `json:"message"`
		Data          struct {
			Code      string  `json:"code"`
			RequestID string  `json:"requestId"`
			Issues    []Issue `json:"issues"`
		} `json:"data"`
	}
	var raw struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &raw) == nil {
		e.Data = raw.Data
	}
	if json.Unmarshal(body, &h3) == nil {
		e.StatusMessage = h3.StatusMessage
		if e.StatusMessage == "" {
			e.StatusMessage = h3.Message
		}
		e.Code = h3.Data.Code
		e.Issues = h3.Data.Issues
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
