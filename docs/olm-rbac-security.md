# OLM RBAC & Security Guide

> **Part of the OLM Documentation Series:**
> 1. [Bundle Management](./olm-bundle-management.md) - Creating and managing operator bundles
> 2. [Catalog Management](./olm-catalog-management.md) - Organizing bundles into catalogs
> 3. [Integration & Lifecycle](./olm-integration-lifecycle.md) - OLM integration and operator lifecycle
> 4. [Testing & Validation](./olm-testing-validation.md) - Testing strategies and validation
> 5. **RBAC & Security** ← You are here

This guide covers Role-Based Access Control (RBAC) and security best practices for OLM operators, focusing on the principle of least privilege and secure operator design.

---

## Table of Contents

- [Overview](#overview)
- [RBAC Architecture](#rbac-architecture)
- [Operator RBAC](#operator-rbac)
- [User RBAC](#user-rbac)
- [Security Context](#security-context)
- [Secrets Management](#secrets-management)
- [Network Security](#network-security)
- [Pod Security Standards](#pod-security-standards)
- [Certificate Management](#certificate-management)
- [Security Best Practices](#security-best-practices)
- [Auditing & Compliance](#auditing--compliance)
- [Troubleshooting RBAC](#troubleshooting-rbac)

---

## Overview

### Why RBAC and Security Matter for Operators

Operators run with elevated privileges and manage critical cluster resources. Proper RBAC and security practices ensure:

- **Least Privilege**: Operators have only the permissions they need
- **Defense in Depth**: Multiple security layers protect the cluster
- **Audit Trail**: All actions are traceable
- **Compliance**: Meet regulatory requirements (SOC 2, PCI-DSS, etc.)
- **Trust**: Users can safely install operators
- **Isolation**: Operator failures don't compromise the cluster

### Security Principles for Operators

```
┌──────────────────────────────────────────────────────────────┐
│              Operator Security Layers                         │
└──────────────────────────────────────────────────────────────┘

Layer 1: RBAC (Authorization)
├── Operator ServiceAccount with minimal ClusterRole/Role
├── User RBAC for CR access
└── Namespace-scoped vs Cluster-scoped permissions

Layer 2: Pod Security
├── Non-root user
├── Read-only root filesystem
├── Drop all capabilities
└── Seccomp profile

Layer 3: Network Security
├── NetworkPolicies
├── Service mesh integration
└── TLS for all communication

Layer 4: Secrets Management
├── Secret encryption at rest
├── Secret rotation
└── Least privilege secret access

Layer 5: Compliance & Auditing
├── Audit logging
├── Security scanning (Preflight, Snyk)
└── Compliance frameworks (PCI-DSS, SOC 2)
```

---

## RBAC Architecture

### RBAC Components in OLM

```
┌────────────────────────────────────────────────────────────┐
│             OLM RBAC Component Flow                         │
└────────────────────────────────────────────────────────────┘

CSV Definition (bundle/manifests/*.clusterserviceversion.yaml)
├── spec.install.spec.clusterPermissions[]  → ClusterRole
│   └── Creates: ClusterRole + ClusterRoleBinding
├── spec.install.spec.permissions[]         → Role
│   └── Creates: Role + RoleBinding (namespace-scoped)
└── spec.install.spec.deployments[].spec.serviceAccountName
    └── Uses: ServiceAccount

OLM creates:
1. ServiceAccount (from CSV deployment spec)
2. ClusterRole (from clusterPermissions)
3. ClusterRoleBinding (SA → ClusterRole)
4. Role (from permissions, in operator namespace)
5. RoleBinding (SA → Role, in operator namespace)

Operator Pod runs as ServiceAccount with combined permissions
```

### Lightspeed Operator RBAC Structure

**Implementation Reference:**
- RBAC Definition: [`config/rbac/role.yaml`](../config/rbac/role.yaml)
- Kubebuilder Markers: [`internal/controller/olsconfig_controller.go`](../internal/controller/olsconfig_controller.go) (lines 141-166)
- CSV Integration: [`bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`](../bundle/manifests/lightspeed-operator.clusterserviceversion.yaml)

```
ServiceAccount: lightspeed-operator-controller-manager
    ↓
    ├─→ ClusterRoleBinding → ClusterRole: manager-role
    │   ├── OLSConfig CRD (full access)
    │   ├── Deployments, Services, ConfigMaps (manage)
    │   ├── Secrets (manage, but restricted for pull-secret)
    │   ├── Console resources (manage plugins)
    │   ├── Monitoring (ServiceMonitors, PrometheusRules)
    │   ├── NetworkPolicies (manage)
    │   ├── RBAC resources (create ClusterRoles/Bindings)
    │   ├── TokenReviews, SubjectAccessReviews (authentication)
    │   └── ClusterVersion, APIServer (read-only)
    │
    └─→ RoleBinding (openshift-lightspeed) → Role: manager-role
        └── RBAC resources (full access in operator namespace)

User Access:
├── ClusterRole: olsconfig-editor-role (create/edit OLSConfig)
├── ClusterRole: olsconfig-viewer-role (view OLSConfig)
└── ClusterRole: query-access (access OLS API endpoints)
```

---

## Operator RBAC

### Defining Operator Permissions in CSV

Operator permissions are defined in the CSV and automatically created by OLM.

#### Cluster-Scoped Permissions

**Location**: `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`

```yaml
spec:
  install:
    strategy: deployment
    spec:
      clusterPermissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            # Custom Resource Definition - Full Access
            - apiGroups:
                - ols.openshift.io
              resources:
                - olsconfigs
              verbs:
                - create
                - delete
                - get
                - list
                - patch
                - update
                - watch
            
            # Status subresource
            - apiGroups:
                - ols.openshift.io
              resources:
                - olsconfigs/status
              verbs:
                - get
                - patch
                - update
            
            # Finalizers
            - apiGroups:
                - ols.openshift.io
              resources:
                - olsconfigs/finalizers
              verbs:
                - update
            
            # Managed Resources - Cluster-wide
            - apiGroups:
                - apps
              resources:
                - deployments
              verbs:
                - create
                - delete
                - get
                - list
                - patch
                - update
                - watch
            
            # Secrets - General Access
            - apiGroups:
                - ""
              resources:
                - secrets
              verbs:
                - create
                - delete
                - get
                - list
                - patch
                - update
                - watch
            
            # Secrets - Restricted Access (pull-secret)
            - apiGroups:
                - ""
              resourceNames:
                - pull-secret
              resources:
                - secrets
              verbs:
                - get
                - list
                - watch
            
            # OpenShift Console Integration
            - apiGroups:
                - console.openshift.io
              resources:
                - consoleplugins
                - consoleplugins/finalizers
                - consolelinks
                - consoleexternalloglinks
              verbs:
                - create
                - delete
                - get
                - update
            
            # Monitoring Integration
            - apiGroups:
                - monitoring.coreos.com
              resources:
                - servicemonitors
                - prometheusrules
              verbs:
                - create
                - delete
                - get
                - list
                - patch
                - update
                - watch
            
            # Authentication & Authorization
            - apiGroups:
                - authentication.k8s.io
              resources:
                - tokenreviews
              verbs:
                - create
            
            - apiGroups:
                - authorization.k8s.io
              resources:
                - subjectaccessreviews
              verbs:
                - create
            
            # Read-only Access to Cluster Config
            - apiGroups:
                - config.openshift.io
              resources:
                - clusterversions
                - apiservers
              verbs:
                - get
                - list
                - watch
            
            # RBAC Management
            - apiGroups:
                - rbac.authorization.k8s.io
              resources:
                - clusterroles
                - clusterrolebindings
              verbs:
                - create
                - list
                - watch
            
            # Network Policies
            - apiGroups:
                - networking.k8s.io
              resources:
                - networkpolicies
              verbs:
                - create
                - delete
                - get
                - list
                - patch
                - update
                - watch
```

#### Namespace-Scoped Permissions

```yaml
spec:
  install:
    spec:
      permissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            # RBAC within operator namespace
            - apiGroups:
                - rbac.authorization.k8s.io
              resources:
                - roles
                - rolebindings
              verbs:
                - '*'
```

### RBAC Best Practices for Operators

**1. Use Least Privilege**

```yaml
# ❌ BAD - Too broad
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]

# ✅ GOOD - Specific permissions
- apiGroups: ["ols.openshift.io"]
  resources: ["olsconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

**2. Use `resourceNames` for Sensitive Resources**

```yaml
# Restrict access to specific secret
- apiGroups: [""]
  resourceNames: ["pull-secret"]  # Only this secret
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]  # Read-only
```

**3. Separate Cluster vs Namespace Permissions**

```yaml
clusterPermissions:  # Use for cluster-wide resources
  - rules:
      - apiGroups: ["ols.openshift.io"]
        resources: ["olsconfigs"]  # Cluster-scoped CRD
        verbs: ["*"]

permissions:  # Use for namespace-scoped resources
  - rules:
      - apiGroups: [""]
        resources: ["configmaps"]  # Namespace-scoped
        verbs: ["get", "list"]
```

**4. Justify Each Permission**

Document why each permission is needed:

```yaml
# Custom Resource - needed to reconcile OLSConfig
- apiGroups: ["ols.openshift.io"]
  resources: ["olsconfigs"]
  verbs: ["get", "list", "watch", "update", "patch"]

# Status updates - needed to report reconciliation state
- apiGroups: ["ols.openshift.io"]
  resources: ["olsconfigs/status"]
  verbs: ["get", "patch", "update"]

# Deployments - needed to manage app server and console plugin
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]

# TokenReviews - needed for authentication in OLS API
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
```

**5. Avoid Wildcard Verbs**

```yaml
# ❌ BAD - Grants all verbs including future ones
verbs: ["*"]

# ✅ GOOD - Explicit verbs
verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Generating RBAC from Code

Lightspeed uses Kubebuilder markers to generate RBAC:

**In controller code:** [`internal/controller/olsconfig_controller.go`](../internal/controller/olsconfig_controller.go)

```go
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins;consoleplugins/finalizers,verbs=get;create;update;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Controller logic
}
```

**Generate RBAC manifests:**

```bash
make manifests
# Creates config/rbac/role.yaml from kubebuilder markers
```

**Then include in bundle:**

```bash
make bundle
# Transfers RBAC from config/rbac/ to bundle CSV
```

---

## User RBAC

### User Access Patterns

Operators typically define three user roles:

1. **Viewer**: Read-only access to custom resources
2. **Editor**: Create and modify custom resources
3. **API User**: Access operator-managed APIs/services

### Lightspeed User Roles

#### 1. OLSConfig Viewer

**Implementation:** [`config/rbac/olsconfig_viewer_role.yaml`](../config/rbac/olsconfig_viewer_role.yaml)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-viewer-role
rules:
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs/status
    verbs:
      - get
```

**Usage:**

```bash
# Grant user view access to OLSConfig
oc adm policy add-cluster-role-to-user olsconfig-viewer-role user@example.com

# Grant group view access
oc adm policy add-cluster-role-to-group olsconfig-viewer-role lightspeed-viewers
```

#### 2. OLSConfig Editor

**Implementation:** [`config/rbac/olsconfig_editor_role.yaml`](../config/rbac/olsconfig_editor_role.yaml)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-editor-role
rules:
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs
    verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs/status
    verbs:
      - get
```

**Usage:**

```bash
# Grant user edit access
oc adm policy add-cluster-role-to-user olsconfig-editor-role admin@example.com

# Grant to service account
oc adm policy add-cluster-role-to-user olsconfig-editor-role \
  system:serviceaccount:automation:ols-manager
```

#### 3. Query Access (API User)

**Implementation:** [`config/user-access/query_access_clusterrole.yaml`](../config/user-access/query_access_clusterrole.yaml)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: query-access
rules:
  - nonResourceURLs:
      - "/ols-access"   # Current OLS API
      - "/ls-access"    # Future Lightspeed Core API
    verbs:
      - "get"
```

**Usage:**

```bash
# Grant API access to users
oc adm policy add-cluster-role-to-user query-access enduser@example.com

# Grant to application service account
oc adm policy add-cluster-role-to-user query-access \
  system:serviceaccount:my-app:default
```

### Creating Custom User Roles

**Example: Restricted Editor (no delete)**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-restricted-editor
rules:
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - patch
      # Explicitly exclude: delete
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs/status
    verbs:
      - get
```

**Example: Namespace-scoped Editor**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: olsconfig-namespace-editor
  namespace: team-alpha
rules:
  - apiGroups:
      - ols.openshift.io
    resources:
      - olsconfigs
    resourceNames:
      - team-alpha-config  # Only this specific resource
    verbs:
      - get
      - update
      - patch
```

### User RBAC Best Practices

**1. Use Groups Instead of Individual Users**

```bash
# ❌ BAD - Manage individual users
oc adm policy add-cluster-role-to-user olsconfig-editor-role user1
oc adm policy add-cluster-role-to-user olsconfig-editor-role user2
oc adm policy add-cluster-role-to-user olsconfig-editor-role user3

# ✅ GOOD - Manage via groups
oc adm policy add-cluster-role-to-group olsconfig-editor-role lightspeed-editors
# Then add users to group in identity provider
```

**2. Separate Read and Write Roles**

```yaml
# Viewer role - read-only
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-viewer
rules:
  - verbs: ["get", "list", "watch"]  # Read-only

---
# Editor role - includes viewer + write
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-editor
rules:
  - verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

**3. Document User Roles in CSV**

Add to CSV for OperatorHub UI:

```yaml
metadata:
  annotations:
    alm-examples: |-
      [...]
    # Document user roles
    operators.operatorframework.io/user-roles: |-
      [
        {
          "name": "olsconfig-viewer-role",
          "description": "View OLSConfig resources",
          "required": false
        },
        {
          "name": "olsconfig-editor-role",
          "description": "Create and manage OLSConfig resources",
          "required": true
        },
        {
          "name": "query-access",
          "description": "Access OLS query API",
          "required": false
        }
      ]
```

**4. Provide User Role Binding Examples**

Include in documentation:

```yaml
# examples/user-rbac/editor-binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: lightspeed-editors
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: olsconfig-editor-role
subjects:
  - kind: Group
    name: lightspeed-admins
    apiGroup: rbac.authorization.k8s.io
```

---

## Security Context

### Pod Security Context

Security context defines privilege and access control settings for pods and containers.

**Lightspeed Operator Security Context:**

**Implementation:** [`config/manager/manager.yaml`](../config/manager/manager.yaml) (lines 56-118)

```yaml
# config/manager/manager.yaml
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        # seccompProfile:  # Uncomment for K8s 1.19+
        #   type: RuntimeDefault
      
      containers:
        - name: manager
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - "ALL"
          resources:
            limits:
              cpu: 500m
              memory: 256Mi
            requests:
              cpu: 10m
              memory: 64Mi
```

### Security Context Fields Explained

| Field | Purpose | Lightspeed Setting |
|-------|---------|-------------------|
| `runAsNonRoot` | Prevent running as UID 0 | `true` |
| `runAsUser` | Specific UID to run as | Not set (uses image default) |
| `readOnlyRootFilesystem` | Prevent writes to container root | `true` |
| `allowPrivilegeEscalation` | Prevent gaining more privileges | `false` |
| `capabilities.drop` | Drop Linux capabilities | `["ALL"]` |
| `seccompProfile` | Seccomp security profile | RuntimeDefault (K8s 1.19+) |

### Security Context Best Practices

**1. Always Run as Non-Root**

```dockerfile
# In Dockerfile
USER 65532:65532  # nonroot user
```

```yaml
# In deployment
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
```

**2. Use Read-Only Root Filesystem**

```yaml
securityContext:
  readOnlyRootFilesystem: true

# If app needs writable directories
volumeMounts:
  - name: tmp
    mountPath: /tmp
  - name: cache
    mountPath: /var/cache

volumes:
  - name: tmp
    emptyDir: {}
  - name: cache
    emptyDir: {}
```

**3. Drop All Capabilities**

```yaml
securityContext:
  capabilities:
    drop:
      - ALL
```

**4. Enable Seccomp Profile**

```yaml
securityContext:
  seccompProfile:
    type: RuntimeDefault  # Or Localhost with custom profile
```

**5. Set Resource Limits**

```yaml
resources:
  limits:
    cpu: 500m
    memory: 256Mi
    ephemeral-storage: 1Gi  # Prevent disk exhaustion
  requests:
    cpu: 10m
    memory: 64Mi
```

### Operand Security Context

Apply security context to managed pods too:

```go
// In controller code
deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
    RunAsNonRoot: ptr.To(true),
}

deployment.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
    ReadOnlyRootFilesystem:   ptr.To(true),
    AllowPrivilegeEscalation: ptr.To(false),
    Capabilities: &corev1.Capabilities{
        Drop: []corev1.Capability{"ALL"},
    },
}
```

---

## Secrets Management

### Secret Access Patterns

**1. Operator Reading Secrets (LLM API Keys)**

```go
// In controller
secret := &corev1.Secret{}
err := r.Get(ctx, types.NamespacedName{
    Name:      cr.Spec.LLM.Providers[0].CredentialsSecretRef.Name,
    Namespace: r.Namespace,
}, secret)

// Use secret data
apiKey := secret.Data["apitoken"]
```

**RBAC Required:**

```yaml
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]
```

**2. Operator Creating Secrets (Generated Credentials)**

```go
// Generate PostgreSQL password
secret := &corev1.Secret{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "postgres-credentials",
        Namespace: r.Namespace,
    },
    Type: corev1.SecretTypeOpaque,
    StringData: map[string]string{
        "username": "ols_user",
        "password": generateSecurePassword(),
    },
}
err := r.Create(ctx, secret)
```

**RBAC Required:**

```yaml
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "update", "patch"]
```

**3. Operator Watching Secret Changes**

```go
// In main.go - watch secrets for updates
if err = (&controller.OLSConfigReconciler{
    // ...
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "OLSConfig")
    os.Exit(1)
}

// Watch secrets referenced by OLSConfig
if err = mgr.GetFieldIndexer().IndexField(ctx, &corev1.Secret{}, "metadata.name", func(obj client.Object) []string {
    return []string{obj.GetName()}
}); err != nil {
    setupLog.Error(err, "unable to create field indexer", "field", "metadata.name")
    os.Exit(1)
}
```

### Secret Management Best Practices

**1. Use Secret References, Not Inline Secrets**

```yaml
# ✅ GOOD - Reference to secret
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
spec:
  llm:
    providers:
      - name: OpenAI
        credentialsSecretRef:
          name: openai-credentials  # Reference

---
# Secret (separate resource)
apiVersion: v1
kind: Secret
metadata:
  name: openai-credentials
type: Opaque
stringData:
  apitoken: sk-...
```

```yaml
# ❌ BAD - Secret data in CR
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
spec:
  llm:
    providers:
      - name: OpenAI
        apiKey: sk-...  # Stored in CR, visible in etcd
```

**2. Validate Secret Existence**

```go
func (r *OLSConfigReconciler) validateSecrets(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    for _, provider := range cr.Spec.LLM.Providers {
        secret := &corev1.Secret{}
        err := r.Get(ctx, types.NamespacedName{
            Name:      provider.CredentialsSecretRef.Name,
            Namespace: r.Namespace,
        }, secret)
        if err != nil {
            return fmt.Errorf("secret %s not found: %w", provider.CredentialsSecretRef.Name, err)
        }
        
        // Validate required keys
        if _, ok := secret.Data["apitoken"]; !ok {
            return fmt.Errorf("secret %s missing required key 'apitoken'", secret.Name)
        }
    }
    return nil
}
```

**3. Watch for Secret Updates**

```go
// Trigger reconciliation when secret changes
func (r *OLSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&olsv1alpha1.OLSConfig{}).
        Owns(&appsv1.Deployment{}).
        Watches(
            &corev1.Secret{},
            handler.EnqueueRequestsFromMapFunc(r.findOLSConfigsForSecret),
        ).
        Complete(r)
}

func (r *OLSConfigReconciler) findOLSConfigsForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
    // Find all OLSConfigs that reference this secret
    // Return reconcile requests for each
}
```

**4. Rotate Secrets Regularly**

Document rotation procedure:

```bash
# 1. Create new secret with updated credentials
oc create secret generic openai-credentials-new \
  --from-literal=apitoken=sk-new-key-123

# 2. Update OLSConfig to reference new secret
oc patch olsconfig cluster --type merge -p '
{
  "spec": {
    "llm": {
      "providers": [{
        "name": "OpenAI",
        "credentialsSecretRef": {"name": "openai-credentials-new"}
      }]
    }
  }
}'

# 3. Wait for pods to restart
oc rollout status deployment/lightspeed-app-server -n openshift-lightspeed

# 4. Delete old secret
oc delete secret openai-credentials
```

**5. Use External Secrets Operator (Optional)**

For production, integrate with external secret management:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: openai-credentials
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: openai-credentials
  data:
    - secretKey: apitoken
      remoteRef:
        key: /secret/data/openai
        property: api_key
```

**6. Restrict Secret Access**

Use `resourceNames` to limit which secrets operator can access:

```yaml
# Only specific secrets
- apiGroups: [""]
  resourceNames:
    - "llm-credentials"
    - "postgres-credentials"
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]

# Wildcard for operator-created secrets
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "update", "patch", "delete"]
  # Apply via admission webhook: only allow if secret name starts with "ols-"
```

---

## Network Security

### NetworkPolicies

NetworkPolicies control traffic between pods.

**Implementation References:**
- Operator NetworkPolicy: [`internal/controller/operator_reconciliator.go`](../internal/controller/operator_reconciliator.go) (lines 120-210)
- App Server NetworkPolicy: [`internal/controller/appserver/reconciler.go`](../internal/controller/appserver/reconciler.go) (lines 536-565), assets in [`internal/controller/appserver/assets.go`](../internal/controller/appserver/assets.go) (lines 626-700)
- PostgreSQL NetworkPolicy: [`internal/controller/postgres/reconciler.go`](../internal/controller/postgres/reconciler.go) (lines 280-307), assets in [`internal/controller/postgres/assets.go`](../internal/controller/postgres/assets.go) (lines 362-427)
- Console NetworkPolicy: [`internal/controller/console/reconciler.go`](../internal/controller/console/reconciler.go) (lines 386-419), assets in [`internal/controller/console/assets.go`](../internal/controller/console/assets.go) (lines 265-319)

**Lightspeed NetworkPolicy (created by operator):**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: lightspeed-app-server
  namespace: openshift-lightspeed
spec:
  podSelector:
    matchLabels:
      app: lightspeed-app-server
  
  policyTypes:
    - Ingress
    - Egress
  
  ingress:
    # Allow from console plugin
    - from:
        - podSelector:
            matchLabels:
              app: lightspeed-console-plugin
      ports:
        - protocol: TCP
          port: 8443
    
    # Allow from Prometheus
    - from:
        - namespaceSelector:
            matchLabels:
              name: openshift-monitoring
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
      ports:
        - protocol: TCP
          port: 8443
  
  egress:
    # Allow to LLM providers (internet)
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443
    
    # Allow to PostgreSQL
    - to:
        - podSelector:
            matchLabels:
              app: lightspeed-postgres
      ports:
        - protocol: TCP
          port: 5432
    
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              name: openshift-dns
      ports:
        - protocol: UDP
          port: 53
```

### Network Security Best Practices

**1. Default Deny**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: openshift-lightspeed
spec:
  podSelector: {}  # Applies to all pods
  policyTypes:
    - Ingress
    - Egress
  # No ingress/egress rules = deny all
```

**2. Explicit Allow Rules**

```yaml
# Allow specific traffic
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-app-server-traffic
spec:
  podSelector:
    matchLabels:
      app: lightspeed-app-server
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: openshift-console
      ports:
        - protocol: TCP
          port: 8443
```

**3. Use Labels for Selection**

```yaml
# Select by pod labels
podSelector:
  matchLabels:
    app: lightspeed-app-server
    component: api

# Select by namespace labels
namespaceSelector:
  matchLabels:
    environment: production
```

**4. Test NetworkPolicies**

```bash
# Test connectivity before applying
oc run test-pod --image=curlimages/curl --rm -it -- curl https://lightspeed-app-server:8443/healthz

# Apply NetworkPolicy
oc apply -f networkpolicy.yaml

# Test connectivity after (should fail if blocked)
oc run test-pod --image=curlimages/curl --rm -it -- curl https://lightspeed-app-server:8443/healthz
```

### Service Mesh Integration

For advanced traffic management, integrate with OpenShift Service Mesh (Istio):

```yaml
apiVersion: networking.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: lightspeed-mtls
  namespace: openshift-lightspeed
spec:
  mtls:
    mode: STRICT  # Require mTLS for all traffic
```

---

## Pod Security Standards

### Pod Security Levels

Kubernetes defines three Pod Security Standards:

| Level | Description | Use Case |
|-------|-------------|----------|
| **Privileged** | Unrestricted (no restrictions) | System components, debug |
| **Baseline** | Minimally restrictive | Most apps |
| **Restricted** | Heavily restricted (defense-in-depth) | Security-sensitive apps |

### Lightspeed Pod Security

Lightspeed operator pods comply with **Restricted** level:

```yaml
# Restricted requirements met:
✅ runAsNonRoot: true
✅ allowPrivilegeEscalation: false
✅ capabilities: drop ALL
✅ seccompProfile: RuntimeDefault
✅ readOnlyRootFilesystem: true
```

### Enforcing Pod Security Standards

**Namespace-level enforcement (K8s 1.23+):**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-lightspeed
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

**OpenShift Security Context Constraints (SCC):**

```yaml
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  name: lightspeed-restricted
allowPrivilegedContainer: false
allowPrivilegeEscalation: false
requiredDropCapabilities:
  - ALL
runAsUser:
  type: MustRunAsNonRoot
seLinuxContext:
  type: MustRunAs
fsGroup:
  type: MustRunAs
volumes:
  - configMap
  - downwardAPI
  - emptyDir
  - persistentVolumeClaim
  - projected
  - secret
users:
  - system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager
```

### Pod Security Best Practices

**1. Start with Restricted Profile**

Always aim for the most restrictive profile that allows your app to function.

**2. Document Security Context Requirements**

```yaml
# In CSV
metadata:
  annotations:
    operators.operatorframework.io/security-context: |-
      {
        "runAsNonRoot": true,
        "readOnlyRootFilesystem": true,
        "allowPrivilegeEscalation": false,
        "seccompProfile": "RuntimeDefault",
        "capabilities": {"drop": ["ALL"]}
      }
```

**3. Test in Restricted Environment**

```bash
# Deploy to namespace with restricted enforcement
oc create namespace test-restricted
oc label namespace test-restricted \
  pod-security.kubernetes.io/enforce=restricted

# Deploy operator
oc apply -f operator.yaml -n test-restricted
```

---

## Certificate Management

### TLS Certificate Usage

Lightspeed uses certificates for:

1. **Operator Metrics Endpoint**: Service-serving certificates
2. **App Server HTTPS**: Service-serving certificates  
3. **Console Plugin**: Service-serving certificates
4. **External LLM Providers**: Custom CA certificates (optional)

### Service-Serving Certificates (OpenShift)

OpenShift automatically provisions TLS certificates:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: lightspeed-app-server
  namespace: openshift-lightspeed
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: lightspeed-tls
spec:
  ports:
    - name: https
      port: 8443
      targetPort: 8443
  selector:
    app: lightspeed-app-server
```

**OpenShift creates:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: lightspeed-tls
  namespace: openshift-lightspeed
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
```

**Mount in pod:**

```yaml
spec:
  volumes:
    - name: tls
      secret:
        secretName: lightspeed-tls
  containers:
    - name: app
      volumeMounts:
        - name: tls
          mountPath: /etc/tls/private
          readOnly: true
```

### Custom CA Certificates

For LLM providers with custom CAs:

```yaml
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
spec:
  ols:
    additionalCAConfigMapRef:
      name: custom-ca-bundle
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: custom-ca-bundle
  namespace: openshift-lightspeed
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAKJ...
    -----END CERTIFICATE-----
```

**Operator mounts CA bundle:**

```go
// In controller
if cr.Spec.OLS.AdditionalCAConfigMapRef != nil {
    deployment.Spec.Template.Spec.Volumes = append(
        deployment.Spec.Template.Spec.Volumes,
        corev1.Volume{
            Name: "additional-ca",
            VolumeSource: corev1.VolumeSource{
                ConfigMap: &corev1.ConfigMapVolumeSource{
                    LocalObjectReference: *cr.Spec.OLS.AdditionalCAConfigMapRef,
                },
            },
        },
    )
    
    deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
        deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
        corev1.VolumeMount{
            Name:      "additional-ca",
            MountPath: "/etc/pki/ca-trust/extracted/pem",
            ReadOnly:  true,
        },
    )
}
```

### Certificate Best Practices

**1. Use Service-Serving Certificates**

Let OpenShift manage certificates automatically.

**2. Validate Certificate Expiry**

```go
// Validate certificate
certData, _ := secret.Data["tls.crt"]
block, _ := pem.Decode(certData)
cert, err := x509.ParseCertificate(block.Bytes)
if err != nil {
    return err
}

// Check expiry
if time.Now().After(cert.NotAfter) {
    return fmt.Errorf("certificate expired on %v", cert.NotAfter)
}

// Warn if expiring soon
if time.Now().Add(30 * 24 * time.Hour).After(cert.NotAfter) {
    log.Warn("certificate expiring soon", "expiry", cert.NotAfter)
}
```

**3. Rotate Certificates**

```bash
# Delete secret to trigger rotation
oc delete secret lightspeed-tls -n openshift-lightspeed

# OpenShift recreates with new certificate
# Restart pods to use new certificate
oc rollout restart deployment/lightspeed-app-server -n openshift-lightspeed
```

**4. Use Cert-Manager (Optional)**

For advanced certificate management:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: lightspeed-tls
  namespace: openshift-lightspeed
spec:
  secretName: lightspeed-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  commonName: lightspeed.example.com
  dnsNames:
    - lightspeed.example.com
    - lightspeed-app-server.openshift-lightspeed.svc.cluster.local
```

---

## Security Best Practices

### Operator Security Checklist

- [ ] **RBAC**: Operator uses least privilege permissions
- [ ] **RBAC**: User roles defined (viewer, editor)
- [ ] **Security Context**: Runs as non-root
- [ ] **Security Context**: Read-only root filesystem
- [ ] **Security Context**: Drops all capabilities
- [ ] **Security Context**: Seccomp profile enabled
- [ ] **Secrets**: Uses secret references, not inline values
- [ ] **Secrets**: Validates secret existence and format
- [ ] **Secrets**: Watches for secret updates
- [ ] **Network**: NetworkPolicies defined
- [ ] **Network**: Default deny policy
- [ ] **Certificates**: Uses TLS for all endpoints
- [ ] **Certificates**: Validates certificate expiry
- [ ] **Resources**: Resource limits set
- [ ] **Image**: Base image is UBI (Red Hat)
- [ ] **Image**: Image scanning enabled (Preflight)
- [ ] **Image**: Uses SHA256 digests
- [ ] **Audit**: Audit logging enabled
- [ ] **Compliance**: Meets target compliance framework

### Defense in Depth Strategy

```
Layer 1: Image Security
├── Use minimal base image (UBI minimal)
├── Scan for vulnerabilities (Snyk, Preflight)
├── Sign images (Cosign)
└── Use image digests, not tags

Layer 2: Pod Security
├── Run as non-root
├── Read-only root filesystem
├── Drop all capabilities
└── Seccomp profile

Layer 3: RBAC
├── Least privilege for operator
├── Separate user roles
└── Namespace isolation

Layer 4: Network Security
├── NetworkPolicies
├── Service mesh (mTLS)
└── Egress filtering

Layer 5: Data Security
├── Secrets encrypted at rest
├── TLS in transit
└── Secret rotation

Layer 6: Monitoring & Audit
├── Audit logging
├── Security alerts
└── Compliance reporting
```

### Common Security Anti-Patterns

**❌ Running as Root**

```yaml
# BAD
securityContext:
  runAsUser: 0
```

**❌ Privileged Containers**

```yaml
# BAD
securityContext:
  privileged: true
```

**❌ Host Path Volumes**

```yaml
# BAD
volumes:
  - name: host-data
    hostPath:
      path: /var/lib/data
```

**❌ Wildcard RBAC**

```yaml
# BAD
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
```

**❌ Inline Secrets**

```yaml
# BAD
spec:
  apiKey: sk-abc123...  # Visible in etcd
```

### Secure Development Practices

**1. Security Review Checklist**

For every release:
- [ ] RBAC permissions reviewed
- [ ] Secrets handling reviewed
- [ ] Image scanning passed
- [ ] Security context validated
- [ ] Dependencies updated
- [ ] CVEs addressed

**2. Automated Security Scanning**

```yaml
# GitHub Actions
- name: Security Scan
  run: |
    # Image scanning
    docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
      aquasec/trivy image ${{ env.IMAGE }}
    
    # RBAC linting
    kubectl auth can-i --list --as=system:serviceaccount:test:operator
    
    # Secret scanning
    gitleaks detect --source . --verbose
```

**3. Least Privilege Testing**

```bash
# Test with minimal permissions
oc adm policy who-can create deployments -n openshift-lightspeed
oc adm policy who-can get secrets -n openshift-lightspeed
```

---

## Auditing & Compliance

### Audit Logging

Enable Kubernetes audit logging to track operator actions:

```yaml
# OpenShift audit policy
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all Secret access
  - level: RequestResponse
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
    resources:
      - group: ""
        resources: ["secrets"]
  
  # Log OLSConfig changes
  - level: RequestResponse
    verbs: ["create", "update", "patch", "delete"]
    resources:
      - group: "ols.openshift.io"
        resources: ["olsconfigs"]
  
  # Log RBAC changes
  - level: RequestResponse
    verbs: ["create", "update", "patch", "delete"]
    resources:
      - group: "rbac.authorization.k8s.io"
        resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
```

**Query audit logs:**

```bash
# Find secret access by operator
oc adm node-logs --role=master --path=kube-apiserver/ | \
  grep 'lightspeed-operator-controller-manager' | \
  grep 'secrets'

# Find OLSConfig changes
oc adm node-logs --role=master --path=kube-apiserver/ | \
  grep 'olsconfigs' | \
  jq 'select(.verb == "create" or .verb == "update")'
```

### Compliance Frameworks

**PCI-DSS Requirements:**
- ✅ Audit logging (Requirement 10)
- ✅ Access control (Requirement 7)
- ✅ Encryption in transit (Requirement 4)
- ✅ Secrets management (Requirement 3)

**SOC 2 Controls:**
- ✅ CC6.1: Logical access controls
- ✅ CC6.3: Removal of access
- ✅ CC7.2: System monitoring
- ✅ CC7.3: Evaluation of security events

**NIST 800-53:**
- ✅ AC-2: Account Management
- ✅ AC-3: Access Enforcement
- ✅ AU-2: Audit Events
- ✅ IA-2: Identification and Authentication

### Compliance Reporting

**Generate compliance report:**

```bash
#!/bin/bash
# compliance-report.sh

echo "=== Operator Security Compliance Report ==="
echo

echo "1. RBAC Permissions:"
oc get clusterrole lightspeed-operator-manager-role -o yaml | \
  yq '.rules[] | {apiGroups, resources, verbs}'

echo
echo "2. Security Context:"
oc get deployment lightspeed-operator-controller-manager \
  -n openshift-lightspeed \
  -o jsonpath='{.spec.template.spec.securityContext}'

echo
echo "3. Image Digests:"
oc get csv -n openshift-lightspeed \
  -o jsonpath='{.items[0].spec.relatedImages[*].image}' | \
  tr ' ' '\n'

echo
echo "4. Secret Access:"
oc auth can-i get secrets \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager

echo
echo "5. NetworkPolicies:"
oc get networkpolicies -n openshift-lightspeed

echo
echo "=== Report Complete ==="
```

---

## Troubleshooting RBAC

### Common RBAC Issues

#### Issue 1: Operator Can't Create Resources

**Symptom:**

```
Error: failed to create deployment: deployments.apps is forbidden: 
User "system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager" 
cannot create resource "deployments" in API group "apps" in the namespace "openshift-lightspeed"
```

**Diagnosis:**

```bash
# Check operator's permissions
oc auth can-i create deployments \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager \
  -n openshift-lightspeed

# Check ClusterRole
oc get clusterrole lightspeed-operator-manager-role -o yaml

# Check ClusterRoleBinding
oc get clusterrolebinding | grep lightspeed
```

**Fix:**

```yaml
# Add missing permission to ClusterRole
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
```

```bash
# Regenerate and redeploy
make manifests
make bundle
```

#### Issue 2: User Can't Create OLSConfig

**Symptom:**

```
Error: olsconfigs.ols.openshift.io is forbidden: 
User "alice@example.com" cannot create resource "olsconfigs"
```

**Diagnosis:**

```bash
# Check user's permissions
oc auth can-i create olsconfigs --as=alice@example.com

# List available roles
oc get clusterroles | grep olsconfig

# Check if user has role
oc get clusterrolebindings -o json | \
  jq '.items[] | select(.subjects[]?.name == "alice@example.com")'
```

**Fix:**

```bash
# Grant editor role
oc adm policy add-cluster-role-to-user olsconfig-editor-role alice@example.com

# Verify
oc auth can-i create olsconfigs --as=alice@example.com
```

#### Issue 3: Service Account Token Invalid

**Symptom:**

```
Error: Unauthorized: the server has asked for the client to provide credentials
```

**Diagnosis:**

```bash
# Check if ServiceAccount exists
oc get sa lightspeed-operator-controller-manager -n openshift-lightspeed

# Check if token secret exists
oc get secrets -n openshift-lightspeed | grep controller-manager-token

# Check token expiry
TOKEN=$(oc sa get-token lightspeed-operator-controller-manager -n openshift-lightspeed)
echo $TOKEN | cut -d. -f2 | base64 -d | jq .exp
```

**Fix:**

```bash
# Recreate ServiceAccount
oc delete sa lightspeed-operator-controller-manager -n openshift-lightspeed
oc create sa lightspeed-operator-controller-manager -n openshift-lightspeed

# Restart operator pod
oc delete pod -l control-plane=controller-manager -n openshift-lightspeed
```

### RBAC Debugging Commands

```bash
# Check specific permission
oc auth can-i <verb> <resource> \
  --as=system:serviceaccount:<namespace>:<serviceaccount> \
  -n <namespace>

# Examples:
oc auth can-i create deployments \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager \
  -n openshift-lightspeed

oc auth can-i get secrets \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager \
  -n openshift-lightspeed

# List all permissions for ServiceAccount
oc policy who-can create deployments -n openshift-lightspeed

# View ClusterRole rules
oc get clusterrole lightspeed-operator-manager-role -o yaml

# View RoleBindings for ServiceAccount
oc get rolebindings -n openshift-lightspeed -o json | \
  jq '.items[] | select(.subjects[]?.name == "lightspeed-operator-controller-manager")'

# View ClusterRoleBindings for ServiceAccount
oc get clusterrolebindings -o json | \
  jq '.items[] | select(.subjects[]?.name == "lightspeed-operator-controller-manager")'

# Describe RBAC for a user
oc describe clusterrolebinding olsconfig-editor

# Test permissions with impersonation
oc get olsconfigs --as=alice@example.com
```

### RBAC Validation Script

```bash
#!/bin/bash
# validate-rbac.sh

NAMESPACE="openshift-lightspeed"
SA="lightspeed-operator-controller-manager"

echo "Validating RBAC for ${SA} in ${NAMESPACE}"
echo

# Required permissions
PERMISSIONS=(
    "create:deployments:apps"
    "get:secrets:"
    "create:services:"
    "update:olsconfigs:ols.openshift.io"
    "create:servicemonitors:monitoring.coreos.com"
)

for perm in "${PERMISSIONS[@]}"; do
    IFS=':' read -r verb resource apiGroup <<< "$perm"
    
    if [ -z "$apiGroup" ]; then
        result=$(oc auth can-i $verb $resource \
            --as=system:serviceaccount:${NAMESPACE}:${SA} \
            -n ${NAMESPACE})
    else
        result=$(oc auth can-i $verb ${resource}.${apiGroup} \
            --as=system:serviceaccount:${NAMESPACE}:${SA} \
            -n ${NAMESPACE})
    fi
    
    if [ "$result" == "yes" ]; then
        echo "✅ Can $verb $resource (${apiGroup:-core})"
    else
        echo "❌ Cannot $verb $resource (${apiGroup:-core})"
    fi
done
```

---

## Additional Resources

### Related Guides

- **[OLM Bundle Management Guide](./olm-bundle-management.md)** - CSV RBAC definition
- **[OLM Integration & Lifecycle Guide](./olm-integration-lifecycle.md)** - How OLM creates RBAC
- **[OLM Testing & Validation Guide](./olm-testing-validation.md)** - Testing RBAC
- **[Contributing Guide](../CONTRIBUTING.md)** - General contribution guidelines
- **[Architecture Documentation](../ARCHITECTURE.md)** - Operator architecture overview

### External Resources

- [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [OpenShift RBAC](https://docs.openshift.com/container-platform/latest/authentication/using-rbac.html)
- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
- [OpenShift SCC](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html)
- [NIST 800-190](https://csrc.nist.gov/publications/detail/sp/800-190/final) - Container Security
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)

### Project RBAC Files

**Lightspeed Operator RBAC:**
- [`config/rbac/role.yaml`](../config/rbac/role.yaml) - Operator ClusterRole/Role
- [`config/rbac/role_binding.yaml`](../config/rbac/role_binding.yaml) - Bindings
- [`config/rbac/service_account.yaml`](../config/rbac/service_account.yaml) - ServiceAccount
- [`config/rbac/leader_election_role.yaml`](../config/rbac/leader_election_role.yaml) - Leader election permissions
- [`config/rbac/olsconfig_editor_role.yaml`](../config/rbac/olsconfig_editor_role.yaml) - User editor role
- [`config/rbac/olsconfig_viewer_role.yaml`](../config/rbac/olsconfig_viewer_role.yaml) - User viewer role
- [`config/user-access/query_access_clusterrole.yaml`](../config/user-access/query_access_clusterrole.yaml) - API access role
- [`config/manager/manager.yaml`](../config/manager/manager.yaml) - Security context configuration

**Note on Leader Election**: The operator uses Kubernetes leader election for high-availability deployments. Leader election RBAC permissions are defined in `config/rbac/leader_election_role.yaml` and include access to ConfigMaps, Coordination.k8s.io Leases, and Events. This is a standard Kubebuilder pattern and is automatically generated.

---

**Security is not optional.** Follow this guide to ensure your operator follows security best practices and protects your cluster.

For questions or issues with the Lightspeed Operator security, see the main [README](../README.md) or [CONTRIBUTING](../CONTRIBUTING.md) guide.

