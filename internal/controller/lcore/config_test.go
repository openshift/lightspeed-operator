package lcore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockLogger implements a simple logger for tests
type mockLogger struct {
	errorMessages []string
}

func (m *mockLogger) Init(info logr.RuntimeInfo)                       {}
func (m *mockLogger) Enabled(level int) bool                           { return true }
func (m *mockLogger) Info(level int, msg string, keysAndValues ...any) {}
func (m *mockLogger) WithValues(keysAndValues ...any) logr.LogSink     { return m }
func (m *mockLogger) WithName(name string) logr.LogSink                { return m }
func (m *mockLogger) WithCallDepth(depth int) logr.LogSink             { return m }
func (m *mockLogger) Error(err error, msg string, keysAndValues ...any) {
	fullMsg := fmt.Sprintf("%s: %v", msg, err)
	if len(keysAndValues) > 0 {
		fullMsg += fmt.Sprintf(" %v", keysAndValues)
	}
	m.errorMessages = append(m.errorMessages, fullMsg)
}

// mockReconcilerWithLogger extends mockReconciler with logger support
type mockReconcilerWithLogger struct {
	*mockReconciler
	logger logr.Logger
}

func (m *mockReconcilerWithLogger) GetLogger() logr.Logger {
	if m.logger.GetSink() == nil {
		mockSink := &mockLogger{}
		m.logger = logr.New(mockSink)
	}
	return m.logger
}

func TestFilterHTTPMCPServers(t *testing.T) {
	// Reset the warning cache before each test
	ResetMCPWarningCache()

	tests := []struct {
		name             string
		servers          []olsv1alpha1.MCPServer
		expectedFiltered int
		expectedWarnings int
		generation       int64
	}{
		{
			name:             "Empty slice",
			servers:          []olsv1alpha1.MCPServer{},
			expectedFiltered: 0,
			expectedWarnings: 0,
			generation:       1,
		},
		{
			name: "All HTTP servers",
			servers: []olsv1alpha1.MCPServer{
				{
					Name: "server1",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://example.com",
					},
				},
				{
					Name: "server2",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "https://example.com",
					},
				},
			},
			expectedFiltered: 2,
			expectedWarnings: 0,
			generation:       1,
		},
		{
			name: "Mixed servers - only HTTP pass through",
			servers: []olsv1alpha1.MCPServer{
				{
					Name: "http-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://example.com",
					},
				},
				{
					Name: "no-transport-server",
					// No transport specified
				},
			},
			expectedFiltered: 1,
			expectedWarnings: 1,
			generation:       1,
		},
		{
			name: "All non-HTTP servers",
			servers: []olsv1alpha1.MCPServer{
				{
					Name: "server1",
					// No transport specified
				},
				{
					Name: "server2",
					// No transport specified
				},
			},
			expectedFiltered: 0,
			expectedWarnings: 2,
			generation:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cache for each test case
			ResetMCPWarningCache()

			// Create mock reconciler with logger
			mockSink := &mockLogger{errorMessages: []string{}}
			mockReconciler := &mockReconcilerWithLogger{
				mockReconciler: &mockReconciler{},
				logger:         logr.New(mockSink),
			}

			cr := &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cluster",
					Generation: tt.generation,
				},
			}

			// First call - should log warnings
			filtered := FilterHTTPMCPServers(mockReconciler, cr, tt.servers)

			if len(filtered) != tt.expectedFiltered {
				t.Errorf("Expected %d filtered servers, got %d", tt.expectedFiltered, len(filtered))
			}

			if len(mockSink.errorMessages) != tt.expectedWarnings {
				t.Errorf("Expected %d warnings, got %d: %v",
					tt.expectedWarnings,
					len(mockSink.errorMessages),
					mockSink.errorMessages)
			}

			// Verify all filtered servers have StreamableHTTP
			for _, server := range filtered {
				if server.StreamableHTTP == nil {
					t.Errorf("Filtered server '%s' does not have StreamableHTTP transport", server.Name)
				}
			}

			// Second call with same generation - should NOT log warnings (cached)
			mockSink.errorMessages = []string{}
			filtered2 := FilterHTTPMCPServers(mockReconciler, cr, tt.servers)

			if len(filtered2) != tt.expectedFiltered {
				t.Errorf("Second call: Expected %d filtered servers, got %d", tt.expectedFiltered, len(filtered2))
			}

			if len(mockSink.errorMessages) != 0 {
				t.Errorf("Second call should not log warnings (cached), but got %d warnings: %v",
					len(mockSink.errorMessages),
					mockSink.errorMessages)
			}
		})
	}
}

func TestFilterHTTPMCPServers_GenerationChange(t *testing.T) {
	ResetMCPWarningCache()

	servers := []olsv1alpha1.MCPServer{
		{
			Name: "no-transport",
		},
	}

	mockSink := &mockLogger{errorMessages: []string{}}
	mockReconciler := &mockReconcilerWithLogger{
		mockReconciler: &mockReconciler{},
		logger:         logr.New(mockSink),
	}

	// First call with generation 1
	cr1 := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster",
			Generation: 1,
		},
	}
	FilterHTTPMCPServers(mockReconciler, cr1, servers)

	if len(mockSink.errorMessages) != 1 {
		t.Errorf("Expected 1 warning for generation 1, got %d", len(mockSink.errorMessages))
	}

	// Second call with same generation - should NOT log
	mockSink.errorMessages = []string{}
	FilterHTTPMCPServers(mockReconciler, cr1, servers)

	if len(mockSink.errorMessages) != 0 {
		t.Errorf("Expected 0 warnings for cached generation 1, got %d", len(mockSink.errorMessages))
	}

	// Third call with NEW generation - SHOULD log again
	mockSink.errorMessages = []string{}
	cr2 := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster",
			Generation: 2,
		},
	}
	FilterHTTPMCPServers(mockReconciler, cr2, servers)

	if len(mockSink.errorMessages) != 1 {
		t.Errorf("Expected 1 warning for new generation 2, got %d", len(mockSink.errorMessages))
	}
}

func TestBuildLCoreMCPServersConfig_NoServers(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 0 {
		t.Errorf("Expected no MCP servers, got %d", len(result))
	}
}

func TestBuildLCoreMCPServersConfig_IntrospectionOnly(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: true,
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server (OpenShift), got %d", len(result))
	}

	// Verify OpenShift MCP server configuration
	openshiftServer := result[0]
	if openshiftServer["name"] != "openshift" {
		t.Errorf("Expected server name 'openshift', got '%v'", openshiftServer["name"])
	}

	expectedURL := fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)
	if openshiftServer["url"] != expectedURL {
		t.Errorf("Expected URL '%s', got '%v'", expectedURL, openshiftServer["url"])
	}

	// Verify authorization headers
	headers, ok := openshiftServer["authorization_headers"].(map[string]string)
	if !ok {
		t.Fatal("Expected authorization_headers to be map[string]string")
	}

	if headers[utils.K8S_AUTH_HEADER] != utils.KUBERNETES_PLACEHOLDER {
		t.Errorf("Expected K8s auth header value '%s', got '%s'",
			utils.KUBERNETES_PLACEHOLDER, headers[utils.K8S_AUTH_HEADER])
	}
}

func TestBuildLCoreMCPServersConfig_UserDefinedServers_KubernetesPlaceholder(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "external-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:     "https://external.example.com/mcp",
						Timeout: 30,
						Headers: map[string]string{
							"Authorization": utils.KUBERNETES_PLACEHOLDER,
						},
					},
				},
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(result))
	}

	server := result[0]

	// Verify basic config
	if server["name"] != "external-server" {
		t.Errorf("Expected server name 'external-server', got '%v'", server["name"])
	}
	if server["url"] != "https://external.example.com/mcp" {
		t.Errorf("Expected URL 'https://external.example.com/mcp', got '%v'", server["url"])
	}
	if server["timeout"] != 30 {
		t.Errorf("Expected timeout 30, got %v", server["timeout"])
	}

	// Verify authorization headers
	headers, ok := server["authorization_headers"].(map[string]string)
	if !ok {
		t.Fatal("Expected authorization_headers to be map[string]string")
	}

	// Check Kubernetes placeholder is preserved
	if headers["Authorization"] != utils.KUBERNETES_PLACEHOLDER {
		t.Errorf("Expected Authorization header value '%s', got '%s'",
			utils.KUBERNETES_PLACEHOLDER, headers["Authorization"])
	}
}

func TestBuildLCoreMCPServersConfig_UserDefinedServers_WithSecretRef(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "external-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:     "https://external.example.com/mcp",
						Timeout: 30,
						Headers: map[string]string{
							"Authorization": utils.KUBERNETES_PLACEHOLDER,
							"X-Custom":      "mcp-auth-secret", // This will be validated during deployment
						},
					},
				},
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(result))
	}

	server := result[0]

	// Verify headers include both kubernetes and secret ref paths
	headers, ok := server["authorization_headers"].(map[string]string)
	if !ok {
		t.Fatal("Expected authorization_headers to be map[string]string")
	}

	// Check paths are correctly formatted
	if headers["Authorization"] != utils.KUBERNETES_PLACEHOLDER {
		t.Errorf("Expected Authorization=%s, got '%s'", utils.KUBERNETES_PLACEHOLDER, headers["Authorization"])
	}

	expectedPath := "/etc/mcp/headers/mcp-auth-secret/header"
	if headers["X-Custom"] != expectedPath {
		t.Errorf("Expected X-Custom path '%s', got '%s'", expectedPath, headers["X-Custom"])
	}
}

func TestBuildLCoreMCPServersConfig_Combined(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: true,
			},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "user-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://user-mcp.example.com",
						Headers: map[string]string{
							"Authorization": "user-secret",
						},
					},
				},
			},
		},
	}

	// Config generation doesn't validate secrets - validation happens during deployment
	result := buildLCoreMCPServersConfig(r, cr)

	// Should have both OpenShift (from introspection) + user server
	if len(result) != 2 {
		t.Fatalf("Expected 2 MCP servers, got %d", len(result))
	}

	// First should be OpenShift
	if result[0]["name"] != "openshift" {
		t.Errorf("Expected first server to be 'openshift', got '%v'", result[0]["name"])
	}

	// Second should be user-defined
	if result[1]["name"] != "user-server" {
		t.Errorf("Expected second server to be 'user-server', got '%v'", result[1]["name"])
	}
}

func TestBuildLCoreMCPServersConfig_FiltersNonHTTP(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster",
			Generation: 1,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "http-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://valid.example.com",
						Headers: map[string]string{
							"Authorization": "test-secret",
						},
					},
				},
				{
					Name: "no-transport-server",
					// No transport - should be filtered out
				},
			},
		},
	}

	// Reset cache to ensure filtering happens
	ResetMCPWarningCache()

	// Config generation doesn't validate secrets - validation happens during deployment
	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server (non-HTTP filtered out), got %d", len(result))
	}

	if result[0]["name"] != "http-server" {
		t.Errorf("Expected filtered server to be 'http-server', got '%v'", result[0]["name"])
	}
}

func TestBuildLCoreMCPServersConfig_EmptyHeadersNotAdded(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "no-auth-server",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://no-auth.example.com",
						// No headers specified
					},
				},
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(result))
	}

	// authorization_headers should not be present when no headers configured
	if _, exists := result[0]["authorization_headers"]; exists {
		t.Error("Expected no authorization_headers field when no headers configured")
	}
}

func TestBuildLCoreMCPServersConfig_SkipsEmptySecretRefs(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
			MCPServers: []olsv1alpha1.MCPServer{
				{
					Name: "server-with-mixed-headers",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL: "http://example.com",
						Headers: map[string]string{
							"Valid-Header":      utils.KUBERNETES_PLACEHOLDER, // Should be included
							"Empty-Header":      "",                           // Should be skipped
							"Kubernetes-Header": utils.KUBERNETES_PLACEHOLDER, // Should be included
						},
					},
				},
			},
		},
	}

	result := buildLCoreMCPServersConfig(r, cr)

	if len(result) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(result))
	}

	headers, ok := result[0]["authorization_headers"].(map[string]string)
	if !ok {
		t.Fatal("Expected authorization_headers to be map[string]string")
	}

	// Empty header should not be present
	if _, exists := headers["Empty-Header"]; exists {
		t.Error("Expected empty header to be skipped")
	}

	// Valid headers should be present
	if _, exists := headers["Valid-Header"]; !exists {
		t.Error("Expected Valid-Header to be present")
	}
	if _, exists := headers["Kubernetes-Header"]; !exists {
		t.Error("Expected Kubernetes-Header to be present")
	}
}

func TestBuildLCoreConfigYAML_WithMCPServers(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: true,
			},
		},
	}

	yamlStr, err := buildLCoreConfigYAML(r, cr)
	if err != nil {
		t.Fatalf("buildLCoreConfigYAML returned error: %v", err)
	}

	// Verify YAML contains MCP servers section
	if !strings.Contains(yamlStr, "mcp_servers:") {
		t.Error("Expected YAML to contain 'mcp_servers:' section")
	}

	// Verify OpenShift server is present
	if !strings.Contains(yamlStr, "name: openshift") {
		t.Error("Expected YAML to contain OpenShift MCP server")
	}
}

func TestBuildLCoreConfigYAML_WithoutMCPServers(t *testing.T) {
	r := utils.NewTestReconciler(nil, logr.Discard(), nil, utils.OLSNamespaceDefault)

	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				IntrospectionEnabled: false,
			},
		},
	}

	yamlStr, err := buildLCoreConfigYAML(r, cr)
	if err != nil {
		t.Fatalf("buildLCoreConfigYAML returned error: %v", err)
	}

	// Verify YAML does NOT contain MCP servers section
	if strings.Contains(yamlStr, "mcp_servers:") {
		t.Error("Expected YAML NOT to contain 'mcp_servers:' section when no servers configured")
	}
}
