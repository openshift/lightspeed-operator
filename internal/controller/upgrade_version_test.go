package controller

import (
	"testing"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func TestReconcilerUsesCompletedVersionAfterUpgrade(t *testing.T) {
	r := &OLSConfigReconciler{Options: utils.OLSConfigReconcilerOptions{OpenShiftMajor: "4", OpenshiftMinor: "22"}}
	if got := r.GetOpenShiftMajor() + "." + r.GetOpenshiftMinor(); got != "4.22" {
		t.Fatalf("initial version = %s, want 4.22", got)
	}
	// Reconcile refreshes this snapshot from ClusterVersion before rendering
	// Classic app-server and console Deployments after a completed upgrade.
	r.activeVersion.Store(&utils.AgenticVersion{State: utils.AgenticGateEnabled, Major: "5", Minor: "0"})
	if got := r.GetOpenShiftMajor() + "." + r.GetOpenshiftMinor(); got != "5.0" {
		t.Fatalf("version after completed upgrade = %s, want 5.0", got)
	}
}
