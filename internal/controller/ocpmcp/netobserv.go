package ocpmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/BurntSushi/toml"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	"k8s.io/client-go/discovery"
)

type netObservPresenceKey struct{}

// NetObservState retains the last successful discovery or persisted decision.
// Its zero value has no prior decision. It must not be copied after first use.
type NetObservState struct {
	lastDecision atomic.Pointer[bool]
}

type netObservPresence struct {
	present bool
	known   bool
	state   *NetObservState
}

// WithPresence records confirmed discovery and carries the decision through
// both phases. Unknown discovery must not replace a previous successful decision.
func (s *NetObservState) WithPresence(ctx context.Context, present, known bool) context.Context {
	if known {
		s.lastDecision.Store(&present)
	}
	return context.WithValue(ctx, netObservPresenceKey{}, netObservPresence{present: present, known: known, state: s})
}

func netObservEnabled(ctx context.Context, existing *corev1.ConfigMap) (bool, error) {
	decision, ok := ctx.Value(netObservPresenceKey{}).(netObservPresence)
	if !ok || decision.known {
		return decision.present, nil
	}
	if previous := decision.state.lastDecision.Load(); previous != nil {
		return *previous, nil
	}
	if existing == nil {
		return false, nil
	}
	var config struct {
		Toolsets []string `toml:"toolsets"`
	}
	if _, err := toml.Decode(existing.Data[utils.OpenShiftMCPServerConfigFilename], &config); err != nil {
		return false, fmt.Errorf("retain NetObserv decision from existing MCP configuration: %w", err)
	}
	present := slices.Contains(config.Toolsets, "netobserv")
	// Restore memory after restart, too: a later ConfigMap deletion or corruption
	// during the same outage must not discard this recovered decision.
	decision.state.lastDecision.CompareAndSwap(nil, &present)
	return *decision.state.lastDecision.Load(), nil
}

const FlowCollectorAPIGroup = "flows.netobserv.io"
const FlowCollectorCRDName = "flowcollectors." + FlowCollectorAPIGroup

// FlowCollectorDiscovery is the API discovery surface needed for the presence gate.
// It deliberately does not expose reads of FlowCollector instances.
type FlowCollectorDiscovery interface {
	ServerGroups() (*metav1.APIGroupList, error)
	ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error)
}

// DetectFlowCollectorAPI reports presence in any served version. Errors affecting
// the relevant API surface are uncertainty, never confirmed absence.
func DetectFlowCollectorAPI(d FlowCollectorDiscovery) (bool, error) {
	if d == nil {
		return false, errors.New("FlowCollector API discovery client is not configured")
	}
	var groups *metav1.APIGroupList
	var aggregatedResources map[schema.GroupVersion]*metav1.APIResourceList
	var groupErr error
	if aggregated, ok := d.(discovery.AggregatedDiscoveryInterface); ok {
		// ServerGroups discards aggregated discovery's stale-version errors.
		// Preserve relevant failures instead of classifying them as absence.
		groupList, resources, failedVersions, err := aggregated.GroupsAndMaybeResources()
		groups, aggregatedResources, groupErr = groupList, resources, err
		for gv, err := range failedVersions {
			if gv.Group == FlowCollectorAPIGroup {
				groupErr = errors.Join(groupErr, fmt.Errorf("discover %s: %w", gv, err))
			}
		}
	} else {
		groups, groupErr = d.ServerGroups()
	}
	if partial := new(discovery.ErrGroupDiscoveryFailed); errors.As(groupErr, &partial) {
		groupErr = nil
		for gv, err := range partial.Groups {
			if gv.Group == FlowCollectorAPIGroup {
				groupErr = errors.Join(groupErr, err)
			}
		}
	}
	if groups == nil {
		return false, errors.Join(errors.New("API discovery returned no group list"), groupErr)
	}
	var resourceErr error
	for _, group := range groups.Groups {
		if group.Name != FlowCollectorAPIGroup {
			continue
		}
		for _, version := range group.Versions {
			var resources *metav1.APIResourceList
			var err error
			if aggregatedResources != nil {
				// Current aggregated discovery already includes the resources.
				// A redundant legacy request must not invalidate this result.
				resources = aggregatedResources[schema.GroupVersion{Group: group.Name, Version: version.Version}]
			} else {
				// Legacy or mixed discovery responses do not include resource data.
				resources, err = d.ServerResourcesForGroupVersion(version.GroupVersion)
			}
			if err != nil {
				resourceErr = errors.Join(resourceErr, fmt.Errorf("discover %s: %w", version.GroupVersion, err))
				continue
			}
			if resources == nil {
				resourceErr = errors.Join(resourceErr, fmt.Errorf("discover %s: no resource list returned", version.GroupVersion))
				continue
			}
			for _, resource := range resources.APIResources {
				if resource.Name == "flowcollectors" && resource.Kind == "FlowCollector" {
					return true, nil
				}
			}
		}
	}
	return false, errors.Join(groupErr, resourceErr)
}
