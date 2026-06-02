// Package llm provides the built-in LLM Call node for cogniflow.
// A single Handler implementation is shared by both llm.openai and llm.anthropic
// node types; the concrete aiprovider.LLMClient is injected at construction time.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
)

var openAIInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["prompt"],
  "properties": {
    "api_key":     { "type": "string",  "title": "API Key",        "x-sensitive": true },
    "model":       { "type": "string",  "title": "Model",          "default": "gpt-4o" },
    "system_msg":  { "type": "string",  "title": "System Message", "x-template": true },
    "prompt":      { "type": "string",  "title": "Prompt",         "x-template": true },
    "max_tokens":  { "type": "integer", "title": "Max Tokens",     "default": 1024 },
    "temperature": { "type": "number",  "title": "Temperature",    "default": 0.7 }
  }
}`)

var anthropicInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["prompt"],
  "properties": {
    "api_key":     { "type": "string",  "title": "API Key",        "x-sensitive": true },
    "model":       { "type": "string",  "title": "Model",          "default": "claude-sonnet-4-6" },
    "system_msg":  { "type": "string",  "title": "System Message", "x-template": true },
    "prompt":      { "type": "string",  "title": "Prompt",         "x-template": true },
    "max_tokens":  { "type": "integer", "title": "Max Tokens",     "default": 1024 },
    "temperature": { "type": "number",  "title": "Temperature",    "default": 0.7 }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "completion":        { "type": "string" },
    "prompt_tokens":     { "type": "integer" },
    "completion_tokens": { "type": "integer" }
  }
}`)

// Handler is the NodeHandler for LLM Call nodes.
type Handler struct {
	typeID      string
	displayName string
	description string
	inputSchema json.RawMessage
	client      aiprovider.LLMClient
}

// NewOpenAI returns a Handler registered as "llm.openai".
func NewOpenAI(client aiprovider.LLMClient) *Handler {
	return &Handler{
		typeID:      "llm.openai",
		displayName: "LLM Call (OpenAI)",
		description: "Send a prompt to an OpenAI chat model and receive a completion.",
		inputSchema: openAIInputSchema,
		client:      client,
	}
}

// NewAnthropic returns a Handler registered as "llm.anthropic".
func NewAnthropic(client aiprovider.LLMClient) *Handler {
	return &Handler{
		typeID:      "llm.anthropic",
		displayName: "LLM Call (Anthropic)",
		description: "Send a prompt to an Anthropic model and receive a completion.",
		inputSchema: anthropicInputSchema,
		client:      client,
	}
}

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       h.typeID,
		DisplayName:  h.displayName,
		Category:     "ai",
		Description:  h.description,
		InputSchema:  h.inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute renders template fields, then calls the configured LLM provider.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	apiKey, _ := input.Config["api_key"].(string)
	model, _ := input.Config["model"].(string)

	rawPrompt, _ := input.Config["prompt"].(string)
	if rawPrompt == "" {
		return node.NodeOutput{}, fmt.Errorf("%s: prompt is required", h.typeID)
	}
	prompt, err := renderTemplate(rawPrompt, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("%s: render prompt: %w", h.typeID, err)
	}

	rawSystem, _ := input.Config["system_msg"].(string)
	systemMsg, err := renderTemplate(rawSystem, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("%s: render system_msg: %w", h.typeID, err)
	}

	maxTokens, _ := toInt(input.Config["max_tokens"])

	// Use a pointer so that explicitly-set zero (greedy sampling) is sent to
	// the provider, while an unconfigured field produces nil (omit from request).
	var temperature *float64
	if t, ok := toFloat(input.Config["temperature"]); ok {
		temperature = &t
	}

	resp, err := h.client.Complete(ctx, aiprovider.LLMRequest{
		APIKey:      apiKey,
		Model:       model,
		SystemMsg:   systemMsg,
		Prompt:      prompt,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("%s: %w", h.typeID, err)
	}

	return node.NodeOutput{Data: map[string]any{
		"completion":        resp.Completion,
		"prompt_tokens":     resp.PromptTokens,
		"completion_tokens": resp.CompletionTokens,
	}}, nil
}

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

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
