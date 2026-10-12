package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client calls an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// Stream sends one streaming chat request.
// The caller must read the channel until it closes, or cancel ctx.
func (c *Client) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("API key is empty")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("model is empty")
	}
	body, err := encodeRequest(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", "pigo")

	resp, err := c.http().Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, httpStatusError(resp)
	}

	ch := make(chan Event)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parseSSE(ctx, resp.Body, ch)
	}()
	return ch, nil
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(c.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func httpStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("model API returned HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("model API returned HTTP %d: %s", resp.StatusCode, msg)
}
