---
name: remote-agent-go
description: 通过 Go 版 remote-agent 的持久 SSH 会话，经 JumpServer 操作远程 Linux 项目；仅在用户明确请求使用 Go 版或 remote-agent-go 时使用。
---

# remote-agent-go

`remote-agent-go` 是本机命令，自动读取配置并管理独立后台会话。Windows 和 Linux 均可使用，不要与 Python 版的会话混用；不要回显配置或凭据。

所有命令串行执行。在支持权限参数的环境中，命令及后台进程须在宿主环境执行；宿主不可用时停止并报告，不要启动第二个沙盒后台进程或手动删除运行文件。

用户给出项目关键词时，在远端默认目录查找一层目录：

```sh
remote-agent-go run 'find -L "$PWD" -maxdepth 1 -mindepth 1 -type d -iname "*关键词*" -print'
```

唯一匹配时记住绝对路径；无匹配或多个匹配时请用户确认。用户未提供后续命令时只报告路径。之后在远端项目目录执行任务，不要混用本地工作区。Windows 使用当前 shell 对参数正确引用。

`run '命令'` 执行远端命令；`send` 发送交互文本，执行命令需发送回车；`read` 获取交互输出；`patch 远端目录` 从标准输入接收带 `a/`、`b/` 前缀的 unified diff，远端需要 GNU patch 和 base64。选项如 `-timeout`、`-wait` 放在位置参数前。

远程编辑前读取目标文件，修改后检查结果和远程 diff。`run` 可能在识别到返回 JumpServer 菜单后重试一次；命令超时或断线时先检查远端状态，再决定是否重试，以免重复执行。不要在没有授权时发送消息或执行破坏性操作。
