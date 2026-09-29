package agenticconsole

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoveAgenticConsoleDeletesOperandResources(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{consolev1.AddToScheme, operatorv1.AddToScheme, appsv1.AddToScheme, corev1.AddToScheme, networkingv1.AddToScheme} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	ns := utils.OLSNamespaceDefault
	name := utils.AgenticConsoleUIPluginName
	objects := []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIServiceCertSecretName, Namespace: ns}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		&consolev1.ConsolePlugin{ObjectMeta: metav1.ObjectMeta{Name: name}},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
	r := utils.NewTestReconciler(c, logr.Discard(), s, ns)
	if err := RemoveAgenticConsole(r, ctx); err != nil {
		t.Fatal(err)
	}
	for _, obj := range objects {
		check := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), check); !apierrors.IsNotFound(err) {
			t.Fatalf("%T %q not removed: %v", obj, obj.GetName(), err)
		}
	}
	if err := RemoveAgenticConsole(r, ctx); err != nil {
		t.Fatalf("cleanup must be idempotent: %v", err)
	}
}
