package image

import (
	"fmt"
	"os"
	"path/filepath"
)

const imageBasePath = "/var/lib/minidocker/images"

// ImageExists checks if an image exists locally
func ImageExists(imageName string) bool {
	imagePath := filepath.Join(imageBasePath, imageName, "rootfs")
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return false
	}

	return true
}

// GetImageRootfs returns the path to image's rootfs
func GetImageRootfs(imageName string) (string, error) {
	if !ImageExists(imageName) {
		return "", fmt.Errorf("image %s not found", imageName)
	}

	return filepath.Join(imageBasePath, imageName, "rootfs")
}

// ListImages returns all the available images
func ListImages() ([]string, error) {
	var images []string

	entries, err := os.ReadDir(imageBasePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		imageName := entry.Name()
		if !ImageExists(imageName) {
			continue
		}

		images = append(images, imageName)
	}

	return images, nil
}
