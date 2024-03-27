# OpenShift Lightspeed operator

A Kubernetes operator for managing [Red Hat OpenShift Lightspeed](https://github.com/openshift/lightspeed-service).

## Getting Started

You'll need an OpenShift cluster to run against.

> [!IMPORTANT]
> The Operator only supports OpenAI and BAM as large language model (LLM) providers.

### Running on the cluster

**Note:** Your controller will automatically use the current context from your `kubeconfig` file (i.e. whatever cluster `oc cluster-info` shows).

1. Deploy the controller to the cluster:

```shell
make deploy
```

2. Create a secret containing the API Key for BAM or OpenAI. The key for API key is `apitoken`.

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
> BAM example

```yaml
apiVersion: v1
data:
  apitoken: <base64 encoded API Key>
kind: Secret
metadata:
  name: bam-api-keys
  namespace: openshift-lightspeed
type: Opaque
```
These `apitoken` values can be updated if user wishes to change them later. They get reflected automatically into the system.

3. Create an `OLSConfig` custom resource

```yaml
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  llm:
    providers:
    - credentialsSecretRef:
        name: openai-api-keys
      models:
      - name: gpt-3.5-turbo
      name: openai
      url: https://api.openai.com/v1
    - credentialsSecretRef:
        name: bam-api-keys
      models:
      - name: ibm/granite-13b-chat-v2
      name: bam
      url: https://bam-api.res.ibm.com
  ols:
    conversationCache:
      redis:
        maxMemory: 2000mb
        maxMemoryPolicy: allkeys-lru
      type: redis
    defaultModel: ibm/granite-13b-chat-v2
    defaultProvider: bam
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
kube-root-ca.crt           1      4m11s
olsconfig                  1      3m5s
openshift-service-ca.crt   1      4m11s

➜ oc get services -n openshift-lightspeed
NAME                                                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
lightspeed-app-server                                    ClusterIP   172.30.176.179   <none>        8080/TCP   4m47s
lightspeed-redis-server                                  ClusterIP   172.30.85.42     <none>        6379/TCP   4m47s
lightspeed-operator-controller-manager-metrics-service   ClusterIP   172.30.35.253    <none>        8443/TCP   4m47s

➜ oc get deployments -n openshift-lightspeed
NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
lightspeed-app-server                    1/1     1            1           7m5s
lightspeed-redis-server                  1/1     1            1           7m5s
lightspeed-operator-controller-manager   1/1     1            1           2d15h

➜ oc get pods -n openshift-lightspeed
NAME                                                      READY   STATUS              RESTARTS      AGE
lightspeed-app-server-f7fd6cf6-k7s7p                      1/1     Running             0             6m47s
lightspeed-operator-controller-manager-7c849865ff-9vwj9   2/2     Running             0             7m19s
lightspeed-redis-server-7b75497676-np7zk                  1/1     Running             0             6m47s

➜ oc logs lightspeed-app-server-f7fd6cf6-k7s7p -n openshift-lightspeed
2024-02-02 12:00:06,982 [ols.app.main:main.py:29] INFO: Embedded Gradio UI is disabled. To enable set enable_dev_ui: true in the dev section of the configuration file
INFO:     Started server process [1]
INFO:     Waiting for application startup.
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8080 (Press CTRL+C to quit)
```

#### Redis Secret Management
By default redis server spins up with a randomly generated password located in the secret `lightspeed-redis-secret`. One can go edit password their password to a desired value to get it reflected across the system. In addition to that redis secret name can also be explicitly specified in cluster CR as shown in the below example.
```
conversationCache:
  redis:
    maxMemory: "2000mb"
    maxMemoryPolicy: "allkeys-lru"
    credentialsSecret: xyz
  type: redis
```

### Modifying the API definitions

If you have updated the API definitions, you must update the CRD manifests with the following command

```shell
make manifests
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
