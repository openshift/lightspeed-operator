package cli

import "encoding/json"

// LLMRequest represents the request payload sent to the lightspeed-service
// /v1/streaming_query endpoint.
type LLMRequest struct {
	Query          string `json:"query"`
	Mode           string `json:"mode"`
	ConversationID string `json:"conversation_id,omitempty"`
	MediaType      string `json:"media_type"`
}

// ReferencedDocument represents a document cited in the LLM response.
type ReferencedDocument struct {
	DocURL   string `json:"doc_url"`
	DocTitle string `json:"doc_title"`
}

// StartEventData represents the parsed JSON payload of an SSE "start" event.
type StartEventData struct {
	ConversationID string `json:"conversation_id"`
}

// TokenEventData represents the parsed JSON payload of an SSE "token" event.
type TokenEventData struct {
	ID    int    `json:"id"`
	Token string `json:"token"`
}

// EndEventData represents the parsed JSON payload of an SSE "end" event.
type EndEventData struct {
	ReferencedDocuments []ReferencedDocument `json:"referenced_documents"`
	Truncated           bool                 `json:"truncated"`
	InputTokens         int                  `json:"input_tokens"`
	OutputTokens        int                  `json:"output_tokens"`
	ReasoningTokens     int                  `json:"reasoning_tokens"`
}

// sseEnvelope is the JSON structure sent by lightspeed-service inside
// each SSE data line: {"event": "<type>", "data": <payload>}
// An optional top-level "available_quotas" field is ignored.
// Data uses json.RawMessage to preserve the original JSON without
// float64 conversion of numbers.
type sseEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}
