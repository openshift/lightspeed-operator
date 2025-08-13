# OpenShift Lightspeed Operator

For users who just want to run OpenShift Lightspeed, please refer to the [OpenShift Lightspeed Repository](https://github.com/openshift/lightspeed-service). This documentation provides instructions needed for setting up and using the service.

A Kubernetes operator for managing [Red Hat OpenShift Lightspeed](https://github.com/openshift/lightspeed-service).

## Getting Started

You'll need an OpenShift 4.16+ cluster to run against.

> [!IMPORTANT]
> Officially, the Operator only supports OpenAI, Azure OpenAI, WatsonX, RHELAI and RHOAI as large language model (LLM) providers, but technically, if you have an OpenAI API compatible model server (Ollama, VLLM, MLX), it should work.

### Running on the cluster

**Note:** Your controller will automatically use the current context from your `kubeconfig` file (i.e. whatever cluster `oc cluster-info` shows).

1. Deploy the controller to the cluster:

```shell
make deploy
```

Alternatively, to build the Docker image and push it to a personal repository, then deploy the operator into the cluster, use the following commands:
```shell
IMG="docker.io/username/ols-operator:0.10" make docker-build docker-push
IMG="docker.io/username/ols-operator:0.10" make deploy
```

2. Create a secret containing the API Key for Watsonx, OpenAI, Azure OpenAI. The key for API key is `apitoken`.

> [!TIP]
> Watsonx example

```yaml
apiVersion: v1
data:
  apitoken: <base64 encoded API Key>
kind: Secret
metadata:
  name: watsonx-api-keys
  namespace: openshift-lightspeed
type: Opaque
```

> [!TIP]
> OpenAI example

```yaml
apiVersion: v1
data:
  apitoken: <base64 encoded API Key>
kind: Secret
metadata:
  name: openai-api-keys
  namespace: openshift-lightspeed
type: Opaque
```

> [!TIP]
> Azure OpenAI apitoken example

```yaml
apiVersion: v1
data:
  apitoken: <base64 encoded API Key>
kind: Secret
metadata:
  name: azure-openai-api-keys
  namespace: openshift-lightspeed
type: Opaque
```

> [!TIP]
> Azure OpenAI user-assigned identity

```yaml
apiVersion: v1
data:
  client_id: <base64 encoded client id>
  client_secret: <base64 encoded client secret>
  tenant_id: <base64 encoded tenant id>
kind: Secret
metadata:
  name: azure-api-keys
  namespace: openshift-lightspeed
type: Opaque
```

These `apitoken` or `client_secret` values can be updated if the user wishes to change them later. The same applies to all the TLS and CA certs related to individual components. They get reflected automatically across the system.

3. Create an `OLSConfig` custom resource

```yaml
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  llm:
    providers:
    - type: openai
      credentialsSecretRef:
        name: openai-api-keys
      models:
      - name: gpt-3.5-turbo
      name: openai
      url: https://api.openai.com/v1
    - type: watsonx
      credentialsSecretRef:
        name: watson-api-keys
      models:
      - name: ibm/granite-13b-chat-v2
      name: watsonx
      url: https://us-south.ml.cloud.ibm.com
    - type: azure_openai
      credentialsSecretRef:
        name: azure-openai-api-keys
      models:
      - name: gpt-3.5-turbo
      name: my_azure_openai
      url: "https://myendpoint.openai.azure.com/"
  ols:
    conversationCache:
      postgres:
        sharedBuffers: 256MB
        maxConnections: 2000
      type: postgres
    defaultModel: gpt-3.5-turbo
    defaultProvider: openai
    logLevel: INFO
    deployment:
      replicas: 1
```

4. The Operator will reconcile the CustomResource (CR) and create all the necessary resources for launching the `Red Hat OpenShift Lightspeed` application server.

### Uninstall CRDs

To delete the CRDs from the cluster:

```shell
make uninstall
```

### Undeploy controller

UnDeploy the controller from the cluster:

```shell
make undeploy
```

### Run locally (outside the cluster)

1. Create a namespace `openshift-lightspeed`

```shell
oc create namespace openshift-lightspeed
```

2. Install the CRDs into the cluster:

```shell
make install
```

3. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```shell
make run
```

4. Create a secret containing the API Key for BAM or OpenAI. The key for API key is `apitoken`.

5. Create an `OLSConfig` custom resource

6. The Operator will reconcile the CustomResource (CR) and create all the necessary resources for launching the `Red Hat OpenShift Lightspeed` application server.

```shell
➜ oc get configmaps -n openshift-lightspeed
NAME                       DATA   AGE
kube-root-ca.crt            1      33m
lightspeed-console-plugin   1      29m
olsconfig                   1      21m
openshift-service-ca.crt    1      33m

➜ oc get services -n openshift-lightspeed
NAME                                                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
lightspeed-app-server                                    ClusterIP   172.30.176.179   <none>        8080/TCP   4m47s
lightspeed-postgres-server                               ClusterIP   172.30.85.42     <none>        6379/TCP   4m47s
lightspeed-operator-controller-manager-metrics-service   ClusterIP   172.30.35.253    <none>        8443/TCP   4m47s
lightspeed-console-plugin                                ClusterIP   172.31.158.29    <none>        9443/TCP   29m
lightspeed-operator-controller-manager-service           ClusterIP   172.31.63.140    <none>        8443/TCP   24m

➜ oc get deployments -n openshift-lightspeed
NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
lightspeed-app-server                    1/1     1            1           7m5s
lightspeed-postgres-server                  1/1     1            1           7m5s
lightspeed-operator-controller-manager   1/1     1            1           2d15h
lightspeed-console-plugin                2/2     2            2           30m

➜ oc get pods -n openshift-lightspeed
NAME                                                      READY   STATUS              RESTARTS      AGE
lightspeed-operator-controller-manager-7c849865ff-9vwj9   2/2     Running             0             7m19s
lightspeed-postgres-server-7b75497676-np7zk               1/1     Running             0             6m47s
lightspeed-app-server-97c9c6d96-6tv6j                     2/2     Running                0          23m
lightspeed-console-plugin-7f6cd7c9fd-wctj8                1/1     Running                0          30m

➜ oc logs lightspeed-app-server-f7fd6cf6-k7s7p -n openshift-lightspeed
2024-02-02 12:00:06,982 [ols.app.main:main.py:29] INFO: Embedded Gradio UI is disabled. To enable set enable_dev_ui: true in the dev section of the configuration file
INFO:     Started server process [1]
INFO:     Waiting for application startup.
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8080 (Press CTRL+C to quit)
```

#### Postgres Secret Management
By default postgres server spins up with a randomly generated password located in the secret `lightspeed-postgres-secret`. One can go edit password their password to a desired value to get it reflected across the system. In addition to that postgres secret name can also be explicitly specified in cluster CR as shown in the below example.
```
conversationCache:
  postgres:
    sharedBuffers: "256MB"
    maxConnections: 2000
    credentialsSecret: xyz
  type: postgres
```

### Modifying the API definitions

If you have updated the API definitions, you must update the CRD manifests with the following command

```shell
make manifests
```

## Tests

### Unit Tests

To run the unit tests, we can run the following command

```shell
make test
```

When using Visual Studio Code, we can use the debugger settings below to execute the test in debug mode

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch Integration test ",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/internal/controller",
            "args": [
                // "--ginkgo.v", # verbose output from Ginkgo test framework
            ],
            "env": {
                "KUBEBUILDER_ASSETS": "${workspaceFolder}/bin/k8s/1.27.1-linux-amd64"
            },
        },
    ]
}
```

### End to End tests

To run the end to end tests with a OpenShift cluster, we need to have a running operator in the namespace `openshift-lightspeed`.
Please refer to the section [Running on the cluster](#running-on-the-cluster).
Then we should set 2 environment variables:

1. $KUBECONFIG - the path to the config file of kubenetes client
2. $LLM_TOKEN - the access token given by the LLM provider, here we use OpenAI for testing.

Then we can launch the end to end test by

```shell
make  test-e2e
```

When using Visual Studio Code, we can use the debugger settings below to execute the test in debug mode

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch E2E test ",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/test/e2e",
            "args": [
                // "--ginkgo.v", # verbose output from Ginkgo test framework
            ],
            "env": {
                "KUBECONFIG": "/path/to/kubeconfig",
                "LLM_TOKEN": "sk-xxxxxxxx"
            },
        },
    ]
}
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

### Update Catalog From Konflux Snapshot

To update the catalog index from a Konflux snapshot, we need to connect to Konflux using `oc login`  command:
1. Go to the Developer Sandbox web portal https://registration-service-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/
2. Copy the proxy login command on the top right corner. It should look like this `oc login --token=$TOKEN --server=https://api-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com`
3. Append our workspace to the server URL `oc login --token=$TOKEN --server=https://api-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/workspaces/crt-nshift-lightspeed/`
4. Login using that command

Now we can use the script `hack/snapshot_to_catalog.sh` to update the catalog index. It takes 3 parameters `snapshot_to_catalog.sh -s <snapshot-ref> -c <catalog-file>`:
- snapshot-ref: required, the snapshot reference to use, example: ols-bnxm2
- catalog-file: optional, the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml

To generate catalog index file `lightspeed-catalog-4.16/index.yaml` from the snapshot `ols-bnxm2`
```
➜  lightspeed-operator ✗ ./hack/snapshot_to_catalog.sh -s ols-bnxm2 -c lightspeed-catalog-4.16/index.yaml

Update catalog lightspeed-catalog-4.16/index.yaml from snapshot ols-bnxm2
using opm from /home/hasun/GitRepo/lightspeed-operator/bin/opm
using yq from /usr/bin/yq
Catalog will use the following images:
BUNDLE_IMAGE=registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-operator-bundle@sha256:b9387e5900e700db47d2b4d7f106b43d0958a3b0d3d4f4b68495141675b66a1c
OPERATOR_IMAGE=registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-rhel9-operator@sha256:4bb81dfec6cce853543c7c0e7f2898ece23105fe3a5c5b17d845b1ff58fdc92a
CONSOLE_IMAGE=registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-console-plugin-rhel9@sha256:4f45c9ba068cf92e592bb3a502764ce6bc93cd154d081fa49d05cb040885155b
SERVICE_IMAGE=registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-service-api-rhel9@sha256:794017379e28cfbbd17c8a8343f3326f2c99b8f9da5e593fa5afd52258d0c563
BUNDLE_IMAGE_ORIGIN=quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/bundle@sha256:b9387e5900e700db47d2b4d7f106b43d0958a3b0d3d4f4b68495141675b66a1c
Bundle version is 0.1.0
Validation passed for lightspeed-catalog-4.16/index.yaml
```

## Prerequisites

You'll need the following tools to develop the Operator:

- [OperatorSDK](https://v1-33-x.sdk.operatorframework.io/docs/installation), version 1.33
- [git](https://git-scm.com/downloads)
- [go](https://golang.org/dl/), version 1.21
- [docker](https://docs.docker.com/install/), version 17.03+.
- [oc](https://kubernetes.io/docs/tasks/tools/install-oc/) or [oc](https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html#installing-openshift-cli) and access to an OpenShift cluster of a compatible version.
- [golangci-lint](https://golangci-lint.run/usage/install/#local-installation), version v1.54.2

