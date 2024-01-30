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

package main

import (
	"flag"
	"fmt"
	"os"
	"slices"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	k8sflag "k8s.io/component-base/cli/flag"
	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	// The default images of operands
	defaultImages = map[string]string{
		"lightspeed-service": controller.OLSAppServerImageDefault,
	}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(olsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// validateImages overides the default images with the images provided by the user
// if the images are not provided, the default images are used.
func validateImages(images *k8sflag.MapStringString) (map[string]string, error) {
	res := defaultImages
	if images.Empty() {
		return res, nil
	}
	imgs := *images.Map
	for k, v := range imgs {
		if _, ok := res[k]; !ok {
			return nil, fmt.Errorf("image %v is unknown", k)
		}
		res[k] = v
	}
	return res, nil
}

func listImages() []string {
	i := 0
	imgs := make([]string, len(defaultImages))
	for k, v := range defaultImages {
		imgs[i] = fmt.Sprintf("%v=%v", k, v)
		i++
	}
	slices.Sort(imgs)
	return imgs
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(controller.WatchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", controller.WatchNamespaceEnvVar)
	}
	return ns, nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	images := k8sflag.NewMapStringString(ptr.To(make(map[string]string)))
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Var(images, "images", fmt.Sprintf("Full images refs to use for containers managed by the operator. E.g lightspeed-service=quay.io/openshift/lightspeed-service-api:latest. Images used are %v", listImages()))
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	watchNamespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err,
			fmt.Sprintf("%s is not set, will watch all namespaces", controller.WatchNamespaceEnvVar))
	} else {
		setupLog.Info("watching namespace", "namespace", watchNamespace)
	}

	imagesMap, err := validateImages(images)
	if err != nil {
		setupLog.Error(err, "invalid images")
		os.Exit(1)
	}
	setupLog.Info("Images setting loaded", "images", listImages())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "0ca034e3.openshift.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
		Namespace: watchNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.OLSConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Options: controller.OLSConfigReconcilerOptions{
			LightspeedServiceImage: imagesMap["lightspeed-service"],
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OLSConfig")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
