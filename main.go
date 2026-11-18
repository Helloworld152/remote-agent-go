package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: remote-agent-go start|status|stop|run|send|read|patch")
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "send", "read":
		action := os.Args[1]
		flags := flag.NewFlagSet(action, flag.ContinueOnError)
		wait := flags.Float64("wait", 0.3, "等待输出的秒数")
		if err = flags.Parse(os.Args[2:]); err != nil {
			break
		}
		if *wait < 0 || (action == "send" && flags.NArg() != 1) || (action == "read" && flags.NArg() != 0) {
			err = fmt.Errorf("用法: remote-agent-go send [-wait 秒数] '文本' 或 read [-wait 秒数]")
			break
		}
		if _, err = request(Request{Action: "status"}, time.Second); err != nil {
			if err = startServer(); err != nil {
				break
			}
		}
		var response Response
		response, err = request(Request{Action: action, Data: flags.Arg(0), Wait: *wait}, time.Duration(*wait*float64(time.Second))+10*time.Second)
		if err == nil {
			if response.Error != "" {
				err = fmt.Errorf("%s", response.Error)
				break
			}
			fmt.Print(response.Output)
			if response.Output != "" && !strings.HasSuffix(response.Output, "\n") {
				fmt.Println()
			}
		}
	case "run", "patch":
		action := os.Args[1]
		flags := flag.NewFlagSet(action, flag.ContinueOnError)
		timeout := flags.Float64("timeout", 120, "命令超时秒数")
		if err = flags.Parse(os.Args[2:]); err != nil {
			break
		}
		if flags.NArg() != 1 || *timeout <= 0 {
			err = fmt.Errorf("用法: remote-agent-go %s [-timeout 秒数] 参数", action)
			break
		}
		command := flags.Arg(0)
		if action == "patch" {
			var patch []byte
			patch, err = io.ReadAll(os.Stdin)
			if err != nil {
				break
			}
			directory := "'" + strings.ReplaceAll(command, "'", "'\\''") + "'"
			encoded := base64.StdEncoding.EncodeToString(patch)
			command = fmt.Sprintf("cd -- %s && printf %%s '%s' | base64 -d | patch -p1 --batch --reject-file=- --no-backup-if-mismatch", directory, encoded)
		}
		if _, err = request(Request{Action: "status"}, time.Second); err != nil {
			if err = startServer(); err != nil {
				break
			}
		}
		var response Response
		response, err = request(Request{Action: "run", Command: command, Timeout: *timeout}, time.Duration(*timeout*float64(time.Second))+10*time.Second)
		if err == nil {
			if response.Error != "" {
				err = fmt.Errorf("%s", response.Error)
				break
			}
			fmt.Print(response.Output)
			if response.Output != "" && !strings.HasSuffix(response.Output, "\n") {
				fmt.Println()
			}
			os.Exit(response.Code)
		}
	case "start":
		err = startServer()
	case "status":
		var response Response
		response, err = request(Request{Action: "status"}, time.Second)
		if err == nil {
			fmt.Println(response.Message)
		}
	case "stop":
		var response Response
		response, err = request(Request{Action: "stop"}, time.Second)
		if err == nil {
			fmt.Println(response.Message)
		}
	case "server":
		err = serve()
	default:
		err = fmt.Errorf("未知命令: %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
