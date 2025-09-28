package cgroup

import (
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

const cgroupBasePath = "/sys/fs/cgroup"

type ContainerLimits struct {
    MemoryMB int
    CPUQuota float64  // 0.5 = 50% of one CPU core
}

// CreateCgroupForContainer creates cgroup and sets limits
func CreateCgroupForContainer(containerID string, limits ContainerLimits) error {
    // Create cgroup directory
    cgroupPath := filepath.Join(cgroupBasePath, "minidocker-"+containerID)
    if err := os.MkdirAll(cgroupPath, 0755); err != nil {
        return fmt.Errorf("failed to create cgroup: %v", err)
    }
    
    // Set memory limit
    if limits.MemoryMB > 0 {
        memoryLimit := limits.MemoryMB * 1024 * 1024 // Convert MB to bytes
        memoryFile := filepath.Join(cgroupPath, "memory.max")
        if err := os.WriteFile(memoryFile, []byte(strconv.Itoa(memoryLimit)), 0644); err != nil {
            return fmt.Errorf("failed to set memory limit: %v", err)
        }
    }
    
    // Set CPU limit
    if limits.CPUQuota > 0 {
        // CPU quota in microseconds (100000 = 100% of one core)
        cpuQuota := int(limits.CPUQuota * 100000)
        cpuPeriod := 100000  // Standard period
        
        cpuMaxFile := filepath.Join(cgroupPath, "cpu.max")
        cpuMaxValue := fmt.Sprintf("%d %d", cpuQuota, cpuPeriod)
        if err := os.WriteFile(cpuMaxFile, []byte(cpuMaxValue), 0644); err != nil {
            return fmt.Errorf("failed to set CPU limit: %v", err)
        }
    }
    
    return nil
}

// AddProcessToCgroup adds a process to the cgroup
func AddProcessToCgroup(containerID string, pid int) error {
    cgroupPath := filepath.Join(cgroupBasePath, "minidocker-"+containerID)
    procsFile := filepath.Join(cgroupPath, "cgroup.procs")
    
    return os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644)
}

// RemoveCgroup removes the cgroup directory
func RemoveCgroup(containerID string) error {
    cgroupPath := filepath.Join(cgroupBasePath, "minidocker-"+containerID)
    return os.RemoveAll(cgroupPath)
}

// GetCgroupStats returns memory and CPU usage
func GetCgroupStats(containerID string) (map[string]string, error) {
    cgroupPath := filepath.Join(cgroupBasePath, "minidocker-"+containerID)
    stats := make(map[string]string)
    
    // Get memory usage
    memCurrentFile := filepath.Join(cgroupPath, "memory.current")
    if data, err := os.ReadFile(memCurrentFile); err == nil {
        stats["memory_usage"] = strings.TrimSpace(string(data))
    }
    
    // Get memory limit
    memMaxFile := filepath.Join(cgroupPath, "memory.max")
    if data, err := os.ReadFile(memMaxFile); err == nil {
        stats["memory_limit"] = strings.TrimSpace(string(data))
    }
    
    return stats, nil
}
