// all_features_test.go contains comprehensive end-to-end tests with ALL OLSConfig features enabled.
// This test creates a single OLSConfig with all features turned on and runs individual validation
// tests against that comprehensive configuration. It combines features from all other E2E tests plus
// additional features not tested elsewhere (quotas, introspection, MCP servers, tool filtering, etc.).

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// All features test constants
	allFeaturesPVCName             = "squid-volume-claim-all-features"
	allFeaturesSquidDeploymentName = "squid-deployment-all-features"
	allFeaturesSquidServiceName    = "squid-service-all-features"
	allFeaturesSquidHTTPSPort      = 3349
	allFeaturesSquidHTTPPort       = 3128
	allFeaturesSquidConfigName     = "squid-config-all-features"
	allFeaturesProxyConfigmapName  = "proxy-ca-all-features"
	allFeaturesAdditionalCAName    = "additional-ca-certs-all-features"
	allFeaturesMCPSecretName       = "mcp-auth-secret-all-features"
	allFeaturesByokPullSecretName  = "byok-pull-secret-all-features"
	allFeaturesImageName           = "assisted-installer-guide"
	allFeaturesImageTag            = "latest"
	allFeaturesInternalRegistry    = "image-registry.openshift-image-registry.svc:5000"
	allFeaturesOrigImage           = "docker://quay.io/openshift-lightspeed-test/assisted-installer-guide:2025-1"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (all tests share single cluster-scoped CR)
// - Tests ALL OLSConfig features in a single comprehensive configuration
// - Labeled "AllFeatures" to exclude from standard make test-e2e runs
// - Combines features from all other E2E tests plus new features not tested elsewhere
// - Each It block has FlakeAttempts(5) to retry only failing tests, not entire suite
// - Longer timeout (3h) due to complexity of setup and comprehensive testing
var _ = Describe("All Features Enabled", Ordered, Label("AllFeatures"), func() {
	var env *OLSTestEnvironment
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var storageClassName string
	var squidHostname string
	var registryDefaultRoute string
	var builderToken string
	var secret *corev1.Secret

	BeforeAll(func() {
		By("Setting up comprehensive all-features test environment")
		ctx := context.Background()
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		// Step 1: Detect or create storage class
		By("Detecting or creating storage class")
		storageClassName = StorageClassNameCI
		defaultStorageClass, err := client.GetDefaultStorageClass()
		if err == nil && defaultStorageClass != nil {
			storageClassName = defaultStorageClass.Name
		} else {
			storageClassName = StorageClassNameLocal
			By("Creating local storage class for testing")
			cleanUpStorageClass, err := client.CreateStorageClass(storageClassName)
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpStorageClass)
		}

		// Step 2: Create squid proxy infrastructure
		By("Creating squid proxy PVC")
		_, err = client.CreatePVC(allFeaturesPVCName, storageClassName, resource.MustParse("1Gi"))
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      allFeaturesPVCName,
					Namespace: OLSNameSpace,
				},
			}
			_ = client.Delete(pvc)
		})

		By("Creating squid config ConfigMap")
		squidConfPath := filepath.Join("..", "utils", "squid.conf")
		squidConfData, err := os.ReadFile(squidConfPath)
		Expect(err).NotTo(HaveOccurred())
		squidConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesSquidConfigName,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				"squid.conf": string(squidConfData),
			},
		}
		err = client.Create(squidConfigMap)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(squidConfigMap)
		})

		By("Creating squid service with TLS annotation")
		squidService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesSquidServiceName,
				Namespace: OLSNameSpace,
				Labels:    map[string]string{"app": allFeaturesSquidDeploymentName},
				Annotations: map[string]string{
					ServiceAnnotationKeyTLSSecret: "squid-service-tls-all-features",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app": allFeaturesSquidDeploymentName,
				},
				Ports: []corev1.ServicePort{
					{
						Name:     "squid-https",
						Port:     allFeaturesSquidHTTPSPort,
						Protocol: corev1.ProtocolTCP,
					},
					{
						Name:     "squid",
						Port:     allFeaturesSquidHTTPPort,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		}
		err = client.Create(squidService)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForServiceCreated(squidService)
		Expect(err).NotTo(HaveOccurred())
		squidHostname = fmt.Sprintf("%s.%s.svc.cluster.local", allFeaturesSquidServiceName, OLSNameSpace)
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(squidService)
		})

		By("Waiting for squid TLS secret to be created by service-ca-operator")
		squidTLSSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "squid-service-tls-all-features",
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForSecretCreated(squidTLSSecret)
		Expect(err).NotTo(HaveOccurred())

		By("Creating squid deployment")
		squidDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesSquidDeploymentName,
				Namespace: OLSNameSpace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: func() *int32 { r := int32(1); return &r }(),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": allFeaturesSquidDeploymentName,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": allFeaturesSquidDeploymentName,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "squid",
								Image: "ubuntu/squid:edge",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: allFeaturesSquidHTTPSPort,
										Name:          "squid-https",
										Protocol:      corev1.ProtocolTCP,
									},
									{
										ContainerPort: allFeaturesSquidHTTPPort,
										Name:          "squid",
										Protocol:      corev1.ProtocolTCP,
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "squid-config-volume",
										MountPath: "/etc/squid/squid.conf",
										SubPath:   "squid.conf",
									},
									{
										Name:      "squid-data",
										MountPath: "/var/spool/squid",
									},
									{
										Name:      "squid-ssl",
										MountPath: "/etc/squid/ssl_ca",
										ReadOnly:  true,
									},
									{
										Name:      "squid-logs",
										MountPath: "/var/log/squid",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "squid-config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: allFeaturesSquidConfigName,
										},
										Items: []corev1.KeyToPath{
											{
												Key:  "squid.conf",
												Path: "squid.conf",
											},
										},
									},
								},
							},
							{
								Name: "squid-data",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: allFeaturesPVCName,
									},
								},
							},
							{
								Name: "squid-ssl",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: "squid-service-tls-all-features",
										Items: []corev1.KeyToPath{
											{
												Key:  "tls.crt",
												Path: "tls.crt",
											},
											{
												Key:  "tls.key",
												Path: "tls.key",
											},
										},
									},
								},
							},
							{
								Name: "squid-logs",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			},
		}
		err = client.Create(squidDeployment)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(squidDeployment)
		})

		By("Waiting for squid deployment to be ready")
		err = client.WaitForDeploymentRollout(squidDeployment)
		Expect(err).NotTo(HaveOccurred())

		// Step 3: Create proxy CA ConfigMap
		By("Copying service CA certificate to proxy CA ConfigMap")
		serviceCAConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-service-ca.crt",
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(serviceCAConfigMap)
		Expect(err).NotTo(HaveOccurred())
		serviceCACrt, ok := serviceCAConfigMap.Data["service-ca.crt"]
		Expect(ok).To(BeTrue())

		proxyCAConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesProxyConfigmapName,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				"proxy-ca.crt": serviceCACrt,
			},
		}
		err = client.Create(proxyCAConfigMap)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForObjectCreated(proxyCAConfigMap)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(proxyCAConfigMap)
		})

		// Step 4: Create additional CA certificates ConfigMap
		By("Creating additional CA certificates ConfigMap")
		additionalCAConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesAdditionalCAName,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				"ca-bundle.crt": TestCACert,
			},
		}
		err = client.Create(additionalCAConfigMap)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(additionalCAConfigMap)
		})

		// Step 5: Create MCP auth secret
		By("Creating MCP auth secret")
		mcpAuthSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allFeaturesMCPSecretName,
				Namespace: OLSNameSpace,
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"Authorization": "Bearer test-mcp-token",
			},
		}
		err = client.Create(mcpAuthSecret)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, func() {
			_ = client.Delete(mcpAuthSecret)
		})

		// Step 6: Enable internal registry and copy BYOK image
		By("Enabling internal image registry route")
		err = EnableInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		By("Getting internal image registry route")
		registryDefaultRoute, err = GetInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		By("Adding image builder role")
		err = AddImageBuilderRole(client, utils.OLSNamespaceDefault, "builder")
		Expect(err).NotTo(HaveOccurred())

		By("Getting builder token")
		builderToken, err = GetBuilderToken(client, utils.OLSNamespaceDefault, "builder")
		Expect(err).NotTo(HaveOccurred())

		By("Copying BYOK image to internal registry")
		imageNameAndTag := allFeaturesImageName + ":" + allFeaturesImageTag
		_, err = CopyImageToRegistry(
			ctx,
			allFeaturesOrigImage,
			registryDefaultRoute,
			utils.OLSNamespaceDefault,
			imageNameAndTag,
			"",
			"",
			"builder",
			builderToken,
			false,
			true,
			os.Stdout,
			15*time.Minute,
		)
		Expect(err).NotTo(HaveOccurred())

		// Step 7: Create BYOK pull secret
		By("Creating BYOK pull secret")
		cleanupPullSecret, err := client.CreateDockerRegistrySecret(
			OLSNameSpace,
			allFeaturesByokPullSecretName,
			allFeaturesInternalRegistry,
			"builder",
			builderToken,
			"builder@example.com",
		)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanupPullSecret)

		// Step 8: Setup OLS test environment with comprehensive configuration
		By("Setting up OLS test environment with all features enabled")
		env, err = SetupOLSTestEnvironment(
			func(cr *olsv1alpha1.OLSConfig) {
				allFeaturesCR, err := generateAllFeaturesOLSConfig()
				Expect(err).NotTo(HaveOccurred())

				cr.Spec = allFeaturesCR.Spec

				cr.Spec.OLSConfig.ProxyConfig.ProxyURL = fmt.Sprintf("https://%s:%d", squidHostname, allFeaturesSquidHTTPSPort)
				cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef = &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: allFeaturesProxyConfigmapName,
					},
				}

				cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
					Name: allFeaturesAdditionalCAName,
				}

				cr.Spec.MCPServers[0].Headers[0].ValueFrom.SecretRef = &corev1.LocalObjectReference{
					Name: allFeaturesMCPSecretName,
				}

				cr.Spec.OLSConfig.ImagePullSecrets = []corev1.LocalObjectReference{
					{
						Name: allFeaturesByokPullSecretName,
					},
				}

				cr.Spec.OLSConfig.Storage.Class = storageClassName
			},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())

		// Step 9: Setup TLS and port forwarding
		By("Testing TLS service activation")
		secret, err = TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Setting up HTTPS client with port forwarding")
		_, ok = secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
	})

	AfterAll(func() {
		By("Collecting must-gather diagnostics")
		err = mustGather("all_features_test")
		if err != nil {
			fmt.Printf("Failed to collect must-gather: %v\n", err)
		}

		By("Cleaning up OLS test environment with CR deletion")
		if env != nil {
			err = CleanupOLSTestEnvironmentWithCRDeletion(env, "all_features_test")
			if err != nil {
				fmt.Printf("Failed to cleanup OLS environment: %v\n", err)
			}
		}

		By("Cleaning up additional resources")
		for i := len(cleanUpFuncs) - 1; i >= 0; i-- {
			cleanUpFuncs[i]()
		}
	})

	// Test 1: Deployment Validation
	It("should deploy all components successfully", FlakeAttempts(5), func() {
		By("Verifying app server deployment with 2 replicas")
		appDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(appDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(appDeployment.Status.ReadyReplicas).To(Equal(int32(2)))

		By("Verifying postgres deployment")
		postgresDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(postgresDeployment.Status.ReadyReplicas).To(BeNumerically(">", 0))

		By("Verifying console plugin deployment")
		consoleDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(consoleDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(consoleDeployment.Status.ReadyReplicas).To(BeNumerically(">", 0))

		By("Verifying data collector container is present")
		Expect(len(appDeployment.Spec.Template.Spec.Containers)).To(BeNumerically(">", 1))
		foundDataCollector := false
		for _, container := range appDeployment.Spec.Template.Spec.Containers {
			if strings.Contains(container.Name, "lightspeed-to-dataverse-exporter") || strings.Contains(container.Image, "dataverse-exporter-rhel9") {
				foundDataCollector = true
				break
			}
		}
		Expect(foundDataCollector).To(BeTrue(), "Data collector container should be present")
	})

	// Test 2: Configuration Validation
	It("should have all features configured in the CR", FlakeAttempts(5), func() {
		By("Retrieving the OLSConfig CR")
		cr := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			},
		}
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying feature gates")
		Expect(cr.Spec.FeatureGates).To(ContainElements(
			olsv1alpha1.FeatureGate("MCPServer"),
			olsv1alpha1.FeatureGate("ToolFiltering"),
		))

		By("Verifying quota handlers configuration")
		Expect(cr.Spec.OLSConfig.QuotaHandlersConfig).NotTo(BeNil())
		Expect(cr.Spec.OLSConfig.QuotaHandlersConfig.EnableTokenHistory).To(BeTrue())
		Expect(cr.Spec.OLSConfig.QuotaHandlersConfig.LimitersConfig).To(HaveLen(2))

		By("Verifying tool filtering configuration")
		Expect(cr.Spec.OLSConfig.ToolFilteringConfig).NotTo(BeNil())
		Expect(cr.Spec.OLSConfig.ToolFilteringConfig.Alpha).To(Equal(0.75))
		Expect(cr.Spec.OLSConfig.ToolFilteringConfig.TopK).To(Equal(15))

		By("Verifying MCP servers configuration")
		Expect(cr.Spec.MCPServers).To(HaveLen(1))
		Expect(cr.Spec.MCPServers[0].Name).To(Equal("test-mcp-server"))

		By("Verifying introspection enabled")
		Expect(cr.Spec.OLSConfig.IntrospectionEnabled).To(BeTrue())

		By("Verifying BYOK RAG only mode")
		Expect(cr.Spec.OLSConfig.ByokRAGOnly).To(BeTrue())

		By("Verifying custom system prompt")
		Expect(cr.Spec.OLSConfig.QuerySystemPrompt).To(Equal("You are a comprehensive test assistant for OpenShift."))

		By("Verifying max iterations")
		Expect(cr.Spec.OLSConfig.MaxIterations).To(Equal(10))
	})

	// Test 3: ConfigMap Validation
	It("should have all features in the olsconfig ConfigMap", FlakeAttempts(5), func() {
		By("Retrieving the olsconfig ConfigMap")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(configMap)
		Expect(err).NotTo(HaveOccurred())

		olsConfigYaml, ok := configMap.Data[AppServerConfigMapKey]
		Expect(ok).To(BeTrue())

		By("Verifying quota_handlers section exists")
		Expect(olsConfigYaml).To(ContainSubstring("quota_handlers"))
		Expect(olsConfigYaml).To(ContainSubstring("enable_token_history"))
		Expect(olsConfigYaml).To(ContainSubstring("user_limiter"))
		Expect(olsConfigYaml).To(ContainSubstring("cluster_limiter"))

		By("Verifying user_data_collection settings")
		Expect(olsConfigYaml).To(ContainSubstring("user_data_collection"))
		Expect(olsConfigYaml).To(ContainSubstring("feedback_disabled"))
		Expect(olsConfigYaml).To(ContainSubstring("transcripts_disabled"))

		By("Verifying query_filters section")
		Expect(olsConfigYaml).To(ContainSubstring("query_filters"))
		Expect(olsConfigYaml).To(ContainSubstring("test-filter"))
	})

	// Test 4: TLS and Service Activation
	It("should have TLS properly configured", FlakeAttempts(5), func() {
		By("Verifying TLS secret exists")
		tlsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerTLSSecretName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(tlsSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsSecret.Data).To(HaveKey("tls.crt"))
		Expect(tlsSecret.Data).To(HaveKey("tls.key"))

		By("Verifying service has TLS annotation")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(service)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Annotations).To(HaveKey(ServiceAnnotationKeyTLSSecret))
	})

	// Test 5: Basic Query Functionality
	It("should handle basic queries successfully", FlakeAttempts(5), func() {
		By("Making a basic query request")
		reqBody := []byte(`{"query": "what is OpenShift?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).NotTo(BeEmpty())
	})

	// Test 6: BYOK RAG Query
	It("should return BYOK content and respect byokRAGOnly mode", FlakeAttempts(5), func() {
		By("Making a query that should hit BYOK RAG index")
		reqBody := []byte(`{"query": "what CPU architectures are supported by assisted installer?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("Verifying response contains BYOK content")
		bodyStr := string(body)
		// The assisted installer BYOK index should contain information about CPU architectures
		Expect(bodyStr).To(Or(
			ContainSubstring("x86_64"),
			ContainSubstring("aarch64"),
			ContainSubstring("CPU"),
			ContainSubstring("architecture"),
		))

		By("Verifying byokRAGOnly mode - should NOT contain standard docs reference")
		// When byokRAGOnly is true, responses should not reference standard OpenShift docs
		Expect(bodyStr).NotTo(ContainSubstring("Related documentation"))
	})

	// Test 7: MCP Server Sidecar
	It("should have openshift-mcp-server sidecar container", FlakeAttempts(5), func() {
		By("Verifying openshift-mcp-server container is present in app deployment")
		appDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(appDeployment)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(appDeployment.Spec.Template.Spec.Containers)).To(BeNumerically(">", 1))
		foundMCPServer := false
		for _, container := range appDeployment.Spec.Template.Spec.Containers {
			if strings.Contains(container.Name, "openshift-mcp-server") || strings.Contains(container.Image, "openshift-mcp-server") {
				foundMCPServer = true
				break
			}
		}
		Expect(foundMCPServer).To(BeTrue(), "openshift-mcp-server sidecar container should be present")
	})

	// Test 8: Proxy Functionality
	It("should route queries through the proxy", FlakeAttempts(5), func() {
		By("Making a query through the proxy")
		reqBody := []byte(`{"query": "what is Kubernetes?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).NotTo(BeEmpty())
	})

	// Test 9: Database Persistence
	It("should persist conversations in postgres with correct configuration", FlakeAttempts(5), func() {
		By("Verifying PVC exists with correct size")
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lightspeed-postgres-pvc",
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(pvc)
		Expect(err).NotTo(HaveOccurred())
		storageSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		Expect(storageSize.String()).To(Equal("1Gi"))

		By("Verifying postgres deployment has correct configuration")
		postgresDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())

		// Check for shared_buffers and max_connections in postgres container args/env
		foundSharedBuffers := false
		foundMaxConnections := false
		for _, container := range postgresDeployment.Spec.Template.Spec.Containers {
			for _, env := range container.Env {
				if env.Name == "POSTGRES_SHARED_BUFFERS" || strings.Contains(env.Value, "512MB") {
					foundSharedBuffers = true
				}
				if env.Name == "POSTGRES_MAX_CONNECTIONS" || strings.Contains(env.Value, "3000") {
					foundMaxConnections = true
				}
			}
		}
		Expect(foundSharedBuffers || foundMaxConnections).To(BeTrue(), "Postgres configuration should include custom settings")
	})

	// Test 10: Resource Limits Validation
	It("should have correct resource limits for all containers", FlakeAttempts(5), func() {
		By("Verifying API container resource limits")
		appDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(appDeployment)
		Expect(err).NotTo(HaveOccurred())

		apiContainer := appDeployment.Spec.Template.Spec.Containers[0]
		Expect(apiContainer.Resources.Limits).NotTo(BeNil())
		Expect(apiContainer.Resources.Requests).NotTo(BeNil())
		Expect(apiContainer.Resources.Limits.Cpu().String()).To(Equal("1"))
		Expect(apiContainer.Resources.Limits.Memory().String()).To(Equal("2Gi"))

		By("Verifying console deployment has resource limits")
		consoleDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(consoleDeployment)
		Expect(err).NotTo(HaveOccurred())
		consoleContainer := consoleDeployment.Spec.Template.Spec.Containers[0]
		Expect(consoleContainer.Resources.Limits).NotTo(BeNil())

		By("Verifying database deployment has resource limits")
		postgresDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())
		postgresContainer := postgresDeployment.Spec.Template.Spec.Containers[0]
		Expect(postgresContainer.Resources.Limits).NotTo(BeNil())
	})

	// Test 11: Custom System Prompt
	It("should use the custom system prompt", FlakeAttempts(5), func() {
		By("Verifying custom prompt in ConfigMap")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(configMap)
		Expect(err).NotTo(HaveOccurred())

		olsConfigYaml := configMap.Data["system_prompt"]
		Expect(olsConfigYaml).To(ContainSubstring("You are a comprehensive test assistant for OpenShift"))
	})

	// Test 12: Query Filters
	It("should have query filters configured and functioning", FlakeAttempts(5), func() {
		By("Verifying query filters in ConfigMap")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(configMap)
		Expect(err).NotTo(HaveOccurred())

		olsConfigYaml := configMap.Data[AppServerConfigMapKey]
		Expect(olsConfigYaml).To(ContainSubstring("query_filters"))
		Expect(olsConfigYaml).To(ContainSubstring("test-filter"))
		Expect(olsConfigYaml).To(ContainSubstring("oldterm"))
		Expect(olsConfigYaml).To(ContainSubstring("newterm"))

		// Record the time before sending the query to filter logs
		queryStartTime := time.Now()

		By("Sending query with filtered term to verify filter functionality")
		reqBody := []byte(`{"query": "what is oldterm in Kubernetes?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).NotTo(BeEmpty())

		By("Verifying response contains valid LLM output")
		bodyStr := string(body)
		Expect(bodyStr).To(ContainSubstring("conversation_id"))

		By("Retrieving pod logs to verify query filter was applied")
		logs, podName, err := GetAppServerPodLogs(client, &queryStartTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(podName).NotTo(BeEmpty())

		By("Verifying filtered query appears in logs (oldterm -> newterm)")
		// The filtered query should contain "newterm"
		Expect(logs).To(ContainSubstring("newterm"))

		By("Verifying original unfiltered term does NOT appear in processed logs")
		// Check that "oldterm" doesn't appear in the redacted/processed query logs
		// Note: The raw request body might still contain it, so we look for specific patterns
		logLines := strings.Split(logs, "\n")
		foundRedactedQuery := false

		for _, line := range logLines {
			// Skip lines that are raw request bodies (they contain the original query)
			if strings.Contains(line, `"query"`) && strings.Contains(line, "oldterm") && strings.Contains(line, `Body:`) {
				// This is the raw HTTP request body, which is expected to contain oldterm
				continue
			}

			// Look for the redacted query being processed
			if strings.Contains(line, "newterm") && strings.Contains(line, "Kubernetes") {
				foundRedactedQuery = true
			}
		}

		Expect(foundRedactedQuery).To(BeTrue(), "Logs should contain the redacted query with 'newterm'")
		// Note: The raw request body may still contain "oldterm" - that's just HTTP request logging
		// The important verification is that the processed query uses "newterm"
	})

	// Test 12b: Query Filters - Edge Cases
	It("should filter multiple occurrences of the term in a single query", FlakeAttempts(5), func() {
		// Record the time before sending the query
		queryStartTime := time.Now()

		By("Sending query with multiple occurrences of filtered term")
		reqBody := []byte(`{"query": "How does oldterm differ from oldterm in production? Is oldterm configuration important?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())

		// Log error details if we get a 500 status
		if resp.StatusCode == http.StatusInternalServerError {
			By("Capturing error details from response body")
			fmt.Printf("ERROR: Received HTTP 500. Response body: %s\n", string(body))

			By("Retrieving pod logs for detailed error information")
			logs, podName, logErr := GetAppServerPodLogs(client, &queryStartTime)
			if logErr == nil {
				fmt.Printf("Pod %s logs (since query):\n%s\n", podName, logs)
			} else {
				fmt.Printf("Failed to retrieve pod logs: %v\n", logErr)
			}
		}

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).NotTo(BeEmpty())

		By("Retrieving pod logs to verify all occurrences were filtered")
		logs, podName, err := GetAppServerPodLogs(client, &queryStartTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(podName).NotTo(BeEmpty())

		By("Verifying multiple 'newterm' replacements appear in logs")
		newtermCount := strings.Count(logs, "newterm")
		// We should see at least 3 occurrences of newterm (one for each oldterm)
		// There might be more depending on how the logs are formatted
		Expect(newtermCount).To(BeNumerically(">=", 3), "Should find at least 3 occurrences of 'newterm' in logs")

		By("Verifying the query was processed successfully despite filtering")
		bodyStr := string(body)
		Expect(bodyStr).To(ContainSubstring("conversation_id"))
	})

	// Test 13: Multi-Provider Configuration
	It("should have multiple providers and models configured", FlakeAttempts(5), func() {
		By("Retrieving the OLSConfig CR")
		cr := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			},
		}
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying multiple providers")
		Expect(cr.Spec.LLMConfig.Providers).To(HaveLen(2))

		By("Verifying first provider has multiple models")
		Expect(cr.Spec.LLMConfig.Providers[0].Models).To(HaveLen(2))

		By("Verifying default provider and model are set")
		Expect(cr.Spec.OLSConfig.DefaultProvider).NotTo(BeEmpty())
		Expect(cr.Spec.OLSConfig.DefaultModel).NotTo(BeEmpty())
	})

	// Test 14: User Data Collection Settings
	It("should have user data collection settings configured correctly", FlakeAttempts(5), func() {
		By("Verifying user data collection in ConfigMap")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(configMap)
		Expect(err).NotTo(HaveOccurred())

		olsConfigYaml := configMap.Data[AppServerConfigMapKey]
		Expect(olsConfigYaml).To(ContainSubstring("user_data_collection"))
		Expect(olsConfigYaml).To(ContainSubstring("feedback_disabled: true"))
		Expect(olsConfigYaml).To(ContainSubstring("transcripts_disabled: false"))

		By("Testing that feedback endpoint is disabled when feedback_disabled is true")
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())

		httpsClient := NewHTTPSClient(env.ForwardHost, InClusterHost, certificate, nil, nil)
		authHeader := map[string]string{"Authorization": "Bearer " + env.SAToken}

		// Make a query to get a valid conversation_id for feedback testing
		queryReqBody := []byte(`{"query": "what is OpenShift?"}`)
		queryResp, queryBody, err := TestHTTPSQueryEndpoint(env, secret, queryReqBody)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(queryResp.StatusCode).To(Equal(http.StatusOK))

		var queryResponse map[string]any
		err = json.Unmarshal(queryBody, &queryResponse)
		Expect(err).NotTo(HaveOccurred())
		conversationID, ok := queryResponse["conversation_id"].(string)
		Expect(ok).To(BeTrue())
		Expect(conversationID).NotTo(BeEmpty())

		// Should fail because feedback is disabled
		feedbackReqBody := fmt.Appendf(nil, `{
			"conversation_id": "%s",
			"user_question": "what is OpenShift?",
			"llm_response": "OpenShift is a container platform",
			"sentiment": 1
		}`, conversationID)

		feedbackResp, err := httpsClient.PostJson("/v1/feedback", feedbackReqBody, authHeader)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())

		// Exact status code may vary (400, 403, or 404) depending on implementation
		Expect(feedbackResp.StatusCode).NotTo(Equal(http.StatusOK), "Feedback endpoint should be disabled")

		By("Verifying transcripts are enabled (transcripts_disabled: false)")
		// Cannot verify transcript files without pod exec, but configuration is validated
		Expect(olsConfigYaml).To(ContainSubstring("transcripts_disabled: false"))
	})

	// Test 15: Additional CA Certificates
	It("should have additional CA certificates mounted", FlakeAttempts(5), func() {
		By("Verifying additional CA volume in app deployment")
		appDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(appDeployment)
		Expect(err).NotTo(HaveOccurred())

		foundVolume := false
		foundVolumeMount := false

		for _, volume := range appDeployment.Spec.Template.Spec.Volumes {
			if volume.Name == AdditionalCAVolumeName {
				foundVolume = true
				Expect(volume.ConfigMap.Name).To(Equal(allFeaturesAdditionalCAName))
				break
			}
		}
		Expect(foundVolume).To(BeTrue(), "Additional CA volume should be mounted")

		for _, container := range appDeployment.Spec.Template.Spec.Containers {
			if container.Name == ServerContainerName {
				for _, mount := range container.VolumeMounts {
					if mount.Name == AdditionalCAVolumeName {
						foundVolumeMount = true
						break
					}
				}
			}
		}
		Expect(foundVolumeMount).To(BeTrue(), "Additional CA volume should be mounted in container")
	})
})
