/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// The go:debug directive sets fips140=on as the binary's default GODEBUG
// setting: the Go native FIPS 140-3 crypto module is always active. This
// matches the previous strictfipsruntime behavior of enforcing FIPS-approved
// crypto unconditionally. An explicit GODEBUG=fips140=off environment
// override can still disable it, which the startup assertion below guards
// against on FIPS-enabled hosts.
//
//go:debug fips140=on

package main

import (
	"crypto/fips140"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// fipsAction is the startup decision for the FIPS state check.
type fipsAction int

const (
	// fipsContinue: FIPS state is acceptable; log the status and start.
	fipsContinue fipsAction = iota
	// fipsContinueUnverifiedHost: the host FIPS state is unreadable but the
	// Go FIPS module is active; log the read failure and start.
	fipsContinueUnverifiedHost
	// fipsRefuse: refuse to start (a FIPS-required host would run without
	// FIPS-approved crypto).
	fipsRefuse
)

// evaluateFIPSState maps the host FIPS state and the Go FIPS module state to
// a startup action. It is a pure function so the startup decision is
// unit-testable without a FIPS-enabled host or process-wide GODEBUG changes.
func evaluateFIPSState(hostFIPSEnabled bool, hostStateErr error, moduleEnabled bool) (fipsAction, string) {
	switch {
	case hostStateErr != nil && moduleEnabled:
		// Unexpected read failure (a missing file means the kernel has no
		// FIPS support and is not an error — readHostFIPSMode returns nil
		// for that). FIPS-approved crypto is in use regardless of host
		// state, so log and continue.
		return fipsContinueUnverifiedHost, "cannot read host FIPS state; Go FIPS module is active, continuing"
	case hostStateErr != nil:
		// Host state unknown and the module is off: a FIPS-enabled host
		// cannot be ruled out — fail closed.
		return fipsRefuse, "cannot read host FIPS state and the Go FIPS 140 crypto module is not enabled; refusing to start"
	case hostFIPSEnabled && !moduleEnabled:
		// The binary must not run in FIPS mode without the FIPS crypto
		// module active; refuse to start rather than use non-approved
		// algorithms on a FIPS-enabled cluster.
		return fipsRefuse, "host is in FIPS mode but the Go FIPS 140 crypto module is not enabled; check the GODEBUG=fips140 setting"
	case hostFIPSEnabled:
		return fipsContinue, "FIPS mode active: Go native FIPS 140-3 crypto provider enabled"
	case moduleEnabled:
		// Normal on non-FIPS hosts: the //go:debug fips140=on default keeps
		// the module active even when the host does not require FIPS.
		return fipsContinue, "FIPS-approved crypto active (Go FIPS module enabled by default); host is not FIPS-enabled"
	default:
		return fipsContinue, "Go FIPS 140 crypto module is not enabled (GODEBUG override); host is not FIPS-enabled"
	}
}

// checkFIPSMode verifies that the Go native FIPS 140-3 crypto provider is
// active when the host OS is in FIPS mode, and refuses startup otherwise.
func checkFIPSMode(log func(msg string, keysAndValues ...interface{})) {
	hostFIPSEnabled, err := readHostFIPSMode(defaultHostFIPSStatePath)
	if reportFIPSState(log, hostFIPSEnabled, err, fips140.Enabled()) == fipsRefuse {
		os.Exit(1)
	}
}

// reportFIPSState logs the startup decision and returns it to the process-exit wrapper.
func reportFIPSState(log func(msg string, keysAndValues ...interface{}), hostFIPSEnabled bool, err error, moduleEnabled bool) fipsAction {
	action, msg := evaluateFIPSState(hostFIPSEnabled, err, moduleEnabled)
	switch action {
	case fipsRefuse:
		attrs := []any{"hostFIPS", hostFIPSEnabled, "fips140", moduleEnabled}
		if err != nil {
			attrs = append(attrs, "error", err)
		}
		log(msg, attrs...)
	case fipsContinueUnverifiedHost:
		log(msg, "error", err)
	default:
		log(msg)
	}
	return action
}

// defaultHostFIPSStatePath is where the kernel exposes the host FIPS state.
const defaultHostFIPSStatePath = "/proc/sys/crypto/fips_enabled"

// readHostFIPSMode reports whether the host OS at the given path is in FIPS
// mode. A missing file means the kernel has no FIPS support: the host is
// treated as non-FIPS-enabled and no error is returned.
func readHostFIPSMode(path string) (bool, error) {
	// The path is the package constant (or a test-injected temp file).
	b, err := os.ReadFile(path) //nolint:gosec // G304: parameterized for testability; production value is a constant
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	switch v := strings.TrimSpace(string(b)); v {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		// Unexpected content means the host state is unknown; surface it as
		// an error so evaluateFIPSState applies its unknown-state decision
		// rather than silently treating the host as non-FIPS.
		return false, fmt.Errorf("unexpected content in %s: %q", path, v)
	}
}
