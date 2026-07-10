// Package reconciler defines the interface contract between the main OLSConfigReconciler
// and component-specific reconcilers (appserver, postgres, console).
//
// This package contains only type definitions (no functions), so Go coverage reports
// "no statements" for production code here. *utils.TestReconciler (envtest) and the
// production reconciler satisfy Reconciler wherever components are wired or tested.
//
// The Reconciler interface provides a clean abstraction that allows component packages
// to access only the functionality they need from the main controller, without creating
// circular dependencies or exposing internal implementation details.
//
// By embedding client.Client and providing specific getter methods, this interface enables:
//   - Component isolation and independent testing
//   - Clear separation of concerns between components
//   - Prevention of circular dependencies
//   - Mock-friendly design for unit testing
//   - Consistent access patterns across all components
//
// Component reconcilers receive this interface and use it to interact with the Kubernetes
// API server and access operator configuration without directly depending on the main
// controller implementation.
package reconciler

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler interface defines the contract that component reconcilers need
// from the main OLSConfigReconciler. This allows components to be separated
// into their own packages without circular dependencies.
type Reconciler interface {
	// Embed client.Client to get Get, Create, Update, Delete, List, Patch methods
	client.Client

	// GetScheme returns the Kubernetes scheme
	GetScheme() *runtime.Scheme

	// GetLogger returns the logger instance
	GetLogger() logr.Logger

	// GetNamespace returns the operator's namespace
	GetNamespace() string

	// GetPostgresImage returns the postgres image to use
	GetPostgresImage() string

	// GetConsoleUIImage returns the console UI image to use
	GetConsoleUIImage() string

	// GetAgenticConsoleImage returns the agentic console UI image to use
	GetAgenticConsoleImage() string

	// GetAlertsAdapterImage returns the alerts adapter image to use
	GetAlertsAdapterImage() string

	// GetOpenShiftMajor returns the OpenShift major version
	GetOpenShiftMajor() string

	// GetOpenshiftMinor returns the OpenShift minor version
	GetOpenshiftMinor() string

	// GetAppServerImage returns the app server image to use
	GetAppServerImage() string

	// GetOpenShiftMCPServerImage returns the OpenShift MCP server image to use
	GetOpenShiftMCPServerImage() string

	// GetDataverseExporterImage returns the dataverse exporter image to use
	GetDataverseExporterImage() string

	// GetRHOOKPImage returns the RH Offline Knowledge Portal (Solr) sidecar image
	GetRHOOKPImage() string

	// GetRosaOKPProductEnv returns the OLS_ROSA_PRODUCT env var on ROSA clusters, or nil.
	GetRosaOKPProductEnv() *corev1.EnvVar

	// IsPrometheusAvailable returns whether Prometheus Operator CRDs are available
	IsPrometheusAvailable() bool

	// GetWatcherConfig returns the watcher configuration for external resource monitoring
	GetWatcherConfig() interface{}
}
