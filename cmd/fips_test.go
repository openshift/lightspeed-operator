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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestEvaluateFIPSState covers the startup decision table, including the
// fail-closed combinations. The inputs are injected, so no FIPS-enabled host
// or process-wide GODEBUG changes are needed.
func TestEvaluateFIPSState(t *testing.T) {
	readErr := errors.New("simulated read failure")
	cases := []struct {
		name     string
		hostFIPS bool
		hostErr  error
		module   bool
		want     fipsAction
	}{
		{"FIPS host with module active continues", true, nil, true, fipsContinue},
		{"FIPS host with module disabled refuses startup", true, nil, false, fipsRefuse},
		{"non-FIPS host with module active continues", false, nil, true, fipsContinue},
		{"non-FIPS host with module disabled continues", false, nil, false, fipsContinue},
		{"unreadable host state with module active continues", false, readErr, true, fipsContinueUnverifiedHost},
		{"unreadable host state with module disabled refuses startup", false, readErr, false, fipsRefuse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, msg := evaluateFIPSState(tc.hostFIPS, tc.hostErr, tc.module)
			if action != tc.want {
				t.Errorf("evaluateFIPSState(%v, %v, %v) action = %v, want %v",
					tc.hostFIPS, tc.hostErr, tc.module, action, tc.want)
			}
			if msg == "" {
				t.Error("expected a non-empty log message")
			}
		})
	}
}

// TestReportFIPSState preserves refusal diagnostics without terminating the test process.
func TestReportFIPSState(t *testing.T) {
	readErr := errors.New("permission denied reading host state")
	for _, tc := range []struct {
		name         string
		host, module bool
		err          error
		want         fipsAction
	}{
		{"unreadable host refuses", false, false, readErr, fipsRefuse},
		{"disabled module refuses without read error", true, false, nil, fipsRefuse},
		{"unreadable host continues with module active", false, true, readErr, fipsContinueUnverifiedHost},
		{"enabled module continues", true, true, nil, fipsContinue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&out, nil))
			if got := reportFIPSState(logger.Info, tc.host, tc.err, tc.module); got != tc.want {
				t.Fatalf("action = %v, want %v", got, tc.want)
			}
			var record map[string]any
			if err := json.Unmarshal(out.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if tc.err != nil {
				if record["error"] != tc.err.Error() {
					t.Errorf("error = %v, want %q", record["error"], tc.err.Error())
				}
			} else if _, found := record["error"]; found {
				t.Error("nil read error must not add an error attribute")
			}
			if tc.want == fipsRefuse {
				if record["hostFIPS"] != tc.host || record["fips140"] != tc.module {
					t.Errorf("refusal attributes not preserved: %v", record)
				}
			}
		})
	}
}

// TestReadHostFIPSMode covers the host-state file reader: content parsing,
// trimming, the missing-file-is-not-an-error rule, and read failures.
func TestReadHostFIPSMode(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"enabled", "1\n", true},
		{"enabled with whitespace", " 1 ", true},
		{"disabled", "0\n", false},
		{"disabled with whitespace", " 0 ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readHostFIPSMode(write(tc.name, tc.content))
			if err != nil {
				t.Fatalf("readHostFIPSMode: %v", err)
			}
			if got != tc.want {
				t.Errorf("readHostFIPSMode(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}

	t.Run("garbage content is an unknown state", func(t *testing.T) {
		if _, err := readHostFIPSMode(write("garbage", "garbage\n")); err == nil {
			t.Error("unexpected content must surface as an error so the unknown-state decision applies")
		}
	})

	t.Run("missing file means non-FIPS host, not an error", func(t *testing.T) {
		got, err := readHostFIPSMode(filepath.Join(dir, "does-not-exist"))
		if err != nil {
			t.Fatalf("expected nil error for missing file, got %v", err)
		}
		if got {
			t.Error("missing file must not report FIPS enabled")
		}
	})

	t.Run("unreadable path returns an error", func(t *testing.T) {
		// A directory cannot be read as a file: exercises the error path
		// that must fail closed when the module is disabled.
		if _, err := readHostFIPSMode(dir); err == nil {
			t.Error("expected an error reading a directory as the FIPS-state file")
		}
	})
}
