package e2e

import (
	"fmt"
	"path"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - All tests share a single cluster-scoped OLSConfig CR
// - Uses DeleteAndWait in AfterAll to prevent resource pollution between test suites
// - Tests verify operator reconciliation behavior by modifying the CR and checking results
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
		err = mustGather("reconciliation_test")
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR and waiting for cleanup")
		Expect(cr).NotTo(BeNil())
		err = client.DeleteAndWait(cr, 3*time.Minute)
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
		err = client.Update(cr, func(obj ctrlclient.Object) error {
			cr := obj.(*olsv1alpha1.OLSConfig)
			*cr.Spec.OLSConfig.DeploymentConfig.Replicas = *cr.Spec.OLSConfig.DeploymentConfig.Replicas + 1
			return nil
		})
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
		var generation int64
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err := client.Get(deployment)
		Expect(err).NotTo(HaveOccurred())
		generation = deployment.Generation

		By("update LogLevel in the OLSConfig CR")
		err = client.Update(cr, func(obj ctrlclient.Object) error {
			cr := obj.(*olsv1alpha1.OLSConfig)
			if cr.Spec.OLSConfig.LogLevel == olsv1alpha1.LogLevelDebug {
				cr.Spec.OLSConfig.LogLevel = olsv1alpha1.LogLevelInfo
			} else {
				cr.Spec.OLSConfig.LogLevel = olsv1alpha1.LogLevelDebug
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}

		By("wait for the app configmap to be updated")
		err = client.WaitForConfigMapContainString(configMap, AppServerConfigMapKey, "app_log_level: "+string(cr.Spec.OLSConfig.LogLevel))
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
		err = client.Update(cr, func(obj ctrlclient.Object) error {
			cr := obj.(*olsv1alpha1.OLSConfig)
			cr.Spec.OLSConfig.DefaultModel = OpenAIAlternativeModel
			if !slices.Contains(cr.Spec.LLMConfig.Providers[0].Models, olsv1alpha1.ModelSpec{Name: OpenAIAlternativeModel}) {
				cr.Spec.LLMConfig.Providers[0].Models = append(cr.Spec.LLMConfig.Providers[0].Models, olsv1alpha1.ModelSpec{Name: OpenAIAlternativeModel})
			}
			return nil
		})
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
		err = client.Update(cr, func(obj ctrlclient.Object) error {
			cr := obj.(*olsv1alpha1.OLSConfig)
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = LLMTokenSecondSecretName
			return nil
		})
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

	It("should setup CA cert volumes and app configs after setting additional CA", func() {
		const (
			cmCACert1Name   = "ca-cert-1"
			caCert1FileName = "ca-cert-1.crt"
			caCert2FileName = "ca-cert-2.crt"
		)
		By("create additional CA configmap")
		caCertConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmCACert1Name,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				caCert1FileName: TestCACert,
			},
		}
		err = client.Create(caCertConfigMap)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			err = client.DeleteAndWait(caCertConfigMap, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		}()

		By("update additional CA in the OLSConfig CR")
		err = client.Update(cr, func(obj ctrlclient.Object) error {
			cr := obj.(*olsv1alpha1.OLSConfig)
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: cmCACert1Name,
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("check the OLS configmap to contain the additional CA cert")
		olsConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerConfigMapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForConfigMapContainString(olsConfigMap, AppServerConfigMapKey, "/etc/certs/ols-user-ca/"+caCert1FileName)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForConfigMapContainString(olsConfigMap, AppServerConfigMapKey, "certificate_directory: /etc/certs/cert-bundle")
		Expect(err).NotTo(HaveOccurred())

		By("check the app deployment to mount the additional CA cert configmap")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		volumeDefaultMode := int32(420)
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			var certVolumeExist, certBundleVolumeExist, certVolumeMountExist, certBundleVolumeMountExist bool
			for _, volume := range dep.Spec.Template.Spec.Volumes {
				if apiequality.Semantic.DeepEqual(volume, corev1.Volume{
					Name: AdditionalCAVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cmCACert1Name,
							},
							DefaultMode: &volumeDefaultMode,
						},
					},
				}) {
					certVolumeExist = true
				}
				if apiequality.Semantic.DeepEqual(volume, corev1.Volume{
					Name: CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}) {
					certBundleVolumeExist = true
				}

			}
			for _, volumeMount := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
				if apiequality.Semantic.DeepEqual(volumeMount, corev1.VolumeMount{
					Name:      AdditionalCAVolumeName,
					MountPath: path.Join(OLSAppCertsMountRoot, UserCACertDir),
					ReadOnly:  true,
				}) {
					certVolumeMountExist = true
				}
				if apiequality.Semantic.DeepEqual(volumeMount, corev1.VolumeMount{
					Name:      CertBundleVolumeName,
					MountPath: path.Join(OLSAppCertsMountRoot, CertBundleDir),
					ReadOnly:  false,
				}) {
					certBundleVolumeMountExist = true
				}
			}

			return certVolumeExist && certBundleVolumeExist && certVolumeMountExist && certBundleVolumeMountExist, nil
		})
		Expect(err).NotTo(HaveOccurred())
		firstCmHash := deployment.Spec.Template.Annotations[AdditionalCAHashKey]

		By("check the app deployment and OLS config adapted to modified CA cert configmap")
		err = client.Update(caCertConfigMap, func(obj ctrlclient.Object) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Data[caCert2FileName] = TestCACert
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForConfigMapContainString(olsConfigMap, AppServerConfigMapKey, "/etc/certs/ols-user-ca/"+caCert2FileName)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			newCmHash := dep.Spec.Template.Annotations[AdditionalCAHashKey]
			return newCmHash != firstCmHash, nil
		})

	})

})
