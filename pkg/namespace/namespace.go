package namespace

import (
	"os"
	"os/exec"
	"syscall"
)

// RunInNewNamespace runs a command in new PID and mount namespace
func RunInNewNamespace(command []string) error {
	cmd := exec.Command(
		"sh",
		"-c",
		"mount -t proc proc /proc && exec " + strings.Join(command, " ")
	)

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
