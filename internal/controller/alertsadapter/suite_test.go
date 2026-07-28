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

package alertsadapter

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

func TestAlertsAdapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerts Adapter Suite")
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
	err = consolev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = openshiftv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = monv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx = context.Background()

	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec:       configv1.ClusterVersionSpec{ClusterID: "foobar"},
	}
	err = k8sClient.Create(ctx, clusterVersion)
	Expect(err).NotTo(HaveOccurred())
	clusterVersion.Status = configv1.ClusterVersionStatus{
		Desired: configv1.Release{Version: "4.16.0"},
	}
	err = k8sClient.Status().Update(ctx, clusterVersion)
	Expect(err).NotTo(HaveOccurred())

	for _, ns := range []string{utils.OLSNamespaceDefault, utils.OpenShiftMonitoringNamespace, utils.TelemetryPullSecretNamespace} {
		err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		Expect(err).NotTo(HaveOccurred())
	}

	tr := utils.NewTestReconciler(
		k8sClient,
		logf.Log.WithName("controller").WithName("OLSConfig"),
		scheme.Scheme,
		utils.OLSNamespaceDefault,
	)
	tr.AlertsAdapterImage = "registry.redhat.io/openshift-lightspeed/lightspeed-agentic-alerts-adapter-rhel9:latest"
	testReconcilerInstance = tr

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
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
