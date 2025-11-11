package appserver

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
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
	//+kubebuilder:scaffold:imports
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

func TestAppserver(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Appserver Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", ".testcrds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = olsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = consolev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = openshiftv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = monv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = configv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx = context.Background()

	By("Create the ClusterVersion object")
	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "foobar",
		},
	}
	err = k8sClient.Create(context.TODO(), clusterVersion)
	Expect(err).NotTo(HaveOccurred())

	clusterVersion.Status = configv1.ClusterVersionStatus{
		Desired: configv1.Release{
			Version: "123.456.789",
		},
	}
	err = k8sClient.Status().Update(context.TODO(), clusterVersion)
	Expect(err).NotTo(HaveOccurred())

	By("Create the namespace openshift-lightspeed")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSNamespaceDefault,
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("Create the namespace openshift-config")
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.TelemetryPullSecretNamespace,
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	testReconcilerInstance = utils.NewTestReconciler(
		k8sClient,
		logf.Log.WithName("controller").WithName("OLSConfig"),
		scheme.Scheme,
		utils.OLSNamespaceDefault,
	)

	// Set default images for test reconciler (can be overridden in specific tests)
	if tr, ok := testReconcilerInstance.(*utils.TestReconciler); ok {
		tr.AppServerImage = utils.OLSAppServerImageDefault
		tr.DataverseExporter = utils.DataverseExporterImageDefault
		tr.McpServerImage = utils.OpenShiftMCPServerImageDefault
	}

	cr = utils.GetDefaultOLSConfigCR()
	crNamespacedName = types.NamespacedName{
		Name: "cluster",
	}

	By("Create a complete OLSConfig custom resource")
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	if err != nil && errors.IsNotFound(err) {
		cr = utils.GetDefaultOLSConfigCR()
		err = k8sClient.Create(ctx, cr)
		Expect(err).NotTo(HaveOccurred())
	} else if err == nil {
		cr = utils.GetDefaultOLSConfigCR()
		err = k8sClient.Update(ctx, cr)
		Expect(err).NotTo(HaveOccurred())
	} else {
		Fail("Failed to create or update the OLSConfig custom resource")
	}

	By("Get the OLSConfig custom resource")
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	Expect(err).NotTo(HaveOccurred())

	By("Create the kube-root-ca.crt configmap")
	kubeRootCA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-root-ca.crt",
			Namespace: utils.OLSNamespaceDefault,
		},
		Data: map[string]string{
			"service-ca.crt": utils.TestCACert,
		},
	}
	err = k8sClient.Create(ctx, kubeRootCA)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Delete the namespace openshift-lightspeed")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSNamespaceDefault,
		},
	}
	err := k8sClient.Delete(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("tearing down the test environment")
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
