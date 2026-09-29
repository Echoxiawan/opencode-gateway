// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package protocol defines the unified intermediate representation shared by
// the three client-facing protocols (OpenAI Chat, Anthropic Messages, OpenAI
// Responses) and converts to/from each wire format.
package protocol

// UnifiedRequest is the protocol-agnostic request representation.
type UnifiedRequest struct {
	Model       string
	System      string
	Messages    []UnifiedMessage
	Tools       []Tool
	Stream      bool
	Temperature *float64
	TopP        *float64
	MaxTokens   int
	SessionHint string // x-opencode-session / session id from client, if any
}

// UnifiedMessage is one conversation turn.
type UnifiedMessage struct {
	Role    string // "user" | "assistant" | "system"
	Content []ContentBlock
}

// ContentBlock is a piece of message content.
type ContentBlock struct {
	Type string // "text" | "image" | "tool_call" | "tool_result" | "thinking"

	// Text content (also used for tool arguments).
	Text string

	// Image data.
	MediaType string
	Data      string // base64

	// Tool call fields (Type == "tool_call").
	ToolCallID string
	ToolName   string

	// Tool result fields (Type == "tool_result").
	ToolUseID string
	IsError   bool
}

// Tool is a function-calling tool definition.
type Tool struct {
	Name        string
	Description string
	Parameters  string // raw JSON schema string
}

// UnifiedStreamEvent is one streaming event from upstream.
type UnifiedStreamEvent struct {
	Type      string // "text_delta" | "reasoning_delta" | "tool_call_start" | "tool_call_delta" | "tool_call_end" | "usage" | "error" | "finish"
	Text      string // delta text
	ToolCall  *ToolCallEvent
	Usage     *Usage
	Finish    string
	Error     string
	ID        string
	CreatedAt int64
	Model     string
}

// ToolCallEvent carries tool-call streaming state.
type ToolCallEvent struct {
	Index     int
	ID        string
	Name      string
	ArgsDelta string
}

// Usage is token accounting.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
}
