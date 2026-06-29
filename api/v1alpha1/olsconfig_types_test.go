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
	"testing"
)

func TestAuditConfig_LoggingEnabled_DefaultTrue(t *testing.T) {
	var config *AuditConfig
	if !config.LoggingEnabled() {
		t.Error("Expected LoggingEnabled to default to true when config is nil")
	}
}

func TestAuditConfig_LoggingEnabled_EmptyField(t *testing.T) {
	config := &AuditConfig{}
	if !config.LoggingEnabled() {
		t.Error("Expected LoggingEnabled to return true when Logging field is empty")
	}
}

func TestAuditConfig_LoggingEnabled_ExplicitEnabled(t *testing.T) {
	config := &AuditConfig{Logging: AuditLoggingEnabled}
	if !config.LoggingEnabled() {
		t.Error("Expected LoggingEnabled to return true when Logging=Enabled")
	}
}

func TestAuditConfig_LoggingEnabled_ExplicitDisabled(t *testing.T) {
	config := &AuditConfig{Logging: AuditLoggingDisabled}
	if config.LoggingEnabled() {
		t.Error("Expected LoggingEnabled to return false when Logging=Disabled")
	}
}

func TestAuditConfig_OTELEndpoint_EmptyWhenZero(t *testing.T) {
	config := &AuditConfig{}
	if endpoint := config.OTELEndpoint(); endpoint != "" {
		t.Errorf("Expected empty endpoint, got %s", endpoint)
	}
}

func TestAuditConfig_OTELEndpoint_NilConfig(t *testing.T) {
	var config *AuditConfig
	if endpoint := config.OTELEndpoint(); endpoint != "" {
		t.Errorf("Expected empty endpoint for nil config, got %s", endpoint)
	}
}

func TestAuditConfig_OTELEndpoint_ReturnsValue(t *testing.T) {
	config := &AuditConfig{
		OTEL: &AuditOTELConfig{Endpoint: "jaeger:4317"},
	}
	if endpoint := config.OTELEndpoint(); endpoint != "jaeger:4317" {
		t.Errorf("Expected 'jaeger:4317', got %s", endpoint)
	}
}

func TestAuditConfig_OTELInsecure_DefaultFalse(t *testing.T) {
	config := &AuditConfig{}
	if config.OTELInsecure() {
		t.Error("Expected OTELInsecure to return false when OTEL is zero")
	}
}

func TestAuditConfig_OTELInsecure_NilConfig(t *testing.T) {
	var config *AuditConfig
	if config.OTELInsecure() {
		t.Error("Expected OTELInsecure to return false for nil config")
	}
}

func TestAuditConfig_OTELInsecure_ReturnsValue(t *testing.T) {
	config := &AuditConfig{
		OTEL: &AuditOTELConfig{Endpoint: "jaeger:4317", TLSMode: AuditOTELTLSInsecure},
	}
	if !config.OTELInsecure() {
		t.Error("Expected OTELInsecure to return true")
	}
}

func TestAuditConfig_OTELInsecure_SecureMode(t *testing.T) {
	config := &AuditConfig{
		OTEL: &AuditOTELConfig{Endpoint: "jaeger:4317", TLSMode: AuditOTELTLSSecure},
	}
	if config.OTELInsecure() {
		t.Error("Expected OTELInsecure to return false for Secure mode")
	}
}
