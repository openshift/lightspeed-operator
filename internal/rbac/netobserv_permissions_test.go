package rbac

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var _ = Describe("NetObserv installation permissions", func() {
	It("keeps the checked-in OLM bundle aligned with the generated named CRD permission", func() {
		role, _ := loadRoleYAML()
		var expected []rbacv1.PolicyRule
		for _, rule := range role.Rules {
			if len(rule.APIGroups) == 1 && rule.APIGroups[0] == "apiextensions.k8s.io" {
				expected = append(expected, rule)
			}
		}
		Expect(expected).To(HaveLen(1))
		Expect(expected[0].ResourceNames).To(ConsistOf("flowcollectors.flows.netobserv.io"))
		Expect(expected[0].Resources).To(ConsistOf("customresourcedefinitions"))
		Expect(expected[0].Verbs).To(ConsistOf("get", "list", "watch"))

		_, thisFile, _, _ := runtime.Caller(0)
		path := filepath.Join(filepath.Dir(thisFile), "..", "..", "bundle", "manifests", "lightspeed-operator.clusterserviceversion.yaml")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var csv struct {
			Spec struct {
				Install struct {
					Spec struct {
						ClusterPermissions []struct {
							ServiceAccountName string              `json:"serviceAccountName"`
							Rules              []rbacv1.PolicyRule `json:"rules"`
						} `json:"clusterPermissions"`
					} `json:"spec"`
				} `json:"install"`
			} `json:"spec"`
		}
		Expect(yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096).Decode(&csv)).To(Succeed())
		var installed []rbacv1.PolicyRule
		for _, permission := range csv.Spec.Install.Spec.ClusterPermissions {
			if permission.ServiceAccountName != "lightspeed-operator-controller-manager" {
				continue
			}
			for _, rule := range permission.Rules {
				if len(rule.APIGroups) == 1 && rule.APIGroups[0] == "apiextensions.k8s.io" {
					installed = append(installed, rule)
				}
			}
		}
		Expect(installed).To(Equal(expected), "OLM must authorize the same mandatory named CRD watch as the development manifests")
	})
})
