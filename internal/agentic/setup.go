// Package agentic wires agentic-operator controllers into the lightspeed operator manager.
// Upstream github.com/openshift/lightspeed-agentic-operator does not publish a
// github.com/openshift/lightspeed-agentic-operator/controller module entrypoint;
// standalone wiring lives in that repo's cmd/main.go — we mirror it here.
package agentic

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openshift/lightspeed-agentic-operator/controller/console"
	"github.com/openshift/lightspeed-agentic-operator/controller/proposal"
)

const defaultSandboxBaseTemplate = "lightspeed-agent"

// Options configures registration of agentic controllers.
type Options struct {
	Namespace    string
	ConsoleImage string
	// AgenticSandboxImage is reserved for cluster bootstrap of the base SandboxTemplate;
	// reconcilers expect that template to exist (see agentic-operator cmd/main.go).
	SandboxImage string
}

// Setup registers agentic Proposal reconciliation and ensures console plugin resources.
func Setup(mgr ctrl.Manager, o Options) error {
	if o.Namespace == "" {
		return fmt.Errorf("agentic setup: namespace is required")
	}

	ctx := context.Background()
	if err := console.EnsureAgenticConsole(ctx, mgr.GetClient(), console.AgenticConsoleConfig{
		Image:     o.ConsoleImage,
		Namespace: o.Namespace,
	}); err != nil {
		return fmt.Errorf("agentic console: %w", err)
	}

	sandboxMgr := proposal.NewSandboxManager(mgr.GetClient(), o.Namespace)
	agentCaller := proposal.NewSandboxAgentCaller(
		sandboxMgr,
		mgr.GetClient(),
		proposal.NewAgentHTTPClient,
		o.Namespace,
		defaultSandboxBaseTemplate,
	)

	r := &proposal.ProposalReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Proposal"),
		Agent:  agentCaller,
	}
	if err := r.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("proposal reconciler: %w", err)
	}

	return nil
}
