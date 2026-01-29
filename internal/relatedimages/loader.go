// Package relatedimages provides default container images from related_images.json.
// Loading is optional: if the file is missing or unreadable (e.g. when the operator runs in-cluster
// with all images passed via flags), GetDefaultImage returns empty string and the caller uses
// command-line or deployment-provided values. RELATED_IMAGES_FILE can set the path; otherwise
// the file is sought at the Go module root as related_images.json.
package relatedimages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type relatedImage struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

var (
	imagesOnce sync.Once
	imagesMap  map[string]string
)

// findModuleRoot walks up from dir looking for go.mod; returns dir if not found.
func findModuleRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

func loadImages() map[string]string {
	imagesOnce.Do(func() {
		imagesMap = make(map[string]string)

		filePath := os.Getenv("RELATED_IMAGES_FILE")
		if filePath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return
			}
			root := findModuleRoot(cwd)
			filePath = filepath.Join(root, "related_images.json")
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return
		}

		var list []relatedImage
		if err := json.Unmarshal(data, &list); err != nil {
			return
		}

		for _, entry := range list {
			if entry.Name != "" && entry.Image != "" {
				imagesMap[entry.Name] = entry.Image
			}
		}
	})
	return imagesMap
}

// DefaultImages returns a copy of the name->image map from related_images.json.
func DefaultImages() map[string]string {
	m := loadImages()
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetDefaultImage returns the image for the given component name (e.g. "lightspeed-service-api").
// Returns empty string if the name is not in related_images.json.
func GetDefaultImage(name string) string {
	return loadImages()[name]
}
