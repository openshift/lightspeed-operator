package controller

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("App redis server assets", func() {

	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions

	validateRedisDeployment := func(dep *appsv1.Deployment, password string) {
		replicas := int32(1)
		revisionHistoryLimit := int32(1)
		Expect(dep.Name).To(Equal(RedisDeploymentName))
		Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceRedisImage))
		Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-redis-server"))
		Expect(dep.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
			{
				ContainerPort: RedisServicePort,
				Name:          "server",
			},
		}))
		Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}))
		Expect(dep.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"redis-server",
			"--port", "0",
			"--tls-port", "6379",
			"--tls-cert-file", path.Join(OLSAppCertsMountRoot, "tls.crt"),
			"--tls-key-file", path.Join(OLSAppCertsMountRoot, "tls.key"),
			"--tls-ca-cert-file", path.Join(OLSAppCertsMountRoot, RedisCAVolume, "service-ca.crt"),
			"--tls-auth-clients", "optional",
			"--protected-mode", "no",
			"--requirepass", password},
		))
		Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateRedisSelectorLabels()))
		Expect(dep.Spec.RevisionHistoryLimit).To(Equal(&revisionHistoryLimit))
		Expect(dep.Spec.Replicas).To(Equal(&replicas))
		Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal([]corev1.VolumeMount{
			{
				Name:      "secret-" + RedisCertsSecretName,
				MountPath: OLSAppCertsMountRoot,
				ReadOnly:  true,
			},
			{
				Name:      RedisCAVolume,
				MountPath: path.Join(OLSAppCertsMountRoot, RedisCAVolume),
				ReadOnly:  true,
			},
		}))
		Expect(dep.Spec.Template.Spec.Volumes).To(Equal([]corev1.Volume{
			{
				Name: "secret-" + RedisCertsSecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: RedisCertsSecretName,
					},
				},
			},
			{
				Name: RedisCAVolume,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: RedisCAConfigMap},
					},
				},
			},
		}))
	}

	validateRedisService := func(service *corev1.Service, err error) {
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Name).To(Equal(RedisServiceName))
		Expect(service.Namespace).To(Equal(OLSNamespaceDefault))
		Expect(service.Labels).To(Equal(generateRedisSelectorLabels()))
		Expect(service.Annotations).To(Equal(map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": RedisCertsSecretName,
		}))
		Expect(service.Spec.Selector).To(Equal(generateRedisSelectorLabels()))
		Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
			{
				Name:       "server",
				Port:       RedisServicePort,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.Parse("server"),
			},
		}))
	}

	validateRedisSecret := func(secret *corev1.Secret) {
		Expect(secret.Namespace).To(Equal(cr.Namespace))
		Expect(secret.Labels).To(Equal(generateRedisSelectorLabels()))
		Expect(secret.Annotations).To(HaveKey(RedisSecretHashKey))
		Expect(secret.Data).To(HaveKey(RedisSecretKeyName))
	}

	Context("complete custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceRedisImage: "lightspeed-service-redis:latest",
				Namespace:                   OLSNamespaceDefault,
			}
			cr = getOLSConfigWithCacheCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
		})

		It("should generate the OLS redis deployment", func() {
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = "dummy-secret-1"
			secret, _ := r.generateRedisSecret(cr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-1",
				},
			})
			secretCreationErr := r.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			secretValues, _ := getSecretContent(r.Client, secret.Name, cr.Namespace, []string{OLSComponentPasswordFileName})
			deployment, err := r.generateRedisDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			validateRedisDeployment(deployment, secretValues[OLSComponentPasswordFileName])
			secretDeletionErr := r.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should work when no update in the OLS redis deployment", func() {
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = "dummy-secret-2"
			secret, _ := r.generateRedisSecret(cr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-2",
				},
			})
			secretCreationErr := r.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			deployment, err := r.generateRedisDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			deployment.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
					UID:        "ownerUID",
					Name:       "lightspeed-redis-server-1",
				},
			})
			deployment.ObjectMeta.Name = "lightspeed-redis-server-1"
			deploymentCreationErr := r.Create(ctx, deployment)
			Expect(deploymentCreationErr).NotTo(HaveOccurred())
			updateErr := r.updateRedisDeployment(ctx, deployment, deployment)
			Expect(updateErr).NotTo(HaveOccurred())
			secretDeletionErr := r.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			deploymentDeletionErr := r.Delete(ctx, deployment)
			Expect(deploymentDeletionErr).NotTo(HaveOccurred())
		})

		It("should work when there is an update in the OLS redis deployment", func() {
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = "dummy-secret-3"
			secret, _ := r.generateRedisSecret(cr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-3",
				},
			})
			secretCreationErr := r.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			deployment, err := r.generateRedisDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			deployment.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
					UID:        "ownerUID",
					Name:       "lightspeed-redis-server-2",
				},
			})
			deployment.ObjectMeta.Name = "lightspeed-redis-server-2"
			deploymentCreationErr := r.Create(ctx, deployment)
			Expect(deploymentCreationErr).NotTo(HaveOccurred())
			deploymentClone := deployment.DeepCopy()
			deploymentClone.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "infinity"}
			updateErr := r.updateRedisDeployment(ctx, deployment, deploymentClone)
			Expect(updateErr).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"sleep", "infinity"}))
			secretDeletionErr := r.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			deploymentDeletionErr := r.Delete(ctx, deployment)
			Expect(deploymentDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate the OLS redis service", func() {
			validateRedisService(r.generateRedisService(cr))
		})

		It("should generate the OLS redis secret", func() {
			secret, err := r.generateRedisSecret(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal("lightspeed-redis-secret"))
			validateRedisSecret(secret)
		})
	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceRedisImage: "lightspeed-service-redis:latest",
				Namespace:                   OLSNamespaceDefault,
			}
			cr = getNoCacheCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
		})

		It("should generate the OLS redis deployment", func() {
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = "dummy-secret-4"
			secret, _ := r.generateRedisSecret(cr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-4",
				},
			})
			secretCreationErr := r.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			secretValues, _ := getSecretContent(r.Client, secret.Name, cr.Namespace, []string{OLSComponentPasswordFileName})
			deployment, err := r.generateRedisDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			validateRedisDeployment(deployment, secretValues[OLSComponentPasswordFileName])
			secretDeletionErr := r.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate the OLS redis service", func() {
			validateRedisService(r.generateRedisService(cr))
		})

		It("should generate the OLS redis secret", func() {
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = RedisSecretName
			secret, err := r.generateRedisSecret(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal("lightspeed-redis-secret"))
			validateRedisSecret(secret)
		})
	})

})

func getOLSConfigWithCacheCR() *olsv1alpha1.OLSConfig {
	OLSRedisMaxMemory := intstr.FromString(RedisMaxMemory)
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: OLSNamespaceDefault,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Redis,
					Redis: olsv1alpha1.RedisSpec{
						MaxMemory:         &OLSRedisMaxMemory,
						MaxMemoryPolicy:   RedisMaxMemoryPolicy,
						CredentialsSecret: RedisSecretName,
					},
				},
			},
		},
	}
}

func getNoCacheCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: OLSNamespaceDefault,
		},
	}

}
