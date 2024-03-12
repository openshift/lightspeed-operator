# OpenShift Lightspeed operator

A Kubernetes operator for managing OpenShift Lightspeed service.

## Getting Started

You'll need a OpenShift cluster to run against.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `oc cluster-info` shows).

### Running on the cluster

1. Deploy the controller to the cluster:

```sh
make deploy
```

2. Install Instances of Custom Resources:

```sh
oc apply -f config/samples/ols_v1alpha1_olsconfig.yaml
```

### Uninstall CRDs

To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller

UnDeploy the controller from the cluster:

```sh
make undeploy
```

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out

#### Run Operator Alone

1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

#### Reconcile OLSConfig Custom Resource

The namespace `openshift-lightspeed` should be created before this test.
All the following resources are deployed into this namespace.

1. Create secret containing the API Key for BAM or OpenAI. The key for API key is `apitoken`.

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

2. Create OLSConfig custom resource

```yaml
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
        maxMemory: "2000mb"
        maxMemoryPolicy: "allkeys-lru"
      type: redis
    defaultModel: ibm/granite-13b-chat-v2
    defaultProvider: bam
    enableDeveloperUI: false
    logLevel: INFO
    deployment:
      replicas: 1
```

3. The operator should create below set of resources for running the [lightspeed-service](https://github.com/openshift/lightspeed-service) application server.

```shell
➜ kubectl get secrets -n openshift-lightspeed
NAME                                                     TYPE                                  DATA   AGE
builder-dockercfg-cqcsz                                  kubernetes.io/dockercfg               1      10m
builder-token-2vkj5                                      kubernetes.io/service-account-token   4      10m
default-dockercfg-tptk6                                  kubernetes.io/dockercfg               1      10m
default-token-l5q4k                                      kubernetes.io/service-account-token   4      10m
deployer-dockercfg-9p7w8                                 kubernetes.io/dockercfg               1      10m
deployer-token-kcmmd                                     kubernetes.io/service-account-token   4      10m
lightspeed-app-server-dockercfg-rd7qk                    kubernetes.io/dockercfg               1      9m33s
lightspeed-app-server-token-4g6cj                        kubernetes.io/service-account-token   4      9m33s
lightspeed-operator-controller-manager-dockercfg-wdgjz   kubernetes.io/dockercfg               1      10m
lightspeed-operator-controller-manager-token-whfr6       kubernetes.io/service-account-token   4      10m
lightspeed-redis-certs                                   kubernetes.io/tls                     2      22s
lightspeed-redis-secret                                  Opaque                                1      2s

➜ kubectl get configmaps -n openshift-lightspeed
NAME                       DATA   AGE
kube-root-ca.crt           1      4m11s
olsconfig                  1      3m5s
openshift-service-ca.crt   1      4m11s

➜ kubectl get services -n openshift-lightspeed
NAME                                                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
lightspeed-app-server                                    ClusterIP   172.30.176.179   <none>        8080/TCP   4m47s
lightspeed-redis-server                                  ClusterIP   172.30.85.42     <none>        6379/TCP   4m47s
lightspeed-operator-controller-manager-metrics-service   ClusterIP   172.30.35.253    <none>        8443/TCP   4m47s

➜ kubectl get deployments -n openshift-lightspeed
NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
lightspeed-app-server                    1/1     1            1           7m5s
lightspeed-redis-server                  1/1     1            1           7m5s
lightspeed-operator-controller-manager   1/1     1            1           2d15h

➜ kubectl get pods -n openshift-lightspeed
NAME                                                      READY   STATUS              RESTARTS      AGE
lightspeed-app-server-f7fd6cf6-k7s7p                      1/1     Running             0             6m47s
lightspeed-operator-controller-manager-7c849865ff-9vwj9   2/2     Running             0             7m19s
lightspeed-redis-server-7b75497676-np7zk                  1/1     Running             0             6m47s

➜ kubectl logs lightspeed-app-server-f7fd6cf6-k7s7p -n openshift-lightspeed
2024-02-02 12:00:06,982 [ols.app.main:main.py:29] INFO: Embedded Gradio UI is disabled. To enable set enable_dev_ui: true in the dev section of the configuration file
INFO:     Started server process [1]
INFO:     Waiting for application startup.
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8080 (Press CTRL+C to quit)
```

#### Redis Secret Management
By default redis server spins up with a randomly generated password located in the secret `lightspeed-redis-secret`. One can go edit password their password to a desired value to get it reflected across the system. In addition to that redis secret name can also be explicitly specified in the cluster CR as shown in the below example.
```
conversationCache:
  redis:
    maxMemory: "2000mb"
    maxMemoryPolicy: "allkeys-lru"
    credentialsSecretRef:
      name: xyz
  type: redis
```

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

## Prerequisites

To develop the Operator you'll need the following tools:

- [OperatorSDK](https://v1-33-x.sdk.operatorframework.io/docs/installation), version 1.33
- [git](https://git-scm.com/downloads)
- [go](https://golang.org/dl/), version 1.21
- [docker](https://docs.docker.com/install/), version 17.03+.
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) or [oc](https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html#installing-openshift-cli) and access to an OpenShift cluster of a compatible version.
- [golangci-lint](https://golangci-lint.run/usage/install/#local-installation), version v1.54.2
