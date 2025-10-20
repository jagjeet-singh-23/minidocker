package namespace

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
    "syscall"
    "time"
)

func RunInNewNamespaceWithCgroup(command []string, rootfsPath, containerID string, enableNetwork bool) (int, error) {
    if rootfsPath == "" {
        return 0, fmt.Errorf("rootfs path required")
    }
    
    cgroupAdd := ""
    if containerID != "" {
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
        return 0, fmt.Errorf("failed to create wrapper script: %v", err)
    }
    defer os.Remove(tmpScript)
    
    cmd := exec.Command("/bin/bash", tmpScript)
    
    cloneFlags := syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS
    if enableNetwork {
	    cloneFlags |= syscall.CLONE_NEWNET
    }

    cmd.SysProcAttr = &syscall.SysProcAttr{
	    Cloneflags: uintptr(cloneFlags),
    }
    
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    // Start the process
    if err := cmd.Start(); err != nil {
        return 0, err
    }
    
    pid := cmd.Process.Pid
    
    // Give process time to enter namespaces
    time.Sleep(100 * time.Millisecond)
    
    return pid, nil
}

func RunInNewNamespace(command []string, rootfsPath string) error {
    _, err := RunInNewNamespaceWithCgroup(command, rootfsPath, "", true)
    return err
}
