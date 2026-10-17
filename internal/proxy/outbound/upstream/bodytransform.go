package upstream

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/JLugagne/claudeproxy/internal/pkg/apierror"
)

const messagesPathPrefix = "/v1/messages"

// attributionPrefix opens the system block Claude Code prepends for billing.
const attributionPrefix = "x-anthropic-billing-header:"

var (
	toolAdditionMarker = []byte(`"tool_addition"`)
	outputConfigMarker = []byte(`"output_config"`)
)

type toolAddition struct {
	Type string `json:"type"`
	Tool struct {
		Type       string          `json:"type"`
		Name       string          `json:"name"`
		Definition json.RawMessage `json:"definition"`
	} `json:"tool"`
}

func midConversationFlattener(next http.Handler) http.Handler {
	return transformRequestBody(next, flattenMidConversationUpdates)
}

func flattenMidConversationUpdates(body []byte) ([]byte, bool) {
	if !bytes.Contains(body, toolAdditionMarker) && !bytes.Contains(body, outputConfigMarker) {
		return nil, false
	}
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(req["messages"], &messages); err != nil {
		return nil, false
	}
	var tools []json.RawMessage
	if raw, ok := req["tools"]; ok {
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, false
		}
	}

	messagesChanged, toolsChanged := false, false
	kept := make([]map[string]json.RawMessage, 0, len(messages))
	for _, msg := range messages {
		if outputConfig, ok := msg["output_config"]; ok {
			req["output_config"] = outputConfig
			delete(msg, "output_config")
			messagesChanged = true
		}
		var blocks []json.RawMessage
		if err := json.Unmarshal(msg["content"], &blocks); err != nil {
			kept = append(kept, msg)
			continue
		}
		remaining := make([]json.RawMessage, 0, len(blocks))
		for _, block := range blocks {
			var addition toolAddition
			if err := json.Unmarshal(block, &addition); err != nil || addition.Type != "tool_addition" {
				remaining = append(remaining, block)
				continue
			}
			var added bool
			if tools, added = addTool(tools, addition); !added {
				remaining = append(remaining, block)
			}
		}
		if len(remaining) == len(blocks) {
			kept = append(kept, msg)
			continue
		}
		messagesChanged, toolsChanged = true, true
		if len(remaining) == 0 {
			continue
		}
		msg["content"], _ = json.Marshal(remaining)
		kept = append(kept, msg)
	}
	if !messagesChanged {
		return nil, false
	}

	req["messages"], _ = json.Marshal(kept)
	if toolsChanged {
		req["tools"], _ = json.Marshal(tools)
	}
	out, err := json.Marshal(req)
	if err != nil {
		return nil, false
	}
	return out, true
}

func addTool(tools []json.RawMessage, addition toolAddition) ([]json.RawMessage, bool) {
	switch addition.Tool.Type {
	case "tool_definition":
		var def struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(addition.Tool.Definition, &def); err != nil || def.Name == "" {
			return tools, false
		}
		if i := indexTool(tools, def.Name); i >= 0 {
			tools[i] = addition.Tool.Definition
			return tools, true
		}
		return append(tools, addition.Tool.Definition), true
	case "tool_reference":
		i := indexTool(tools, addition.Tool.Name)
		if i < 0 {
			return tools, false
		}
		var tool map[string]json.RawMessage
		if err := json.Unmarshal(tools[i], &tool); err != nil {
			return tools, false
		}
		delete(tool, "defer_loading")
		undeferred, err := json.Marshal(tool)
		if err != nil {
			return tools, false
		}
		tools[i] = undeferred
		return tools, true
	default:
		return tools, false
	}
}

func indexTool(tools []json.RawMessage, name string) int {
	for i, raw := range tools {
		var tool struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &tool) == nil && tool.Name == name {
			return i
		}
	}
	return -1
}

func transformRequestBody(next http.Handler, transform func([]byte) ([]byte, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, messagesPathPrefix) || r.Header.Get("Content-Encoding") != "" || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, apierror.TypeInvalidRequest, "claudeproxy: cannot read request body")
			return
		}
		if transformed, ok := transform(body); ok {
			body = transformed
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.TransferEncoding = nil
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		next.ServeHTTP(w, r)
	})
}

func claudeMetadataStripper(next http.Handler) http.Handler {
	return transformRequestBody(next, stripClaudeMetadata)
}

func stripClaudeMetadata(body []byte) ([]byte, bool) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false
	}
	changed := false
	if _, ok := req["metadata"]; ok {
		delete(req, "metadata")
		changed = true
	}
	if raw, ok := req["system"]; ok {
		if kept, systemChanged := stripAttributionBlocks(raw); systemChanged {
			if kept == nil {
				delete(req, "system")
			} else {
				req["system"] = kept
			}
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(req)
	if err != nil {
		return nil, false
	}
	return out, true
}

func stripAttributionBlocks(raw json.RawMessage) (json.RawMessage, bool) {
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw, false
	}
	kept := make([]json.RawMessage, 0, len(blocks))
	removed := false
	for _, block := range blocks {
		var text struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(block, &text) == nil && strings.HasPrefix(text.Text, attributionPrefix) {
			removed = true
			continue
		}
		kept = append(kept, block)
	}
	if !removed {
		return raw, false
	}
	if len(kept) == 0 {
		return nil, true
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return raw, false
	}
	return out, true
}
