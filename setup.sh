#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
config_file="${REMOTE_AGENT_CONFIG:-$HOME/.config/remote-agent/config}"
bin_dir="$HOME/bin"
skill_dir="$HOME/.agents/skills/remote-agent-go"

"$project_dir/compile.sh"

if [[ ! -f "$config_file" ]]; then
  [[ -t 0 ]] || { printf '需要交互式终端创建配置: %s\n' "$config_file" >&2; exit 1; }
  read -r -p 'JumpServer 地址: ' jump_host
  read -r -p 'JumpServer 端口 [2222]: ' jump_port
  read -r -p 'JumpServer 用户名: ' jump_user
  read -r -s -p 'JumpServer 密码: ' jump_pass; printf '\n'
  read -r -p '目标资产名称: ' asset
  read -r -s -p '目标机器密码: ' target_pass; printf '\n'
  read -r -p '远程默认目录 [~/shared]: ' remote_dir
  [[ -n "$jump_host" && -n "$jump_user" && -n "$jump_pass" && -n "$asset" && -n "$target_pass" ]] || { printf '必填配置不能为空\n' >&2; exit 1; }
  mkdir -p "$(dirname -- "$config_file")"
  umask 077
  {
    printf 'REMOTE_AGENT_JUMP_HOST=%s\n' "$jump_host"
    printf 'REMOTE_AGENT_JUMP_PORT=%s\n' "${jump_port:-2222}"
    printf 'REMOTE_AGENT_JUMP_USER=%s\n' "$jump_user"
    printf 'REMOTE_AGENT_JUMP_PASS=%s\n' "$jump_pass"
    printf 'REMOTE_AGENT_ASSET=%s\n' "$asset"
    printf 'REMOTE_AGENT_TARGET_PASS=%s\n' "$target_pass"
    printf 'REMOTE_AGENT_REMOTE_DIR=%s\n' "${remote_dir:-'~/shared'}"
  } > "$config_file"
fi

mkdir -p "$bin_dir" "$skill_dir"
install -m 0755 "$project_dir/bin/remote-agent-go" "$bin_dir/remote-agent-go"
install -m 0644 "$project_dir/skills/remote-agent-go/SKILL.md" "$skill_dir/SKILL.md"
printf '命令: %s\nSkill: %s\n配置: %s\n请确保 %s 在 PATH 中，并配置 ~/.ssh/known_hosts。\n' "$bin_dir/remote-agent-go" "$skill_dir" "$config_file" "$bin_dir"
