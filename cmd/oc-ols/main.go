package main

import (
	"os"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/lightspeed-operator/cli"
)

func main() {
	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	cmd := cli.NewRootCmd(streams)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
