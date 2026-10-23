#!/usr/bin/env bash
# Keep the Bash entry point; concurrency and reboot handling live in Python.
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/provision_rhelai.py" "$@"
