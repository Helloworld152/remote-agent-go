package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func pipeIdentity() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func dialLocal(timeout time.Duration) (net.Conn, error) {
	sid, err := pipeIdentity()
	if err != nil {
		return nil, err
	}
	return winio.DialPipe(`\\.\pipe\remote-agent-go-`+sid, &timeout)
}

func listenLocal() (net.Listener, error) {
	sid, err := pipeIdentity()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(`\\.\pipe\remote-agent-go-`+sid, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")"})
}

func lockStart() (func(), error) {
	f, err := os.OpenFile(filepath.Join(userHome(), ".remote-agent-go.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlap := &windows.Overlapped{}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlap); err != nil {
		f.Close()
		return nil, err
	}
	return func() { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlap); f.Close() }, nil
}

func prepareLocal() error {
	conn, err := dialLocal(time.Second)
	if err == nil {
		conn.Close()
		return fmt.Errorf("后台进程正在处理请求，请稍后重试")
	}
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

func configureBackground(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}
