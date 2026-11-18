package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type Request struct {
	Action  string  `json:"action"`
	Command string  `json:"command,omitempty"`
	Timeout float64 `json:"timeout,omitempty"`
	Data    string  `json:"data,omitempty"`
	Wait    float64 `json:"wait"`
}

type Response struct {
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
	Code    int    `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
}

func serve() error {
	session, err := connect()
	if err != nil {
		return err
	}
	defer session.close()
	listener, err := net.Listen("unix", socketPath())
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath(), 0600); err != nil {
		return err
	}
	pidPath := filepath.Join(os.Getenv("HOME"), ".remote-agent-go.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return err
	}
	defer os.Remove(pidPath)
	finished := make(chan struct{})
	defer close(finished)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			listener.Close()
		case <-finished:
		}
	}()
	go func() {
		session.client.Wait()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		var req Request
		err = json.NewDecoder(conn).Decode(&req)
		if err == nil {
			select {
			case <-session.disconnected:
				conn.Close()
				return nil
			default:
			}
			switch req.Action {
			case "status":
				err = json.NewEncoder(conn).Encode(Response{Message: "remote-agent-go 运行中"})
			case "stop":
				err = json.NewEncoder(conn).Encode(Response{Message: "remote-agent-go 已停止"})
			case "run":
				output, code, runErr := session.run(req.Command, time.Duration(req.Timeout*float64(time.Second)))
				response := Response{Output: output, Code: code}
				if runErr != nil {
					response.Error = runErr.Error()
				}
				err = json.NewEncoder(conn).Encode(response)
			case "send", "read":
				var output string
				var sessionErr error
				wait := time.Duration(req.Wait * float64(time.Second))
				if req.Action == "send" {
					output, sessionErr = session.send(req.Data, wait)
				} else {
					output, sessionErr = session.read(wait + 200*time.Millisecond)
				}
				response := Response{Output: output}
				if sessionErr != nil {
					response.Error = sessionErr.Error()
				}
				err = json.NewEncoder(conn).Encode(response)
			default:
				err = json.NewEncoder(conn).Encode(Response{Message: "未知请求"})
			}
		}
		conn.Close()
		select {
		case <-session.disconnected:
			return nil
		default:
		}
		if err == nil && req.Action == "stop" {
			return nil
		}
	}
}
