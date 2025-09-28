package namespace

import (
    "fmt"
    "os"
    "os/exec"
    "syscall"
)

func RunInNewNamespace(command []string, rootfsPath string) error {
    if rootfsPath == "" {
        return fmt.Errorf("rootfs path required")
    }
    
    // Very simple, safe approach - no risky mount operations
    cmd := exec.Command("chroot", rootfsPath, command[0])
    if len(command) > 1 {
        cmd.Args = append([]string{"chroot", rootfsPath}, command...)
    }
    
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
    }
    
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    return cmd.Run()
}
