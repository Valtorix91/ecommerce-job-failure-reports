package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunDecision(t *testing.T) {
	for _, tc := range []struct {
		job, status, want string
		calls             int
	}{
		{"checkout", "failed", "captured", 1},
		{"fulfillment", "failed", "captured", 1},
		{"receipts", "failed", "captured", 1},
		{"customer_order_updates", "failed", "captured", 1},
		{"checkout", "succeeded", "completed", 0},
	} {
		t.Run(tc.job+"/"+tc.status, func(t *testing.T) {
			calls := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/v1/errors/capture" || r.Header.Get("Idempotency-Key") != "daily-2026-09-19-42" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("request boundary: %s %s", r.Method, r.URL.Path)
				}
				var body struct {
					Fingerprint []string `json:"fingerprint"`
					Exception   string   `json:"exception"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Fingerprint) != 2 || body.Fingerprint[1] != tc.job || body.Exception != "delivery rejected" {
					t.Errorf("capture payload: %+v", body)
				}
				io.WriteString(w, `{"ok":true,"data":{},"error":null,"metadata":{}}`)
			}))
			defer remote.Close()
			client := captureClient{http: remote.Client(), base: remote.URL, key: "test-key", sleep: pause}
			req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job":"`+tc.job+`","order_id":"42","run_id":"daily-2026-09-19-42","status":"`+tc.status+`","message":"delivery rejected"}`))
			response := httptest.NewRecorder()
			handler(client).ServeHTTP(response, req)
			if response.Code != 200 || !strings.Contains(response.Body.String(), tc.want) || calls != tc.calls {
				t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), calls)
			}
		})
	}
}

func TestCaptureEnvelopeAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		retry                 bool
		wantStatus, wantCalls int
	}{
		{"business rejection", false, 400, 1},
		{"rate limit", true, 200, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.retry && calls == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(429)
					io.WriteString(w, `{"ok":false,"error":{"code":"RATE_LIMIT","message":"retry"}}`)
					return
				}
				if !tc.retry {
					w.WriteHeader(400)
					io.WriteString(w, `{"ok":false,"error":{"message":"invalid input"}}`)
					return
				}
				io.WriteString(w, `{"ok":true,"data":{},"metadata":{}}`)
			}))
			defer remote.Close()
			client := captureClient{http: remote.Client(), base: remote.URL, key: "test-key", sleep: func(context.Context, time.Duration) error { return nil }}
			req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job":"receipts","order_id":"42","run_id":"run-42","status":"failed","message":"delivery rejected"}`))
			response := httptest.NewRecorder()
			handler(client).ServeHTTP(response, req)
			if response.Code != tc.wantStatus || calls != tc.wantCalls {
				t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), calls)
			}
		})
	}
}
