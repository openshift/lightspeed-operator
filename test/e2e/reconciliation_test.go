package e2e

import (
	"fmt"
	"path"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Reconciliation From OLSConfig CR", Ordered, func() {
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())

	})

	It("should setup application server", func() {

		By("make application server deployment running")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("exposing its HTTPS port in a service")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Spec.Ports).To(ContainElement(corev1.ServicePort{
			Name:       "https",
			Protocol:   corev1.ProtocolTCP,
			Port:       AppServerServiceHTTPSPort,
			TargetPort: intstr.FromString("https"),
		}))

	})

	It("should setup console plugin", func() {

		By("make console plugin deployment running")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("exposing its HTTPS port in a service")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConsolePluginServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Spec.Ports).To(ContainElement(corev1.ServicePort{
			Name:       "https",
			Protocol:   corev1.ProtocolTCP,
			Port:       OLSConsolePluginServiceHTTPSPort,
			TargetPort: intstr.FromString("https"),
		}))
	})

	It("should setup a cache", func() {
		// todo: implement this test after replacing redis with other solution
	})

	It("should reconcile app deployment after changing deployment settings", func() {

		By("update the replica number in the OLSConfig CR")
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())
		*cr.Spec.OLSConfig.DeploymentConfig.Replicas = *cr.Spec.OLSConfig.DeploymentConfig.Replicas + 1
		err = client.Update(cr)
		Expect(err).NotTo(HaveOccurred())

		By("check the replica number of the deployment that should be updated")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			if *dep.Spec.Replicas != *cr.Spec.OLSConfig.DeploymentConfig.Replicas {
				return false, nil
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should reconcile app configmap after changing application settings", func() {
		By("fetch the app deployment generation")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(deployment)
		Expect(err).NotTo(HaveOccurred())
		generation := deployment.Generation

		By("update LogLevel in the OLSConfig CR")
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())
		if cr.Spec.OLSConfig.LogLevel == "DEBUG" {
			cr.Spec.OLSConfig.LogLevel = "INFO"
		} else {
			cr.Spec.OLSConfig.LogLevel = "DEBUG"
		}
		err = client.Update(cr)
		Expect(err).NotTo(HaveOccurred())
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}

		By("wait for the app configmap to be updated")
		err = client.WaitForConfigMapContainString(configMap, AppServerConfigMapKey, "app_log_level: "+cr.Spec.OLSConfig.LogLevel)
		Expect(err).NotTo(HaveOccurred())

		By("check the app deployment generation that should be inscreased")
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			if dep.Generation <= generation {
				return false, fmt.Errorf("current generation %d is inferior to observed generation %d", dep.Generation, generation)
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
		generation = deployment.Generation

		By("update models in the OLSConfig CR")
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())

		cr.Spec.OLSConfig.DefaultModel = OpenAIAlternativeModel
		if !slices.Contains(cr.Spec.LLMConfig.Providers[0].Models, olsv1alpha1.ModelSpec{Name: OpenAIAlternativeModel}) {
			cr.Spec.LLMConfig.Providers[0].Models = append(cr.Spec.LLMConfig.Providers[0].Models, olsv1alpha1.ModelSpec{Name: OpenAIAlternativeModel})
		}

		err = client.Update(cr)
		Expect(err).NotTo(HaveOccurred())
		By("wait for the app configmap to be updated")
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForConfigMapContainString(configMap, AppServerConfigMapKey, "default_model: "+OpenAIAlternativeModel)
		Expect(err).NotTo(HaveOccurred())

		By("check the app deployment generation that should be inscreased")
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			if dep.Generation <= generation {
				return false, fmt.Errorf("current generation %d is inferior to observed generation %d", dep.Generation, generation)
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
		generation = deployment.Generation

		By("change LLM token secret reference")
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())

		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = LLMTokenSecondSecretName
		err = client.Update(cr)
		Expect(err).NotTo(HaveOccurred())

		By("check the app deployment generation that should be inscreased")
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			if dep.Generation <= generation {
				return false, fmt.Errorf("current generation %d is inferior to observed generation %d", dep.Generation, generation)
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("check the app configmap to contain the new secret volume")
		err = client.WaitForConfigMapContainString(configMap, AppServerConfigMapKey, path.Join("/etc/apikeys", LLMTokenSecondSecretName))
		Expect(err).NotTo(HaveOccurred())

		By("check the deployment to mounted the new secret volume")
		var secretVolumeDefaultMode = int32(420)
		Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
			Name: "secret-" + LLMTokenSecondSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  LLMTokenSecondSecretName,
					DefaultMode: &secretVolumeDefaultMode,
				},
			},
		}))

	})

})
