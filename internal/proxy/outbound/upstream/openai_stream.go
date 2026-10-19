package upstream

import (
	"bytes"
	"encoding/json"
)

type chatChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string         `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// streamTranslator converts a chat-completions SSE stream into the Messages
// API event stream. Reasoning deltas are dropped. The closing message_delta is
// held back until the stream ends because chat completions report usage in a
// trailing chunk.
type streamTranslator struct {
	emit       func(event string, payload map[string]any)
	buf        []byte
	started    bool
	finished   bool
	id, model  string
	nextBlock  int
	open       int
	openKind   string
	toolBlocks map[int]int
	hasTool    bool
	stopReason string
	usage      *chatUsage
}

func newStreamTranslator(emit func(event string, payload map[string]any)) *streamTranslator {
	return &streamTranslator{emit: emit, open: -1, toolBlocks: map[int]int{}}
}

// write feeds raw upstream bytes; complete SSE data lines are translated.
func (s *streamTranslator) write(p []byte) {
	s.buf = append(s.buf, p...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			return
		}
		line := bytes.TrimSpace(s.buf[:i])
		s.buf = s.buf[i+1:]
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			s.handleData(bytes.TrimSpace(data))
		}
	}
}

func (s *streamTranslator) handleData(data []byte) {
	if s.finished {
		return
	}
	if string(data) == "[DONE]" {
		s.finish()
		return
	}
	var chunk chatChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return
	}
	if chunk.Error != nil {
		s.emit("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": chunk.Error.Message}})
		s.finished = true
		return
	}
	if chunk.ID != "" && s.id == "" {
		s.id = chunk.ID
	}
	if chunk.Model != "" && s.model == "" {
		s.model = chunk.Model
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}
	s.start()
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.text(choice.Delta.Content)
		}
		for _, call := range choice.Delta.ToolCalls {
			s.toolCall(call)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			s.stopReason = *choice.FinishReason
		}
	}
}

func (s *streamTranslator) start() {
	if s.started {
		return
	}
	s.started = true
	s.emit("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": messageID(s.id), "type": "message", "role": "assistant", "model": s.model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
	}})
}

func (s *streamTranslator) closeBlock() {
	if s.open < 0 {
		return
	}
	s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": s.open})
	s.open, s.openKind = -1, ""
}

func (s *streamTranslator) text(text string) {
	if s.open < 0 || s.openKind != "text" {
		s.closeBlock()
		s.open, s.openKind = s.nextBlock, "text"
		s.nextBlock++
		s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": s.open, "content_block": map[string]any{"type": "text", "text": ""}})
	}
	s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": s.open, "delta": map[string]any{"type": "text_delta", "text": text}})
}

func (s *streamTranslator) toolCall(call chatToolCall) {
	block, ok := s.toolBlocks[call.Index]
	if !ok {
		s.closeBlock()
		block = s.nextBlock
		s.nextBlock++
		s.toolBlocks[call.Index] = block
		s.open, s.openKind = block, "tool"
		s.hasTool = true
		s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": block, "content_block": map[string]any{
			"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": map[string]any{},
		}})
	}
	if call.Function.Arguments != "" {
		s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": block, "delta": map[string]any{
			"type": "input_json_delta", "partial_json": call.Function.Arguments,
		}})
	}
}

// finish closes the open block and ends the message. It is idempotent so both
// [DONE] and the end of the upstream body can call it.
func (s *streamTranslator) finish() {
	if s.finished {
		return
	}
	s.finished = true
	s.start()
	if s.nextBlock == 0 {
		s.text("")
	}
	s.closeBlock()
	usage := s.usage.toMessagesUsage()
	s.emit("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": mapFinishReason(s.stopReason, s.hasTool), "stop_sequence": nil},
		"usage": usage,
	})
	s.emit("message_stop", map[string]any{"type": "message_stop"})
}
