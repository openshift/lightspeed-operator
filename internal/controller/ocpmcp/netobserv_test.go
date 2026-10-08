package ocpmcp

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

type netObservDiscoveryStub struct {
	groups         *metav1.APIGroupList
	groupErr       error
	resources      map[string]*metav1.APIResourceList
	resourceErrors map[string]error
}

func (d *netObservDiscoveryStub) ServerGroups() (*metav1.APIGroupList, error) {
	return d.groups, d.groupErr
}
func (d *netObservDiscoveryStub) ServerResourcesForGroupVersion(gv string) (*metav1.APIResourceList, error) {
	return d.resources[gv], d.resourceErrors[gv]
}

func TestDetectFlowCollectorAPI(t *testing.T) {
	group := func(versions ...string) *metav1.APIGroupList {
		g := metav1.APIGroup{Name: "flows.netobserv.io"}
		for _, v := range versions {
			g.Versions = append(g.Versions, metav1.GroupVersionForDiscovery{GroupVersion: "flows.netobserv.io/" + v, Version: v})
		}
		return &metav1.APIGroupList{Groups: []metav1.APIGroup{g}}
	}
	resource := func(name, kind string) *metav1.APIResourceList {
		return &metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: name, Kind: kind}}}
	}
	failure := errors.New("discovery unavailable")
	cases := []struct {
		name             string
		d                *netObservDiscoveryStub
		present, wantErr bool
	}{
		{name: "group absent", d: &netObservDiscoveryStub{groups: &metav1.APIGroupList{}}},
		{name: "v1beta2 present", d: &netObservDiscoveryStub{groups: group("v1beta2"), resources: map[string]*metav1.APIResourceList{"flows.netobserv.io/v1beta2": resource("flowcollectors", "FlowCollector")}}, present: true},
		{name: "arbitrary served version", d: &netObservDiscoveryStub{groups: group("v9"), resources: map[string]*metav1.APIResourceList{"flows.netobserv.io/v9": resource("flowcollectors", "FlowCollector")}}, present: true},
		{name: "wrong kind", d: &netObservDiscoveryStub{groups: group("v1beta2"), resources: map[string]*metav1.APIResourceList{"flows.netobserv.io/v1beta2": resource("flowcollectors", "Other")}}},
		{name: "subresource is not presence", d: &netObservDiscoveryStub{groups: group("v1beta2"), resources: map[string]*metav1.APIResourceList{"flows.netobserv.io/v1beta2": resource("flowcollectors/status", "FlowCollector")}}},
		{name: "group error", d: &netObservDiscoveryStub{groupErr: failure}, wantErr: true},
		{name: "nil group response", d: &netObservDiscoveryStub{}, wantErr: true},
		{name: "permission or timeout error", d: &netObservDiscoveryStub{groups: group("v1beta2"), resourceErrors: map[string]error{"flows.netobserv.io/v1beta2": failure}}, wantErr: true},
		{name: "nil resource response", d: &netObservDiscoveryStub{groups: group("v1beta2")}, wantErr: true},
		{name: "another version establishes presence", d: &netObservDiscoveryStub{groups: group("v1beta2", "v1beta1"), resourceErrors: map[string]error{"flows.netobserv.io/v1beta2": failure}, resources: map[string]*metav1.APIResourceList{"flows.netobserv.io/v1beta1": resource("flowcollectors", "FlowCollector")}}, present: true},
		{name: "relevant partial discovery failure", d: &netObservDiscoveryStub{groups: &metav1.APIGroupList{}, groupErr: &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{{Group: "flows.netobserv.io", Version: "v1beta2"}: failure}}}, wantErr: true},
		{name: "unrelated partial failure does not obscure absence", d: &netObservDiscoveryStub{groups: &metav1.APIGroupList{}, groupErr: &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{{Group: "unrelated.io", Version: "v1"}: failure}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			present, err := DetectFlowCollectorAPI(tc.d)
			if present != tc.present || (err != nil) != tc.wantErr {
				t.Fatalf("got present=%v err=%v; want present=%v error=%v", present, err, tc.present, tc.wantErr)
			}
		})
	}
}
