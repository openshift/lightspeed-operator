package utils

import (
	"strings"
	"testing"
)

func TestOpenShiftMCPServerConfigTOML(t *testing.T) {
	config := OpenShiftMCPServerConfigTOML

	if !strings.Contains(config, `kind = "Secret"`) {
		t.Error("TOML config should deny v1/Secret resources")
	}
	if !strings.Contains(config, `group = ""`) {
		t.Error("TOML config should have empty group for core API resources")
	}
	if !strings.Contains(config, `version = "v1"`) {
		t.Error("TOML config should target v1 API version")
	}
	if !strings.Contains(config, "rbac.authorization.k8s.io") {
		t.Error("TOML config should deny RBAC resources")
	}
	if !strings.Contains(config, "[[denied_resources]]") {
		t.Error("TOML config should use denied_resources table array syntax")
	}

	if !strings.Contains(config, `toolsets = ["core", "config", "helm", "metrics"]`) {
		t.Error("TOML config should pin default toolsets explicitly")
	}
	if !strings.Contains(config, "[toolset_configs.obs-mcp]") {
		t.Error("TOML config should define obs-mcp toolset config for metrics")
	}
	if !strings.Contains(config, "prometheus_url = \"https://thanos-querier.openshift-monitoring.svc.cluster.local:9091\"") {
		t.Error("TOML config should set Thanos Querier prometheus_url for obs-mcp")
	}
	if !strings.Contains(config, "alertmanager_url = \"https://alertmanager-main.openshift-monitoring.svc.cluster.local:9094\"") {
		t.Error("TOML config should set Alertmanager URL for obs-mcp")
	}
	if !strings.Contains(config, "guardrails = \"none\"") {
		t.Error("TOML config should set guardrails to none for OCP Thanos compatibility")
	}

	// Verify there are exactly 2 denied_resources entries
	count := strings.Count(config, "[[denied_resources]]")
	if count != 2 {
		t.Errorf("Expected 2 denied_resources entries, got %d", count)
	}
}

func TestGetOpenShiftMCPServerConfigVolumeAndMount(t *testing.T) {
	volume, mount := GetOpenShiftMCPServerConfigVolumeAndMount()

	if volume.Name != OpenShiftMCPServerConfigVolumeName {
		t.Errorf("Expected volume name '%s', got '%s'", OpenShiftMCPServerConfigVolumeName, volume.Name)
	}

	if volume.ConfigMap == nil {
		t.Fatal("Volume should be a ConfigMap volume")
	}

	if volume.ConfigMap.Name != OpenShiftMCPServerConfigCmName {
		t.Errorf("Expected ConfigMap name '%s', got '%s'", OpenShiftMCPServerConfigCmName, volume.ConfigMap.Name)
	}

	if volume.ConfigMap.DefaultMode == nil {
		t.Fatal("Volume should have a default mode set")
	}

	if *volume.ConfigMap.DefaultMode != VolumeDefaultMode {
		t.Errorf("Expected default mode %d, got %d", VolumeDefaultMode, *volume.ConfigMap.DefaultMode)
	}

	if mount.Name != OpenShiftMCPServerConfigVolumeName {
		t.Errorf("Expected mount name '%s', got '%s'", OpenShiftMCPServerConfigVolumeName, mount.Name)
	}

	if !mount.ReadOnly {
		t.Error("Config volume mount should be read-only")
	}

	expectedMountPath := GetOpenShiftMCPServerConfigPath()
	if mount.MountPath != expectedMountPath {
		t.Errorf("Expected mount path '%s', got '%s'", expectedMountPath, mount.MountPath)
	}

	if mount.SubPath != OpenShiftMCPServerConfigFilename {
		t.Errorf("Expected SubPath '%s', got '%s'", OpenShiftMCPServerConfigFilename, mount.SubPath)
	}
}

func TestGetOpenShiftMCPServerConfigPath(t *testing.T) {
	path := GetOpenShiftMCPServerConfigPath()
	expected := "/etc/mcp-server/config.toml"
	if path != expected {
		t.Errorf("Expected config path '%s', got '%s'", expected, path)
	}
}
