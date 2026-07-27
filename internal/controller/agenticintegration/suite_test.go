/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agenticintegration

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var (
	ctx                    context.Context
	cfg                    *rest.Config
	k8sClient              client.Client
	testEnv                *envtest.Environment
	cr                     *olsv1alpha1.OLSConfig
	testReconcilerInstance reconciler.Reconciler
	crNamespacedName       types.NamespacedName
)

func TestAgenticIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agentic Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", ".testcrds"),
		},
		CRDInstallOptions: envtest.CRDInstallOptions{
			MaxTime: utils.EnvTestCRDInstallMaxTime,
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = olsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = configv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx = context.Background()

	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: utils.OLSNamespaceDefault}})
	Expect(err).NotTo(HaveOccurred())

	testReconcilerInstance = utils.NewTestReconciler(
		k8sClient,
		logf.Log.WithName("controller").WithName("OLSConfig"),
		scheme.Scheme,
		utils.OLSNamespaceDefault,
	)

	cr = &olsv1alpha1.OLSConfig{}
	crNamespacedName = types.NamespacedName{Name: utils.OLSConfigName}
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	if err != nil && errors.IsNotFound(err) {
		cr = utils.GetDefaultOLSConfigCR()
		err = k8sClient.Create(ctx, cr)
		Expect(err).NotTo(HaveOccurred())
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// ensureHandoffCreatePrerequisites stubs OTEL (and optional MCP) Services plus
// client CA Secrets required before the first handoff ConfigMap create.
func ensureHandoffCreatePrerequisites(introspectionEnabled bool) {
	ns := utils.OLSNamespaceDefault
	otelSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: utils.OtelCollectorServiceName, Namespace: ns},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "otel"},
			Ports:    []corev1.ServicePort{{Name: "otlp-grpc", Port: int32(utils.OtelCollectorGRPCPort)}},
		},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, otelSvc))).NotTo(HaveOccurred())

	otelCA := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticOtelCASecretName, Namespace: ns},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{utils.AgenticOtelCASecretDataKey: []byte(utils.TestCACert)},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, otelCA))).NotTo(HaveOccurred())

	if !introspectionEnabled {
		return
	}

	mcpSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerServiceName, Namespace: ns},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "mcp"},
			Ports:    []corev1.ServicePort{{Name: "https", Port: int32(utils.OpenShiftMCPServerHTTPSPort)}},
		},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, mcpSvc))).NotTo(HaveOccurred())

	mcpCA := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticMCPCASecretName, Namespace: ns},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{utils.AgenticMCPCASecretDataKey: []byte(utils.TestCACert)},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, mcpCA))).NotTo(HaveOccurred())
}

func expectOwnedByOLSConfig(obj metav1.Object) {
	olsConfig := &olsv1alpha1.OLSConfig{}
	Expect(k8sClient.Get(ctx, crNamespacedName, olsConfig)).To(Succeed())

	var ownerRef *metav1.OwnerReference
	for i := range obj.GetOwnerReferences() {
		ref := &obj.GetOwnerReferences()[i]
		if ref.APIVersion == utils.OLSConfigAPIVersion &&
			ref.Kind == utils.OLSConfigKind &&
			ref.Name == olsConfig.Name {
			ownerRef = ref
			break
		}
	}
	Expect(ownerRef).NotTo(BeNil(), "expected %T %s to be owned by OLSConfig", obj, obj.GetName())
	Expect(ownerRef.Name).To(Equal(olsConfig.Name))
}
