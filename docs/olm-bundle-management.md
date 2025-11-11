# OLM Bundle Management Guide

This guide provides detailed information about managing Operator Lifecycle Manager (OLM) bundles for the OpenShift Lightspeed Operator.

## Table of Contents

- [Overview](#overview)
- [Bundle Structure](#bundle-structure)
- [ClusterServiceVersion Anatomy](#clusterserviceversion-anatomy)
- [Bundle Annotations](#bundle-annotations)
- [Bundle Generation Workflow](#bundle-generation-workflow)
- [Related Images Management](#related-images-management)
- [Version Management](#version-management)
- [Bundle Validation](#bundle-validation)
- [Common Tasks](#common-tasks)
- [Troubleshooting](#troubleshooting)

---

## Overview

OLM bundles are the packaging format for Kubernetes operators. A bundle contains:

- **Manifests**: Kubernetes resources that define the operator (CRDs, RBAC, CSV)
- **Metadata**: Information about the bundle for OLM consumption
- **Dockerfile**: Instructions for building the bundle image

The bundle is the unit of distribution for operators and is consumed by OLM to install and manage operator lifecycle.

---

## Bundle Structure

The bundle lives in the `bundle/` directory with the following structure:

```
bundle/
├── manifests/                                    # Kubernetes manifests
│   ├── lightspeed-operator.clusterserviceversion.yaml  # Main CSV file
│   ├── ols.openshift.io_olsconfigs.yaml         # CRD definition
│   ├── *_service.yaml                           # Service definitions
│   ├── *_clusterrole*.yaml                      # RBAC resources
│   └── *_servicemonitor.yaml                    # Monitoring resources
├── metadata/
│   └── annotations.yaml                         # Bundle metadata
└── tests/
    └── scorecard/
        └── config.yaml                          # Scorecard test configuration

bundle.Dockerfile                                 # Bundle image build instructions
```

### Key Files

#### `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`

The ClusterServiceVersion (CSV) is the centerpiece of the bundle. It contains:
- Operator metadata (name, version, description, icon)
- Install strategy (deployments, permissions, service accounts)
- CRD ownership information
- Related images
- Upgrade information

#### `bundle/metadata/annotations.yaml`

Contains OLM-specific metadata:
```yaml
annotations:
  # OLM bundle format
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  
  # Package and channel information
  operators.operatorframework.io.bundle.package.v1: lightspeed-operator
  operators.operatorframework.io.bundle.channels.v1: alpha
  operators.operatorframework.io.bundle.channel.default.v1: alpha
  
  # OpenShift-specific annotations
  com.redhat.openshift.versions: v4.16-v4.20  # OCP version compatibility
  features.operators.openshift.io/fips-compliant: "true"
```

---

## ClusterServiceVersion Anatomy

The CSV is structured into several key sections. Below is a comprehensive breakdown of all major properties and their usage.

### Metadata Section

```yaml
metadata:
  name: lightspeed-operator.v1.0.6
  namespace: openshift-lightspeed
  annotations:
    alm-examples: '[...]'  # Example CRs for the operator
    capabilities: Basic Install
    features.operators.openshift.io/disconnected: "true"
    features.operators.openshift.io/fips-compliant: "true"
    operators.operatorframework.io/suggested-namespace: openshift-lightspeed
    createdAt: "2025-10-03T15:49:27Z"
    repository: https://github.com/openshift/lightspeed-operator
```

**Key Fields:**

| Field | Required | Description | Example |
|-------|----------|-------------|---------|
| `name` | Yes | CSV name following format `<package>.v<version>` | `lightspeed-operator.v1.0.6` |
| `namespace` | No | Suggested installation namespace | `openshift-lightspeed` |
| `annotations.alm-examples` | No | JSON array of example CRs shown in console | `'[{"apiVersion":"ols.openshift.io/v1alpha1",...}]'` |
| `annotations.capabilities` | Yes | Operator maturity level | `Basic Install`, `Seamless Upgrades`, `Full Lifecycle`, `Deep Insights` |
| `annotations.createdAt` | No | Timestamp of CSV creation | `"2025-10-03T15:49:27Z"` |
| `annotations.repository` | No | Source code repository URL | `https://github.com/org/repo` |
| `annotations.containerImage` | No | Main operator container image | `quay.io/org/operator:v1.0.0` |
| `annotations.features.operators.openshift.io/*` | No | OpenShift feature declarations | `disconnected`, `fips-compliant`, `proxy-aware` |
| `annotations.operators.operatorframework.io/suggested-namespace` | No | Recommended installation namespace | `openshift-lightspeed` |

**Capability Levels:**
1. **Basic Install**: Operator can be installed
2. **Seamless Upgrades**: Supports upgrades between versions
3. **Full Lifecycle**: Can manage complete application lifecycle
4. **Deep Insights**: Provides metrics and alerts
5. **Auto Pilot**: Fully autonomous operation

### Spec Section

The spec contains all the information OLM needs to install and manage the operator.

#### Top-Level Spec Properties

```yaml
spec:
  displayName: OpenShift Lightspeed Operator
  description: |
    OpenShift Lightspeed Operator provides generative AI-based virtual assistant...
  version: 1.0.6
  maturity: alpha
  minKubeVersion: 1.28.0
  
  provider:
    name: Red Hat, Inc
    url: https://github.com/openshift/lightspeed-service
  
  maintainers:
    - name: OpenShift Lightspeed Team
      email: openshift-lightspeed-contact-requests@redhat.com
  
  links:
    - name: Lightspeed Operator
      url: https://github.com/openshift/lightspeed-operator
  
  keywords:
    - ai
    - assistant
    - openshift
    - llm
  
  icon:
    - base64data: iVBORw0KG...
      mediatype: image/png
  
  replaces: lightspeed-operator.v1.0.5  # For upgrades
  skips: []  # Versions that can be skipped during upgrade
```

**Property Reference:**

| Property | Required | Type | Description | Example |
|----------|----------|------|-------------|---------|
| `displayName` | Yes | string | Human-readable operator name | `OpenShift Lightspeed Operator` |
| `description` | Yes | string | Detailed operator description (supports markdown) | Multi-line description |
| `version` | Yes | string | Semantic version of the operator | `1.0.6` |
| `maturity` | No | string | Development phase | `alpha`, `beta`, `stable`, `deprecated` |
| `minKubeVersion` | No | string | Minimum Kubernetes version | `1.28.0` |
| `provider.name` | Yes | string | Organization providing the operator | `Red Hat, Inc` |
| `provider.url` | No | string | Provider's website | `https://redhat.com` |
| `maintainers` | No | array | List of maintainer contacts | `[{name, email}]` |
| `links` | No | array | Related URLs (docs, source, etc.) | `[{name, url}]` |
| `keywords` | No | array | Search keywords for OperatorHub | `["ai", "ml"]` |
| `icon` | No | array | Base64-encoded icon | `[{base64data, mediatype}]` |
| `replaces` | No | string | Previous version this replaces | `operator.v1.0.5` |
| `skips` | No | array | Versions skippable during upgrade | `["operator.v1.0.4"]` |

**Upgrade Path Properties:**
- `replaces`: Defines the upgrade path. Set to the previous version to create a linear upgrade chain
- `skips`: Advanced feature to skip intermediate versions during upgrade
- If neither is set, this is treated as a new installation (no upgrade path)

#### Install Modes

Defines where the operator can be installed:

```yaml
spec:
  installModes:
    - type: OwnNamespace      # Install in operator's namespace
      supported: true
    - type: SingleNamespace   # Install in one specific namespace
      supported: false
    - type: MultiNamespace    # Install watching multiple namespaces
      supported: false
    - type: AllNamespaces     # Install watching all namespaces
      supported: true
```

**Install Mode Types:**

| Mode | Description | Use Case |
|------|-------------|----------|
| `OwnNamespace` | Operator watches its own namespace | Development/testing |
| `SingleNamespace` | Operator watches one namespace | Namespace isolation |
| `MultiNamespace` | Operator watches specific namespaces | Multi-tenant with selection |
| `AllNamespaces` | Operator watches cluster-wide | Cluster-scoped resources (like Lightspeed) |

**Note:** Lightspeed Operator only supports `AllNamespaces` because `OLSConfig` is cluster-scoped.

#### Install Strategy

Defines how OLM should install the operator:

```yaml
spec:
  install:
    strategy: deployment
    spec:
      clusterPermissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            - apiGroups: ["ols.openshift.io"]
              resources: ["olsconfigs"]
              verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
      
      permissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            - apiGroups: [""]
              resources: ["configmaps"]
              verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
      
      deployments:
        - name: lightspeed-operator-controller-manager
          spec:
            replicas: 1
            selector:
              matchLabels:
                control-plane: controller-manager
            template:
              spec:
                containers:
                  - name: manager
                    image: quay.io/openshift-lightspeed/lightspeed-operator:latest
                    args:
                      - --leader-elect
                      - --service-image=<service-image>
                      - --console-image=<console-image>
```

**Key Components:**
- **clusterPermissions**: Cluster-wide RBAC rules
- **permissions**: Namespace-scoped RBAC rules
- **deployments**: The operator deployment specification

#### Custom Resource Definitions

Declares which CRDs the operator owns and provides:

```yaml
spec:
  customresourcedefinitions:
    owned:
      - name: olsconfigs.ols.openshift.io
        version: v1alpha1
        kind: OLSConfig
        displayName: OLSConfig
        description: Red Hat OpenShift Lightspeed instance
        specDescriptors:
          - path: llm.providers[0].name
            displayName: Name
            description: Provider name
          - path: ols.deployment.replicas
            displayName: Number of replicas
            description: Defines the number of desired OLS pods. Default is 1
            x-descriptors:
              - 'urn:alm:descriptor:com.tectonic.ui:podCount'
        statusDescriptors:
          - path: conditions
            displayName: Conditions
            x-descriptors:
              - 'urn:alm:descriptor:io.kubernetes.conditions'
    
    required:  # CRDs that must exist (provided by other operators)
      - name: servicemonitors.monitoring.coreos.com
        version: v1
        kind: ServiceMonitor
        displayName: Service Monitor
```

**CRD Property Reference:**

| Field | Description | Example |
|-------|-------------|---------|
| `name` | Fully qualified CRD name | `olsconfigs.ols.openshift.io` |
| `version` | CRD API version | `v1alpha1` |
| `kind` | CRD Kind | `OLSConfig` |
| `displayName` | Human-readable name | `OLS Configuration` |
| `description` | Detailed description | Shown in console UI |
| `resources` | Kubernetes resources created by this CR | `[{kind, version, name}]` |
| `specDescriptors` | Describe spec fields for UI | See below |
| `statusDescriptors` | Describe status fields for UI | See below |

**Descriptors** define how fields appear in the OpenShift Console:

```yaml
specDescriptors:
  - path: llm.providers[0].name           # JSONPath to field
    displayName: Provider Name             # Label in UI
    description: The LLM provider name     # Help text
    x-descriptors:                         # UI component hints
      - 'urn:alm:descriptor:com.tectonic.ui:text'
```

**Common x-descriptors:**

| Descriptor | Usage | Example Field |
|------------|-------|---------------|
| `urn:alm:descriptor:com.tectonic.ui:text` | Text input | Name, URL |
| `urn:alm:descriptor:com.tectonic.ui:password` | Password input | API token |
| `urn:alm:descriptor:com.tectonic.ui:number` | Number input | Port |
| `urn:alm:descriptor:com.tectonic.ui:booleanSwitch` | Toggle switch | Enabled flag |
| `urn:alm:descriptor:com.tectonic.ui:podCount` | Pod count input | Replicas |
| `urn:alm:descriptor:com.tectonic.ui:resourceRequirements` | Resource editor | CPU/Memory |
| `urn:alm:descriptor:com.tectonic.ui:nodeSelector` | Node selector | Node labels |
| `urn:alm:descriptor:com.tectonic.ui:advanced` | Advanced section | Optional configs |
| `urn:alm:descriptor:io.kubernetes:Secret` | Secret reference | Secret name |
| `urn:alm:descriptor:io.kubernetes:ConfigMap` | ConfigMap reference | ConfigMap name |

**Owned vs Required:**
- `owned`: CRDs provided by this operator (must be in bundle)
- `required`: CRDs that must exist before installation (from other operators)

#### Related Images

Lists all container images used by the operator and its operands:

```yaml
spec:
  relatedImages:
    - name: lightspeed-service-api
      image: registry.redhat.io/.../lightspeed-service-api@sha256:...
    - name: lightspeed-console-plugin
      image: registry.redhat.io/.../lightspeed-console-plugin@sha256:...
    - name: lightspeed-operator
      image: registry.redhat.io/.../lightspeed-operator@sha256:...
    - name: openshift-mcp-server
      image: quay.io/.../openshift-mcp-server@sha256:...
```

**Purpose:**
- **Image mirroring**: Enable disconnected installations by listing all images
- **Vulnerability scanning**: Tools can scan all images referenced
- **Image pinning**: Prevent drift by using specific image versions
- **Compliance**: Required for OpenShift certification

**Best Practices:**
- Always use image digests (SHA256) in production bundles
- Include all operand images (service, console, database, etc.)
- Keep in sync with deployment arguments
- Include operator's own image
- List init containers and sidecar images

#### API Service Definitions

For operators that provide Kubernetes API extensions via aggregated API servers:

```yaml
spec:
  apiservicedefinitions:
    owned:
      - name: v1.custom.metrics.k8s.io
        group: custom.metrics.k8s.io
        version: v1
        kind: CustomMetric
        displayName: Custom Metrics
        description: Custom metrics API
        deploymentName: custom-metrics-server
        containerPort: 443
```

**Note:** Lightspeed Operator doesn't use API services (uses CRDs instead).

#### Webhook Definitions

For operators that provide admission webhooks:

```yaml
spec:
  webhookdefinitions:
    - type: ValidatingAdmissionWebhook
      admissionReviewVersions:
        - v1
        - v1beta1
      containerPort: 443
      targetPort: 9443
      deploymentName: lightspeed-operator-webhook
      failurePolicy: Fail
      generateName: validate.ols.openshift.io
      rules:
        - apiGroups:
            - ols.openshift.io
          apiVersions:
            - v1alpha1
          operations:
            - CREATE
            - UPDATE
          resources:
            - olsconfigs
      sideEffects: None
      webhookPath: /validate-ols-openshift-io-v1alpha1-olsconfig
```

**Webhook Types:**
- `ValidatingAdmissionWebhook`: Validate resource changes
- `MutatingAdmissionWebhook`: Modify resources during admission
- `ConversionWebhook`: Convert between API versions

**Note:** Lightspeed Operator currently doesn't use webhooks but may add validation webhooks in the future.

#### Dependency Definitions

Declare dependencies on other operators or GVKs (Group/Version/Kind) that must exist:

```yaml
spec:
  dependencies:
    # Depend on another operator
    - type: olm.package
      packageName: prometheus-operator
      version: ">=0.47.0"
    
    # Depend on a specific GVK
    - type: olm.gvk
      group: monitoring.coreos.com
      version: v1
      kind: ServiceMonitor
    
    # Depend on a label on another operator
    - type: olm.label
      label: "prometheus"
```

**Dependency Types:**

| Type | Description | Example Use Case |
|------|-------------|------------------|
| `olm.package` | Requires another operator package | Need Prometheus Operator installed |
| `olm.gvk` | Requires a specific API (Group/Version/Kind) | Need ServiceMonitor CRD available |
| `olm.label` | Requires operator with specific label | Flexible dependency matching |
| `olm.constraint` | Generic constraint expression | Complex dependency logic |

**Package Dependency Properties:**

```yaml
- type: olm.package
  packageName: prometheus-operator    # Required: package name
  version: ">=0.47.0"                 # Optional: version constraint
```

**Version Constraints:**
- `=1.0.0` - Exact version
- `>=1.0.0` - Greater than or equal
- `>1.0.0 <2.0.0` - Range
- `>=1.0.0 !1.5.0` - Exclude specific version

**GVK Dependency Properties:**

```yaml
- type: olm.gvk
  group: monitoring.coreos.com        # API group
  version: v1                         # API version
  kind: ServiceMonitor                # Resource kind
```

**Constraint Dependencies (Advanced):**

The `olm.constraint` type allows complex dependency expressions using Common Expression Language (CEL):

```yaml
- type: olm.constraint
  value: |
    # Package version constraint
    package.name == "prometheus-operator" && 
    package.version >= "0.47.0" && 
    package.version < "1.0.0"
```

**Common Constraint Patterns:**

1. **All-of (AND) - Multiple packages must exist:**
   ```yaml
   - type: olm.constraint
     value: |
       all:
         - package.name == "prometheus-operator"
         - package.name == "cert-manager"
   ```

2. **Any-of (OR) - At least one package must exist:**
   ```yaml
   - type: olm.constraint
     value: |
       any:
         - package.name == "aws-provider"
         - package.name == "azure-provider"
         - package.name == "gcp-provider"
   ```

3. **Not - Package must not exist:**
   ```yaml
   - type: olm.constraint
     value: |
       not:
         package.name == "conflicting-operator"
   ```

4. **Complex version ranges:**
   ```yaml
   - type: olm.constraint
     value: |
       package.name == "my-dependency" &&
       (package.version >= "1.0.0" && package.version < "2.0.0") ||
       (package.version >= "2.5.0" && package.version < "3.0.0")
   ```

5. **Property-based constraints:**
   ```yaml
   - type: olm.constraint
     value: |
       properties.exists(p, p.type == "olm.gvk" && 
         p.value.group == "monitoring.coreos.com" && 
         p.value.kind == "ServiceMonitor")
   ```

**Constraint Expression Fields:**

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `package.name` | string | Package name | `"prometheus-operator"` |
| `package.version` | semver | Package version | `"0.47.0"` |
| `properties` | list | Package properties | CRDs, labels, etc. |

**Comparison Operators:**
- `==` - Equals
- `!=` - Not equals
- `>`, `>=` - Greater than (or equal)
- `<`, `<=` - Less than (or equal)

**Logical Operators:**
- `&&` - AND
- `||` - OR
- `!` - NOT
- `all:` - All conditions must be true
- `any:` - At least one condition must be true

**Functions:**
- `properties.exists(var, condition)` - Check if a property exists matching condition
- `properties.all(var, condition)` - All properties must match condition
- `properties.any(var, condition)` - Any property must match condition

**When to Use Constraints:**

| Use Case | Recommended Dependency Type | Reason |
|----------|----------------------------|--------|
| Simple package dependency | `olm.package` | Clearer, simpler |
| API/CRD requirement | `olm.gvk` | Most flexible, version-agnostic |
| Label-based selection | `olm.label` | Simple label matching |
| Complex version logic | `olm.constraint` | Full expressiveness |
| Multiple alternatives | `olm.constraint` with `any:` | Can't express with simple types |
| Exclusions | `olm.constraint` with `not:` | Can't express with simple types |

**Real-World Examples:**

**Example 1: Require one of multiple storage operators**
```yaml
- type: olm.constraint
  value: |
    any:
      - package.name == "rook-ceph-operator"
      - package.name == "portworx-operator"
      - package.name == "longhorn-operator"
```

**Example 2: Require specific feature in dependency**
```yaml
- type: olm.constraint
  value: |
    package.name == "prometheus-operator" &&
    properties.exists(p, 
      p.type == "olm.label" && 
      p.value == "monitoring.coreos.com/prometheus-operator")
```

**Example 3: Version with exclusions**
```yaml
- type: olm.constraint
  value: |
    package.name == "my-dependency" &&
    package.version >= "1.0.0" &&
    package.version != "1.5.0" &&  # Known broken version
    package.version != "1.7.0"     # Security issue
```

**Example 4: Platform-specific dependencies**
```yaml
- type: olm.constraint
  value: |
    # Require AWS provider on AWS, GCP provider on GCP, etc.
    (cluster.platform == "AWS" && package.name == "aws-cloud-controller") ||
    (cluster.platform == "GCP" && package.name == "gcp-cloud-controller") ||
    (cluster.platform == "Azure" && package.name == "azure-cloud-controller")
```

**Note:** `olm.constraint` is powerful but more complex. Use simpler dependency types when possible for better readability.

**Example: Lightspeed Dependencies**

```yaml
spec:
  dependencies:
    # Require Prometheus Operator for ServiceMonitors
    - type: olm.gvk
      group: monitoring.coreos.com
      version: v1
      kind: ServiceMonitor
    
    # Require OpenShift Console Operator (implicit in OpenShift)
    - type: olm.gvk
      group: console.openshift.io
      version: v1
      kind: ConsolePlugin
```

**Best Practices:**
- Use `olm.gvk` for API dependencies (more flexible than package dependencies)
- Specify minimum versions with `>=` to allow newer versions
- Avoid overly restrictive version constraints
- Document why each dependency is needed
- Test installation with minimum dependency versions

#### Native API Definitions

Declare native Kubernetes APIs the operator requires (beyond standard APIs):

```yaml
spec:
  nativeAPIs:
    - group: apps
      version: v1
      kind: Deployment
    - group: rbac.authorization.k8s.io
      version: v1
      kind: ClusterRole
```

**Note:** Usually not needed as core APIs are assumed available. Use for optional APIs like CustomMetrics or Aggregation layer.

#### Resource Requirements

Define resource requirements for the operator's own deployment:

**In the Install Strategy Deployment Spec:**

```yaml
spec:
  install:
    spec:
      deployments:
        - name: lightspeed-operator-controller-manager
          spec:
            template:
              spec:
                containers:
                  - name: manager
                    resources:
                      limits:
                        cpu: 500m
                        memory: 256Mi
                      requests:
                        cpu: 10m
                        memory: 64Mi
```

**Resource Properties:**

| Field | Description | Example |
|-------|-------------|---------|
| `requests.cpu` | Minimum CPU guaranteed | `10m`, `100m`, `1` (1 core) |
| `requests.memory` | Minimum memory guaranteed | `64Mi`, `128Mi`, `1Gi` |
| `limits.cpu` | Maximum CPU allowed | `500m`, `1`, `2` |
| `limits.memory` | Maximum memory allowed | `256Mi`, `512Mi`, `2Gi` |

**CPU Units:**
- `m` = millicores (1000m = 1 core)
- `1` = 1 core
- `2` = 2 cores

**Memory Units:**
- `Ki`, `Mi`, `Gi`, `Ti` - Binary (1024-based)
- `K`, `M`, `G`, `T` - Decimal (1000-based)

**Best Practices:**

1. **Set Requests Lower Than Limits**
   ```yaml
   requests:
     cpu: 100m      # Guaranteed minimum
     memory: 128Mi
   limits:
     cpu: 1         # Can burst up to this
     memory: 512Mi
   ```

2. **Consider Burst Patterns**
   - Operators often idle with occasional reconciliation bursts
   - Set requests low for efficient bin-packing
   - Set limits higher to handle reconciliation spikes

3. **Memory Limits = Requests for Stability**
   ```yaml
   requests:
     memory: 256Mi
   limits:
     memory: 256Mi  # Same as request to prevent OOM
   ```

4. **Test Under Load**
   - Profile operator under various scenarios
   - Monitor actual resource usage
   - Adjust based on real-world data

**Example: Lightspeed Operator Resources**

```yaml
resources:
  limits:
    cpu: 500m       # Can spike during reconciliation
    memory: 256Mi   # Conservative limit
  requests:
    cpu: 10m        # Very low idle usage
    memory: 64Mi    # Minimal memory footprint
```

**Container-Specific Resources:**

For operators with multiple containers (main + sidecars):

```yaml
spec:
  install:
    spec:
      deployments:
        - name: operator
          spec:
            template:
              spec:
                containers:
                  - name: manager
                    resources:
                      requests:
                        cpu: 10m
                        memory: 64Mi
                      limits:
                        cpu: 500m
                        memory: 256Mi
                  
                  - name: kube-rbac-proxy
                    resources:
                      requests:
                        cpu: 5m
                        memory: 32Mi
                      limits:
                        cpu: 100m
                        memory: 128Mi
```

**Ephemeral Storage:**

For operators that use temporary storage:

```yaml
resources:
  requests:
    ephemeral-storage: 1Gi
  limits:
    ephemeral-storage: 2Gi
```

**Quality of Service (QoS) Classes:**

Based on resource configuration, pods get QoS classes:

| QoS Class | Condition | Behavior |
|-----------|-----------|----------|
| **Guaranteed** | `requests == limits` for all resources | Highest priority, last to be evicted |
| **Burstable** | `requests < limits` or only requests set | Medium priority, evicted before Guaranteed |
| **BestEffort** | No requests or limits set | Lowest priority, first to be evicted |

**Lightspeed Operator QoS:**
```yaml
# Burstable QoS - good balance for operators
requests:
  cpu: 10m
  memory: 64Mi
limits:
  cpu: 500m      # Different from request = Burstable
  memory: 256Mi
```

#### Min/Max Kubernetes Version Constraints

Specify Kubernetes version requirements:

```yaml
spec:
  minKubeVersion: 1.28.0  # Minimum supported version
```

**Version Format:**
- Use semantic versioning: `major.minor.patch`
- Patch version can be omitted: `1.28` (implies `1.28.0`)
- No `v` prefix

**OpenShift Version Mapping:**

| OpenShift Version | Kubernetes Version |
|-------------------|-------------------|
| 4.16 | 1.29 |
| 4.17 | 1.30 |
| 4.18 | 1.31 |
| 4.19 | 1.32 |
| 4.20 | 1.33 |

**Set Based on Features Used:**
- Check which Kubernetes APIs your operator uses
- Test against minimum version in CI
- Document why specific version is needed

**Example:**
```yaml
# Lightspeed requires 1.28.0 for:
# - Improved CRD validation
# - Specific RBAC features
minKubeVersion: 1.28.0
```

**Note:** There's no `maxKubeVersion` - operators should be forward-compatible.

### Complete CSV Structure Reference

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: operator.v1.0.0
  namespace: default
  annotations:
    # Required
    capabilities: Basic Install
    
    # Recommended
    alm-examples: '[{...}]'
    categories: AI/Machine Learning
    certified: "true"
    repository: https://github.com/org/repo
    containerImage: quay.io/org/operator:v1.0.0
    createdAt: "2024-01-01T00:00:00Z"
    support: Support Team
    
    # OpenShift specific
    features.operators.openshift.io/disconnected: "true"
    features.operators.openshift.io/fips-compliant: "false"
    operators.operatorframework.io/suggested-namespace: my-namespace

spec:
  # Identity and display
  displayName: My Operator
  description: |
    Long description with markdown support
  version: 1.0.0
  maturity: stable
  minKubeVersion: 1.24.0
  
  # Branding
  provider:
    name: Company Name
    url: https://company.com
  icon:
    - base64data: <base64-encoded-image>
      mediatype: image/png
  keywords: [keyword1, keyword2]
  maintainers:
    - name: Team
      email: team@company.com
  links:
    - name: Documentation
      url: https://docs.company.com
  
  # Upgrade path
  replaces: operator.v0.9.0
  skips: []
  
  # Installation
  installModes:
    - type: OwnNamespace
      supported: true
    - type: SingleNamespace
      supported: true
    - type: MultiNamespace
      supported: false
    - type: AllNamespaces
      supported: true
  
  # Install strategy
  install:
    strategy: deployment
    spec:
      clusterPermissions: [...]
      permissions: [...]
      deployments:
        - name: operator-controller-manager
          spec:
            replicas: 1
            template:
              spec:
                containers:
                  - name: manager
                    image: operator:v1.0.0
                    resources:
                      requests:
                        cpu: 10m
                        memory: 64Mi
                      limits:
                        cpu: 500m
                        memory: 256Mi
  
  # CRDs
  customresourcedefinitions:
    owned: [...]
    required: [...]
  
  # Dependencies
  dependencies:
    - type: olm.gvk
      group: monitoring.coreos.com
      version: v1
      kind: ServiceMonitor
  
  # API services (optional)
  apiservicedefinitions:
    owned: [...]
  
  # Webhooks (optional)
  webhookdefinitions: [...]
  
  # Native APIs (optional)
  nativeAPIs:
    - group: apps
      version: v1
      kind: Deployment
  
  # Images
  relatedImages: [...]
```

---

## Bundle Annotations

Bundle annotations serve different purposes:

### OLM Core Annotations

```yaml
operators.operatorframework.io.bundle.mediatype.v1: registry+v1
operators.operatorframework.io.bundle.manifests.v1: manifests/
operators.operatorframework.io.bundle.metadata.v1: metadata/
```

These define the bundle format and structure. Don't modify unless you know what you're doing.

### Package and Channel Annotations

```yaml
operators.operatorframework.io.bundle.package.v1: lightspeed-operator
operators.operatorframework.io.bundle.channels.v1: alpha
operators.operatorframework.io.bundle.channel.default.v1: alpha
```

- **package**: The operator package name (must be consistent across versions)
- **channels**: Comma-separated list of channels this bundle belongs to
- **channel.default**: The default channel for this bundle

### OpenShift Compatibility

```yaml
com.redhat.openshift.versions: v4.16-v4.20
```

Declares which OpenShift versions this operator supports. Format: `v<min>-v<max>` or specific versions `v4.16,v4.18`.

### Feature Annotations

```yaml
features.operators.openshift.io/disconnected: "true"
features.operators.openshift.io/fips-compliant: "true"
features.operators.openshift.io/proxy-aware: "false"
```

Declare operator capabilities for filtering in OperatorHub UI.

---

## Bundle Generation Workflow

### Automated Bundle Generation

**Implementation:**
- Makefile target: [`Makefile`](../Makefile) (lines 329-346)
- Generation script: [`hack/update_bundle.sh`](../hack/update_bundle.sh)
- Related images: [`related_images.json`](../related_images.json)
- Bundle Dockerfile: [`bundle.Dockerfile`](../bundle.Dockerfile)

The primary way to generate/update the bundle:

```bash
make bundle BUNDLE_TAG=1.0.7
```

This executes `hack/update_bundle.sh` which:

1. **Generates base manifests** using `operator-sdk`:
   ```bash
   operator-sdk generate kustomize manifests -q
   kustomize build config/manifests | operator-sdk generate bundle
   ```

2. **Updates image references** in the CSV using `related_images.json` or current CSV values

3. **Adds OpenShift compatibility** annotations to `bundle/metadata/annotations.yaml`

4. **Generates bundle Dockerfile** using the template `hack/template_bundle.Containerfile`

5. **Validates the bundle**:
   ```bash
   operator-sdk bundle validate ./bundle
   ```

### Manual Bundle Updates

Sometimes you need to manually edit bundle files:

1. **Edit the CSV** (`bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`):
   - Update descriptions
   - Modify RBAC rules
   - Add/update specDescriptors for better UI representation
   - Update icon or display name

2. **Edit annotations** (`bundle/metadata/annotations.yaml`):
   - Change channel membership
   - Update OpenShift version compatibility

3. **Validate changes**:
   ```bash
   operator-sdk bundle validate ./bundle
   ```

### Bundle Generation Script

The `hack/update_bundle.sh` script accepts several options:

```bash
./hack/update_bundle.sh \
  -v 1.0.7 \                          # Bundle version (required)
  -i related_images.json              # Related images file (optional)
```

**Key Environment Variables:**
- `BUNDLE_GEN_FLAGS`: Flags passed to `operator-sdk generate bundle`
- `BASE_IMAGE`: Base image for bundle (default: `registry.redhat.io/ubi9/ubi-minimal:9.6`)

---

## Related Images Management

### Purpose

The `related_images.json` file is used to:
1. Track all container images used by the operator
2. Update CSV with correct image references during bundle generation
3. Support CI/CD image promotion workflows
4. Enable image mirroring for disconnected environments

### File Format

```json
[
  {
    "name": "lightspeed-operator",
    "image": "quay.io/openshift-lightspeed/lightspeed-operator:latest"
  },
  {
    "name": "lightspeed-service-api",
    "image": "quay.io/openshift-lightspeed/lightspeed-service-api:latest"
  },
  {
    "name": "lightspeed-console-plugin",
    "image": "quay.io/openshift-lightspeed/lightspeed-console-plugin:latest"
  }
]
```

### Image Reference Flow

```
related_images.json
        ↓
hack/update_bundle.sh
        ↓
CSV relatedImages section
        ↓
CSV deployment args (--service-image, --console-image)
        ↓
Controller code reads args
        ↓
Operand deployments use images
```

### Updating Images

**Option 1: Update `related_images.json` before bundle generation**

```bash
# Edit related_images.json with new image references
vim related_images.json

# Generate bundle with updated images
make bundle BUNDLE_TAG=1.0.7 RELATED_IMAGES_FILE=related_images.json
```

**Option 2: Let bundle generation extract from existing CSV**

If `related_images.json` doesn't exist or isn't specified, the script extracts images from the current CSV.

### Image Digests vs Tags

**Development**: Use tags for faster iteration
```json
{"name": "lightspeed-operator", "image": "quay.io/.../lightspeed-operator:latest"}
```

**Production**: Always use digests for reproducibility
```json
{"name": "lightspeed-operator", "image": "quay.io/.../lightspeed-operator@sha256:abc123..."}
```

The bundle Dockerfile performs image reference replacements during build.

---

## Version Management

### Version Bumping Strategy

1. **Update `Makefile`**:
   ```makefile
   BUNDLE_TAG ?= 1.0.7
   ```

2. **Generate new bundle**:
   ```bash
   make bundle BUNDLE_TAG=1.0.7
   ```

3. **Review changes**:
   ```bash
   git diff bundle/
   ```

4. **Commit bundle changes**:
   ```bash
   git add bundle/ bundle.Dockerfile
   git commit -m "chore: bump bundle version to v1.0.7"
   ```

### Version Patches

For complex version updates across multiple files, use `hack/version_patches/`:

```
hack/version_patches/
├── 1.0.5.patch
├── 1.0.6.patch
└── ...
```

These patches can update:
- Image tags in the CSV
- Version references in documentation
- Channel information

### Semantic Versioning

Follow semantic versioning:
- **Major (x.0.0)**: Breaking changes, incompatible API updates
- **Minor (1.x.0)**: New features, backward-compatible
- **Patch (1.0.x)**: Bug fixes, backward-compatible

### Version in Multiple Places

Ensure version consistency across:
1. `Makefile` (`BUNDLE_TAG`)
2. CSV metadata name (`lightspeed-operator.v1.0.7`)
3. CSV spec version field
4. Bundle Dockerfile labels
5. Related catalog entries

---

## Bundle Validation

### Automatic Validation

Bundle generation automatically validates:

```bash
operator-sdk bundle validate ./bundle
```

### Manual Validation

Run validation explicitly:

```bash
# Basic validation
operator-sdk bundle validate ./bundle

# Validation for OpenShift
operator-sdk bundle validate ./bundle \
  --select-optional suite=operatorframework \
  --select-optional name=operatorhub
```

### Common Validation Errors

#### Missing required fields

```
Error: Value : (lightspeed-operator.v1.0.7) csv.Spec.minKubeVersion not specified
```

**Fix**: Add `minKubeVersion` to CSV spec:
```yaml
spec:
  minKubeVersion: 1.28.0
```

#### Invalid image references

```
Error: Value : (lightspeed-operator.v1.0.7) csv.Spec.relatedImages[0].image invalid
```

**Fix**: Ensure all images use valid references (preferably digests).

#### RBAC issues

```
Error: csv.Spec.install.spec.clusterPermissions[0] invalid
```

**Fix**: Verify RBAC rules are properly formatted and include all required fields.

### Validation Levels

- **Errors**: Must be fixed before bundle can be used
- **Warnings**: Should be fixed but won't prevent installation
- **Info**: Best practice suggestions

---

## Common Tasks

### Task 1: Update Operator Image

```bash
# 1. Update image in related_images.json
vim related_images.json

# 2. Regenerate bundle
make bundle BUNDLE_TAG=1.0.7 RELATED_IMAGES_FILE=related_images.json

# 3. Verify CSV has new image
grep "image:" bundle/manifests/lightspeed-operator.clusterserviceversion.yaml
```

### Task 2: Add New RBAC Permission

```bash
# 1. Update RBAC in config/rbac/
vim config/rbac/role.yaml

# 2. Regenerate manifests and bundle
make manifests
make bundle BUNDLE_TAG=1.0.7

# 3. Verify new permission in CSV
yq '.spec.install.spec.clusterPermissions[0].rules' \
  bundle/manifests/lightspeed-operator.clusterserviceversion.yaml
```

### Task 3: Change OpenShift Version Support

```bash
# 1. Edit annotations
vim bundle/metadata/annotations.yaml

# Change from:
com.redhat.openshift.versions: v4.16-v4.19

# To:
com.redhat.openshift.versions: v4.16-v4.20

# 2. Validate
operator-sdk bundle validate ./bundle
```

### Task 4: Add New Operand Image

```bash
# 1. Add to related_images.json
{
  "name": "new-component",
  "image": "quay.io/openshift-lightspeed/new-component:v1.0.0"
}

# 2. Update controller to use new image (code changes)

# 3. Add command-line arg in CSV deployment spec
args:
  - --new-component-image=<image>

# 4. Add to relatedImages in CSV (done by update_bundle.sh)

# 5. Regenerate bundle
make bundle BUNDLE_TAG=1.0.7 RELATED_IMAGES_FILE=related_images.json
```

### Task 5: Create Bundle Image

```bash
# 1. Generate/update bundle
make bundle BUNDLE_TAG=1.0.7

# 2. Build bundle image
make bundle-build BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v1.0.7

# 3. Push bundle image
make bundle-push BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v1.0.7

# 4. Verify bundle image
podman pull quay.io/myorg/lightspeed-operator-bundle:v1.0.7
```

---

## Troubleshooting

### Issue: Bundle Validation Fails

**Symptom**:
```
Error: Value : (lightspeed-operator.v1.0.7) this bundle is not valid
```

**Diagnosis**:
```bash
# Run validation with verbose output
operator-sdk bundle validate ./bundle -o text
```

**Common Fixes**:
- Check CSV syntax (YAML indentation)
- Verify all required fields are present
- Ensure image references are valid
- Check RBAC rules format

### Issue: Images Not Updated in CSV

**Symptom**: After bundle generation, CSV still has old image references

**Diagnosis**:
```bash
# Check what update_bundle.sh is seeing
YQ=$(which yq) JQ=$(which jq) ./hack/update_bundle.sh -v 1.0.7 -i related_images.json
```

**Common Fixes**:
- Verify `related_images.json` format
- Check image names match expected pattern
- Ensure `yq` and `jq` are installed
- Review `hack/update_bundle.sh` logic

### Issue: Bundle Build Fails

**Symptom**:
```
Error: failed to build bundle image
```

**Diagnosis**:
```bash
# Check bundle.Dockerfile syntax
cat bundle.Dockerfile

# Try building manually
podman build -f bundle.Dockerfile -t test-bundle .
```

**Common Fixes**:
- Regenerate bundle.Dockerfile: `make bundle`
- Check base image is accessible
- Verify all referenced files exist in `bundle/` directory

### Issue: OLM Can't Install Bundle

**Symptom**: Bundle installs but operator doesn't start

**Diagnosis**:
```bash
# Check OLM catalog pod logs
oc logs -n olm <catalog-pod>

# Check subscription status
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml

# Check install plan
oc get installplan -n openshift-lightspeed
```

**Common Fixes**:
- Verify all RBAC permissions are present
- Check service account exists
- Ensure CRD is valid and installs successfully
- Review deployment specification in CSV

### Issue: Wrong Channel

**Symptom**: Bundle appears in wrong channel or no channel

**Diagnosis**:
```bash
# Check bundle annotations
cat bundle/metadata/annotations.yaml | grep channel
```

**Fix**:
```bash
# Update channel annotations
vim bundle/metadata/annotations.yaml

# Ensure these match your intent:
operators.operatorframework.io.bundle.channels.v1: alpha
operators.operatorframework.io.bundle.channel.default.v1: alpha
```

---

## Best Practices

### 1. Version Control

- Always commit bundle changes together with code changes
- Tag releases after bundle updates
- Keep bundle versions in sync with operator versions

### 2. Image Management

- Use digests in production bundles
- Test with tags during development
- Keep `related_images.json` up to date

### 3. RBAC

- Follow principle of least privilege
- Document why each permission is needed
- Separate cluster-wide and namespace permissions

### 4. Testing

- Validate bundle after every change
- Test installation in a real cluster
- Verify upgrade paths

### 5. Documentation

- Update CSV descriptions when features change
- Keep `alm-examples` current
- Use meaningful specDescriptors for better UX

---

## Additional Resources

### Related Guides

- **[OLM Catalog Management Guide](./olm-catalog-management.md)** - Learn about organizing bundles into catalogs (next step after bundle creation)
- **[Contributing Guide](../CONTRIBUTING.md)** - General contribution guidelines
- **[Architecture Documentation](../ARCHITECTURE.md)** - Operator architecture overview

### External Resources

- [Operator SDK Bundle Documentation](https://sdk.operatorframework.io/docs/olm-integration/tutorial-bundle/)
- [OLM Bundle Format Specification](https://olm.operatorframework.io/docs/tasks/creating-operator-bundle/)
- [ClusterServiceVersion Spec](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/)
- [OpenShift Operator Certification](https://redhat-connect.gitbook.io/certified-operator-guide/)
- Project Scripts:
  - `hack/update_bundle.sh` - Bundle generation
  - `hack/bundle_to_catalog.sh` - Catalog creation
  - `hack/release_tools.md` - Release process

---

## Quick Reference

### Bundle Generation

```bash
# Standard bundle generation
make bundle BUNDLE_TAG=1.0.7

# With custom images
make bundle BUNDLE_TAG=1.0.7 RELATED_IMAGES_FILE=related_images.json

# With custom channel
make bundle BUNDLE_TAG=1.0.7 CHANNELS=stable DEFAULT_CHANNEL=stable
```

### Bundle Building

```bash
# Build bundle image
make bundle-build BUNDLE_IMG=quay.io/org/bundle:v1.0.7

# Push bundle image
make bundle-push BUNDLE_IMG=quay.io/org/bundle:v1.0.7
```

### Validation

```bash
# Validate bundle
operator-sdk bundle validate ./bundle

# Validate for OpenShift
operator-sdk bundle validate ./bundle --select-optional name=operatorhub
```

### Inspection

```bash
# View CSV
cat bundle/manifests/lightspeed-operator.clusterserviceversion.yaml

# View annotations
cat bundle/metadata/annotations.yaml

# List all bundle files
find bundle -type f
```

