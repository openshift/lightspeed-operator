package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func fakeStreams() (genericclioptions.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	streams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    out,
		ErrOut: errOut,
	}
	return streams, out, errOut
}

// sseEvent builds a single SSE data line in the lightspeed-service format:
//
//	data: {"event": "<eventType>", "data": <payload>}
//
// payload is JSON-marshaled. Returns the line with trailing double newline.
func sseEvent(eventType string, payload interface{}) string {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("sseEvent: failed to marshal payload: %v", err))
	}
	// Build the envelope as raw JSON to avoid double-encoding the payload.
	return fmt.Sprintf("data: {\"event\": %q, \"data\": %s}\n\n", eventType, string(payloadJSON))
}

// sseEndEvent builds an end event with optional available_quotas field.
func sseEndEvent(payload interface{}) string {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("sseEndEvent: failed to marshal payload: %v", err))
	}
	return fmt.Sprintf("data: {\"event\": \"end\", \"data\": %s, \"available_quotas\": {}}\n\n", string(payloadJSON))
}
