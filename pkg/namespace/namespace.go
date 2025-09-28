package namespace

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
    "syscall"
)

func RunInNewNamespaceWithCgroup(command []string, rootfsPath, containerID string) error {
    if rootfsPath == "" {
        return fmt.Errorf("rootfs path required")
    }
    
    // Create a wrapper script that will add itself to cgroup
    script := fmt.Sprintf(`#!/bin/bash
# Add current process to cgroup
echo $$ > /sys/fs/cgroup/minidocker-%s/cgroup.procs 2>/dev/null || true
# Execute the container command
exec chroot %s %s
`, containerID, rootfsPath, strings.Join(command, " "))
    
    // Write script to temporary file
    tmpScript := "/tmp/container_wrapper.sh"
    if err := os.WriteFile(tmpScript, []byte(script), 0755); err != nil {
        return fmt.Errorf("failed to create wrapper script: %v", err)
    }
    defer os.Remove(tmpScript)
    
    // Execute the wrapper script
    cmd := exec.Command("/bin/bash", tmpScript)
    
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
    }
    
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    return cmd.Run()
}

// Legacy function for backward compatibility
func RunInNewNamespace(command []string, rootfsPath string) error {
    return RunInNewNamespaceWithCgroup(command, rootfsPath, "")
}
