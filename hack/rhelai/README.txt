= RHELAI with TLS

== End-to-end RHEL AI provisioning

`provision_rhelai.sh` provisions a new G6/L4 instance from plain RHEL,
converts it to the same RHEL AI bootc image as the working instance, and waits
for a verified Granite API. It does not modify the existing instance. The Bash
launcher delegates to Python 3.9+ (standard library; no boto3 dependency).

Local prerequisites: configured AWS CLI, Podman, Skopeo, OpenSSH (`ssh`, `ssh-keygen`),
and Red Hat registry access. `curl` is needed only if `MY_IP_CIDR` is unset.
Renew expired AWS credentials first; STS identity is checked before key imports
or resource creation. Red Hat access is checked for both pinned images before
paid provisioning. Skopeo performs metadata-only inspection of OCI manifests
and indexes; no image layers are downloaded during preflight. On Fedora/RHEL,
install it with `sudo dnf install skopeo` if it is missing.

Log in locally as the same user that runs the provisioner (without sudo):

```bash
podman login registry.redhat.io
```

`--registry-authfile` is optional. An explicit flag wins, then REGISTRY_AUTH_FILE
if set, otherwise Podman's primary login file:

* Linux: `${XDG_RUNTIME_DIR}/containers/auth.json`; when XDG_RUNTIME_DIR is unset,
  `/run/containers/<uid>/auth.json`.
* macOS/Windows: `~/.config/containers/auth.json`.

Linux's default auth file is temporary and may disappear after reboot; log in
again if needed. For persistent credentials, optionally use:

```bash
mkdir -p ~/.config/containers
podman login --authfile ~/.config/containers/auth.json registry.redhat.io
chmod 600 ~/.config/containers/auth.json
export REGISTRY_AUTH_FILE="$HOME/.config/containers/auth.json"
```

The provisioner does not search Docker fallback files or resolve credential
helpers; use an explicit auth file containing the registry credentials instead.

Use your Red Hat account or a registry service account from
https://access.redhat.com/terms-based-registry/ . Treat the file as a secret:
credentials are encoded, not encrypted. Never commit it or share its contents.
The file must contain explicit registry credentials, not credential-helper
references (helpers on your laptop cannot be used on the EC2 instance).

From this directory, using the existing SSH key:

```bash
USERNAME=syedriko ./provision_rhelai.sh \
  --key-file ~/.ssh/syedriko-keys.pem
```

For a new setup, choose a resource prefix and omit `--key-file`:

```bash
USERNAME=my-rhelai-lab ./provision_rhelai.sh
```

Key lookup defaults to `~/.ssh/${USERNAME}-keys.pem`, then an existing
`~/rhelai/${USERNAME}-keys.pem`. If neither exists, a new unencrypted RSA-3072 PEM
key is created at the first location (0600), accompanied by its `.pub`. Neither
private keys nor orphan public-key files are overwritten. Missing regional
`${USERNAME}-keys` pairs are imported from the local public key; existing pairs
must match and are never replaced. Same-named local files in different folders
can contain different keys; use the one matching the registered EC2 pair, not
necessarily the key used to access an older instance. No source key in us-west-2 is needed for this
end-to-end path. Keep the private key; imported EC2 keys persist after cleanup.

The existing parallel US-region/AZ first-success capacity search is reused.
Only the winning instance is bootstrapped. Installation begins as soon as its
owned election is recorded, while losing attempts finish cleanup in parallel.
The process waits for both bootstrap and cleanup before returning success.
Cleanup failures still produce a nonzero result even if the winner's API is ready.
The default stack is `${USERNAME}-rhelai-stack` (`--stack-name` overrides it).
Launch-template names are stack-scoped so an existing CUDA stack using the same
key prefix can coexist. Existing stacks are never adopted or deleted by name.

Default hardware: g6.2xlarge (one 24 GB L4, 32 GiB RAM, 8 vCPUs), 200 GB gp3
root disk. Select hardware/model using CLI parameters; defaults are unchanged:

```bash
./provision_rhelai.sh \
  --username syedriko \
  --key-file ~/.ssh/syedriko-keys.pem \
  --instance-type g5.12xlarge \
  --model RedHatAI/gemma-3-12b-it \
  --tensor-parallel-size 4 \
  --max-num-seqs 16
```

* `--instance-type`: supported x86_64 NVIDIA GPU family; default `g6.2xlarge`.
  AZ discovery and CloudFormation both use the selected type. ARM `g5g` is not
  supported by the pinned x86_64 images. Availability is not capacity assurance.
* `--model`: Hugging Face model ID; default `ibm-granite/granite-3.3-2b-instruct`.
  The model must be supported by the pinned serving image. No Hugging Face login
  is configured; use a publicly accessible checkpoint.
* `--tensor-parallel-size`: GPUs to shard across (1-64); default `1`.
  Set `4` for the four A10Gs on `g5.12xlarge`. Bootstrap checks visible GPU count;
  model attention-head divisibility and serving support remain model-specific.

* `--max-num-seqs`: maximum concurrent sequences (1-4096); new runs default
  to `16`. Gemma 3 12B was verified on g5.12xlarge with TP4 and this value after
  the previous default of 256 caused CUDA OOM during sampler warm-up. Higher
  values require more runtime memory; this setting does not change context length
  and is not a guarantee that every allowed value will fit.

Context stays at 4096. Gemma 3 12B here is BF16 (~22.7 GiB weights), not a
quantized checkpoint. The runtime defaults to the model-config precision.
Model/hardware parameters do not guarantee compatibility; startup and inference
verification still must pass. Multi-region attempts may temporarily allocate
multiple billable GPU instances before cleanup retains only the winner.
Image versions are pinned by registry-valid digest in the Python coordinator;
references use `repository@sha256:...`, not the unsupported `tag@digest` form.
The inference pin is the published Linux/amd64 manifest for version 3.4.5,
not the locally cached manifest digest from the existing instance:

* OS: registry.redhat.io/rhelai3/bootc-cuda-rhel9:3.4.5-1790868254
* Inference: registry.redhat.io/rhaii/vllm-cuda-rhel9:3.4.5
* Default model: ibm-granite/granite-3.3-2b-instruct
* Default arguments: --tensor-parallel-size 1 --max-model-len 4096 --max-num-seqs 16

Granite is public: no Hugging Face token is required. The OS image supplies its
own kernel, NVIDIA drivers and container tooling; no separate DKMS installation
or blanket kernel update is performed. The confirmed privileged Podman/bootc
conversion is run without interactive TTY flags, then the instance reboots.
`--skip-fetch-check` is preserved and the target image is digest-pinned; registry
authentication for subsequent OS updates is not configured by this script.

Initial SSH uses `ec2-user`, then `cloud-user` after reboot. Bootstrap jobs run
under systemd so SSH disconnects/local cancellation do not kill them. Private,
phase-specific known_hosts files avoid touching your global known_hosts. Host-key
checking is not disabled. During the expected reboot only, a prematurely cached,
recognized old RHEL key can be removed; other changes are rejected. The owned
EC2 instance ID, changed boot ID and booted image are checked before inference.

bootc's new OSTree /var is separate from the original RHEL /var. The agent, log
and root-only auth file are staged into the new stateroot before reboot. The
original auth copy is removed before reboot and the new copy after the inference
image is cached. Credentials travel via SSH stdin only, never CloudFormation
parameters, user data, command arguments or logs. Interrupted jobs may retain
a root-only auth copy until resumed; failed agent jobs remove their copies.
The local auth file is never removed.

The factory Quadlet gets a selected-model drop-in, NVIDIA CDI access and port 8000;
it starts at boot. Readiness requires /health, the exact /v1/models identity,
and a nonempty /v1/completions answer identifying Paris. Success prints API,
SSH, tunnel, log and exact-stack cleanup commands.

HTTP port 8000 is unauthenticated but restricted to `MY_IP_CIDR` (auto-detected
public IP/32 by default). Use the printed SSH tunnel for encrypted access at
http://127.0.0.1:8080/v1. TLS/NGINX setup is not automated. The manual TLS example
later in this file refers to an older model; update its model IDs for Granite.

Run state: `~/.local/state/rhelai/<run-id>/state.json` (directory 0700, file 0600).
It records returned stack IDs, winner, progress, instance type, model and tensor
parallelism and maximum concurrent sequences, never registry credentials.
`--state-dir PATH` selects a NEW private directory; existing runs are not
clobbered. `--timeout` sets the winner's bootstrap budget (default
7200 seconds), independent of ongoing losing-worker cleanup; `--poll-interval` defaults to 10 seconds. Capacity-search settings
are documented in the next section.

Failures or Ctrl-C/SIGTERM after winner selection retain the BILLABLE GPU and
print its exact cleanup command. Remote jobs can finish/reboot after the local
script exits. SIGKILL, crashes and ambiguous AWS create timeouts still require
manual inspection. An interrupted run does not necessarily mean no instance.

Resume only with a state file produced by this script:

```bash
./provision_rhelai.sh \
  --resume ~/.local/state/rhelai/<run-id>/state.json
```

Resume restores instance type/model/parallelism/max-num-seqs from state; these flags
cannot be combined with `--resume`. Older states without those fields retain the
original g6.2xlarge/Granite/1 defaults. States without `max_num_seqs` restore
`256`, not the new-run default; configured services are still not restarted by
resume. The script does not migrate an older state or a live service to `16`.
Resume verifies the AWS account, owned
stack, instance ID and pinned OS. It
reconciles recorded owned attempts and outstanding loser cleanup alongside
winner bootstrap. It cannot report success while that cleanup is unresolved. If election was not
checkpointed, it can recover an already-created successful owned stack, without
adopting anything by name. Do not run two resumes of the same state concurrently.
Resume does not create another instance or redo completed conversion. An unstarted conversion
can start; stopped/failed partial conversion requires inspection rather than a
blind destructive retry. Configuration can be retried after successful reboot;
a configured server is checked without restarting it.

Bootstrap log: `/var/lib/rhelai-provision/<run-id>/install.log` (same path on the
original RHEL root during conversion). Inference logs:
`sudo journalctl -u rhaiis.service -f` or `sudo podman logs -f rhaii`.
Delete the exact retained stack using the printed command, optionally followed
by `aws cloudformation wait stack-delete-complete --region REGION --stack-name
STACK_ID`. Stack cleanup removes the instance/network/root disk, not key pairs.

Offline provisioning tests (mocked AWS/SSH/registry; no EC2 instances launched):

```bash
python3 test_provision_rhelai.py
python3 test_provision_orchestration.py
```

== Deploy a CUDA x86_64 instance

From this directory, run:

```bash
USERNAME=your-username ./deploy_cuda_x86_64.sh
```

The script uses curl to detect this machine's public IPv4 through
`https://checkip.amazonaws.com` and restricts inbound access to that IP (`/32`).
Set `MY_IP_CIDR` explicitly to skip detection, for example when you will connect
from a different network or your VPN uses a different route for SSH.

Requires Python 3.9+ and configured AWS CLI credentials. `deploy_cuda_x86_64.sh`
is a compatibility launcher for `deploy_cuda_x86_64.py`, which uses Python's
standard library and the AWS CLI (no boto3 dependency). Use
`./deploy_cuda_x86_64.sh --help` for usage; help never starts a deployment.

The script launches one concurrent worker per commercial US region:
`us-east-1`, `us-east-2`, `us-west-1`, and `us-west-2`. Each worker tries that
region's AZs sequentially. `AWS_REGION` selects the first region in the preflight
and submission order (default: `us-west-2`), not a preferred winner or a search
restriction. GovCloud uses separate credentials and is not included. Regions
without a G6 offering are skipped.

Before launching any workers, the script checks for `${USERNAME}-keys` in each
candidate region. If it is
missing, it imports the public key from the same-named key pair in `us-west-2`.
Your existing private key works with these imported copies. Existing destination
key pairs are left untouched and are not checked for matching key material.
Imported key pairs persist after capacity failures; the script does not delete
them. The source public key is cached in a temporary file removed before workers
start.
AWS permissions must include `ec2:DescribeKeyPairs` and `ec2:ImportKeyPair`.

The US AMIs are the exact Red Hat image
`RHEL-9.8.0_HVM-20260728-x86_64-0-Hourly2-GP3`, owner `309956199498`:

| Region | AMI ID |
| us-east-1 | ami-0225644bfadc204f7 |
| us-east-2 | ami-0fa0cf9725e3938ae |
| us-west-1 | ami-0fd6ac6c5f71fe86e |
| us-west-2 | ami-02990c099b7180454 |

The first successful deployment wins, selected under a lock. Its region, stack
ID, and outputs are printed immediately, and no further attempts are started.
The winning stack is retained; other workers clean up their own stacks. Multiple
billable GPU instances may briefly exist when several regions succeed together.
The script waits for losing-worker cleanup before returning. A cleanup failure
produces a nonzero exit status and explicit stack IDs/manual cleanup commands,
even if a winner was retained.

An offering does not guarantee available capacity. For an instance capacity
shortage, a worker waits for rollback, deletes only the stack created by that
attempt, waits for deletion, and tries its next AZ unless the search has stopped.
Each AZ is tried once. Other errors or timeouts signal all workers to stop and
attempt cleanup of their owned stacks. Mixed capacity/non-capacity failures do
not authorize another attempt. Pre-existing stacks are never deleted.

Ctrl-C/SIGTERM requests cooperative cancellation; workers are not killed while
an AWS create request is in flight. AWS CLI subprocesses run in separate sessions,
so terminal Ctrl-C cannot interrupt a create response before its ID is captured.
Returned stack IDs are tracked for cleanup. Output writes are nonblocking: if
stdout closes or becomes backpressured, cancellation is requested and logging
falls back to stderr without delaying cleanup. If both streams are unavailable,
messages can be lost; the script still attempts cleanup and returns nonzero.
Pending creation or rollback may have to finish before cleanup, and this can take
time. If creation fails without returning an ID, its stack name/region are
reported for inspection; ownership is not inferred from the name. SIGKILL,
process crashes, and network failures can leave resources needing manual cleanup.
In particular, a timed-out create call may have been accepted remotely without
returning its stack ID. Such ambiguous attempts are reported for manual inspection,
not deleted by name, to avoid touching pre-existing resources.

Creation and rollback status are polled every 5 seconds. Rollback is reported
as soon as it is observed, and cleanup proceeds after it reaches a terminal state.
Set `STACK_POLL_INTERVAL_SECONDS` to adjust the shared polling interval,
`STACK_CREATE_MAX_POLLS` to adjust the creation limit, and
`STACK_ROLLBACK_MAX_POLLS` to adjust the rollback limit (defaults: 5, 720, and
720, approximately one hour per phase). Poll limits must be positive integers;
the interval must be a positive finite number of seconds. AWS CLI calls have
bounded connection/read and subprocess timeouts. Deletion still uses the AWS
CLI waiter. CUDA installation is a separate step.

Existing stacks are never deleted automatically. If a previous run left a failed
stack, inspect it and confirm it can be removed before running:

```bash
aws cloudformation delete-stack --region "$AWS_REGION" \
  --stack-name "$USERNAME-cuda-x86-64-stack"
aws cloudformation wait stack-delete-complete --region "$AWS_REGION" \
  --stack-name "$USERNAME-cuda-x86-64-stack"
```

Set AWS_REGION to the region containing the stack (reported on success) and
USERNAME to its resource prefix in your shell for these cleanup commands.

Run offline tests with:

```bash
bash test_deploy_cuda_x86_64.sh
python3 test_cuda_cloud_formation_x86_64.py
```

The template rejects every `/0` ingress CIDR, including noncanonical spellings.

== Replace the IP address

The following assumes the public IP of the RHELAI EC2 instance is 18.191.144.20. Find out the public IP of the RHELAI EC2 instance you're working with and replace 18.191.144.20 in openssl.cnf and nginx.conf.

== Generate the signing key and the certificate

You're going to need a signing key and a certificate for the TLS endpoint. Generate both with the following command on your laptop:

``` bash
$ openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout certs/rhelai.key -out certs/rhelai.crt -config openssl.cnf
```
They are going to end up in the certs directory:

$ ls -l certs/
total 8
-rw-r--r--. 1 username username 1403 Jun 17 14:08 rhelai.crt
-rw-------. 1 username username 1704 Jun 17 14:08 rhelai.key
$

== Copy the certs and NGINX config over to the RHELAI instance

```bash
$ scp -i your_aws_keys.pem -r certs nginx.conf cloud-user@18.191.144.20:~
```

== Run nginx container on the RHELAI instance

Nginx plays the role of a reverse proxy, accepting TLS connections from the OLS instance on the cluster and forwarding traffic over HTTP to the vLLM server. Its worth repeating, this runs on the RHELAI instance:

```bash
$ podman run -it --rm --network=host \
  --name my-nginx \
  -p 8443:8443 \
  -v $PWD/nginx.conf:/etc/nginx/nginx.conf:ro,Z \
  -v $PWD/certs:/etc/nginx/certs:ro,Z \
  docker.io/library/nginx:stable
```

Nginx will log to the terminal, so it's going to be easy to see if any issues arise.

== Configure the OLS to talk to the TLS endpoint on the RHELAI instance

=== Create a ConfigMap with the nginx certificate in it:

```bash
$ oc project openshift-lightspeed
Now using project "openshift-lightspeed" on server "https://api.crc.testing:6443".
$ oc create configmap rhelai-ca --from-file=certs/rhelai.crt
configmap/rhelai-ca created
```

Create an OLSConfig instance from the following YAML, replacing the IP address with the one for your RHELAI instance. Note spec.ols.additionalCAConfigMapRef referring to the ConfigMap we just created.

```yaml
---
apiVersion: v1
data:
  apitoken: Zm9vYmFyCg==
kind: Secret
metadata:
  name: openai-api-keys
  namespace: openshift-lightspeed
type: Opaque
---
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
  namespace: openshift-lightspeed
spec:
  llm:
    providers:
    - credentialsSecretRef:
        name: openai-api-keys
      models:
      - name: /var/home/cloud-user/.cache/instructlab/models/granite-7b-redhat-lab
        contextWindowSize: 4096
        parameters:
          maxTokensForResponse: 2048
      name: rhelai_vllm
      type: rhelai_vllm
      url: https://18.191.144.20:8443/v1
  ols:
    defaultProvider: rhelai_vllm
    defaultModel: /var/home/cloud-user/.cache/instructlab/models/granite-7b-redhat-lab
    logLevel: DEBUG
    additionalCAConfigMapRef:
      name: rhelai-ca
```
