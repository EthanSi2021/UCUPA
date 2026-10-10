//go:build !windows

package acceptance_test

import (
	"os"
	"os/exec"
)

func configureProductProcess(*exec.Cmd) {}

func interruptProductProcess(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}
