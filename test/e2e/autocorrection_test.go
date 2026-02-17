package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - All tests share a single cluster-scoped OLSConfig CR
// - Uses DeleteAndWait in AfterAll to prevent resource pollution between test suites
// - Uses Update with automatic retry to handle concurrent operator reconciliation
var _ = Describe("Automatic correction against modifications on managed resources", Ordered, func() {
	var cr *olsv1alpha1.OLSConfig
	var client *Client

	BeforeAll(func() {
		var err error
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		var err error
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		err = mustGather("autocorrection_test")
		Expect(err).NotTo(HaveOccurred())
		By("Ensuring operator is ready before CR deletion")
		operatorDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OperatorDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(operatorDeployment)
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR and waiting for cleanup")
		Expect(cr).NotTo(BeNil())
		err = client.DeleteAndWait(cr, 3*time.Minute)
		Expect(err).NotTo(HaveOccurred())

	})

	It("should restore console plugin resources", func() {
		var err error
		By("wait for all resources created")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForObjectCreated(deployment)
		Expect(err).NotTo(HaveOccurred())
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginServiceName,
				Namespace: OLSNameSpace,
			},
		}
		Expect(err).NotTo(HaveOccurred())
		consoleplugin := &consolev1.ConsolePlugin{
			ObjectMeta: metav1.ObjectMeta{
				Name: ConsoleUIPluginName,
			},
		}
		err = client.WaitForObjectCreated(consoleplugin)
		Expect(err).NotTo(HaveOccurred())

		configmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsoleUIConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForObjectCreated(configmap)
		Expect(err).NotTo(HaveOccurred())

		By("restoring console plugin deployment")
		err = client.WaitForObjectCreated(deployment)
		Expect(err).NotTo(HaveOccurred())
		originDeployment := deployment.DeepCopy()
		By("restoring console plugin deployment")
		err = client.Update(deployment, func(obj ctrlclient.Object) error {
			dep := obj.(*appsv1.Deployment)
			dep.Spec.Replicas = Ptr(1 + *dep.Spec.Replicas)
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		var lastErr error
		err = wait.PollUntilContextTimeout(client.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
			err := client.Get(deployment)
			if err != nil {
				lastErr = fmt.Errorf("failed to get Deployment: %w", err)
				return false, nil
			}
			if *deployment.Spec.Replicas != *originDeployment.Spec.Replicas {
				lastErr = fmt.Errorf("the number of replicas (%d) does not match the expected number (%d)",
					*deployment.Spec.Replicas, *originDeployment.Spec.Replicas)
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			Fail(fmt.Sprintf("failed to restore console plugin deployment: %v LastErr: %v", err, lastErr))
		}

		By("restoring console plugin service")
		err = client.Get(service)
		Expect(err).NotTo(HaveOccurred())
		originService := service.DeepCopy()
		By("restoring console plugin service")
		err = client.Update(service, func(obj ctrlclient.Object) error {
			svc := obj.(*corev1.Service)
			svc.Spec.Ports[0].Name = "wrong-port-name"
			svc.Spec.Selector = map[string]string{
				"wrong": "label",
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		err = wait.PollUntilContextTimeout(client.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
			err := client.Get(service)
			if err != nil {
				lastErr = fmt.Errorf("failed to get Service: %w", err)
				return false, nil
			}
			if !apiequality.Semantic.DeepEqual(service.Spec, originService.Spec) {
				lastErr = fmt.Errorf("the specs are not equal")
				return false, nil
			}

			return true, nil
		})
		if err != nil {
			Fail(fmt.Sprintf("failed to restore console plugin service: %v LastErr: %v", err, lastErr))
		}

		By("restoring console plugin CR")
		err = client.Get(consoleplugin)
		Expect(err).NotTo(HaveOccurred())
		originConsolePlugin := consoleplugin.DeepCopy()
		By("restoring console plugin CR")
		err = client.Update(consoleplugin, func(obj ctrlclient.Object) error {
			cp := obj.(*consolev1.ConsolePlugin)
			cp.Spec.DisplayName = "New Console Plugin Name"
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		err = wait.PollUntilContextTimeout(client.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
			err := client.Get(consoleplugin)
			if err != nil {
				lastErr = fmt.Errorf("failed to get ConsolePlugin: %w", err)
				return false, nil
			}
			if !apiequality.Semantic.DeepEqual(consoleplugin.Spec, originConsolePlugin.Spec) {
				lastErr = fmt.Errorf("the actual consoleplugin (%v) does not match the original (%v)",
					consoleplugin.Spec, originConsolePlugin.Spec)
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			Fail(fmt.Sprintf("failed to restore console plugin CR: %v LastErr: %v", err, lastErr))
		}

		By("restoring console configmap")
		err = client.Get(configmap)
		Expect(err).NotTo(HaveOccurred())
		originConfigMap := configmap.DeepCopy()
		err = client.Update(configmap, func(obj ctrlclient.Object) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Data["nginx.conf"] = "new-config"
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		err = wait.PollUntilContextTimeout(client.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
			err := client.Get(configmap)
			if err != nil {
				lastErr = fmt.Errorf("failed to get ConfigMap: %w", err)
				return false, nil
			}
			if !apiequality.Semantic.DeepEqual(configmap.Data, originConfigMap.Data) {
				lastErr = fmt.Errorf("the actual configmap (%v) does not match the original (%v)",
					configmap.Data, originConfigMap.Data)
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			Fail(fmt.Sprintf("failed to restore console configmap: %v LastErr: %v", err, lastErr))
		}
	})

})
