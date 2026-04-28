package relatedimages

import (
	"os"
	"path/filepath"
	"testing"
)

func resetImageCache() {
	imagesMu.Lock()
	defer imagesMu.Unlock()
	loaded = false
	imagesMap = nil
}

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module testmod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findModuleRoot(sub); got != root {
		t.Fatalf("findModuleRoot(%q) = %q, want %q", sub, got, root)
	}
	if got := findModuleRoot(root); got != root {
		t.Fatalf("findModuleRoot(%q) = %q, want %q", root, got, root)
	}
}

func TestGetDefaultImage_customFile(t *testing.T) {
	resetImageCache()
	t.Cleanup(resetImageCache)

	f := filepath.Join(t.TempDir(), "ri.json")
	content := `[{"name":"comp-a","image":"registry.example/a:v1"},{"name":"","image":"ignored"},{"name":"skip","image":""}]`
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELATED_IMAGES_FILE", f)

	if got := GetDefaultImage("comp-a"); got != "registry.example/a:v1" {
		t.Fatalf("GetDefaultImage(comp-a) = %q", got)
	}
	if got := GetDefaultImage("missing"); got != "" {
		t.Fatalf("GetDefaultImage(missing) = %q, want empty", got)
	}
	// Cached path: second call does not re-read disk
	if got := GetDefaultImage("comp-a"); got != "registry.example/a:v1" {
		t.Fatalf("second GetDefaultImage = %q", got)
	}
}

func TestGetDefaultImage_invalidJSON(t *testing.T) {
	resetImageCache()
	t.Cleanup(resetImageCache)

	f := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(f, []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELATED_IMAGES_FILE", f)

	if got := GetDefaultImage("anything"); got != "" {
		t.Fatalf("GetDefaultImage = %q, want empty on bad JSON", got)
	}
}

func TestGetDefaultImage_missingFile(t *testing.T) {
	resetImageCache()
	t.Cleanup(resetImageCache)

	t.Setenv("RELATED_IMAGES_FILE", filepath.Join(t.TempDir(), "does-not-exist.json"))

	if got := GetDefaultImage("lightspeed-service-api"); got != "" {
		t.Fatalf("GetDefaultImage = %q, want empty", got)
	}
}

func TestGetDefaultImage_repoRelatedImages(t *testing.T) {
	resetImageCache()
	t.Cleanup(resetImageCache)
	t.Setenv("RELATED_IMAGES_FILE", "")

	// From module root (go test ./internal/relatedimages), related_images.json should resolve.
	img := GetDefaultImage("lightspeed-service-api")
	if img == "" {
		t.Fatal("expected non-empty image for lightspeed-service-api from repo related_images.json")
	}
	if img2 := GetDefaultImage("lightspeed-service-api"); img2 != img {
		t.Fatal("cached value mismatch")
	}
}
