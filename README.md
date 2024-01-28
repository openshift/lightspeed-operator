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

1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

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
