package ocpmcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func TestDetectFlowCollectorAggregatedDiscovery(t *testing.T) {
	version := func(name string, stale bool) apidiscoveryv2.APIVersionDiscovery {
		freshness := apidiscoveryv2.DiscoveryFreshnessCurrent
		if stale {
			freshness = apidiscoveryv2.DiscoveryFreshnessStale
		}
		return apidiscoveryv2.APIVersionDiscovery{
			Version: name, Freshness: freshness,
			Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: "flowcollectors", Scope: apidiscoveryv2.ScopeCluster,
				ResponseKind: &metav1.GroupVersionKind{Group: FlowCollectorAPIGroup, Version: name, Kind: "FlowCollector"}}},
		}
	}
	group := func(name string, versions ...apidiscoveryv2.APIVersionDiscovery) apidiscoveryv2.APIGroupDiscovery {
		return apidiscoveryv2.APIGroupDiscovery{ObjectMeta: metav1.ObjectMeta{Name: name}, Versions: versions}
	}
	wrongKind := version("v1beta1", false)
	wrongKind.Resources[0].ResponseKind.Kind = "OtherFlowCollector"
	noResources := version("v1beta1", false)
	noResources.Resources = nil
	for _, tc := range []struct {
		name             string
		groups           []apidiscoveryv2.APIGroupDiscovery
		aggregatedCore   bool
		present, wantErr bool
	}{
		{name: "relevant stale version remains unknown", groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, version("v1beta2", true))}, wantErr: true},
		{name: "unrelated stale version does not obscure absence", groups: []apidiscoveryv2.APIGroupDiscovery{group("unrelated.example", version("v1", true))}},
		{name: "another current version establishes presence", groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, version("v1beta2", true), version("v1beta1", false))}, present: true},
		{name: "unrelated failure does not obscure presence", groups: []apidiscoveryv2.APIGroupDiscovery{group("unrelated.example", version("v1", true)), group(FlowCollectorAPIGroup, version("v1beta1", false))}, present: true},
		{name: "aggregated presence needs no legacy request", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, version("v1beta2", false))}, present: true},
		{name: "aggregated current version outweighs stale version", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, version("v1beta2", true), version("v1beta1", false))}, present: true},
		{name: "aggregated stale version remains unknown", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, version("v1beta2", true))}, wantErr: true},
		{name: "aggregated unrelated failure does not obscure absence", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group("unrelated.example", version("v1", true))}},
		{name: "aggregated wrong kind is absent", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, wrongKind)}},
		{name: "aggregated resource absence needs no legacy request", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, noResources)}},
		{name: "aggregated resource absence with stale version remains unknown", aggregatedCore: true, groups: []apidiscoveryv2.APIGroupDiscovery{group(FlowCollectorAPIGroup, noResources, version("v1beta2", true))}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.aggregatedCore && r.URL.Path != "/api" && r.URL.Path != "/apis" {
					// A current aggregated response already establishes presence or
					// absence. A redundant legacy request must not erase that result.
					t.Errorf("unexpected legacy discovery request: %s", r.URL.Path)
					http.Error(w, "legacy discovery unavailable", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				var response any
				switch r.URL.Path {
				case "/api":
					response = &metav1.APIVersions{Versions: []string{"v1"}}
					if tc.aggregatedCore {
						w.Header().Set("Content-Type", "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList")
						response = &apidiscoveryv2.APIGroupDiscoveryList{TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"}}
					}
				case "/apis":
					w.Header().Set("Content-Type", "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList")
					response = &apidiscoveryv2.APIGroupDiscoveryList{TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"}, Items: tc.groups}
				case "/apis/flows.netobserv.io/v1beta1":
					response = &metav1.APIResourceList{GroupVersion: "flows.netobserv.io/v1beta1", APIResources: []metav1.APIResource{{Name: "flowcollectors", Kind: "FlowCollector"}}}
				default:
					t.Errorf("unexpected request outside relevant discovery: %s", r.URL.Path)
					http.Error(w, "unexpected path", http.StatusNotFound)
					return
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			d, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			present, err := DetectFlowCollectorAPI(d)
			if present != tc.present || (err != nil) != tc.wantErr {
				t.Fatalf("got present=%v err=%v; want present=%v error=%v", present, err, tc.present, tc.wantErr)
			}
		})
	}
}
