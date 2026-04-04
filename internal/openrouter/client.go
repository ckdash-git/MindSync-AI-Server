package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
)

// Client is the OpenRouter API client with SSE streaming support.
type Client struct {
	httpClient *http.Client
	baseURL    string
	maxRetries int
	log        *logger.Logger
}

// ClientConfig holds configuration for the OpenRouter client.
type ClientConfig struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

// NewClient creates a new OpenRouter API client.
func NewClient(cfg ClientConfig, log *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		maxRetries: cfg.MaxRetries,
		log:        log,
	}
}

// SendChat sends a non-streaming chat completion request.
func (c *Client) SendChat(ctx context.Context, apiKey string, req *ChatRequest) (*ChatResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	var resp *ChatResponse
	err = c.doWithRetry(ctx, apiKey, body, func(httpResp *http.Response) error {
		defer httpResp.Body.Close()

		if httpResp.StatusCode != http.StatusOK {
			return c.handleErrorResponse(httpResp)
		}

		resp = &ChatResponse{}
		if err := json.NewDecoder(httpResp.Body).Decode(resp); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// StreamChat sends a streaming chat completion request.
// Returns a channel of StreamEvents. The channel is closed when the stream ends.
// Caller must consume the channel to prevent goroutine leak.
func (c *Client) StreamChat(ctx context.Context, apiKey string, req *ChatRequest) (<-chan StreamEvent, <-chan error, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// No timeout for streaming — context cancellation handles it
	streamClient := &http.Client{}
	httpResp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("sending request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		return nil, nil, c.handleErrorResponse(httpResp)
	}

	eventCh := make(chan StreamEvent, 64) // buffered for backpressure
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)
		defer httpResp.Body.Close()

		scanner := bufio.NewScanner(httpResp.Body)
		// Increase scanner buffer for large chunks
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Parse SSE data
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// [DONE] signals end of stream
			if data == "[DONE]" {
				return
			}

			var event StreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				c.log.Warn("failed to parse stream event", "error", err, "data", data)
				continue
			}

			select {
			case eventCh <- event:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	return eventCh, errCh, nil
}

// doWithRetry executes an HTTP request with exponential backoff retry.
func (c *Client) doWithRetry(ctx context.Context, apiKey string, body []byte, handler func(*http.Response) error) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoffDelay(attempt)
			c.log.InfoContext(ctx, "retrying OpenRouter request",
				"attempt", attempt,
				"delay", delay,
			)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("sending request: %w", err)
			continue
		}

		// Don't retry on 4xx (client errors) except 429 (rate limit)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return handler(resp)
		}

		// Retry on 5xx and 429
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = c.handleErrorResponse(resp)
			resp.Body.Close()
			continue
		}

		return handler(resp)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// backoffDelay calculates exponential backoff with jitter.
func (c *Client) backoffDelay(attempt int) time.Duration {
	base := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
	delay := base + jitter

	// Cap at 30 seconds
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	return delay
}

// handleErrorResponse reads and maps an error response.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return MapHTTPError(resp.StatusCode, nil)
	}

	return MapHTTPError(resp.StatusCode, &errResp)
}
