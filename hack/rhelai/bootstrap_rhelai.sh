#!/usr/bin/env bash
# Root-only agent, uploaded exclusively to the newly owned winning instance.
# Run under systemd during conversion so SSH loss does not kill the installer.
set -Eeuo pipefail
umask 077

MODE="${1:?convert or configure required}"
BOOTC_IMAGE="${2:?bootc image required}"
INFERENCE_IMAGE="${3:?inference image required}"
EXPECTED_INSTANCE_ID="${4:?owned instance ID required}"
MODEL="${5:-ibm-granite/granite-3.3-2b-instruct}"
TENSOR_PARALLEL_SIZE="${6:-1}"
MAX_NUM_SEQS="${7:-256}"
ROOT="$(dirname -- "$(readlink -f -- "$0")")"
AUTH_FILE="$ROOT/auth.json"


phase() {
    printf '%s\n' "$1" > "$ROOT/phase.new"
    mv -f -- "$ROOT/phase.new" "$ROOT/phase"
}
stage_for_reboot() {
    # bootc uses a new stateroot /var; the old RHEL /var is left in /sysroot.
    STAGED_ROOT="$1"
    phase rebooting
    install -d -m 0700 -- "$STAGED_ROOT"
    cp -a -- "$ROOT/." "$STAGED_ROOT/"
    rm -f -- "$AUTH_FILE"
    sync
}
failed() {
    local status="$?"
    trap - ERR
    # Cleanup must still happen if a full/read-only disk prevents phase writes.
    rm -f -- "$AUTH_FILE" || true
    if [[ -n "${STAGED_ROOT:-}" ]]; then
        rm -f -- "$STAGED_ROOT/auth.json" || true
        printf 'failed\n' > "$STAGED_ROOT/phase" || true
    fi
    phase failed || true
    printf 'Bootstrap failed with status %s; inspect install.log.\n' "$status" >&2 || true
    exit "$status"
}
trap failed ERR

# Values become Quadlet arguments: reject newlines/options/config injection.
[[ "$MODEL" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*(/[A-Za-z0-9][A-Za-z0-9_.-]*)?$ ]]
[[ "$TENSOR_PARALLEL_SIZE" =~ ^([1-9]|[1-5][0-9]|6[0-4])$ ]]
[[ "$MAX_NUM_SEQS" =~ ^[1-9][0-9]{0,3}$ ]]
(( MAX_NUM_SEQS <= 4096 ))

# Defense in depth: do not install packages or change the OS on another host.
python3 - "$EXPECTED_INSTANCE_ID" <<'PY'
import sys
import urllib.request
base = 'http://169.254.169.254/latest/'
request = urllib.request.Request(base + 'api/token', method='PUT',
    headers={'X-aws-ec2-metadata-token-ttl-seconds': '60'})
with urllib.request.urlopen(request, timeout=5) as response:
    token = response.read().decode()
request = urllib.request.Request(base + 'meta-data/instance-id',
    headers={'X-aws-ec2-metadata-token': token})
with urllib.request.urlopen(request, timeout=5) as response:
    actual = response.read().decode().strip()
if actual != sys.argv[1]:
    raise SystemExit('Instance identity mismatch; refusing bootstrap')
PY

case "$MODE" in
    convert)
        phase pulling
        # Use the RHEL AI image's drivers/kernel, not a separate DKMS installer.
        dnf -y install podman
        test -s /root/.ssh/authorized_keys
        podman pull --authfile "$AUTH_FILE" "$BOOTC_IMAGE"
        phase converting
        podman run --rm --privileged \
            --authfile "$AUTH_FILE" \
            -v /dev:/dev \
            -v /var/lib/containers:/var/lib/containers \
            -v /:/target \
            --pid=host --security-opt label=type:unconfined_t \
            "$BOOTC_IMAGE" bootc install to-existing-root \
            --root-ssh-authorized-keys /target/root/.ssh/authorized_keys \
            --skip-fetch-check --acknowledge-destructive \
            --target-imgref "$BOOTC_IMAGE"
        test -d /ostree/deploy/default/var
        stage_for_reboot "/ostree/deploy/default/var/lib/rhelai-provision/$(basename -- "$ROOT")"
        systemctl reboot
        ;;
    configure)
        phase configuring
        # The factory Quadlet supplies the cache volumes and SELinux settings.
        test -f /etc/containers/systemd/rhaiis.container
        nvidia-smi
        gpu_count="$(nvidia-smi --query-gpu=index --format=csv,noheader | wc -l)"
        if (( TENSOR_PARALLEL_SIZE > gpu_count )); then
            printf 'Requested tensor parallelism %s exceeds visible GPUs (%s).\n' "$TENSOR_PARALLEL_SIZE" "$gpu_count" >&2
            false
        fi
        devices="$(nvidia-ctk cdi list 2>/dev/null || true)"
        if ! grep -q '^nvidia.com/gpu=all$' <<< "$devices"; then
            mkdir -p /etc/cdi
            nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
        fi
        if ! podman image exists "$INFERENCE_IMAGE"; then
            podman pull --authfile "$AUTH_FILE" "$INFERENCE_IMAGE"
        fi
        # The runtime user is 1001:0; group 0 needs writable bind mounts.
        install -d -o 0 -g 0 -m 0775 \
            /var/lib/rhaii/cache /var/lib/rhaii/models \
            /var/lib/rhaii/tmp /var/lib/rhaii/config
        # Pull secrets are unnecessary once both pinned images are cached.
        rm -f -- "$AUTH_FILE"
        mkdir -p /etc/containers/systemd/rhaiis.container.d
        cat > /etc/containers/systemd/rhaiis.container.d/zz-rhelai-provision.conf <<EOF
[Container]
Image=$INFERENCE_IMAGE
Pull=never
Exec=
Exec=--model $MODEL --tensor-parallel-size $TENSOR_PARALLEL_SIZE --max-model-len 4096 --max-num-seqs $MAX_NUM_SEQS
AddDevice=
AddDevice=nvidia.com/gpu=all
PublishPort=
PublishPort=8000:8000
ShmSize=4g
Environment=HF_HUB_OFFLINE=0

[Service]
Restart=on-failure
RestartSec=30
TimeoutStartSec=900

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl restart rhaiis.service
        phase configured
        ;;
    *)
        printf 'Unknown bootstrap mode\n' >&2
        exit 2
        ;;
esac
