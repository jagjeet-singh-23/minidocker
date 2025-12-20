package namespace

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
    "syscall"
    "time"
)

func RunInNewNamespaceWithCgroup(command []string, rootfsPath, containerID string, enableNetwork bool, env []string, workingDir string) (int, error) {
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
    
    // Build environment variables for the script
    envVars := ""
    for _, e := range env {
        // Export each environment variable
        envVars += fmt.Sprintf("export %s\n", shellescape(e))
    }
    
    // Set working directory (default to / if not specified)
    if workingDir == "" {
        workingDir = "/"
    }
    
    // Escape command arguments
    escapedCmd := make([]string, len(command))
    for i, arg := range command {
        escapedCmd[i] = shellescape(arg)
    }

    // Build the script with env vars and working directory
    script := fmt.Sprintf(`#!/bin/bash
%s
%s
exec chroot %s /bin/sh -c 'cd %s && exec %s'
`, cgroupAdd, envVars, rootfsPath, shellescape(workingDir), strings.Join(escapedCmd, " "))
    
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
    _, err := RunInNewNamespaceWithCgroup(command, rootfsPath, "", true, []string{}, "/")
    return err
}

func shellescape(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
