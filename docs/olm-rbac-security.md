# OLM RBAC & Security Guide

This guide covers RBAC and security configuration for the OpenShift Lightspeed Operator.

> **📖 For K8s RBAC Fundamentals:** See [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)  
> **📖 For Security Basics:** See [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)

---

## Overview

The operator requires specific permissions to manage resources. We follow least privilege principles:
- **Operator RBAC**: Minimal permissions to manage OLSConfig and operand resources
- **User RBAC**: Three roles for CR access (viewer, editor, query-access)
- **Security Context**: Non-root, read-only filesystem, restricted capabilities

---

## Our RBAC Structure

### Operator RBAC

**Location:** `config/rbac/role.yaml` (generated from kubebuilder markers)

**Key permissions:**
- **OLSConfig**: Full CRUD + status + finalizers
- **Core resources**: Deployments, Services, ConfigMaps, Secrets
- **OpenShift**: ConsolePlugin
- **Monitoring**: ServiceMonitors
- **RBAC**: ClusterRoles, ClusterRoleBindings

### User RBAC Roles

**1. Viewer Role** (`lightspeed-operator-olsconfig-viewer-role`)
- Read-only access to OLSConfig
- **Use case:** Monitoring, auditing

**2. Editor Role** (`lightspeed-operator-olsconfig-editor-role`)
- Full CRUD on OLSConfig
- **Use case:** Managing operator configuration

**3. Query Access Role** (`lightspeed-operator-query-access`)
- Read access to metrics endpoint
- **Use case:** Prometheus scraping

**Location:** `config/rbac/olsconfig_editor_role.yaml`, `olsconfig_viewer_role.yaml`, `query_access_role.yaml`

**Grant access:**
```bash
# Viewer
oc adm policy add-role-to-user lightspeed-operator-olsconfig-viewer-role <username> -n openshift-lightspeed

# Editor
oc adm policy add-role-to-user lightspeed-operator-olsconfig-editor-role <username> -n openshift-lightspeed

# Query access
oc adm policy add-cluster-role-to-user lightspeed-operator-query-access <service-account>
```

---

## Generating RBAC from Code

We use kubebuilder RBAC markers in controller code to generate permissions.

### Adding Permissions

**In your reconciler code:**

```go
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/finalizers,verbs=update

func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Your code
}
```

**Regenerate RBAC:**
```bash
make manifests              # Regenerate from markers
make bundle BUNDLE_TAG=0.1.0
git diff config/rbac/role.yaml
```

**Marker syntax:**
```go
//+kubebuilder:rbac:groups=<group>,resources=<resource>,verbs=<verbs>
```

> **📖 Kubebuilder RBAC Markers:** See [Kubebuilder Markers](https://book.kubebuilder.io/reference/markers/rbac.html)

---

## Security Context Configuration

**Location:** `config/manager/manager.yaml`

**Our security settings:**
- `runAsNonRoot: true` - Don't run as root
- `readOnlyRootFilesystem: true` - Read-only filesystem
- `allowPrivilegeEscalation: false` - Prevent privilege escalation
- `capabilities.drop: [ALL]` - Drop all capabilities
- `seccompProfile.type: RuntimeDefault` - Use default seccomp profile

**Why:**
- Non-root reduces attack surface
- Read-only filesystem prevents tampering
- Dropped capabilities = minimal privileges
- Resource limits prevent exhaustion

---

## Network Security

**Location:** `config/rbac/network_policy.yaml`

**Our NetworkPolicy:**
- Allows metrics scraping on port 8443
- Allows API server access (required for reconciliation)
- Restricts all other traffic

---

## Secrets Management

**Best practices we follow:**

1. **Secret References** - Use `credentialsSecretRef`, never inline values
2. **Validation** - Operator validates secret exists and has required keys
3. **Watching** - Automatic operand restart on secret changes
4. **Rotation** - Update secret content, operator auto-detects and restarts

> **📖 Watcher Implementation:** See [ARCHITECTURE.md](../ARCHITECTURE.md) - Resource Management section

**Implementation:** `internal/controller/watchers/` for secret watching logic

---

## Common Tasks

### Update Operator Permissions

```bash
# 1. Add kubebuilder marker to code
vim internal/controller/mycontroller.go
# Add: //+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list

# 2. Regenerate and verify
make manifests
git diff config/rbac/role.yaml

# 3. Update bundle
make bundle BUNDLE_TAG=0.1.0
```

### Grant User Access

```bash
# Viewer role
oc create rolebinding user-viewer \
  --clusterrole=lightspeed-operator-olsconfig-viewer-role \
  --user=alice \
  -n openshift-lightspeed

# Editor role
oc create rolebinding user-editor \
  --clusterrole=lightspeed-operator-olsconfig-editor-role \
  --user=bob \
  -n openshift-lightspeed
```

### Check Permissions

```bash
# Check operator permissions
oc auth can-i create deployments \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager

# Check user permissions
oc auth can-i create olsconfig --as=alice

# View all operator permissions
oc describe clusterrole lightspeed-operator-manager-role
```

### Review Security Settings

```bash
# View security context
oc get pod -n openshift-lightspeed <pod> -o jsonpath='{.spec.securityContext}'
oc get pod -n openshift-lightspeed <pod> -o jsonpath='{.spec.containers[0].securityContext}'

# Verify non-root
oc get pod -n openshift-lightspeed <pod> -o jsonpath='{.spec.containers[0].securityContext.runAsNonRoot}'
```

---

## Troubleshooting

### Permission Denied Errors

**Symptoms:** Operator logs show "forbidden" or "cannot create/update resource"

**Diagnosis:**
```bash
# Check service account
oc get sa lightspeed-operator-controller-manager -n openshift-lightspeed

# Check role bindings
oc get clusterrolebinding | grep lightspeed-operator

# Test permission
oc auth can-i create deployments \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager
```

**Fix:**
1. Add missing kubebuilder marker to code
2. `make manifests`
3. `make bundle`
4. Redeploy operator

### User Cannot Access OLSConfig

**Diagnosis:**
```bash
# Check user permissions
oc auth can-i get olsconfig --as=<username>

# List role bindings
oc get rolebinding -n openshift-lightspeed -o yaml | grep <username>
```

**Fix:**
```bash
oc create rolebinding <username>-viewer \
  --clusterrole=lightspeed-operator-olsconfig-viewer-role \
  --user=<username> \
  -n openshift-lightspeed
```

### Security Context Violations

**Diagnosis:**
```bash
# Check pod events
oc describe pod -n openshift-lightspeed <pod>

# Check security context constraints (OpenShift)
oc get scc
```

**Fix:**
- Verify `config/manager/manager.yaml` has correct security context
- Ensure no privileged settings (root user, `privileged: true`)

---

## Additional Resources

- [OLM Bundle Management](./olm-bundle-management.md) - RBAC in CSV
- [ARCHITECTURE.md](../ARCHITECTURE.md) - Resource management patterns
- [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Kubebuilder RBAC Markers](https://book.kubebuilder.io/reference/markers/rbac.html)

**Project Files:**
- `config/rbac/` - All RBAC manifests
- `config/manager/manager.yaml` - Security context
- `internal/controller/*_controller.go` - RBAC markers in code
