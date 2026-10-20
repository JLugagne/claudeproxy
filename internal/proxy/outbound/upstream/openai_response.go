package upstream

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/JLugagne/claudeproxy/internal/pkg/apierror"
)

// reasoningSignature is put on thinking blocks built from the provider's
// reasoning content. The provider does not check it: Claude Code only echoes it
// back, which is enough to round-trip the reasoning as reasoning_content.
const reasoningSignature = "claudeproxy"

type chatUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type chatToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning_content"`
			ToolCalls        []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

type messagesUsage struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheReadInputTokens int64 `json:"cache_read_input_tokens,omitempty"`
}

type messagesResponse struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	Model        string        `json:"model"`
	Content      []any         `json:"content"`
	StopReason   string        `json:"stop_reason"`
	StopSequence *string       `json:"stop_sequence"`
	Usage        messagesUsage `json:"usage"`
}

// toMessagesUsage converts chat-completions usage. Cached prompt tokens are
// reported separately, as the Messages API does.
func (u *chatUsage) toMessagesUsage() messagesUsage {
	if u == nil {
		return messagesUsage{}
	}
	var cached int64
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return messagesUsage{
		InputTokens:          max(u.PromptTokens-cached, 0),
		OutputTokens:         u.CompletionTokens,
		CacheReadInputTokens: cached,
	}
}

func mapFinishReason(reason string, hasToolCalls bool) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "stop", "content_filter":
		return "end_turn"
	}
	if hasToolCalls {
		return "tool_use"
	}
	return "end_turn"
}

func messageID(chatID string) string {
	if chatID == "" {
		return "msg_claudeproxy"
	}
	return "msg_" + strings.TrimPrefix(chatID, "chatcmpl-")
}

// chatToMessagesResponse converts a non-streaming chat-completions response
// into a Messages API message. Reasoning content becomes a leading thinking
// block carrying reasoningSignature.
func chatToMessagesResponse(body []byte) ([]byte, error) {
	var in chatResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	out := messagesResponse{
		ID:         messageID(in.ID),
		Type:       "message",
		Role:       "assistant",
		Model:      in.Model,
		Content:    []any{},
		StopReason: "end_turn",
		Usage:      in.Usage.toMessagesUsage(),
	}
	if len(in.Choices) > 0 {
		choice := in.Choices[0]
		if choice.Message.ReasoningContent != "" {
			out.Content = append(out.Content, map[string]any{
				"type": "thinking", "thinking": choice.Message.ReasoningContent, "signature": reasoningSignature,
			})
		}
		if choice.Message.Content != "" {
			out.Content = append(out.Content, map[string]any{"type": "text", "text": choice.Message.Content})
		}
		for _, call := range choice.Message.ToolCalls {
			out.Content = append(out.Content, map[string]any{
				"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": toolInput(call.Function.Arguments),
			})
		}
		out.StopReason = mapFinishReason(choice.FinishReason, len(choice.Message.ToolCalls) > 0)
	}
	if len(out.Content) == 0 {
		out.Content = append(out.Content, map[string]any{"type": "text", "text": ""})
	}
	return json.Marshal(out)
}

// toolInput parses the JSON arguments string of a tool call. Invalid or empty
// arguments become an empty object so the client still receives a valid input.
func toolInput(arguments string) json.RawMessage {
	if json.Valid([]byte(arguments)) && strings.TrimSpace(arguments) != "" {
		return json.RawMessage(arguments)
	}
	return json.RawMessage("{}")
}

// chatErrorToMessagesError rewrites a chat-completions error body in the
// Anthropic error envelope, keeping the provider's message.
func chatErrorToMessagesError(status int, body []byte) []byte {
	var in struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	message := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &in) == nil {
		switch {
		case in.Error.Message != "":
			message = in.Error.Message
		case in.Message != "":
			message = in.Message
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": errorTypeForStatus(status), "message": message},
	})
	return payload
}

func errorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return apierror.TypeInvalidRequest
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return apierror.TypeNotFound
	case http.StatusRequestEntityTooLarge:
		return apierror.TypeRequestTooLarge
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	}
	return apierror.TypeAPI
}
