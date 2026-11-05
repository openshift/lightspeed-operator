#!/usr/bin/env bash
# Compatibility launcher. The Python implementation uses only the standard library.
# USERNAME=your-username ./deploy_cuda_x86_64.sh
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec python3 "$script_dir/deploy_cuda_x86_64.py" "$@"
