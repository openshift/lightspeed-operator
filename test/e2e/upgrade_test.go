package e2e

import (
	"context"
	"os"
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

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests operator upgrade scenario - CR persists across operator version changes
// - Uses DeleteAndWait in AfterAll to prevent resource pollution between test suites
// - CR is created before upgrade and should remain functional after upgrade
var _ = Describe("Upgrade operator tests", Ordered, Label("Upgrade"), func() {
	const testSAName = "test-sa"
	const queryAccessClusterRole = "lightspeed-operator-query-access"
	var env *OLSTestEnvironment
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var storageClassName string
	var registryDefaultRoute string
	var dstToken string

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		ctx := context.Background()

		By("setup the imageregistry")
		err = EnableInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		registryDefaultRoute, err = GetInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		err = AddImageBuilderRole(client, utils.OLSNamespaceDefault, dstUserName)
		Expect(err).NotTo(HaveOccurred())

		dstToken, err = GetBuilderToken(client, utils.OLSNamespaceDefault, dstUserName)
		Expect(err).NotTo(HaveOccurred())

		_, err = CopyImageToRegistry(
			ctx,
			origImage1,
			registryDefaultRoute,
			utils.OLSNamespaceDefault,
			imageNameAndTag,
			"",
			"",
			dstUserName,
			dstToken,
			false,
			true,
			os.Stdout,
			15*time.Minute,
		)
		Expect(err).NotTo(HaveOccurred())

		By("get the default storage class")
		storageClassName = StorageClassNameCI
		defaultStorageClass, err := client.GetDefaultStorageClass()
		if err == nil {
			storageClassName = defaultStorageClass.Name
		}

		By("create a storage class with its PV for testing")
		if defaultStorageClass == nil {
			storageClassName = StorageClassNameLocal
			By("Cannot find the default storage class, using local storage class for testing, this test will be flaky if cluster has more than 1 worker node")
			By("Creating a StorageClass")
			cleanUpStorageClass, err := client.CreateStorageClass(storageClassName)
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpStorageClass)

			By("Creating a PersistentVolume")
			cleanUpPV, err := client.CreatePersistentVolume(PvName, storageClassName, resource.MustParse("1Gi"))
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpPV)
		}

		By("Creating a OLSConfig CR")
		env, err = SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image: latestImageName,
				},
			}
			cr.Spec.OLSConfig.ByokRAGOnly = true
			cr.Spec.OLSConfig.Storage = &olsv1alpha1.Storage{
				Size:  resource.MustParse("768Mi"),
				Class: storageClassName,
			}
			cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}
			cr.Spec.OLSConfig.QuotaHandlersConfig = &olsv1alpha1.QuotaHandlersConfig{
				LimitersConfig: []olsv1alpha1.LimiterConfig{
					{
						Name:          "user_limiter",
						Type:          "user_limiter",
						InitialQuota:  1000,
						QuotaIncrease: 100,
						Period:        "1 hour",
					},
					{
						Name:          "cluster_limiter",
						Type:          "cluster_limiter",
						InitialQuota:  5000,
						QuotaIncrease: 500,
						Period:        "1 hour",
					},
				},
				EnableTokenHistory: true,
			}
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{
				"MCPServer",
			}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "test-mcp-server",
					URL:  "http://test-mcp.example.com:8080",
				},
			}
		}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err = mustGather("upgrade_test")
		Expect(err).NotTo(HaveOccurred())

		err = CleanupOLSTestEnvironmentWithCRDeletion(env, "upgrade_test")
		Expect(err).NotTo(HaveOccurred())

		for _, cleanUpFunc := range cleanUpFuncs {
			cleanUpFunc()
		}
	})

	It("should continue working after operator upgrade", func() {
		By("Wait for the application service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())

		err = client.UpgradeOperator(OLSNameSpace)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}

		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

	})
})
