package namespace

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// RunInNewNamespace runs a command in new PID and mount namespace
func RunInNewNamespace(command []string, rootfsPath string) error {
	var cmd *exec.Cmd
	var script string

	if rootfsPath == "" {
		// Original behaviour without rootfs (host filesystem)
		script = "mount -t proc proc /proc 2>dev/null && exec " + strings.Join(command, " ")
	} else {
		// Create /proc directory in rootfs if it does not exits
		os.MkdirAll(rootfsPath+"/proc", 0755)

		// Use chroot with the provided rootfs (Container filesystem)
		script = fmt.Sprintf(`
			mount -t proc proc %[1]s/proc 2>dev/null || true
			chroot %[1]s %[2]s
		`, rootfsPath, strings.Join(command, " "))
	}

	cmd = exec.Command("sh", "-c", script)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID |
			    syscall.CLONE_NEWNS |
			    syscall.CLONE_NEWUTS,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
