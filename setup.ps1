$ErrorActionPreference = 'Stop'
$projectDir = $PSScriptRoot
$configFile = if ($env:REMOTE_AGENT_CONFIG) { $env:REMOTE_AGENT_CONFIG } else { Join-Path $HOME '.config/remote-agent/config' }
$binDir = Join-Path $HOME 'bin'
$skillDir = Join-Path $HOME '.agents/skills/remote-agent-go'

if (-not (Test-Path -LiteralPath (Join-Path $projectDir 'bin/remote-agent-go.exe'))) {
    & (Join-Path $projectDir 'compile.ps1')
}

if (-not (Test-Path -LiteralPath $configFile)) {
    $jumpHost = Read-Host 'JumpServer 地址'
    $jumpPort = Read-Host 'JumpServer 端口 [2222]'
    $jumpUser = Read-Host 'JumpServer 用户名'
    $jumpSecret = Read-Host 'JumpServer 密码' -AsSecureString
    $asset = Read-Host '目标资产名称'
    $targetSecret = Read-Host '目标机器密码' -AsSecureString
    $remoteDir = Read-Host '远程默认目录 [~/shared]'
    if (-not $jumpHost -or -not $jumpUser -or -not $jumpSecret.Length -or -not $asset -or -not $targetSecret.Length) { throw '必填配置不能为空' }
    if (-not $jumpPort) { $jumpPort = '2222' }
    if (-not $remoteDir) { $remoteDir = '~/shared' }
    $jumpPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($jumpSecret)
    $targetPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($targetSecret)
    try {
        $jumpPass = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($jumpPtr)
        $targetPass = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($targetPtr)
        $content = @(
            "REMOTE_AGENT_JUMP_HOST=$jumpHost"
            "REMOTE_AGENT_JUMP_PORT=$jumpPort"
            "REMOTE_AGENT_JUMP_USER=$jumpUser"
            "REMOTE_AGENT_JUMP_PASS=$jumpPass"
            "REMOTE_AGENT_ASSET=$asset"
            "REMOTE_AGENT_TARGET_PASS=$targetPass"
            "REMOTE_AGENT_REMOTE_DIR=$remoteDir"
        ) -join "`n"
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $configFile) | Out-Null
        $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
        $acl = New-Object System.Security.AccessControl.FileSecurity
        $acl.SetOwner($sid)
        $acl.SetAccessRuleProtection($true, $false)
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($sid, 'FullControl', 'Allow')
        $acl.AddAccessRule($rule)
        $stream = [System.IO.File]::Create($configFile)
        $stream.Close()
        Set-Acl -LiteralPath $configFile -AclObject $acl
        [System.IO.File]::WriteAllText($configFile, $content + "`n", (New-Object System.Text.UTF8Encoding($false)))
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($jumpPtr)
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($targetPtr)
    }
}

New-Item -ItemType Directory -Force -Path $binDir, $skillDir | Out-Null
Copy-Item -LiteralPath (Join-Path $projectDir 'bin/remote-agent-go.exe') -Destination (Join-Path $binDir 'remote-agent-go.exe') -Force
Copy-Item -LiteralPath (Join-Path $projectDir 'skills/remote-agent-go/SKILL.md') -Destination (Join-Path $skillDir 'SKILL.md') -Force
Write-Host "命令: $binDir\remote-agent-go.exe"
Write-Host "Skill: $skillDir"
Write-Host "配置: $configFile"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @($userPath -split ';' | Where-Object { $_ })
$hasBinDir = $false
foreach ($entry in $pathEntries) {
    if ([Environment]::ExpandEnvironmentVariables($entry).Trim().TrimEnd('\') -ieq $binDir.TrimEnd('\')) {
        $hasBinDir = $true
        break
    }
}
if (-not $hasBinDir) {
    $newPath = if ($userPath) { $userPath.TrimEnd(';') + ';' + $binDir } else { $binDir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
Write-Host "已将 $binDir 加入用户 PATH；请重新打开终端或 Codex。"
Write-Host "请配置 $HOME\.ssh\known_hosts。"
