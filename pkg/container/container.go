package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"github.com/jagjeet-singh-23/minidocker/pkg/volume"
)

const containerStorePath = "/var/lib/minidocker/containers"

type ContainerState string

const (
	StateCreated ContainerState  = "created"
	StateRunning ContainerState  = "running"
	StateStopped ContainerState  = "stopped"
	StateExited  ContainerState  = "exited"
)

type Container struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Image       string            `json:"image"`
    Command     []string          `json:"command"`
    State       ContainerState    `json:"state"`
    PID         int               `json:"pid"`
    ExitCode    int               `json:"exit_code"`
    Created     time.Time         `json:"created"`
    Started     time.Time         `json:"started"`
    Finished    time.Time         `json:"finished"`
    LogPath     string            `json:"log_path"`
    IPAddress   string            `json:"ip_address"`
    Mounts      []volume.Mount    `json:"mounts"`
    NetworkMode string            `json:"network_mode"`
}

// SaveContainer persists container metadata
func SaveContainer(container *Container) error {
	if err := os.MkdirAll(containerStorePath, 0755); err != nil {
		return err
	}

	containerFile := filepath.Join(containerStorePath, container.ID+".json")
	data, err := json.MarshalIndent(container, "", "  ")

	if err != nil {
		return err
	}

	return os.WriteFile(containerFile, data, 0644)
}

// LoadContainer loads container metadata
func LoadContainer(containerID string) (*Container, error) {
	containerFile := filepath.Join(containerStorePath, containerID+".json")
	data, err := os.ReadFile(containerFile)

	if err != nil {
		return nil, err
	}

	var container Container
	if err := json.Unmarshal(data, &container); err != nil {
		return nil, err
	}

	return &container, nil
}

// ListContainers() returns all containers
func ListContainers() ([]*Container, error) {
	var containers []*Container

	if _, err := os.Stat(containerStorePath); os.IsNotExist(err) {
		return containers, nil
	}

	files, err := os.ReadDir(containerStorePath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			containerID := file.Name()[:len(file.Name()) - 5]
			if container, err := LoadContainer(containerID); err == nil {
				containers = append(containers, container)
			}
		}
	}

	return containers, nil
}

// RemoveContainer deletes container metadata and logs
func RemoveContainer(containerID string) error {
	container, err := LoadContainer(containerID)
	if err != nil {
		return err
	}

	if container.LogPath != "" {
		os.Remove(container.LogPath)
	}

	containerFile := filepath.Join(containerStorePath, containerID+".json")
	return os.Remove(containerFile)
}

// GenerateContainerID creates a unique container ID
func GenerateContainerID() string {
	return fmt.Sprintf("c%d", time.Now().UnixNano())
}

// FindContainerByPrefix finds container by ID prefix
func FindContainerByPrefix(prefix string) (*Container, error) {
    containers, err := ListContainers()
    if err != nil {
        return nil, err
    }
    
    var matches []*Container
    for _, c := range containers {
        if len(c.ID) >= len(prefix) && c.ID[:len(prefix)] == prefix {
            matches = append(matches, c)
        }
    }
    
    if len(matches) == 0 {
        return nil, fmt.Errorf("no container found with ID prefix: %s", prefix)
    }
    if len(matches) > 1 {
        return nil, fmt.Errorf("multiple containers found with ID prefix: %s", prefix)
    }
    
    return matches[0], nil
}
