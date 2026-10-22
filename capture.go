package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type captureClient struct {
	http  *http.Client
	base  string
	key   string
	sleep func(context.Context, time.Duration) error
}

type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return fmt.Sprintf("capture: %s: %s", e.code, e.message) }

func (c captureClient) capture(ctx context.Context, run jobRun) error {
	payload := struct {
		Title       string            `json:"title"`
		Message     string            `json:"message"`
		Level       string            `json:"level"`
		Fingerprint []string          `json:"fingerprint"`
		Exception   string            `json:"exception"`
		Context     map[string]string `json:"context"`
	}{
		Title:       run.Job + " scheduled job failed",
		Message:     run.Message,
		Level:       "error",
		Fingerprint: []string{"ecommerce", run.Job},
		Exception:   run.Message,
		Context:     map[string]string{"order_id": run.OrderID, "run_id": run.RunID, "job": run.Job},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		// infrai.errors.capture: POST /v1/errors/capture
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/errors/capture", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", run.RunID)
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		limited := io.LimitReader(res.Body, 1<<20)
		var envelope apiEnvelope
		decodeErr := json.NewDecoder(limited).Decode(&envelope)
		res.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("capture response: %w", decodeErr)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Second << attempt
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if !envelope.OK {
			msg := envelope.Error.Message
			if msg == "" {
				msg = envelope.Error.Hint
			}
			return &apiError{status: res.StatusCode, code: envelope.Error.Code, message: msg}
		}
		if res.StatusCode >= 400 {
			return fmt.Errorf("capture HTTP %d", res.StatusCode)
		}
		return nil
	}
	return errors.New("capture retry limit reached")
}

func pause(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
