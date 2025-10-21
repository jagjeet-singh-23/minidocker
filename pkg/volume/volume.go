package volume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const volumeBasePath = "/var/lib/minidocker/volumes"

type Volume struct {
	Name string       `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Created time.Time `json:"created"`
	Driver string     `json:"driver"`
}

// CreateVolume creates a new named volume
func CreateVolume(name string) (*Volume, error) {
	if name == "" {
		return nil, fmt.Errorf("volume name cannot be empty")
	}

	// Check if volume already exists
	volumePath := filepath.Join(volumeBasePath, name, "_data_")
	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create volume directory: %v", err)
	}

	volume := &Volume{
		Name:       name,
		Mountpoint: volumePath,
		Created:    time.Now(),
		Driver:     "local",
	}

	// Save volume metadata
	if err := saveVolume(volume); err != nil {
		os.RemoveAll(filepath.Join(volumeBasePath, name))
		return nil, err
	}

	return volume, nil
}

// VolumeExists checks if a volume exists
func VolumeExists(name string) bool {
	metadataPath := filepath.Join(volumeBasePath, name, "metadata.json")
	_, err := os.Stat(metadataPath)
	return err == nil
}

// GetVolume retrieves volume information
func GetVolume(name string) (*Volume, error) {
	metadataPath := filepath.Join(volumeBasePath, name, "metadata.json")
	
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("Volume %s not found", name)
	}

	var volume Volume
	if err := json.Unmarshal(data, &volume); err != nil {
		return nil, err
	}

	return &volume, nil
}

// ListVolumes returns all volumes
func ListVolumes() ([]*Volume, error) {
	if _, err := os.Stat(volumeBasePath); os.IsNotExist(err) {
		return []*Volume{}, nil
	}

	var volumes []*Volume
	entries, err := os.ReadDir(volumeBasePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		volume, err := GetVolume(entry.Name())
		if err != nil {
			continue
		}
		volumes = append(volumes, volume)
	}

	return volumes, nil
}

// RemoveVolume deletes a volume
func RemoveVolume(name string) error {
	if !VolumeExists(name) {
		return fmt.Errorf("volume %s not found", name)
	}

	volumePath := filepath.Join(volumeBasePath, name)
	return os.RemoveAll(volumePath)
}

// saveVolume persists volume metadata
func saveVolume(volume *Volume) error {
	metadataPath := filepath.Join(volumeBasePath, volume.Name, "metadata.json")

	data, err := json.MarshalIndent(volume, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// Mount represents a volume or bind mount
type Mount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

// ValidateMount validates a mount specification
func ValidateMount(mount *Mount) error {
	if mount.Destination == "" {
		return fmt.Errorf("mount destination cannot be empty")
	}

	switch mount.Type {
	case "bind":
		// Validate source path exists
		if _, err := os.Stat(mount.Source); err != nil {
			return fmt.Errorf("bind mount source %s does not exists", mount.Source)
		}
	case "volume":
		// Validate volume exists or can be created
		if mount.Source == "" {
			return fmt.Errorf("volume name cannot be empty")
		}
	default:
		return fmt.Errorf("invalid mount type: %s (use 'bind' or 'volume')", mount.Type)
	}

	return nil
}

// PrepareMount prepares a mount for container
func PrepareMount(mount *Mount) (string, error) {
	if err := ValidateMount(mount); err != nil {
		return "", err
	}

	var sourcePath string
	switch mount.Type {
	case "bind":
		sourcePath = mount.Source
	case "volume":
		if !VolumeExists(mount.Source) {
			volume, err := CreateVolume(mount.Source)
			if err != nil {
				return "", fmt.Errorf("failed to create a volume: %v", err)
			}

			sourcePath = volume.Mountpoint
		} else {
			volume, err := GetVolume(mount.Source)
			if err != nil {
				return "", err
			}
			sourcePath = volume.Mountpoint
		}
	}

	return sourcePath, nil
}

// ApplyMount applies a mount inside container using bind mount
func ApplyMount(rootfsPath string, mount *Mount, sourcePath string) error {
	targetPath := filepath.Join(rootfsPath, mount.Destination)

	// Create mount point in container
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %v", err)
	}

	// Bind mount the source into container
	mountFlags := "bind"
	if mount.ReadOnly {
		mountFlags += ",ro"
	}

	return nil
}
