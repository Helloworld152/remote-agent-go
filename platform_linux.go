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
)

func socketPath() string { return filepath.Join(userHome(), ".remote-agent-go.sock") }

func dialLocal(timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", socketPath(), timeout)
}

func listenLocal() (net.Listener, error) {
	listener, err := net.Listen("unix", socketPath())
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath(), 0600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func lockStart() (func(), error) {
	f, err := os.OpenFile(filepath.Join(userHome(), ".remote-agent-go.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

func prepareLocal() error {
	conn, err := dialLocal(time.Second)
	if err == nil {
		conn.Close()
		return fmt.Errorf("后台进程正在处理请求，请稍后重试")
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		info, err := os.Lstat(socketPath())
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("socket 路径被其他文件占用")
		}
		return os.Remove(socketPath())
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func configureBackground(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
