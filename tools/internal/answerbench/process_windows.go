package answerbench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func child(ctx context.Context, binary string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, binary, args...)
}

func ownProcessTree(cmd *exec.Cmd) (func() error, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		return nil, errors.Join(err, windows.CloseHandle(job))
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return nil, errors.Join(err, windows.CloseHandle(job))
	}
	err = windows.AssignProcessToJobObject(job, process)
	closeErr := windows.CloseHandle(process)
	if err != nil {
		return nil, errors.Join(err, closeErr, windows.CloseHandle(job))
	}
	if closeErr != nil {
		return nil, errors.Join(closeErr, windows.CloseHandle(job))
	}
	return func() error { return windows.CloseHandle(job) }, nil
}

func killChild(cmd *exec.Cmd) error {
	treeErr := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
	if treeErr == nil {
		return nil
	}
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return errors.Join(treeErr, err)
}

func terminateChild(cmd *exec.Cmd) error { return cmd.Process.Kill() }
