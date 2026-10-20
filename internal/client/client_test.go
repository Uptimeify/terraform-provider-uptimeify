package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "wsm_test", "1.2.3")
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	return c, &slept
}

func TestGetTagSuccess(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags/7" || r.Header.Get("Authorization") != "Bearer wsm_test" ||
			r.Header.Get("User-Agent") != "terraform-provider-uptimeify/1.2.3" {
			t.Errorf("unexpected request %s %v", r.URL.Path, r.Header)
		}
		_, _ = w.Write([]byte(`{"id":7,"publicId":"p","customerId":null,"name":"prod","color":"blue","createdAt":"a","updatedAt":"b"}`))
	})
	tag, err := c.GetTag(context.Background(), "7")
	if err != nil || tag.ID != 7 || tag.Name != "prod" || tag.CustomerID != nil {
		t.Fatalf("got %+v, %v", tag, err)
	}
}

func TestErrorFormAndNotFound(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"Tag not found","data":{"code":"tagNotFound","requestId":"req-1"}}`))
	})
	_, err := c.GetTag(context.Background(), "9")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "tagNotFound" || apiErr.RequestID != "req-1" || apiErr.StatusMessage != "Tag not found" {
		t.Fatalf("got %#v", err)
	}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound = false")
	}
}

func TestRetry429ThenSuccess(t *testing.T) {
	calls := 0
	c, slept := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "x" {
			t.Errorf("body not resent on retry: %v", body)
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"name":"x","color":"red"}`))
	})
	if _, err := c.CreateTag(context.Background(), TagCreate{Name: "x", Color: "red"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(*slept) != 1 || (*slept)[0] != 2*time.Second {
		t.Fatalf("calls=%d slept=%v", calls, *slept)
	}
}

func TestRetryIsBounded(t *testing.T) {
	calls := 0
	c, slept := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.GetTag(context.Background(), "1")
	if apiErr, ok := err.(*APIError); !ok || apiErr.StatusCode != 429 {
		t.Fatalf("got %v", err)
	}
	if calls != maxRetries+1 || (*slept)[0] != maxRetryAfter {
		t.Fatalf("calls=%d slept=%v", calls, *slept)
	}
}

func TestNoRetryOf503ForPost(t *testing.T) {
	calls := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if _, err := c.CreateTag(context.Background(), TagCreate{Name: "x"}); err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	calls = 0
	_, _ = c.GetTag(context.Background(), "1")
	if calls != maxRetries+1 {
		t.Fatalf("GET 503 calls=%d", calls)
	}
}
