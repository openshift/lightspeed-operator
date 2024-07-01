# OpenShift Lightspeed Operator

For users who just want to run OpenShift Lightspeed, please refer to the [OpenShift Lightspeed Repository](https://github.com/openshift/lightspeed-service). This documentation provides instructions needed for setting up and using the service.

A Kubernetes operator for managing [Red Hat OpenShift Lightspeed](https://github.com/openshift/lightspeed-service).

## Getting Started

You'll need an OpenShift 4.15+ cluster to run against.

> [!IMPORTANT]
> Officially, the Operator only supports OpenAI, Azure OpenAI and WatsonX as large language model (LLM) providers, but technically, if you have an OpenAI API compatible model server (Ollama, VLLM, MLX), it should work.

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
> OpenAPI example

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
> Azure OpenAI example

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
These `apitoken` values can be updated if the user wishes to change them later. The same applies to all the TLS and CA certs related to individual components. They get reflected automatically across the system.

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
    - credentialsSecretRef:
        name: openai-api-keys
      models:
      - name: gpt-3.5-turbo
      name: openai
      url: https://api.openai.com/v1
    - type: watsonx
    - credentialsSecretRef:
        name: watson-api-keys
      models:
      - name: ibm/granite-13b-chat-v2
      name: watsonx
      url: https://us-south.ml.cloud.ibm.com
    - type: azure_openai
    - credentialsSecretRef:
        name: azure-openai-api-keys
      models:
      - name: gpt-3.5-turbo
      name: my_azure_openai
      url: "https://myendpoint.openai.azure.com/"
  ols:
    conversationCache:
      redis:
        maxMemory: 2000mb
        maxMemoryPolicy: allkeys-lru
      type: redis
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
lightspeed-app-server                                     ClusterIP   172.31.165.151   <none>        8443/TCP   22m
lightspeed-console-plugin                                 ClusterIP   172.31.158.29    <none>        9443/TCP   29m
lightspeed-operator-controller-manager-service            ClusterIP   172.31.63.140    <none>        8443/TCP   24m

➜ oc get deployments -n openshift-lightspeed
NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
lightspeed-app-server                    1/1     1            1           23m
lightspeed-console-plugin                2/2     2            2           30m
lightspeed-operator-controller-manager   1/1     1            1           25m

➜ oc get pods -n openshift-lightspeed
NAME                                                      READY   STATUS              RESTARTS      AGE
lightspeed-app-server-97c9c6d96-6tv6j                     2/2     Running                0          23m
lilightspeed-console-plugin-7f6cd7c9fd-6lp7x              1/1     Running                0          30m
lightspeed-console-plugin-7f6cd7c9fd-wctj8                1/1     Running                0          30m
lightspeed-operator-controller-manager-69585cc7fc-xltpc   1/1     Running                0          26m

➜ oc logs lightspeed-app-server-f7fd6cf6-k7s7p -n openshift-lightspeed
2024-02-02 12:00:06,982 [ols.app.main:main.py:29] INFO: Embedded Gradio UI is disabled. To enable set enable_dev_ui: true in the dev section of the configuration file
INFO:     Started server process [1]
INFO:     Waiting for application startup.
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8080 (Press CTRL+C to quit)
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
            "mode": "test",
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

To run the end to end tests with a Openshift cluster, we need to have a running operator in the namespace `openshift-lightspeed`.
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
            "mode": "test",
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

## Prerequisites

You'll need the following tools to develop the Operator:

- [OperatorSDK](https://v1-33-x.sdk.operatorframework.io/docs/installation), version 1.33
- [git](https://git-scm.com/downloads)
- [go](https://golang.org/dl/), version 1.21
- [docker](https://docs.docker.com/install/), version 17.03+.
- [oc](https://kubernetes.io/docs/tasks/tools/install-oc/) or [oc](https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html#installing-openshift-cli) and access to an OpenShift cluster of a compatible version.
- [golangci-lint](https://golangci-lint.run/usage/install/#local-installation), version v1.54.2
