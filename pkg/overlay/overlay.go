package overlay

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const overlayBasePath = "/var/lib/minidocker/overlay"

type OverlayMount struct {
	ContainerID string
	LowerDirs []string // Layer paths
	UpperDir string  // Container's writable layer
	WorkDir string // OverlayFS work directory
	MergedDir string // Final merged view
}

// CreateOverlay sets up an overlay filesystem for a container
func CreateOverlay(containerID string, layerPaths []string) (*OverlayMount, error) {
	overlay := &OverlayMount{
		ContainerID: containerID,
		LowerDirs: layerPaths,
		UpperDir: filepath.Join(overlayBasePath, containerID, "diff"),
		WorkDir: filepath.Join(overlayBasePath, containerID, "work"),
		MergedDir: filepath.Join(overlayBasePath, containerID, "merged"),
	}

	// Create directories
	for _, dir := range []string{overlay.UpperDir, overlay.WorkDir, overlay.MergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create a directory %s: %v", dir, err)
		}
	}

	// Mount overlay
	if err := overlay.Mount(); err != nil {
		return nil, err
	}

	return overlay, nil
}

// Mount performs the overlay mount
func (o *OverlayMount) Mount() error {
	// Build lowerdir string (reverse order: top layer first)
	// OverlayFS reads from left to right, so we reverse our layer order
	reversedLayers := make([]string, len(o.LowerDirs))
	for i, layer := range o.LowerDirs {
		reversedLayers[len(o.LowerDirs)-1-i] = layer
	}
	lowerdir := strings.Join(reversedLayers, ":")

	// Build mount options
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, o.UpperDir, o.WorkDir)

	// Execute mount command
	cmd := exec.Command("mount", "-t", "overlay", "overlay", "-o", options, o.MergedDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("overlay mount failed: %v, output: %s", err, string(output))
	}

	return nil
}

// Unmount unmounts the overlay filesystem
func (o *OverlayMount) Unmount() error {
	cmd := exec.Command("unmount", o.MergedDir)
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("unmount", "-l", o.MergedDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to unmount overlay: %v", err)
		}
	}

	return nil
}

// Cleanup removes overlay directories
func (o *OverlayMount) Cleanup() error {
	o.Unmount()

	overlayDir := filepath.Join(overlayBasePath, o.ContainerID)
	return os.RemoveAll(overlayDir)
}

// GetOverlay retreives overlay configurations for a container
func GetOverlay(containerID string) *OverlayMount {
	return &OverlayMount{
		ContainerID: containerID,
		UpperDir: filepath.Join(overlayBasePath, containerID, "diff"),
		WorkDir: filepath.Join(overlayBasePath, containerID, "work"),
		MergedDir: filepath.Join(overlayBasePath, containerID, "merged"),
	}
}

// CleanupOverlay removes overlay for a container
func CleanupOverlay(containerID string) error {
	overlay := GetOverlay(containerID)
	return overlay.Cleanup()
}
