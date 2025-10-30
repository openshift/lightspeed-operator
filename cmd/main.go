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

// Package main is the entry point for the OpenShift Lightspeed Operator.
//
// This package initializes and starts the Kubernetes controller manager that
// manages the lifecycle of the OpenShift Lightspeed application.
//
// The main function performs the following initialization:
//   - Parses command-line flags for configuration (image URLs, namespaces, intervals)
//   - Sets up the Kubernetes scheme with required API types (Console, Monitoring, etc.)
//   - Configures the controller manager with metrics, health probes, and leader election
//   - Detects OpenShift version and selects appropriate console plugin image
//   - Configures TLS security for metrics server (if enabled)
//   - Initializes and starts the OLSConfigReconciler
//
// Command-line Flags:
//   - metrics-bind-address: Address for metrics endpoint (default: :8080)
//   - health-probe-bind-address: Address for health probe endpoint (default: :8081)
//   - leader-elect: Enable leader election for HA deployments
//   - reconcile-interval: Interval in minutes for reconciliation (default: 10)
//   - secure-metrics-server: Enable mTLS for metrics server
//   - service-image: Override default lightspeed-service image
//   - console-image: Override default console plugin image (PatternFly 6)
//   - console-image-pf5: Override default console plugin image (PatternFly 5)
//   - postgres-image: Override default PostgreSQL image
//   - openshift-mcp-server-image: Override default MCP server image
//   - namespace: Operator namespace (defaults to WATCH_NAMESPACE env var or "openshift-lightspeed")
//
// Environment Variables:
//   - WATCH_NAMESPACE: Namespace to watch for OLSConfig resources
//
// The operator runs as a singleton in the cluster (with optional leader election)
// and continuously reconciles the OLSConfig custom resource to maintain the
// desired state of all OpenShift Lightspeed components.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
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
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	utiltls "github.com/openshift/lightspeed-operator/internal/tls"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	// The default images of operands
	defaultImages = map[string]string{
		"lightspeed-service":         utils.OLSAppServerImageDefault,
		"postgres-image":             utils.PostgresServerImageDefault,
		"console-plugin":             utils.ConsoleUIImageDefault,
		"console-plugin-pf5":         utils.ConsoleUIImagePF5Default,
		"openshift-mcp-server-image": utils.OpenShiftMCPServerImageDefault,
		"lightspeed-core":            utils.LlamaStackImageDefault,
		"dataverse-exporter-image":   utils.DataverseExporterImageDefault,
		"ocp-rag-image":              utils.OcpRagImageDefault,
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
func overrideImages(serviceImage string, consoleImage string, consoleImage_pf5 string, postgresImage string, openshiftMCPServerImage string, lcoreImage string, dataverseExporterImage string, ocpRagImage string) map[string]string {
	res := defaultImages
	if serviceImage != "" {
		res["lightspeed-service"] = serviceImage
	}
	if consoleImage != "" {
		res["console-plugin"] = consoleImage
	}
	if consoleImage_pf5 != "" {
		res["console-plugin-pf5"] = consoleImage_pf5
	}
	if postgresImage != "" {
		res["postgres-image"] = postgresImage
	}
	if openshiftMCPServerImage != "" {
		res["openshift-mcp-server-image"] = openshiftMCPServerImage
	}
	if lcoreImage != "" {
		res["lightspeed-core"] = lcoreImage
	}
	if dataverseExporterImage != "" {
		res["dataverse-exporter-image"] = dataverseExporterImage
	}
	if ocpRagImage != "" {
		res["ocp-rag-image"] = ocpRagImage
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
	var consoleImage_pf5 string
	var namespace string
	var postgresImage string
	var openshiftMCPServerImage string
	var lcoreImage string
	var dataverseExporterImage string
	var ocpRagImage string
	var useLCore bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.UintVar(&reconcilerIntervalMinutes, "reconcile-interval", utils.DefaultReconcileInterval, "The interval in minutes to reconcile the OLSConfig CR")
	flag.BoolVar(&secureMetricsServer, "secure-metrics-server", false, "Enable secure serving of the metrics server using mTLS.")
	flag.StringVar(&certDir, "cert-dir", utils.OperatorCertDirDefault, "The directory where the TLS certificates are stored.")
	flag.StringVar(&certName, "cert-name", utils.OperatorCertNameDefault, "The name of the TLS certificate file.")
	flag.StringVar(&keyName, "key-name", utils.OperatorKeyNameDefault, "The name of the TLS key file.")
	flag.StringVar(&caCertPath, "ca-cert", utils.OperatorCACertPathDefault, "The path to the CA certificate file.")
	flag.StringVar(&serviceImage, "service-image", utils.OLSAppServerImageDefault, "The image of the lightspeed-service container.")
	flag.StringVar(&consoleImage, "console-image", utils.ConsoleUIImageDefault, "The image of the console-plugin container using PatternFly 6.")
	flag.StringVar(&consoleImage_pf5, "console-image-pf5", utils.ConsoleUIImagePF5Default, "The image of the console-plugin container using PatternFly 5.")
	flag.StringVar(&namespace, "namespace", "", "The namespace where the operator is deployed.")
	flag.StringVar(&postgresImage, "postgres-image", utils.PostgresServerImageDefault, "The image of the PostgreSQL server.")
	flag.StringVar(&openshiftMCPServerImage, "openshift-mcp-server-image", utils.OpenShiftMCPServerImageDefault, "The image of the OpenShift MCP server container.")
	flag.StringVar(&lcoreImage, "lcore-image", utils.LlamaStackImageDefault, "The image of the LCore container.")
	flag.StringVar(&dataverseExporterImage, "dataverse-exporter-image", utils.DataverseExporterImageDefault, "The image of the dataverse exporter container.")
	flag.StringVar(&ocpRagImage, "ocp-rag-image", utils.OcpRagImageDefault, "The image with the OCP RAG databases.")
	flag.BoolVar(&useLCore, "use-lcore", false, "Use LCore instead of AppServer for the application server deployment.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if namespace == "" {
		namespace = getWatchNamespace()
	}

	imagesMap := overrideImages(serviceImage, consoleImage, consoleImage_pf5, postgresImage, openshiftMCPServerImage, lcoreImage, dataverseExporterImage, ocpRagImage)
	setupLog.Info("Images setting loaded", "images", listImages())

	// Log which backend is being used
	backendType := "AppServer"
	if useLCore {
		backendType = "LCore"
	}
	setupLog.Info("========================================")
	setupLog.Info(">>> BACKEND CONFIGURATION <<<", "backendType", backendType)
	setupLog.Info("========================================")

	setupLog.Info("Starting the operator", "metricsAddr", metricsAddr, "probeAddr", probeAddr, "reconcilerIntervalMinutes", reconcilerIntervalMinutes, "certDir", certDir, "certName", certName, "keyName", keyName, "namespace", namespace)
	// Get K8 client and context
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

	var tlsSecurityProfileSpec configv1.TLSProfileSpec
	if secureMetricsServer {
		apiAuthConfigmap := &corev1.ConfigMap{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ClientCACmName, Namespace: utils.ClientCACmNamespace}, apiAuthConfigmap)
		if err != nil {
			setupLog.Error(err, fmt.Sprintf("failed to get %s/%s configmap.", utils.ClientCACmNamespace, utils.ClientCACmName))
			os.Exit(1)
		}
		var exists bool
		metricsClientCA, exists = apiAuthConfigmap.Data[utils.ClientCACertKey]
		if !exists {
			setupLog.Error(err, fmt.Sprintf("the key %s is not found in %s/%s configmap.", utils.ClientCACertKey, utils.ClientCACmNamespace, utils.ClientCACmName))
			os.Exit(1)
		}

		olsconfig := &olsv1alpha1.OLSConfig{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigName}, olsconfig)
		if err != nil && client.IgnoreNotFound(err) != nil {
			setupLog.Error(err, fmt.Sprintf("failed to get %s OLSConfig.", utils.OLSConfigName))
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
						namespace:                          {},
						utils.TelemetryPullSecretNamespace: {},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Get Openshift version
	major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
	if err != nil {
		setupLog.Error(err, "failed to get Openshift version.")
		os.Exit(1)
	}

	// Setup required console image
	mVersion, err := strconv.Atoi(minor)
	if err != nil {
		setupLog.Error(err, "failed to get Openshift version.")
		os.Exit(1)
	}
	if mVersion < 19 {
		// Use PF5
		consoleImage = imagesMap["console-plugin-pf5"]
	} else {
		// Use PF6
		consoleImage = imagesMap["console-plugin"]
	}

	// Check if Prometheus Operator CRDs are available
	prometheusAvailable := utils.IsPrometheusOperatorAvailable(ctx, k8sClient)
	prometheusStatus := "NOT AVAILABLE"
	if prometheusAvailable {
		prometheusStatus = "AVAILABLE"
	}
	setupLog.Info("========================================")
	setupLog.Info(">>> PROMETHEUS OPERATOR STATUS <<<", "status", prometheusStatus)
	setupLog.Info("========================================")
	if prometheusAvailable {
		setupLog.Info("ServiceMonitor and PrometheusRule resources will be created")
	} else {
		setupLog.Info("ServiceMonitor and PrometheusRule resources will be skipped")
	}

	if err = (&controller.OLSConfigReconciler{
		Client:     mgr.GetClient(),
		Logger:     ctrl.Log.WithName("controller").WithName("OLSConfig"),
		StateCache: make(map[string]string),
		Options: utils.OLSConfigReconcilerOptions{
			OpenShiftMajor:                 major,
			OpenshiftMinor:                 minor,
			ConsoleUIImage:                 consoleImage,
			LightspeedServiceImage:         imagesMap["lightspeed-service"],
			LightspeedServicePostgresImage: imagesMap["postgres-image"],
			OpenShiftMCPServerImage:        imagesMap["openshift-mcp-server-image"],
			DataverseExporterImage:         imagesMap["dataverse-exporter-image"],
			LightspeedCoreImage:            imagesMap["lightspeed-core"],
			UseLCore:                       useLCore,
			Namespace:                      namespace,
			ReconcileInterval:              time.Duration(reconcilerIntervalMinutes) * time.Minute, // #nosec G115
			PrometheusAvailable:            prometheusAvailable,
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

// get the namespace to watch or use the default namespace
func getWatchNamespace() string {
	ns, found := os.LookupEnv("WATCH_NAMESPACE")
	if !found {
		return utils.OLSNamespaceDefault
	}
	return ns
}
