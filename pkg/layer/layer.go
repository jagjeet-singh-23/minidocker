package layer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	layerBasePath = "/var/lib/minidocker/layers"
	overlayBasePath = "/var/lib/minidocker/overlay"
)

// Layer represents a filesystem layer
type Layer struct {
	ID         string    `json:"id"`          // SHA256 hash
	ParentID   string    `json:"parent_id"`   // Parent layer ID
	Size       int64     `json:"size"`        // Size in bytes
	Created    time.Time `json:"created"`
	CreatedBy  string    `json:"created_by"`  // Command that created this layer
	Comment    string    `json:"comment"`
}

// LayerMetadata stores information about a layer
type LayerMetadata struct {
	Layer
	Path string `json:"path"` // Filesystem path to layer content
}

// CreateLayer creates a new layer from a directory
func CreateLayer(sourcePath, createdBy, comment string) (*Layer, error) {
	// Calculate SHA256 of directory contents
	layerID, err := calculateDirHash(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate layer hash: %v", err)
	}

	// Get directory size
	size, err := getDirSize(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate size: %v", err)
	}

	layer := &Layer{
		ID:        layerID,
		Size:      size,
		Created:   time.Now(),
		CreatedBy: createdBy,
		Comment:   comment,
	}

	// Create layer directory
	layerPath := filepath.Join(layerBasePath, layerID)
	if err := os.MkdirAll(layerPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create layer directory: %v", err)
	}

	// Copy contents to layer storage
	if err := copyDir(sourcePath, layerPath); err != nil {
		return nil, fmt.Errorf("failed to copy layer contents: %v", err)
	}

	// Save layer metadata
	if err := saveLayerMetadata(layer); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %v", err)
	}

	return layer, nil
}

// GetLayer retrieves layer metadata
func GetLayer(layerID string) (*LayerMetadata, error) {
	metadataPath := filepath.Join(layerBasePath, layerID, "metadata.json")
	
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("layer not found: %s", layerID)
	}

	var layer Layer
	if err := json.Unmarshal(data, &layer); err != nil {
		return nil, err
	}

	return &LayerMetadata{
		Layer: layer,
		Path:  filepath.Join(layerBasePath, layerID),
	}, nil
}

// FindLayerByPrefix finds a layer by ID prefix (like container prefix matching)
func FindLayerByPrefix(prefix string) (*LayerMetadata, error) {
	layers, err := ListLayers()
	if err != nil {
		return nil, err
	}
	
	var matches []*LayerMetadata
	for _, l := range layers {
		if len(l.ID) >= len(prefix) && l.ID[:len(prefix)] == prefix {
			matches = append(matches, l)
		}
	}
	
	if len(matches) == 0 {
		return nil, fmt.Errorf("no layer found with ID prefix: %s", prefix)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple layers found with ID prefix: %s", prefix)
	}
	
	return matches[0], nil
}

// GetLayerOrPrefix tries to get layer by full ID or prefix
func GetLayerOrPrefix(layerID string) (*LayerMetadata, error) {
	// Try exact match first
	layer, err := GetLayer(layerID)
	if err == nil {
		return layer, nil
	}
	
	// Try prefix match
	return FindLayerByPrefix(layerID)
}

// ListLayers returns all layers
func ListLayers() ([]*LayerMetadata, error) {
	if _, err := os.Stat(layerBasePath); os.IsNotExist(err) {
		return []*LayerMetadata{}, nil
	}

	var layers []*LayerMetadata
	entries, err := os.ReadDir(layerBasePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		layer, err := GetLayer(entry.Name())
		if err != nil {
			continue
		}
		layers = append(layers, layer)
	}

	return layers, nil
}

// RemoveLayer deletes a layer
func RemoveLayer(layerID string) error {
	layerPath := filepath.Join(layerBasePath, layerID)
	return os.RemoveAll(layerPath)
}

// saveLayerMetadata persists layer metadata
func saveLayerMetadata(layer *Layer) error {
	metadataPath := filepath.Join(layerBasePath, layer.ID, "metadata.json")
	
	data, err := json.MarshalIndent(layer, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// calculateDirHash computes SHA256 hash of directory contents
func calculateDirHash(dirPath string) (string, error) {
	hash := sha256.New()
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip the root directory itself
		if path == dirPath {
			return nil
		}
		
		// Get relative path
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		
		// Hash the path
		hash.Write([]byte(relPath))
		
		// Hash file contents if it's a regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			if _, err := io.Copy(hash, file); err != nil {
				return err
			}
		}
		
		return nil
	})
	
	if err != nil {
		return "", err
	}
	
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// getDirSize calculates total size of directory
func getDirSize(dirPath string) (int64, error) {
	var size int64
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	
	return size, err
}

// copyDir recursively copies a directory using system tar command
func copyDir(src, dst string) error {
	// Ensure destination exists
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	
	// Use tar for reliable copying (handles symlinks, permissions, special files)
	// tar -C source -cf - . | tar -C dest -xf -
	cmd := exec.Command("sh", "-c", 
		fmt.Sprintf("cd %s && tar -cf - . | tar -C %s -xf -", 
			shellQuote(src), shellQuote(dst)))
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar copy failed: %v, output: %s", err, string(output))
	}
	
	return nil
}

// shellQuote quotes a string for use in shell commands
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
