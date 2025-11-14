package utils

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// TestReconciler is a test implementation of the reconciler.Reconciler interface
// used across controller test suites
type TestReconciler struct {
	client.Client
	logger            logr.Logger
	scheme            *runtime.Scheme
	StateCache        map[string]string
	namespace         string
	PostgresImage     string
	ConsoleImage      string
	AppServerImage    string
	McpServerImage    string
	DataverseExporter string
	LCoreImage        string
	openShiftMajor    string
	openShiftMinor    string
}

func (r *TestReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme()
}

func (r *TestReconciler) GetLogger() logr.Logger {
	return r.logger
}

func (r *TestReconciler) GetStateCache() map[string]string {
	return r.StateCache
}

func (r *TestReconciler) GetNamespace() string {
	return r.namespace
}

func (r *TestReconciler) GetPostgresImage() string {
	return r.PostgresImage
}

func (r *TestReconciler) GetConsoleUIImage() string {
	return r.ConsoleImage
}

func (r *TestReconciler) GetOpenShiftMajor() string {
	return r.openShiftMajor
}

func (r *TestReconciler) GetOpenshiftMinor() string {
	return r.openShiftMinor
}

func (r *TestReconciler) GetAppServerImage() string {
	return r.AppServerImage
}

func (r *TestReconciler) GetOpenShiftMCPServerImage() string {
	return r.McpServerImage
}

func (r *TestReconciler) GetDataverseExporterImage() string {
	return r.DataverseExporter
}

func (r *TestReconciler) GetLCoreImage() string {
	return r.LCoreImage
}

// NewTestReconciler creates a new TestReconciler instance with the provided parameters
func NewTestReconciler(
	client client.Client,
	logger logr.Logger,
	scheme *runtime.Scheme,
	namespace string,
) *TestReconciler {
	return &TestReconciler{
		Client:            client,
		logger:            logger,
		scheme:            scheme,
		StateCache:        make(map[string]string),
		namespace:         namespace,
		PostgresImage:     PostgresServerImageDefault,
		ConsoleImage:      ConsoleUIImageDefault,
		AppServerImage:    OLSAppServerImageDefault,
		McpServerImage:    OLSAppServerImageDefault,
		LCoreImage:        LlamaStackImageDefault,
		DataverseExporter: DataverseExporterImageDefault,
		openShiftMajor:    "123",
		openShiftMinor:    "456",
	}
}

// StatusHasCondition checks if an OLSConfig status contains a specific condition.
// It ignores ObservedGeneration and LastTransitionTime when comparing.
func StatusHasCondition(status olsv1alpha1.OLSConfigStatus, condition metav1.Condition) bool {
	for _, c := range status.Conditions {
		if c.Type == condition.Type &&
			c.Status == condition.Status &&
			c.Reason == condition.Reason &&
			c.Message == condition.Message {
			return true
		}
	}
	return false
}
