package httprequest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

// defaultTimeout is the client-level deadline applied when no per-request
// timeout_seconds is configured on the node. It acts as a safety net against
// hung connections; the node's own context timeout (from timeout_seconds config)
// will typically fire first for configured workflows.
const defaultTimeout = 30 * time.Second

// Option configures the HTTP client used by the http.request Handler.
type Option func(*http.Client)

// WithTimeout overrides the default client timeout.
func WithTimeout(d time.Duration) Option {
	return func(hc *http.Client) { hc.Timeout = d }
}

// WithTransport replaces the HTTP transport. Primarily used in tests.
func WithTransport(rt http.RoundTripper) Option {
	return func(hc *http.Client) { hc.Transport = rt }
}

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["url", "method"],
  "properties": {
    "url":     { "type": "string",  "title": "URL",    "x-template": true },
    "method":  { "type": "string",  "title": "Method", "enum": ["GET","POST","PUT","PATCH","DELETE"], "default": "GET" },
    "headers": { "type": "object",  "title": "Headers", "additionalProperties": { "type": "string", "x-template": true } },
    "body":    { "type": "string",  "title": "Body",   "x-template": true },
    "timeout_seconds": { "type": "integer", "title": "Timeout (seconds)", "default": 30 }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "status_code": { "type": "integer" },
    "body":        { "type": "string" },
    "headers":     { "type": "object", "additionalProperties": { "type": "string" } }
  }
}`)

// Handler implements the http.request built-in node.
type Handler struct {
	client *http.Client
}

// New returns a new HTTPRequest handler with a default 30 s client timeout.
// Pass Option values to override timeout or transport.
func New(opts ...Option) *Handler {
	hc := &http.Client{Timeout: defaultTimeout}
	for _, o := range opts {
		o(hc)
	}
	return &Handler{client: hc}
}

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "http.request",
		DisplayName:  "HTTP Request",
		Category:     "deterministic",
		Description:  "Make an HTTP request and return the status code, headers, and body.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute makes an HTTP request using the node's config.
// URL, headers, and body support Go text/template syntax with upstream node outputs
// accessible by node ID (e.g. {{.n1.status_code}}).
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	rawURL, _ := input.Config["url"].(string)
	if rawURL == "" {
		return node.NodeOutput{}, fmt.Errorf("http.request: url is required")
	}

	method, _ := input.Config["method"].(string)
	if method == "" {
		method = "GET"
	}

	// Apply timeout override from config.
	if ts, ok := input.Config["timeout_seconds"]; ok {
		if secs, ok := toInt(ts); ok && secs > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(secs)*time.Second)
			defer cancel()
		}
	}

	// Render templates using upstream data as the context.
	urlStr, err := renderTemplate(rawURL, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("http.request: render url: %w", err)
	}

	// Build optional body.
	var bodyReader io.Reader
	if rawBody, ok := input.Config["body"].(string); ok && rawBody != "" {
		rendered, err := renderTemplate(rawBody, input.UpstreamData)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("http.request: render body: %w", err)
		}
		bodyReader = bytes.NewBufferString(rendered)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("http.request: build request: %w", err)
	}

	// Apply headers from config.
	if headersRaw, ok := input.Config["headers"]; ok {
		if headersMap, ok := headersRaw.(map[string]any); ok {
			for k, v := range headersMap {
				val, _ := v.(string)
				rendered, err := renderTemplate(val, input.UpstreamData)
				if err != nil {
					return node.NodeOutput{}, fmt.Errorf("http.request: render header %q: %w", k, err)
				}
				req.Header.Set(k, rendered)
			}
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("http.request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("http.request: read response: %w", err)
	}

	// Collect response headers as map[string]string (first value per key).
	respHeaders := make(map[string]any, len(resp.Header))
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			respHeaders[k] = vals[0]
		}
	}

	return node.NodeOutput{Data: map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
		"headers":     respHeaders,
	}}, nil
}

// renderTemplate applies Go text/template to s with data as the template context.
// No-op if s contains no template directives.
func renderTemplate(s string, data map[string]any) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(s)
	if err != nil {
		return s, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return s, err
	}
	return buf.String(), nil
}

// toInt converts numeric config values (JSON unmarshals numbers as float64).
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
