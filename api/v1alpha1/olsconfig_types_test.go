/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"encoding/json"
	"testing"
)

func TestAuditConfig_JSONRoundTrip(t *testing.T) {
	in := AuditConfig{
		Logging:         boolPtr(false),
		TracingEndpoint: "jaeger:4317",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AuditConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TracingEndpoint != in.TracingEndpoint {
		t.Errorf("TracingEndpoint: got %q, want %q", out.TracingEndpoint, in.TracingEndpoint)
	}
	if out.Logging == nil || *out.Logging != false {
		t.Errorf("Logging: got %v, want false", out.Logging)
	}
}

func TestOLSConfigSpec_ZeroAudit_EmptyObject(t *testing.T) {
	spec := OLSConfigSpec{
		LLMConfig: LLMSpec{Providers: []ProviderSpec{{Name: "openai", Type: "openai"}}},
		OLSConfig: OLSSpec{DefaultModel: "gpt-4", DefaultProvider: "openai"},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	audit, ok := raw["audit"]
	if !ok {
		t.Fatal("expected audit key in JSON")
	}
	if string(audit) != "{}" {
		t.Errorf("expected empty audit object {}, got %s", audit)
	}
}

func TestOLSSpec_AuditEventsEnabled_JSONRoundTrip(t *testing.T) {
	in := OLSSpec{
		DefaultModel:       "gpt-4",
		DefaultProvider:    "openai",
		AuditEventsEnabled: boolPtr(true),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected non-empty JSON")
	}
	var out OLSSpec
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AuditEventsEnabled == nil || !*out.AuditEventsEnabled {
		t.Errorf("AuditEventsEnabled: got %v, want true", out.AuditEventsEnabled)
	}
}

func TestAgenticOLSSpec_JSONRoundTrip(t *testing.T) {
	in := AgenticOLSSpec{
		SandboxMode: SandboxModeSandboxClaim,
		AgenticSandboxConfig: Config{
			NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
		},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AgenticOLSSpec
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SandboxMode != SandboxModeSandboxClaim {
		t.Errorf("SandboxMode: got %q, want %q", out.SandboxMode, SandboxModeSandboxClaim)
	}
	if out.AgenticSandboxConfig.NodeSelector["node-role.kubernetes.io/worker"] != "" {
		t.Errorf("AgenticSandboxConfig.NodeSelector: got %v", out.AgenticSandboxConfig.NodeSelector)
	}
}

func TestOLSConfigSpec_AgenticOLS_Omitempty(t *testing.T) {
	spec := OLSConfigSpec{
		LLMConfig: LLMSpec{Providers: []ProviderSpec{{Name: "openai", Type: "openai"}}},
		OLSConfig: OLSSpec{DefaultModel: "gpt-4", DefaultProvider: "openai"},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["agenticOLS"]; ok {
		t.Errorf("expected agenticOLS omitted when nil, got %s", raw["agenticOLS"])
	}
}

func boolPtr(v bool) *bool {
	return &v
}
