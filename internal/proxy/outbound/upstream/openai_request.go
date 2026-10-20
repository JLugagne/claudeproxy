package upstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// errInvalidMessagesRequest is returned when a Messages API body cannot be
// converted to a chat-completions request.
var errInvalidMessagesRequest = errors.New("invalid Anthropic Messages API request")

type anthropicRequest struct {
	Model         string           `json:"model"`
	MaxTokens     *int             `json:"max_tokens"`
	Temperature   *float64         `json:"temperature"`
	TopP          *float64         `json:"top_p"`
	StopSequences []string         `json:"stop_sequences"`
	Stream        bool             `json:"stream"`
	System        json.RawMessage  `json:"system"`
	Messages      []anthropicMsg   `json:"messages"`
	Tools         []anthropicTool  `json:"tools"`
	ToolChoice    *anthropicChoice `json:"tool_choice"`
}

type anthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	Source    struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source"`
}

type openAIRequest struct {
	Model         string               `json:"model"`
	Messages      []openAIMessage      `json:"messages"`
	MaxTokens     *int                 `json:"max_tokens,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	Stop          []string             `json:"stop,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
	Tools         []openAITool         `json:"tools,omitempty"`
	ToolChoice    any                  `json:"tool_choice,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openAIPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// messagesToChatRequest converts a Messages API request body into a
// chat-completions request body. Fields chat completions has no equivalent
// for (the top-level thinking parameter, metadata, cache control, server
// tools...) are dropped, but thinking content blocks are converted.
func messagesToChatRequest(body []byte) ([]byte, bool, error) {
	var in anthropicRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, false, errors.Join(errInvalidMessagesRequest, err)
	}
	out := openAIRequest{
		Model:       in.Model,
		MaxTokens:   in.MaxTokens,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stop:        in.StopSequences,
		Stream:      in.Stream,
	}
	if in.Stream {
		out.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	// Chat-completions providers only accept system messages at the start, but
	// Claude Code sends the environment and skills as system-role entries that
	// follow the user turn. They are hoisted into the leading system message.
	var system []string
	if text := systemText(in.System); text != "" {
		system = append(system, text)
	}
	var conversation []openAIMessage
	for i, msg := range in.Messages {
		if msg.Role == "system" {
			if text := systemText(msg.Content); text != "" {
				system = append(system, text)
			}
			continue
		}
		converted, err := convertMessage(msg)
		if err != nil {
			return nil, false, errors.Join(errInvalidMessagesRequest, fmt.Errorf("messages[%d]", i), err)
		}
		conversation = append(conversation, converted...)
	}
	if len(system) > 0 {
		out.Messages = append(out.Messages, openAIMessage{Role: "system", Content: strings.Join(system, "\n\n")})
	}
	out.Messages = append(out.Messages, conversation...)
	for _, tool := range in.Tools {
		if tool.Name == "" || len(tool.InputSchema) == 0 {
			continue
		}
		out.Tools = append(out.Tools, openAITool{Type: "function", Function: openAIFunction{
			Name: tool.Name, Description: tool.Description, Parameters: stripSchemaPatterns(tool.InputSchema),
		}})
	}
	if len(out.Tools) > 0 {
		out.ToolChoice = convertToolChoice(in.ToolChoice)
	}
	encoded, err := json.Marshal(out)
	return encoded, in.Stream, err
}

// systemText flattens the system prompt, a string or an array of text blocks.
func systemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []anthropicBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func convertToolChoice(choice *anthropicChoice) any {
	if choice == nil {
		return nil
	}
	switch choice.Type {
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		return map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}}
	default:
		return "auto"
	}
}

func convertMessage(msg anthropicMsg) ([]openAIMessage, error) {
	var text string
	if json.Unmarshal(msg.Content, &text) == nil {
		return []openAIMessage{{Role: msg.Role, Content: text}}, nil
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, err
	}
	if msg.Role == "assistant" {
		return []openAIMessage{convertAssistantBlocks(blocks)}, nil
	}
	return convertUserBlocks(msg.Role, blocks), nil
}

// convertAssistantBlocks folds an assistant turn into one message: text blocks
// are joined and tool_use blocks become tool calls. Thinking blocks become the
// reasoning_content DeepSeek's thinking mode requires to be passed back.
func convertAssistantBlocks(blocks []anthropicBlock) openAIMessage {
	out := openAIMessage{Role: "assistant"}
	var texts, thinking []string
	for _, block := range blocks {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				thinking = append(thinking, block.Thinking)
			}
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, openAIToolCall{
				ID: block.ID, Type: "function",
				Function: openAIFunctionCall{Name: block.Name, Arguments: compactJSON(block.Input)},
			})
		}
	}
	if len(texts) > 0 {
		out.Content = strings.Join(texts, "\n")
	}
	if len(thinking) > 0 {
		out.ReasoningContent = strings.Join(thinking, "\n")
	}
	return out
}

// convertUserBlocks turns tool_result blocks into tool messages, which must
// directly follow the assistant tool calls, then keeps the remaining text and
// images as one user message.
func convertUserBlocks(role string, blocks []anthropicBlock) []openAIMessage {
	var out []openAIMessage
	var parts []openAIPart
	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			out = append(out, openAIMessage{Role: "tool", ToolCallID: block.ToolUseID, Content: toolResultText(block.Content)})
		case "text":
			parts = append(parts, openAIPart{Type: "text", Text: block.Text})
		case "image":
			if url := imageURL(block); url != "" {
				parts = append(parts, openAIPart{Type: "image_url", ImageURL: &openAIImageURL{URL: url}})
			}
		}
	}
	switch {
	case len(parts) == 0:
	case len(parts) == 1 && parts[0].Type == "text":
		out = append(out, openAIMessage{Role: role, Content: parts[0].Text})
	default:
		out = append(out, openAIMessage{Role: role, Content: parts})
	}
	return out
}

func imageURL(block anthropicBlock) string {
	switch block.Source.Type {
	case "base64":
		return "data:" + block.Source.MediaType + ";base64," + block.Source.Data
	case "url":
		return block.Source.URL
	}
	return ""
}

// toolResultText flattens a tool_result content, a string or an array of text
// blocks.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []anthropicBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw)
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// stripSchemaPatterns removes the "pattern" keyword from a tool's JSON schema.
// Chat-completions providers validate schemas strictly and refuse expressions
// they cannot compile (Claude Code's own tools carry "^[^\0]*$"), sometimes
// depending on the backend that serves the request. The keyword is only a hint
// to the model: the tool validates its own input. A property named "pattern"
// is untouched, because only string values of the keyword are removed. The
// schema is returned as received when it holds no pattern.
func stripSchemaPatterns(schema json.RawMessage) json.RawMessage {
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(schema))
	decoder.UseNumber()
	if decoder.Decode(&tree) != nil {
		return schema
	}
	if !dropPatterns(tree) {
		return schema
	}
	stripped, err := json.Marshal(tree)
	if err != nil {
		return schema
	}
	return stripped
}

func dropPatterns(node any) bool {
	changed := false
	switch value := node.(type) {
	case map[string]any:
		if _, isString := value["pattern"].(string); isString {
			delete(value, "pattern")
			changed = true
		}
		for _, child := range value {
			if dropPatterns(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range value {
			if dropPatterns(child) {
				changed = true
			}
		}
	}
	return changed
}
