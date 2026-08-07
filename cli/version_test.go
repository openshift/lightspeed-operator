package cli

import (
	"strings"
	"testing"
)

func TestVersionCmd_PrintsVersion(t *testing.T) {
	streams, out, _ := fakeStreams()
	cmd := NewVersionCmd(streams)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "oc-ols dev") {
		t.Errorf("expected output to contain 'oc-ols dev', got: %s", out.String())
	}
}

func TestVersionCmd_InjectedVersion(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = "v1.2.3-abc"
	streams, out, _ := fakeStreams()
	cmd := NewVersionCmd(streams)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "oc-ols v1.2.3-abc") {
		t.Errorf("expected output to contain injected version, got: %s", out.String())
	}
}
