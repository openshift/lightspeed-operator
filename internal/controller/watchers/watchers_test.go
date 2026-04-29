package watchers

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func TestWatchers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Watchers Suite")
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(olsv1alpha1.AddToScheme(s))
	return s
}

func createTestReconciler(objs ...client.Object) reconciler.Reconciler {
	s := testScheme()
	b := fake.NewClientBuilder().WithScheme(s)
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	fakeClient := b.Build()
	logger := zap.New(zap.UseDevMode(true))
	tr := utils.NewTestReconciler(fakeClient, logger, s, utils.OLSNamespaceDefault)

	watcherConfig := &utils.WatcherConfig{
		ConfigMaps: utils.ConfigMapWatcherConfig{
			SystemResources: []utils.SystemConfigMap{
				{
					Name:                utils.DefaultOpenShiftCerts,
					Namespace:           utils.OLSNamespaceDefault,
					AffectedDeployments: []string{"ACTIVE_BACKEND"},
					Description:         "test openshift CA",
				},
			},
		},
		Secrets: utils.SecretWatcherConfig{
			SystemResources: []utils.SystemSecret{
				{
					Namespace:           utils.TelemetryPullSecretNamespace,
					Name:                utils.TelemetryPullSecretName,
					AffectedDeployments: []string{utils.ConsoleUIDeploymentName},
					Description:         "test telemetry",
				},
			},
		},
		AnnotatedSecretMapping: map[string][]string{
			"mapped-secret": {utils.PostgresDeploymentName},
		},
		AnnotatedConfigMapMapping: map[string][]string{
			"mapped-cm": {utils.OLSAppServerDeploymentName},
		},
	}
	tr.SetWatcherConfig(watcherConfig)
	return tr
}

var _ = Describe("Watchers", func() {
	ctx := context.Background()

	Describe("isSecretReferencedInCR", func() {
		It("returns true for LLM credential secret referenced on the CR", func() {
			cr := utils.GetDefaultOLSConfigCR()
			Expect(isSecretReferencedInCR(cr, "test-secret")).To(BeTrue())
			Expect(isSecretReferencedInCR(cr, "not-referenced")).To(BeFalse())
		})
	})

	Describe("isConfigMapReferencedInCR", func() {
		It("returns true only when the CR references the configmap", func() {
			cr := utils.GetDefaultOLSConfigCR()
			Expect(isConfigMapReferencedInCR(cr, "any")).To(BeFalse())

			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: "extra-ca"}
			Expect(isConfigMapReferencedInCR(cr, "extra-ca")).To(BeTrue())
			Expect(isConfigMapReferencedInCR(cr, "other")).To(BeFalse())
		})
	})

	Describe("SecretWatcherFilter", func() {
		It("matches a system secret from WatcherConfig without calling restart when inCluster is false", func() {
			r := createTestReconciler()
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.TelemetryPullSecretNamespace,
					Name:      utils.TelemetryPullSecretName,
				},
				Data: map[string][]byte{".dockerconfigjson": []byte("{}")},
			}
			Expect(func() { SecretWatcherFilter(r, ctx, sec, false) }).NotTo(Panic())
		})

		It("matches an annotated secret using AnnotatedSecretMapping when inCluster is false", func() {
			r := createTestReconciler()
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      "mapped-secret",
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: "1",
					},
				},
				Data: map[string][]byte{"k": []byte("v")},
			}
			Expect(func() { SecretWatcherFilter(r, ctx, sec, false) }).NotTo(Panic())
		})

		It("uses default ACTIVE_BACKEND when annotation present but name not in mapping", func() {
			r := createTestReconciler()
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      "unmapped-secret",
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: "1",
					},
				},
				Data: map[string][]byte{"k": []byte("v")},
			}
			Expect(func() { SecretWatcherFilter(r, ctx, sec, false) }).NotTo(Panic())
		})
	})

	Describe("ConfigMapWatcherFilter", func() {
		It("matches a system configmap from WatcherConfig when inCluster is false", func() {
			r := createTestReconciler()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      utils.DefaultOpenShiftCerts,
				},
				Data: map[string]string{"ca-bundle.crt": "dummy"},
			}
			Expect(func() { ConfigMapWatcherFilter(r, ctx, cm, false) }).NotTo(Panic())
		})

		It("matches an annotated configmap using AnnotatedConfigMapMapping when inCluster is false", func() {
			r := createTestReconciler()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      "mapped-cm",
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: "1",
					},
				},
				Data: map[string]string{"k": "v"},
			}
			Expect(func() { ConfigMapWatcherFilter(r, ctx, cm, false) }).NotTo(Panic())
		})
	})

	Describe("restartDeployment", func() {
		It("skips unknown deployment keys without panicking", func() {
			r := createTestReconciler()
			Expect(func() {
				restartDeployment(r, ctx, []string{"not-a-real-deployment-key"}, utils.OLSNamespaceDefault, "res")
			}).NotTo(Panic())
		})
	})

	Describe("SecretUpdateHandler", func() {
		It("Update returns early when secret data is unchanged", func() {
			r := createTestReconciler()
			h := &SecretUpdateHandler{Reconciler: r}
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "s"},
				Data:       map[string][]byte{"k": []byte("same")},
			}
			other := sec.DeepCopy()
			h.Update(ctx, event.UpdateEvent{ObjectOld: sec, ObjectNew: other}, nil)
		})

		It("Create is a no-op for non-Secret objects", func() {
			r := createTestReconciler()
			h := &SecretUpdateHandler{Reconciler: r}
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
			h.Create(ctx, event.CreateEvent{Object: cm}, nil)
		})

		It("Create skips secrets owned by OLSConfig", func() {
			r := createTestReconciler()
			h := &SecretUpdateHandler{Reconciler: r}
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned",
					Namespace: "ns",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: utils.OLSConfigAPIVersion,
						Kind:       utils.OLSConfigKind,
						Name:       utils.OLSConfigName,
						UID:        "1",
					}},
				},
			}
			h.Create(ctx, event.CreateEvent{Object: sec}, nil)
		})

		It("Update runs when secret data changes (may log deployment get errors)", func() {
			cr := utils.GetDefaultOLSConfigCR()
			r := createTestReconciler(cr)
			h := &SecretUpdateHandler{Reconciler: r}
			oldS := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: utils.OLSNamespaceDefault, Name: "mapped-secret"},
				Data:       map[string][]byte{"k": []byte("old")},
			}
			utils.AnnotateSecretWatcher(oldS)
			newS := oldS.DeepCopy()
			newS.Data["k"] = []byte("new")
			utils.AnnotateSecretWatcher(newS)
			h.Update(ctx, event.UpdateEvent{ObjectOld: oldS, ObjectNew: newS}, nil)
		})

		It("Delete and Generic are no-ops", func() {
			r := createTestReconciler()
			h := &SecretUpdateHandler{Reconciler: r}
			sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
			h.Delete(ctx, event.DeleteEvent{Object: sec}, nil)
			h.Generic(ctx, event.GenericEvent{Object: sec}, nil)
		})
	})

	Describe("ConfigMapUpdateHandler", func() {
		It("Update returns early when data and binaryData are unchanged", func() {
			r := createTestReconciler()
			h := &ConfigMapUpdateHandler{Reconciler: r}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c"},
				Data:       map[string]string{"k": "v"},
			}
			h.Update(ctx, event.UpdateEvent{ObjectOld: cm, ObjectNew: cm.DeepCopy()}, nil)
		})

		It("Create is a no-op for non-ConfigMap objects", func() {
			r := createTestReconciler()
			h := &ConfigMapUpdateHandler{Reconciler: r}
			sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s"}}
			h.Create(ctx, event.CreateEvent{Object: sec}, nil)
		})

		It("Create skips configmaps owned by OLSConfig", func() {
			r := createTestReconciler()
			h := &ConfigMapUpdateHandler{Reconciler: r}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-cm",
					Namespace: "ns",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: utils.OLSConfigAPIVersion,
						Kind:       utils.OLSConfigKind,
						Name:       utils.OLSConfigName,
						UID:        "1",
					}},
				},
			}
			h.Create(ctx, event.CreateEvent{Object: cm}, nil)
		})

		It("Delete and Generic are no-ops", func() {
			r := createTestReconciler()
			h := &ConfigMapUpdateHandler{Reconciler: r}
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
			h.Delete(ctx, event.DeleteEvent{Object: cm}, nil)
			h.Generic(ctx, event.GenericEvent{Object: cm}, nil)
		})
	})

	Describe("SecretUpdateHandler Create annotates referenced external secret", func() {
		It("annotates and updates a referenced secret when OLSConfig exists", func() {
			cr := utils.GetDefaultOLSConfigCR()
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      "test-secret",
				},
				StringData: map[string]string{"x": "y"},
			}
			r := createTestReconciler(cr, sec)
			h := &SecretUpdateHandler{Reconciler: r}
			h.Create(ctx, event.CreateEvent{Object: sec}, nil)

			got := &corev1.Secret{}
			Expect(r.Get(ctx, client.ObjectKeyFromObject(sec), got)).To(Succeed())
			Expect(got.Annotations).To(HaveKey(utils.WatcherAnnotationKey))
		})
	})

	Describe("ConfigMapUpdateHandler Create annotates referenced external configmap", func() {
		It("annotates and updates a referenced configmap when OLSConfig exists", func() {
			cr := utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: "user-ca"}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.OLSNamespaceDefault,
					Name:      "user-ca",
				},
				Data: map[string]string{"ca": "pem"},
			}
			r := createTestReconciler(cr, cm)
			h := &ConfigMapUpdateHandler{Reconciler: r}
			h.Create(ctx, event.CreateEvent{Object: cm}, nil)

			got := &corev1.ConfigMap{}
			Expect(r.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
			Expect(got.Annotations).To(HaveKey(utils.WatcherAnnotationKey))
		})
	})

	Describe("restartDeployment with in-cluster restart", func() {
		It("resolves ACTIVE_BACKEND and attempts app server restart", func() {
			cr := utils.GetDefaultOLSConfigCR()
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OLSAppServerDeploymentName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ols"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ols"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
					},
				},
			}
			r := createTestReconciler(cr, dep)
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   utils.OLSNamespaceDefault,
					Name:        "annot",
					Annotations: map[string]string{utils.WatcherAnnotationKey: "1"},
				},
				Data: map[string][]byte{"k": []byte("v")},
			}
			SecretWatcherFilter(r, ctx, sec, true)

			updated := &appsv1.Deployment{}
			Expect(r.Get(ctx, client.ObjectKeyFromObject(dep), updated)).To(Succeed())
			Expect(updated.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})
	})
})
