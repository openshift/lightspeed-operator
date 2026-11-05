#!/usr/bin/env bash
# Offline integration tests; all AWS and public-IP requests are mocked.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec python3 "$script_dir/test_deploy_cuda_x86_64.py" "$@"
