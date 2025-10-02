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
    
    // Create a wrapper script
    cgroupAdd := ""
    if containerID != "" {
        // Only add to cgroup if it exists
        cgroupPath := fmt.Sprintf("/sys/fs/cgroup/minidocker-%s", containerID)
        cgroupAdd = fmt.Sprintf(`
if [ -d "%s" ]; then
    echo $$ > %s/cgroup.procs 2>/dev/null || true
fi
`, cgroupPath, cgroupPath)
    }
    
    script := fmt.Sprintf(`#!/bin/bash
%s
exec chroot %s %s
`, cgroupAdd, rootfsPath, strings.Join(command, " "))
    
    tmpScript := "/tmp/container_wrapper.sh"
    if err := os.WriteFile(tmpScript, []byte(script), 0755); err != nil {
        return fmt.Errorf("failed to create wrapper script: %v", err)
    }
    defer os.Remove(tmpScript)
    
    cmd := exec.Command("/bin/bash", tmpScript)
    
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
    }
    
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    return cmd.Run()
}

func RunInNewNamespace(command []string, rootfsPath string) error {
    return RunInNewNamespaceWithCgroup(command, rootfsPath, "")
}
