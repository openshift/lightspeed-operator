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

package controller

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
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx              context.Context
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	cr               *olsv1alpha1.OLSConfig
	reconciler       *OLSConfigReconciler
	crNamespacedName types.NamespacedName
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", ".testcrds"),
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
			Name: OLSNamespaceDefault,
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("Create the namespace openshift-config")
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift-config",
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	reconciler = &OLSConfigReconciler{
		Options: OLSConfigReconcilerOptions{
			OpenShiftMajor:                 "123",
			OpenshiftMinor:                 "456",
			LightspeedServiceImage:         OLSAppServerImageDefault,
			LightspeedServicePostgresImage: PostgresServerImageDefault,
			ConsoleUIImage:                 ConsoleUIImageDefault,
			Namespace:                      OLSNamespaceDefault,
		},
		logger:     logf.Log.WithName("olsconfig.reconciler"),
		Client:     k8sClient,
		Scheme:     k8sClient.Scheme(),
		stateCache: make(map[string]string),
	}
	cr = &olsv1alpha1.OLSConfig{}
	crNamespacedName = types.NamespacedName{
		Name: "cluster",
	}

	By("Create a complete OLSConfig custom resource")
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	if err != nil && errors.IsNotFound(err) {
		cr = getDefaultOLSConfigCR()
		err = k8sClient.Create(ctx, cr)
		Expect(err).NotTo(HaveOccurred())
	} else if err == nil {
		cr = getDefaultOLSConfigCR()
		err = k8sClient.Update(ctx, cr)
		Expect(err).NotTo(HaveOccurred())
	} else {
		Fail("Failed to create or update the OLSConfig custom resource")
	}

	By("Get the OLSConfig custom resource")
	err = k8sClient.Get(ctx, crNamespacedName, cr)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Delete the namespace openshift-lightspeed")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSNamespaceDefault,
		},
	}
	err := k8sClient.Delete(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("tearing down the test environment")
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
