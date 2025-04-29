package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Try to retry test case multipletimes when failing.
var _ = Describe("Prometheus Metrics", FlakeAttempts(5), Ordered, func() {
	const metricsViewerSAName = "metrics-viewer-sa"
	const clusterMonitoringViewClusterRole = "cluster-monitoring-view"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var saToken string
	var prometheusClient *PrometheusClient

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		var cleanUp func()

		By("create a service account")
		cleanUp, err := client.CreateServiceAccount(OLSNameSpace, metricsViewerSAName)
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)
		Expect(err).NotTo(HaveOccurred())

		By("create a role binding for application metrics access")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, metricsViewerSAName, clusterMonitoringViewClusterRole)
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)
		Expect(err).NotTo(HaveOccurred())

		By("fetch the service account token")
		saToken, err = client.GetServiceAccountToken(OLSNameSpace, metricsViewerSAName)
		Expect(err).NotTo(HaveOccurred())

		By("fetch a Kubernetes rest config")
		cfg, err := config.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		openshiftRouteClient, err := routev1.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		By("fetch a Prometheus route")
		// Retry multiple times to fetch route
		// mitigates networking issues
		Eventually(func() error {
			prometheusClient, err = NewPrometheusClientFromRoute(
				context.Background(),
				openshiftRouteClient,
				"openshift-monitoring", "thanos-querier",
				saToken,
			)
			return err
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OperatorDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterAll(func() {
		for _, cleanUp := range cleanUpFuncs {
			cleanUp()
		}

		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should have operator metrics in Prometheus", func() {
		By("verify Prometheus is working correctly by querying prometheus' own metrics")
		err = prometheusClient.WaitForQueryReturnGreaterEqualOne("topk(1,prometheus_build_info)", DefaultPrometheusQueryTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("verify prometheus scrapes metrics from operator, this should happen every 60 seconds")
		err = prometheusClient.WaitForQueryReturn("avg(scrape_duration_seconds{service=\"lightspeed-operator-controller-manager-service\"})",
			60*time.Second,
			func(val float64) error {
				if val > 0.0 {
					return nil
				}
				return err
			})
		Expect(err).NotTo(HaveOccurred())

		By("fetching the operator metrics from Prometheus")
		err = prometheusClient.WaitForQueryReturnGreaterEqualOne("count(controller_runtime_reconcile_total{namespace=\"openshift-lightspeed\"})", DefaultPrometheusQueryTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

})
