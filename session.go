package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type remoteSession struct {
	client         *ssh.Client
	session        *ssh.Session
	input          io.WriteCloser
	output         chan []byte
	errors         chan error
	done           chan struct{}
	disconnected   chan struct{}
	buffer         []byte
	password       string
	asset          string
	directory      string
	connectTimeout time.Duration
}

func connect() (*remoteSession, error) {
	path := os.Getenv("REMOTE_AGENT_CONFIG")
	if path == "" {
		path = filepath.Join(os.Getenv("HOME"), ".config/remote-agent/config")
	} else if strings.HasPrefix(path, "~/") {
		path = filepath.Join(os.Getenv("HOME"), path[2:])
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	config := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			config[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if config["REMOTE_AGENT_JUMP_PASS"] == "" || config["REMOTE_AGENT_TARGET_PASS"] == "" {
		return nil, fmt.Errorf("配置文件缺少跳板机或目标机器密码")
	}
	get := func(key, fallback string) string {
		if value, ok := config[key]; ok {
			return value
		}
		return fallback
	}
	seconds, err := strconv.ParseFloat(get("REMOTE_AGENT_CONNECT_TIMEOUT", "20"), 64)
	if err != nil || seconds <= 0 {
		return nil, fmt.Errorf("无效的连接超时")
	}
	timeout := time.Duration(seconds * float64(time.Second))
	callback, err := knownhosts.New(filepath.Join(os.Getenv("HOME"), ".ssh/known_hosts"))
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(get("REMOTE_AGENT_JUMP_HOST", "192.168.1.195"), get("REMOTE_AGENT_JUMP_PORT", "2222"))
	client, err := ssh.Dial("tcp", address, &ssh.ClientConfig{
		User:            get("REMOTE_AGENT_JUMP_USER", "yruan"),
		Auth:            []ssh.AuthMethod{ssh.Password(config["REMOTE_AGENT_JUMP_PASS"])},
		HostKeyCallback: callback,
		Timeout:         timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("连接跳板机: %w", err)
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	r := &remoteSession{client: client, session: session, output: make(chan []byte, 16), errors: make(chan error, 1), done: make(chan struct{}), disconnected: make(chan struct{}), password: config["REMOTE_AGENT_TARGET_PASS"], asset: get("REMOTE_AGENT_ASSET", "上海伯兄-固定资产-CP4"), directory: get("REMOTE_AGENT_REMOTE_DIR", "~/shared"), connectTimeout: timeout}
	if err := r.login(); err != nil {
		r.close()
		return nil, err
	}
	return r, nil
}

func (r *remoteSession) login() error {
	var err error
	r.input, err = r.session.StdinPipe()
	if err != nil {
		return err
	}
	output, err := r.session.StdoutPipe()
	if err != nil {
		return err
	}
	if err = r.session.RequestPty("xterm", 50, 200, ssh.TerminalModes{}); err != nil {
		return err
	}
	if err = r.session.Shell(); err != nil {
		return err
	}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := output.Read(buffer)
			if n > 0 {
				select {
				case r.output <- append([]byte(nil), buffer[:n]...):
				case <-r.done:
					return
				}
			}
			if err != nil {
				r.errors <- err
				close(r.disconnected)
				return
			}
		}
	}()
	if err := r.waitFor(`Opt>`, r.connectTimeout); err != nil {
		return err
	}
	return r.loginTarget()
}

func (r *remoteSession) loginTarget() error {
	if _, err := io.WriteString(r.input, r.asset+"\r"); err != nil {
		return err
	}
	if err := r.waitFor(`password:`, r.connectTimeout); err != nil {
		return err
	}
	if _, err := io.WriteString(r.input, r.password+"\r"); err != nil {
		return err
	}
	if err := r.waitFor(`[$#]`, r.connectTimeout); err != nil {
		return err
	}
	if _, err := io.WriteString(r.input, "cd -- "+r.directory+" && echo __REMOTE_AGENT_\"\"READY__\r"); err != nil {
		return err
	}
	if err := r.waitFor(`__REMOTE_AGENT_READY__`, r.connectTimeout); err != nil {
		return err
	}
	return r.waitFor(`[$#]`, r.connectTimeout)
}

func (r *remoteSession) waitFor(pattern string, timeout time.Duration) error {
	rx := regexp.MustCompile(pattern)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if match := rx.FindIndex(r.buffer); match != nil {
			r.buffer = r.buffer[match[1]:]
			return nil
		}
		select {
		case part := <-r.output:
			r.buffer = append(r.buffer, part...)
		case err := <-r.errors:
			return fmt.Errorf("SSH 会话关闭: %w", err)
		case <-timer.C:
			return fmt.Errorf("等待 %s 超时", pattern)
		}
	}
}

func (r *remoteSession) run(command string, timeout time.Duration) (string, int, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	r.buffer = nil
	for {
		select {
		case <-r.output:
		default:
			goto drained
		}
	}
drained:
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", 0, err
	}
	begin := "__REMOTE_AGENT_BEGIN_" + hex.EncodeToString(id[:]) + "__"
	end := "__REMOTE_AGENT_END_" + hex.EncodeToString(id[:]) + "__"
	encoded := base64.StdEncoding.EncodeToString([]byte(command))
	var parts []string
	for len(encoded) > 1024 {
		parts = append(parts, "'"+encoded[:1024]+"'")
		encoded = encoded[1024:]
	}
	parts = append(parts, "'"+encoded+"'")
	begin64 := base64.StdEncoding.EncodeToString([]byte(begin))
	end64 := base64.StdEncoding.EncodeToString([]byte(end))
	payload := fmt.Sprintf("printf '\\n%%s\\n' \"$(printf %%s '%s' | base64 -d)\"; rc=0; ( eval \"$(printf %%s %s | base64 -d; printf '\\n')\" ) || rc=$?; printf '\\n%%s:%%s\\n' \"$(printf %%s '%s' | base64 -d)\" \"$rc\"", begin64, strings.Join(parts, " \\\r"), end64)
	if _, err := io.WriteString(r.input, payload+"\r"); err != nil {
		return "", 0, err
	}
	rx := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(end) + `:(\d+)[ \t\r]*\n`)
	lost := regexp.MustCompile(`\[Host\]>|搜索：|页码：|空闲时间超过60分钟`)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	retried := false
	for {
		if !retried && lost.Match(r.buffer) {
			r.buffer = nil
			if err := r.loginTarget(); err != nil {
				return "", 0, err
			}
			if _, err := io.WriteString(r.input, payload+"\r"); err != nil {
				return "", 0, err
			}
			retried = true
			timer.Reset(timeout)
			continue
		}
		if match := rx.FindSubmatchIndex(r.buffer); match != nil {
			start := strings.LastIndex(string(r.buffer[:match[0]]), begin)
			code, _ := strconv.Atoi(string(r.buffer[match[2]:match[3]]))
			if start < 0 {
				return "", 0, fmt.Errorf("未找到命令开始标记")
			}
			output := string(r.buffer[start+len(begin) : match[0]])
			r.buffer = r.buffer[match[1]:]
			return output, code, nil
		}
		select {
		case data := <-r.output:
			r.buffer = append(r.buffer, data...)
		case err := <-r.errors:
			return "", 0, fmt.Errorf("SSH 会话关闭: %w", err)
		case <-timer.C:
			r.buffer = nil
			return "", 0, fmt.Errorf("命令执行超时")
		}
	}
}

func (r *remoteSession) send(data string, wait time.Duration) (string, error) {
	if _, err := r.read(50 * time.Millisecond); err != nil {
		return "", err
	}
	if _, err := io.WriteString(r.input, data); err != nil {
		return "", err
	}
	return r.read(wait + 200*time.Millisecond)
}

func (r *remoteSession) read(wait time.Duration) (string, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var output []byte
	output = append(output, r.buffer...)
	r.buffer = nil
	for {
		select {
		case data := <-r.output:
			output = append(output, data...)
		case err := <-r.errors:
			return "", fmt.Errorf("SSH 会话关闭: %w", err)
		case <-timer.C:
			return string(output), nil
		}
	}
}

func (r *remoteSession) close() {
	close(r.done)
	r.session.Close()
	r.client.Close()
}
