package cli

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LLMRequest", func() {
	It("marshals to JSON with correct field names", func() {
		req := LLMRequest{
			Query:     "why is my pod crashing",
			Mode:      "ask",
			MediaType: "application/json",
		}
		data, err := json.Marshal(req)
		Expect(err).NotTo(HaveOccurred())

		var m map[string]interface{}
		Expect(json.Unmarshal(data, &m)).To(Succeed())
		Expect(m).To(HaveKeyWithValue("query", "why is my pod crashing"))
		Expect(m).To(HaveKeyWithValue("mode", "ask"))
		Expect(m).To(HaveKeyWithValue("media_type", "application/json"))
		Expect(m).NotTo(HaveKey("conversation_id"))
	})

	It("includes conversation_id when set", func() {
		req := LLMRequest{
			Query:          "follow up",
			Mode:           "ask",
			ConversationID: "abc-123",
			MediaType:      "application/json",
		}
		data, err := json.Marshal(req)
		Expect(err).NotTo(HaveOccurred())

		var m map[string]interface{}
		Expect(json.Unmarshal(data, &m)).To(Succeed())
		Expect(m).To(HaveKeyWithValue("conversation_id", "abc-123"))
	})
})

var _ = Describe("StartEventData", func() {
	It("unmarshals start event payload", func() {
		payload := `{"conversation_id": "81dbe98a-7805-4090-af57-8410d2b08ee6"}`

		var start StartEventData
		Expect(json.Unmarshal([]byte(payload), &start)).To(Succeed())
		Expect(start.ConversationID).To(Equal("81dbe98a-7805-4090-af57-8410d2b08ee6"))
	})
})

var _ = Describe("TokenEventData", func() {
	It("unmarshals token event payload", func() {
		payload := `{"id": 3, "token": " world"}`

		var tok TokenEventData
		Expect(json.Unmarshal([]byte(payload), &tok)).To(Succeed())
		Expect(tok.ID).To(Equal(3))
		Expect(tok.Token).To(Equal(" world"))
	})
})

var _ = Describe("EndEventData", func() {
	It("unmarshals end event payload matching real server format", func() {
		payload := `{
			"referenced_documents": [
				{"doc_url": "https://docs.example.com/page", "doc_title": "Example Page"}
			],
			"truncated": false,
			"input_tokens": 2899,
			"output_tokens": 191,
			"reasoning_tokens": 0
		}`

		var end EndEventData
		Expect(json.Unmarshal([]byte(payload), &end)).To(Succeed())
		Expect(end.ReferencedDocuments).To(HaveLen(1))
		Expect(end.ReferencedDocuments[0].DocURL).To(Equal("https://docs.example.com/page"))
		Expect(end.ReferencedDocuments[0].DocTitle).To(Equal("Example Page"))
		Expect(end.Truncated).To(BeFalse())
		Expect(end.InputTokens).To(Equal(2899))
		Expect(end.OutputTokens).To(Equal(191))
		Expect(end.ReasoningTokens).To(Equal(0))
	})

	It("handles end event with empty referenced documents", func() {
		payload := `{
			"referenced_documents": [],
			"truncated": false,
			"input_tokens": 100,
			"output_tokens": 50,
			"reasoning_tokens": 0
		}`

		var end EndEventData
		Expect(json.Unmarshal([]byte(payload), &end)).To(Succeed())
		Expect(end.ReferencedDocuments).To(BeEmpty())
	})
})

var _ = Describe("sseEnvelope", func() {
	It("unmarshals a token envelope", func() {
		raw := `{"event": "token", "data": {"id": 0, "token": "Hello"}}`

		var env sseEnvelope
		Expect(json.Unmarshal([]byte(raw), &env)).To(Succeed())
		Expect(env.Event).To(Equal("token"))
	})

	It("unmarshals an end envelope with available_quotas", func() {
		raw := `{"event": "end", "data": {"referenced_documents": [], "truncated": false, "input_tokens": 100, "output_tokens": 50, "reasoning_tokens": 0}, "available_quotas": {}}`

		var env sseEnvelope
		Expect(json.Unmarshal([]byte(raw), &env)).To(Succeed())
		Expect(env.Event).To(Equal("end"))
	})
})
