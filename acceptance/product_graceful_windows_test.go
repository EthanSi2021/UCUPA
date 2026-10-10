package acceptance_test

import (
	"os/exec"
	"syscall"
)

// A separate process group lets this test deliver Ctrl+Break to only the
// product host. Go converts that console event to os.Interrupt, which is the
// same orderly-stop path used by a foreground Ctrl+C.
func configureProductProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func interruptProductProcess(cmd *exec.Cmd) error {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	defer dll.Release()
	proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}
	result, _, err := proc.Call(syscall.CTRL_BREAK_EVENT, uintptr(cmd.Process.Pid))
	if result == 0 {
		return err
	}
	return nil
}
