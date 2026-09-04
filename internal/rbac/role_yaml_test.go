package rbac

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func loadRoleYAML() (rbacv1.ClusterRole, rbacv1.Role) {
	_, thisFile, _, _ := runtime.Caller(0)
	roleFile := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "rbac", "role.yaml")

	data, err := os.ReadFile(roleFile)
	Expect(err).NotTo(HaveOccurred(), "reading role.yaml")

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

	var cr rbacv1.ClusterRole
	Expect(decoder.Decode(&cr)).NotTo(HaveOccurred(), "decoding ClusterRole")

	var r rbacv1.Role
	Expect(decoder.Decode(&r)).NotTo(HaveOccurred(), "decoding Role")

	return cr, r
}

var _ = Describe("manager-role YAML", func() {
	var (
		clusterRole rbacv1.ClusterRole
		role        rbacv1.Role
	)

	BeforeEach(func() {
		clusterRole, role = loadRoleYAML()
	})

	Describe("ClusterRole", func() {
		It("restricts secrets access to exactly pull-secret", func() {
			found := false
			for _, rule := range clusterRole.Rules {
				for _, res := range rule.Resources {
					if res == "secrets" {
						found = true
						Expect(rule.APIGroups).To(ConsistOf(""),
							"ClusterRole secrets rule must be in the core API group")
						Expect(rule.ResourceNames).To(ConsistOf("pull-secret"),
							"ClusterRole secrets rule must be pinned to exactly pull-secret")
						Expect(rule.Verbs).To(ConsistOf("get", "list", "watch"),
							"ClusterRole secrets rule must grant read-only verbs")
					}
				}
			}
			Expect(found).To(BeTrue(), "ClusterRole must include a pinned pull-secret rule")
		})

		It("does not grant cluster-wide networkpolicies access", func() {
			for _, rule := range clusterRole.Rules {
				for _, res := range rule.Resources {
					Expect(res).NotTo(Equal("networkpolicies"),
						"ClusterRole must not grant networkpolicies access — should be in namespaced Role")
				}
			}
		})

		It("does not use create verb with resourceNames for RBAC resources", func() {
			rbacResources := map[string]bool{
				"clusterroles":        true,
				"clusterrolebindings": true,
				"rolebindings":        true,
				"roles":               true,
			}
			for _, rule := range clusterRole.Rules {
				if len(rule.ResourceNames) == 0 {
					continue
				}
				for _, res := range rule.Resources {
					if rbacResources[res] {
						Expect(rule.Verbs).NotTo(ContainElement("create"),
							"create verb combined with resourceNames %v is silently denied by Kubernetes RBAC", rule.ResourceNames)
					}
				}
			}
		})

		It("pins update and delete verbs on RBAC resources to resourceNames", func() {
			rbacResources := map[string]bool{
				"clusterroles":        true,
				"clusterrolebindings": true,
				"rolebindings":        true,
			}
			restrictedVerbs := map[string]bool{"update": true, "delete": true, "patch": true}

			for _, rule := range clusterRole.Rules {
				hasRBAC := false
				for _, res := range rule.Resources {
					if rbacResources[res] {
						hasRBAC = true
					}
				}
				if !hasRBAC {
					continue
				}
				for _, verb := range rule.Verbs {
					if restrictedVerbs[verb] {
						Expect(rule.ResourceNames).NotTo(BeEmpty(),
							"RBAC rule with %q verb must be pinned to resourceNames to prevent privilege escalation", verb)
					}
					Expect(verb).NotTo(Equal("deletecollection"),
						"RBAC rule for RBAC resources must not grant deletecollection: resourceNames cannot constrain collection-scope verbs")
				}
			}
		})

		It("only grants access to managed resource names", func() {
			allowedClusterRoles := map[string]bool{
				"lightspeed-app-server-sar-role":                true,
				"lightspeed-agentic-alerts-adapter-agenticruns": true,
			}
			allowedCRBs := map[string]bool{
				"lightspeed-app-server-sar-role-binding":        true,
				"lightspeed-agentic-alerts-adapter-agenticruns": true,
				"lightspeed-operator-ols-metrics-reader":        true,
			}
			allowedRoleBindings := map[string]bool{
				"lightspeed-agentic-alerts-adapter-alertmanager": true,
			}

			for _, rule := range clusterRole.Rules {
				if len(rule.ResourceNames) == 0 {
					continue
				}
				for _, res := range rule.Resources {
					var allowed map[string]bool
					switch res {
					case "clusterroles":
						allowed = allowedClusterRoles
					case "clusterrolebindings":
						allowed = allowedCRBs
					case "rolebindings":
						allowed = allowedRoleBindings
					default:
						continue
					}
					for _, name := range rule.ResourceNames {
						Expect(allowed).To(HaveKey(name),
							"ClusterRole grants access to unexpected %s: %s", res, name)
					}
				}
			}
		})
	})

	Describe("Role", func() {
		It("uses the 'system' namespace placeholder", func() {
			Expect(role.Namespace).To(Equal("system"))
		})

		It("grants secrets access with the required verbs including deletecollection", func() {
			expected := []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}
			found := false
			for _, rule := range role.Rules {
				for _, res := range rule.Resources {
					if res == "secrets" {
						found = true
						actual := make([]string, len(rule.Verbs))
						copy(actual, rule.Verbs)
						sort.Strings(actual)
						Expect(actual).To(Equal(expected), "Role secrets verbs mismatch")
					}
				}
			}
			Expect(found).To(BeTrue(), "namespaced Role must include secrets access")
		})

		It("grants networkpolicies access without deletecollection", func() {
			expected := []string{"create", "delete", "get", "list", "patch", "update", "watch"}
			found := false
			for _, rule := range role.Rules {
				for _, res := range rule.Resources {
					if res == "networkpolicies" {
						found = true
						actual := make([]string, len(rule.Verbs))
						copy(actual, rule.Verbs)
						sort.Strings(actual)
						Expect(actual).To(Equal(expected), "Role networkpolicies verbs mismatch")
					}
				}
			}
			Expect(found).To(BeTrue(), "namespaced Role must include networkpolicies access")
		})
	})
})
