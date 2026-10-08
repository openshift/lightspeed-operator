package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/ocpmcp"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type controllerDiscoveryStub struct {
	present bool
	err     error
}

func (d controllerDiscoveryStub) ServerGroups() (*metav1.APIGroupList, error) {
	if !d.present {
		return &metav1.APIGroupList{}, d.err
	}
	return &metav1.APIGroupList{Groups: []metav1.APIGroup{{Name: ocpmcp.FlowCollectorAPIGroup, Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "flows.netobserv.io/v1beta2", Version: "v1beta2"}}}}}, d.err
}
func (d controllerDiscoveryStub) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return &metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "flowcollectors", Kind: "FlowCollector"}}}, d.err
}

func TestNetObservDiscoveryContext(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		present, crd, disabled, wantErr bool
		err                             error
		kind                            string
	}{
		{name: "absent"}, {name: "present", present: true, crd: true},
		{name: "removal race retries", present: true, wantErr: true},
		{name: "publication race retries", crd: true, wantErr: true},
		{name: "discovery failure", err: errors.New("unavailable"), wantErr: true},
		{name: "wrong kind is confirmed absence", crd: true, kind: "OtherFlowCollector"},
		{name: "disabled does not require discovery", disabled: true, err: errors.New("must not discover")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := corev1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			if err := apiextensionsv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			if err := olsv1alpha1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			objects := []client.Object{}
			if tc.crd {
				kind := tc.kind
				if kind == "" {
					kind = "FlowCollector"
				}
				objects = append(objects, &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: ocpmcp.FlowCollectorCRDName}, Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: ocpmcp.FlowCollectorAPIGroup, Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "flowcollectors", Kind: kind}, Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1beta2", Served: true, Storage: true}}}})
			}
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
			r := &OLSConfigReconciler{Client: c, APIReader: c, Logger: logr.Discard(), DiscoveryClient: controllerDiscoveryStub{present: tc.present, err: tc.err}}
			cr := utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(!tc.disabled)
			_, err := r.netObservContext(context.Background(), cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestFlowCollectorCRDWatch(t *testing.T) {
	p := flowCollectorCRDPredicate()
	crd := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: ocpmcp.FlowCollectorCRDName}}
	other := crd.DeepCopy()
	other.Name = "other.example.io"
	if !p.Create(event.CreateEvent{Object: crd}) || !p.Delete(event.DeleteEvent{Object: crd}) {
		t.Fatal("missing lifecycle event")
	}
	if p.Create(event.CreateEvent{Object: other}) || p.Delete(event.DeleteEvent{Object: other}) || p.Generic(event.GenericEvent{Object: crd}) {
		t.Fatal("accepted unrelated event")
	}
	if p.Update(event.UpdateEvent{ObjectOld: crd, ObjectNew: crd.DeepCopy()}) {
		t.Fatal("unchanged CRD update accepted")
	}
	labels := crd.DeepCopy()
	labels.Labels = map[string]string{"unrelated": "change"}
	if p.Update(event.UpdateEvent{ObjectOld: crd, ObjectNew: labels}) {
		t.Fatal("metadata-only update accepted")
	}
	for _, change := range []func(*apiextensionsv1.CustomResourceDefinition){
		func(c *apiextensionsv1.CustomResourceDefinition) {
			c.Spec.Versions = []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true}}
		},
		func(c *apiextensionsv1.CustomResourceDefinition) {
			c.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}}
		},
		func(c *apiextensionsv1.CustomResourceDefinition) { now := metav1.Now(); c.DeletionTimestamp = &now },
	} {
		next := crd.DeepCopy()
		change(next)
		if !p.Update(event.UpdateEvent{ObjectOld: crd, ObjectNew: next}) {
			t.Fatal("missed relevant update")
		}
	}
	requests := flowCollectorCRDRequests(context.Background(), crd)
	if len(requests) != 1 || requests[0].Name != utils.OLSConfigName {
		t.Fatalf("wrong requests %v", requests)
	}
	if len(flowCollectorCRDRequests(context.Background(), other)) != 0 {
		t.Fatal("unrelated CRD enqueued reconciliation")
	}
}
