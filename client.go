package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func socketPath() string {
	return filepath.Join(os.Getenv("HOME"), ".remote-agent-go.sock")
}

func request(req Request, timeout time.Duration) (Response, error) {
	var response Response
	conn, err := net.DialTimeout("unix", socketPath(), time.Second)
	if err != nil {
		return response, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return response, err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response, err
	}
	err = json.NewDecoder(conn).Decode(&response)
	return response, err
}

func startServer() error {
	lockFile, err := os.OpenFile(filepath.Join(os.Getenv("HOME"), ".remote-agent-go.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	if _, err := request(Request{Action: "status"}, time.Second); err == nil {
		fmt.Println("remote-agent-go 已运行")
		return nil
	}
	conn, err := net.DialTimeout("unix", socketPath(), time.Second)
	if err == nil {
		conn.Close()
		return fmt.Errorf("后台进程正在处理请求，请稍后重试")
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		info, statErr := os.Lstat(socketPath())
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("socket 路径被其他文件占用")
		}
		if err := os.Remove(socketPath()); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(os.Getenv("HOME"), ".remote-agent-go.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(executable, "server")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	for i := 0; i < 900; i++ {
		select {
		case err := <-exited:
			return fmt.Errorf("后台进程启动失败: %v，请查看 ~/.remote-agent-go.log", err)
		default:
		}
		if _, err := request(Request{Action: "status"}, time.Second); err == nil {
			fmt.Println("remote-agent-go 已启动")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("后台进程启动失败，请查看 ~/.remote-agent-go.log")
}
