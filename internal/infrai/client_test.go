package infrai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRolloutRetriesRateLimitWithStableIdempotencyKey(t *testing.T) {
	var calls int
	var firstID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer authorization")
		}
		if calls == 1 {
			firstID = r.Header.Get("Idempotency-Key")
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.Header.Get("Idempotency-Key") != firstID || firstID == "" {
			t.Fatal("idempotency key changed across retry")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"key":"checkout_v2","percentage":10,"salt":"checkout_v2","sticky_unit":"user_id","version":6}` {
			t.Fatalf("body = %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		sleep:      func(context.Context, time.Duration) error { return nil },
	}
	if err := client.Rollout(context.Background(), "checkout_v2", 10, 6); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestSetBooleanSurfacesEnvelopeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"key":"checkout_v2","type":"bool","default_value":false,"enabled":true}` {
			t.Fatalf("body = %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"message":"request rejected"},"metadata":{}}`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		sleep:      sleepContext,
	}
	if _, err := client.SetBoolean(context.Background(), "checkout_v2", false); err == nil {
		t.Fatal("expected envelope error")
	}
}
