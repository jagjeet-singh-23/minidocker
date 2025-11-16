package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ImageManifest represents an image with its layers
type ImageManifest struct {
	Name        string            `json:"name"`
	Tag         string            `json:"tag"`
	Layers      []string          `json:"layers"`       // Layer IDs in order (bottom to top)
	Created     time.Time         `json:"created"`
	Author      string            `json:"author"`
	Config      ImageConfig       `json:"config"`
	Size        int64             `json:"size"`         // Total size of all layers
}

// ImageConfig contains runtime configuration
type ImageConfig struct {
	Cmd         []string          `json:"cmd"`          // Default command
	Entrypoint  []string          `json:"entrypoint"`   // Default entrypoint
	Env         []string          `json:"env"`          // Environment variables
	WorkingDir  string            `json:"working_dir"`
	User        string            `json:"user"`
	ExposedPorts []string         `json:"exposed_ports"`
}

// CreateImageFromLayers creates a new image from layer IDs
func CreateImageFromLayers(name, tag string, layerIDs []string, config ImageConfig) (*ImageManifest, error) {
	manifest := &ImageManifest{
		Name:    name,
		Tag:     tag,
		Layers:  layerIDs,
		Created: time.Now(),
		Config:  config,
	}

	// Calculate total size
	// TODO: Sum up layer sizes from layer metadata

	// Create image directory
	imagePath := filepath.Join(imageBasePath, name)
	if err := os.MkdirAll(imagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create image directory: %v", err)
	}

	// Save manifest
	if err := saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("failed to save manifest: %v", err)
	}

	return manifest, nil
}

// GetImageManifest retrieves image manifest
func GetImageManifest(imageName string) (*ImageManifest, error) {
	manifestPath := filepath.Join(imageBasePath, imageName, "manifest.json")
	
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("image manifest not found: %s", imageName)
	}

	var manifest ImageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// saveManifest persists image manifest
func saveManifest(manifest *ImageManifest) error {
	manifestPath := filepath.Join(imageBasePath, manifest.Name, "manifest.json")
	
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(manifestPath, data, 0644)
}

// ImageHasManifest checks if an image has a manifest (layered image)
func ImageHasManifest(imageName string) bool {
	manifestPath := filepath.Join(imageBasePath, imageName, "manifest.json")
	_, err := os.Stat(manifestPath)
	return err == nil
}
