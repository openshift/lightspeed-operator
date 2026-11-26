package hack

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func readImageFile(t *testing.T, path string) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var images []map[string]string
	if err := json.Unmarshal(data, &images); err != nil {
		t.Fatal(err)
	}
	return images
}

func TestHubImageGeneration(t *testing.T) {
	images := readImageFile(t, "../related_images.json")
	var hub, adapter map[string]string
	for _, image := range images {
		switch image["name"] {
		case "lightspeed-hub":
			hub = image
		case "lightspeed-agentic-alerts-adapter":
			adapter = image
		}
	}
	if hub["operator_arg"] != "hub-image" || hub["operator_target"] != "" || hub["snapshot_component"] != "lightspeed-hub" {
		t.Fatalf("invalid hub operand metadata: %v", hub)
	}
	for _, image := range []map[string]string{hub, adapter} {
		if !strings.HasPrefix(image["image"], "registry.redhat.io/") || !strings.Contains(image["image"], "@sha256:") {
			t.Fatalf("expected released digest-pinned image: %v", image)
		}
	}

	// Generate and substitute a temporary deployment patch, including mirror overrides.
	for _, mirror := range []bool{false, true} {
		t.Run(map[bool]string{false: "released", true: "mirrored"}[mirror], func(t *testing.T) {
			dir := t.TempDir()
			input, patch := filepath.Join(dir, "images.json"), filepath.Join(dir, "patch.yaml")
			selected := make([]map[string]string, 0, len(images))
			for _, image := range images {
				copy := make(map[string]string)
				for k, v := range image {
					copy[k] = v
				}
				if mirror {
					copy["image"] = "mirror.example.com/" + image["name"] + "@sha256:" + strings.Repeat("a", 64)
				}
				selected = append(selected, copy)
			}
			data, err := json.Marshal(selected)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(input, data, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "-c", `source ./image_args_lib.sh; image_args::generate_deployment_patch "$1" "$2"; image_args::substitute_deployment_patch "$2" "$1" "$(jq -r '.[] | select(.name == "lightspeed-operator") | .image' "$1")"`, "test", input, patch)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("patch generation: %v\n%s", err, output)
			}
			data, err = os.ReadFile(patch)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "__REPLACE_") {
				t.Fatal("unresolved image placeholder")
			}
			for _, image := range selected {
				if image["name"] == hub["name"] || image["name"] == adapter["name"] {
					if strings.Count(string(data), "--"+image["operator_arg"]+"="+image["image"]) != 1 {
						t.Fatalf("missing/duplicate image argument: %s", data)
					}
				}
			}
		})
	}

	// Check the final generated artifact, not only the source metadata.
	data, err := os.ReadFile("../bundle/manifests/lightspeed-operator.clusterserviceversion.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var csv struct {
		Spec struct {
			RelatedImages []struct {
				Name  string
				Image string
			}
			Install struct {
				Spec struct {
					Deployments []struct {
						Name string
						Spec struct {
							Template struct {
								Spec struct{ Containers []struct{ Args []string } }
							}
						}
					}
				}
			}
		}
	}
	if err := yaml.Unmarshal(data, &csv); err != nil {
		t.Fatal(err)
	}
	if len(csv.Spec.Install.Spec.Deployments) != 2 {
		t.Fatal("expected only Classic and Agentic CSV deployments")
	}
	for _, image := range []map[string]string{hub, adapter} {
		count := 0
		for _, related := range csv.Spec.RelatedImages {
			if related.Name == image["name"] {
				count++
				if related.Image != image["image"] {
					t.Fatalf("stale related image: %v", related)
				}
			}
		}
		if count != 1 {
			t.Fatalf("expected exactly one %s related image, got %d", image["name"], count)
		}
		count = 0
		for _, deployment := range csv.Spec.Install.Spec.Deployments {
			if deployment.Name != "lightspeed-operator-controller-manager" {
				continue
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				for _, arg := range container.Args {
					if arg == "--"+image["operator_arg"]+"="+image["image"] {
						count++
					}
				}
			}
		}
		if count != 1 {
			t.Fatalf("missing/duplicate Classic image argument for %s", image["name"])
		}
	}
}

func TestHubSnapshotRegistryMapping(t *testing.T) {
	dir := t.TempDir()
	// Stub only snapshot retrieval; exercise the real registry conversion and metadata retention.
	stub := `#!/bin/sh
printf '%s\n' '{"spec":{"components":[{"name":"lightspeed-hub","containerImage":"quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-hub@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":{"git":{"revision":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}]}}'
`
	if err := os.WriteFile(filepath.Join(dir, "oc"), []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	for _, registry := range []string{"stable", "preview", "ci"} {
		t.Run(registry, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "images.json")
			cmd := exec.Command("bash", "./snapshot_to_image_list.sh", "-s", "fixture", "-r", registry, "-o", output)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("snapshot conversion: %v\n%s", err, data)
			}
			prefix := map[string]string{"stable": "registry.redhat.io/openshift-lightspeed/lightspeed-hub-rhel9-operator", "preview": "registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-hub-rhel9-operator", "ci": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-hub"}[registry]
			for _, image := range readImageFile(t, output) {
				if image["name"] != "lightspeed-hub" {
					continue
				}
				if image["image"] != prefix+"@sha256:"+strings.Repeat("a", 64) || image["revision"] != strings.Repeat("b", 40) || image["operator_arg"] != "hub-image" {
					t.Fatalf("hub snapshot metadata lost: %v", image)
				}
				return
			}
			t.Fatal("hub missing from converted image list")
		})
	}
}
