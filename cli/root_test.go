package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_VersionSubcommand(t *testing.T) {
	streams, out, _ := fakeStreams()
	cmd := NewRootCmd(streams)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "oc-ols") {
		t.Errorf("expected 'oc-ols' in output, got: %s", out.String())
	}
}

func TestRootCmd_NoArgsShowsHelp(t *testing.T) {
	streams, _, _ := fakeStreams()
	cmd := NewRootCmd(streams)
	// Cobra writes help to cmd.OutOrStdout(), so redirect it to a buffer
	helpOut := &bytes.Buffer{}
	cmd.SetOut(helpOut)
	cmd.SetArgs([]string{})
	_ = cmd.Execute()
	if helpOut.Len() == 0 {
		t.Error("expected help output, got nothing")
	}
}

func TestRootCmd_HasGlobalFlags(t *testing.T) {
	streams, _, _ := fakeStreams()
	cmd := NewRootCmd(streams)

	flags := []string{"kubeconfig", "insecure-skip-tls-verify", "ca-cert"}
	for _, name := range flags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected persistent flag %q to be registered", name)
		}
	}
}

func TestRootCmd_DefaultModeDispatch(t *testing.T) {
	streams, _, errOut := fakeStreams()
	cmd := NewRootCmd(streams)
	cmd.SetArgs([]string{"why is my pod crashing"})
	err := cmd.Execute()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unknown command") {
			t.Errorf("default mode dispatch failed — got 'unknown command' error: %v", err)
		}
	}
	if !strings.Contains(errOut.String(), "not yet implemented") {
		t.Errorf("expected stub message on stderr, got: %s", errOut.String())
	}
}
