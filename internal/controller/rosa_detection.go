package controller

import (
	"context"
	"fmt"

	openshiftv1 "github.com/openshift/api/operator/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// detectROSAEnvironment detects if the cluster is running in a ROSA environment
// by checking the Console CR's customization brand field
func (r *OLSConfigReconciler) detectROSAEnvironment(ctx context.Context) (bool, error) {
	console := &openshiftv1.Console{}
	err := r.Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Info("Console object not found, assuming non-ROSA environment")
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", ErrGetConsoleForROSA, err)
	}

	brand := string(console.Spec.Customization.Brand)
	isROSA := brand == ROSABrandName

	r.logger.Info("ROSA environment detection", "isROSA", isROSA, "brand", brand)
	return isROSA, nil
}
