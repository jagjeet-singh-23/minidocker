package image

import (
	"fmt"
	"os"
	"path/filepath"
)

const imageBasePath = "/var/lib/minidocker/images"

// ImageExists checks if an image exists locally
func ImageExists(imageName string) bool {
	imagePath := filepath.Join(imageBasePath, imageName)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return false
	}
	return true
}

// GetImageRootfs returns the path to image's rootfs
// Works with both layered and non-layered images
func GetImageRootfs(imageName string) (string, error) {
	if !ImageExists(imageName) {
		return "", fmt.Errorf("image %s not found", imageName)
	}

	// Check if this is a layered image
	if ImageHasManifest(imageName) {
		// For layered images, we'll handle this differently in the container run
		// For now, return empty to indicate layered image
		return "", fmt.Errorf("layered image - use GetImageManifest instead")
	}

	// Non-layered image: return direct rootfs path
	rootfsPath := filepath.Join(imageBasePath, imageName, "rootfs")
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("image rootfs not found for %s", imageName)
	}

	return rootfsPath, nil
}

// IsLayeredImage checks if an image uses layers
func IsLayeredImage(imageName string) bool {
	return ImageHasManifest(imageName)
}

// ListImages returns all the available images
func ListImages() ([]string, error) {
	var images []string

	if _, err := os.Stat(imageBasePath); os.IsNotExist(err) {
		return images, nil
	}

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

		// Add layer indicator
		if IsLayeredImage(imageName) {
			imageName += " (layered)"
		}

		images = append(images, imageName)
	}

	return images, nil
}

// GetImageType returns the type of image
func GetImageType(imageName string) string {
	if IsLayeredImage(imageName) {
		return "layered"
	}
	return "monolithic"
}
