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
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"slices"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	configv1 "github.com/openshift/api/config/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/lightspeed-operator/internal/controller"
	utiltls "github.com/openshift/lightspeed-operator/internal/tls"
	"github.com/openshift/lightspeed-operator/internal/webhook"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	// The default images of operands
	defaultImages = map[string]string{
		"lightspeed-service":         controller.OLSAppServerImageDefault,
		"postgres-image":             controller.PostgresServerImageDefault,
		"console-plugin":             controller.ConsoleUIImageDefault,
		"openshift-mcp-server-image": controller.OpenShiftMCPServerImageDefault,
	}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(openshiftv1.AddToScheme(scheme))
	utilruntime.Must(monv1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))

	utilruntime.Must(olsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// overrideImages overides the default images with the images provided by the user
// if an images is not provided, the default is used.
func overrideImages(serviceImage string, consoleImage string, postgresImage string, openshiftMCPServerImage string) map[string]string {
	res := defaultImages
	if serviceImage != "" {
		res["lightspeed-service"] = serviceImage
	}
	if consoleImage != "" {
		res["console-plugin"] = consoleImage
	}
	if postgresImage != "" {
		res["postgres-image"] = postgresImage
	}
	if openshiftMCPServerImage != "" {
		res["openshift-mcp-server-image"] = openshiftMCPServerImage
	}
	return res
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

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var reconcilerIntervalMinutes uint
	var secureMetricsServer bool
	var certDir string
	var certName string
	var keyName string
	var caCertPath string
	var metricsClientCA string
	var serviceImage string
	var consoleImage string
	var namespace string
	var postgresImage string
	var openshiftMCPServerImage string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.UintVar(&reconcilerIntervalMinutes, "reconcile-interval", controller.DefaultReconcileInterval, "The interval in minutes to reconcile the OLSConfig CR")
	flag.BoolVar(&secureMetricsServer, "secure-metrics-server", false, "Enable secure serving of the metrics server using mTLS.")
	flag.StringVar(&certDir, "cert-dir", controller.OperatorCertDirDefault, "The directory where the TLS certificates are stored.")
	flag.StringVar(&certName, "cert-name", controller.OperatorCertNameDefault, "The name of the TLS certificate file.")
	flag.StringVar(&keyName, "key-name", controller.OperatorKeyNameDefault, "The name of the TLS key file.")
	flag.StringVar(&caCertPath, "ca-cert", controller.OperatorCACertPathDefault, "The path to the CA certificate file.")
	flag.StringVar(&serviceImage, "service-image", controller.OLSAppServerImageDefault, "The image of the lightspeed-service container.")
	flag.StringVar(&consoleImage, "console-image", controller.ConsoleUIImageDefault, "The image of the console-plugin container.")
	flag.StringVar(&namespace, "namespace", "", "The namespace where the operator is deployed.")
	flag.StringVar(&postgresImage, "postgres-image", controller.PostgresServerImageDefault, "The image of the PostgreSQL server.")
	flag.StringVar(&openshiftMCPServerImage, "openshift-mcp-server-image", controller.OpenShiftMCPServerImageDefault, "The image of the OpenShift MCP server container.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if namespace == "" {
		namespace = getWatchNamespace()
	}

	imagesMap := overrideImages(serviceImage, consoleImage, postgresImage, openshiftMCPServerImage)
	setupLog.Info("Images setting loaded", "images", listImages())
	setupLog.Info("Starting the operator", "metricsAddr", metricsAddr, "probeAddr", probeAddr, "reconcilerIntervalMinutes", reconcilerIntervalMinutes, "certDir", certDir, "certName", certName, "keyName", keyName, "namespace", namespace)

	var tlsSecurityProfileSpec configv1.TLSProfileSpec
	if secureMetricsServer {
		cfg, err := config.GetConfig()
		if err != nil {
			setupLog.Error(err, "unable to get Kubernetes config")
			os.Exit(1)
		}

		k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "unable to create Kubernetes client")
			os.Exit(1)
		}

		ctx := context.Background()
		apiAuthConfigmap := &corev1.ConfigMap{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: controller.ClientCACmName, Namespace: controller.ClientCACmNamespace}, apiAuthConfigmap)
		if err != nil {
			setupLog.Error(err, fmt.Sprintf("failed to get %s/%s configmap.", controller.ClientCACmNamespace, controller.ClientCACmName))
			os.Exit(1)
		}
		var exists bool
		metricsClientCA, exists = apiAuthConfigmap.Data[controller.ClientCACertKey]
		if !exists {
			setupLog.Error(err, fmt.Sprintf("the key %s is not found in %s/%s configmap.", controller.ClientCACertKey, controller.ClientCACmNamespace, controller.ClientCACmName))
			os.Exit(1)
		}

		olsconfig := &olsv1alpha1.OLSConfig{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: controller.OLSConfigName}, olsconfig)
		if err != nil && client.IgnoreNotFound(err) != nil {
			setupLog.Error(err, fmt.Sprintf("failed to get %s OLSConfig.", controller.OLSConfigName))
			os.Exit(1)
		}
		if olsconfig.Spec.OLSConfig.TLSSecurityProfile != nil {
			tlsSecurityProfileSpec = utiltls.GetTLSProfileSpec(olsconfig.Spec.OLSConfig.TLSSecurityProfile)
		} else {
			setupLog.Info("TLS profile is not defined in OLSConfig, fetch from API server")
			profileAPIServer, err := utiltls.FetchAPIServerTlsProfile(k8sClient)
			if err != nil {
				setupLog.Error(err, "unable to get TLS profile from API server")
				os.Exit(1)
			}
			tlsSecurityProfileSpec = utiltls.GetTLSProfileSpec(profileAPIServer)
		}

	}

	metricsTLSSetup := func(tlsConf *tls.Config) {
		if !secureMetricsServer {
			return
		}
		tlsConf.ClientCAs = x509.NewCertPool()
		tlsConf.ClientCAs.AppendCertsFromPEM([]byte(metricsClientCA))
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConf.MinVersion = utiltls.VersionCode(configv1.TLSProtocolVersion(utiltls.MinTLSVersion(tlsSecurityProfileSpec)))
		ciphers, unsupportedCiphers := utiltls.CipherCodes(utiltls.TLSCiphers(tlsSecurityProfileSpec))
		tlsConf.CipherSuites = ciphers
		if len(unsupportedCiphers) > 0 {
			setupLog.Info("TLS setup for metrics server contains unsupported ciphers", "unsupportedCiphers", unsupportedCiphers)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			SecureServing: secureMetricsServer,
			BindAddress:   metricsAddr,
			CertDir:       certDir,
			CertName:      certName,
			KeyName:       keyName,
			TLSOpts:       []func(*tls.Config){metricsTLSSetup},
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "0ca034e3.openshift.io",
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Namespaces: map[string]cache.Config{
						namespace:                               {},
						controller.TelemetryPullSecretNamespace: {},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.OLSConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Options: controller.OLSConfigReconcilerOptions{
			LightspeedServiceImage:         imagesMap["lightspeed-service"],
			ConsoleUIImage:                 imagesMap["console-plugin"],
			LightspeedServicePostgresImage: imagesMap["postgres-image"],
			OpenShiftMCPServerImage:        imagesMap["openshift-mcp-server-image"],
			Namespace:                      namespace,
			ReconcileInterval:              time.Duration(reconcilerIntervalMinutes) * time.Minute, // #nosec G115
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OLSConfig")
		os.Exit(1)
	}
	if err = webhook.SetupWebhookWithManager(mgr, mgr.GetClient(), namespace); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "OLSConfig")
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

// get the namespace to watch or use the default namespace
func getWatchNamespace() string {
	ns, found := os.LookupEnv("WATCH_NAMESPACE")
	if !found {
		return controller.OLSNamespaceDefault
	}
	return ns
}
