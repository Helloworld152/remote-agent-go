package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func request(req Request, timeout time.Duration) (Response, error) {
	var response Response
	conn, err := dialLocal(time.Second)
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
	unlock, err := lockStart()
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := request(Request{Action: "status"}, time.Second); err == nil {
		fmt.Println("remote-agent-go 已运行")
		return nil
	}
	if err := prepareLocal(); err != nil {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(userHome(), ".remote-agent-go.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(executable, "server")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureBackground(cmd)
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
