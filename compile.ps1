$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -buildvcs=false -o bin/remote-agent-go.exe .
if ($LASTEXITCODE -ne 0) { throw 'Go 编译失败' }
