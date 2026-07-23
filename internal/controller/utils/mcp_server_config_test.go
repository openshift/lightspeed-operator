package utils

import (
	"testing"
)

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
