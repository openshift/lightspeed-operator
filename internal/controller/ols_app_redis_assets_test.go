package controller

import (
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
	deploymentSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "redis-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-redis",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}

	validateRedisDeployment := func(dep *appsv1.Deployment, err error) {
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Name).To(Equal(OLSAppRedisDeploymentName))
		Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceRedisImage))
		Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-redis-server"))
		Expect(dep.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
			{
				ContainerPort: OLSAppRedisServicePort,
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
		Expect(dep.Spec.Selector.MatchLabels).To(Equal(deploymentSelectorLabels))
	}

	validateRedisService := func(service *corev1.Service, err error) {
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Name).To(Equal(OLSAppRedisServiceName))
		Expect(service.Namespace).To(Equal(OLSNamespaceDefault))
		Expect(service.Spec.Selector).To(Equal(deploymentSelectorLabels))
		Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
			{
				Name:       "server",
				Port:       OLSAppRedisServicePort,
				TargetPort: intstr.Parse("server"),
			},
		}))
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
			validateRedisDeployment(r.generateRedisDeployment(cr))

		})

		It("should generate the OLS redis service", func() {
			validateRedisService(r.generateRedisService(cr))
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
			validateRedisDeployment(r.generateRedisDeployment(cr))

		})

		It("should generate the OLS redis service", func() {
			validateRedisService(r.generateRedisService(cr))
		})
	})

})

func getOLSConfigWithCacheCR() *olsv1alpha1.OLSConfig {
	OLSRedisMaxMemory := intstr.FromString(OLSAppRedisMaxMemory)
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
						MaxMemory:       &OLSRedisMaxMemory,
						MaxMemoryPolicy: OLSAppRedisMaxMemoryPolicy,
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
