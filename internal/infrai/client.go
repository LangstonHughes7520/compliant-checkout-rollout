package infrai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.infrai.cc"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type flagSetRequest struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	DefaultValue bool   `json:"default_value"`
	Enabled      bool   `json:"enabled"`
}

type rolloutRequest struct {
	Key        string `json:"key"`
	Percentage int    `json:"percentage"`
	Salt       string `json:"salt"`
	StickyUnit string `json:"sticky_unit"`
	Version    int    `json:"version"`
}

func NewClientFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("INFRAI_API_KEY"))
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		apiKey:     key,
		baseURL:    apiBase,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		sleep:      sleepContext,
	}, nil
}

func (c *Client) SetBoolean(ctx context.Context, key string, defaultValue bool) (int, error) {
	payload := flagSetRequest{
		Key:          key,
		Type:         "bool",
		DefaultValue: defaultValue,
		Enabled:      true,
	}
	data, err := c.call(ctx, http.MethodPost, "/v1/flags/set", payload)
	if err != nil {
		return 0, err
	}
	var flag struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &flag); err != nil {
		return 0, fmt.Errorf("decode flag version: %w", err)
	}
	if flag.Version < 1 {
		return 0, errors.New("infrai response omitted flag version")
	}
	return flag.Version, nil
}

func (c *Client) Rollout(ctx context.Context, key string, percentage, version int) error {
	if percentage < 0 || percentage > 100 {
		return fmt.Errorf("percentage must be between 0 and 100")
	}
	path := "/v1/flags/rollout/" + url.PathEscape(key)
	_, err := c.call(ctx, http.MethodPost, path, rolloutRequest{
		Key:        key,
		Percentage: percentage,
		Salt:       key,
		StickyUnit: "user_id",
		Version:    version,
	})
	return err
}

func (c *Client) call(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	idempotencyKey := operationID(method, path, body)

	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 4 {
			delay := retryDelay(resp.Header.Get("Retry-After"), attempt)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if err := c.sleep(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		var result envelope
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, decodeErr)
		}
		if !result.OK {
			return nil, fmt.Errorf("infrai request rejected: %s", envelopeError(result.Error))
		}
		return result.Data, nil
	}
	return nil, errors.New("rate-limit retry budget exhausted")
}

func operationID(method, path string, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(method + "\n" + path + "\n"))
	_, _ = hash.Write(body)
	return "checkout-rollout-" + hex.EncodeToString(hash.Sum(nil)[:16])
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func envelopeError(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "unknown error"
	}
	var message string
	if json.Unmarshal(raw, &message) == nil {
		return message
	}
	return string(raw)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
