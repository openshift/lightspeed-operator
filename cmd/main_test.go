package main

import (
	"maps"
	"slices"
	"testing"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func TestHubAndSharedAdapterImages(t *testing.T) {
	original := maps.Clone(defaultImages)
	t.Cleanup(func() { defaultImages = original })

	images := overrideImages("", "", "", "", "", "", "", "", "", "", "")
	if images["hub-image"] == "" || images["hub-image"] != utils.HubImageDefault {
		t.Fatalf("hub default not loaded: %q", images["hub-image"])
	}
	if images["alerts-adapter"] == "" || images["alerts-adapter"] != utils.AlertsAdapterImageDefault {
		t.Fatalf("shared adapter default not loaded: %q", images["alerts-adapter"])
	}

	const hub = "mirror.example.com/hub@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const adapter = "mirror.example.com/adapter@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	images = overrideImages("", "", "", adapter, "", "", "", "", "", "", hub)
	if images["hub-image"] != hub || images["alerts-adapter"] != adapter {
		t.Fatalf("image overrides lost: %v", images)
	}
	listed := listImages()
	if !slices.Contains(listed, "hub-image="+hub) || !slices.Contains(listed, "alerts-adapter="+adapter) {
		t.Fatalf("configured images missing from image listing: %v", listed)
	}
}
