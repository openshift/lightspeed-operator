package controller

import (
	"context"
	"fmt"
	"reflect"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/ocpmcp"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *OLSConfigReconciler) netObservContext(ctx context.Context, cr *olsv1alpha1.OLSConfig) (context.Context, error) {
	if !utils.BoolDeref(cr.Spec.OLSConfig.IntrospectionEnabled, true) {
		return ctx, nil
	}
	present, err := ocpmcp.DetectFlowCollectorAPI(r.DiscoveryClient)
	if err == nil {
		// CRD events can precede discovery publication or removal. Retry until
		// both surfaces agree instead of relying on another event to converge.
		crd := &apiextensionsv1.CustomResourceDefinition{}
		readErr := r.GetAPIReader().Get(ctx, client.ObjectKey{Name: ocpmcp.FlowCollectorCRDName}, crd)
		if readErr != nil && !apierrors.IsNotFound(readErr) {
			err = fmt.Errorf("check FlowCollector API publication: %w", readErr)
		} else {
			served := false
			if readErr == nil && crd.DeletionTimestamp.IsZero() &&
				crd.Spec.Group == ocpmcp.FlowCollectorAPIGroup &&
				crd.Spec.Names.Plural == "flowcollectors" && crd.Spec.Names.Kind == "FlowCollector" {
				for _, version := range crd.Spec.Versions {
					served = served || version.Served
				}
			}
			if served != present {
				err = fmt.Errorf("FlowCollector API discovery has not converged with its CRD: served=%t advertised=%t", served, present)
			}
		}
	}
	if err != nil {
		r.Logger.Error(err, "NetObserv discovery uncertain; retaining prior configuration and retrying")
	}
	return r.netObservState.WithPresence(ctx, present, err == nil), err
}

func flowCollectorCRDRequests(_ context.Context, obj client.Object) []reconcile.Request {
	if obj == nil || obj.GetName() != ocpmcp.FlowCollectorCRDName {
		return nil
	}
	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: utils.OLSConfigName}}}
}

func flowCollectorCRDPredicate() predicate.Predicate {
	isFlowCollector := func(obj client.Object) bool { return obj != nil && obj.GetName() == ocpmcp.FlowCollectorCRDName }
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return isFlowCollector(e.Object) },
		DeleteFunc: func(e event.DeleteEvent) bool { return isFlowCollector(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCRD, oldOK := e.ObjectOld.(*apiextensionsv1.CustomResourceDefinition)
			newCRD, newOK := e.ObjectNew.(*apiextensionsv1.CustomResourceDefinition)
			return oldOK && newOK && isFlowCollector(newCRD) &&
				(!reflect.DeepEqual(oldCRD.Spec, newCRD.Spec) ||
					!reflect.DeepEqual(oldCRD.Status.Conditions, newCRD.Status.Conditions) ||
					!reflect.DeepEqual(oldCRD.DeletionTimestamp, newCRD.DeletionTimestamp))
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
