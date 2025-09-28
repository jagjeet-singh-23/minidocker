package namespace

import (
	"os"
	"os/exec"
	"syscall"
)

// RunInNewNamespace runs a command in new PID and mount namespace
func RunInNewNamespace(command []string) error {
	// create a new namespace using unshare
	args := []string{"unshare", "--pid", "--mount", "--uts", "--fork"}
	args = append(args, command...)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
