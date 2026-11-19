# Build-Desktop.ps1 — 台式机同步 + 构建 UnBox 测试包（0.9.17 轮）。
#
# 放在仓库里就是为了能经 codeload ZIP 同步到台式机（台式机无 git，
# 不能直接拉代码；WinRM 单条命令又有长度限制，大脚本只能走 ZIP）。
# 在台式机上经计划任务 UB-BUILD 跑（构建不需要桌面交互，session 0 即可）：
#   powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\temp\unbox-src\UnBox-master\test\e2e-cloudpc\Build-Desktop.ps1
# 前置：台式机已装 Go 1.27.4、Wails v3.1.0、Node 24（TeleAgent 自带）。
# 产物：C:\temp\unbox0917\UnBox-0.9.17-test.exe；日志：sync0917.log。
$ErrorActionPreference = 'Stop'
$RepoRoot = $PSScriptRoot | Split-Path -Parent | Split-Path -Parent
$OutDir = 'C:\temp\unbox0917'
$Log = Join-Path $OutDir 'sync0917.log'
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir -Force | Out-Null }
"build start $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') repo=$RepoRoot" | Out-File $Log -Encoding utf8
function Log([string]$s) {
  $line = "$(Get-Date -Format 'HH:mm:ss') $s"
  $line | Out-File $Log -Encoding utf8 -Append
  Write-Output $line
}
Set-Location $RepoRoot

# Go / Node / wails 进 PATH（台式机固定位置）。
$env:Path = "C:\Program Files\Go\bin;$env:USERPROFILE\go\bin;$env:USERPROFILE\.local\share\TeleAgent\runtimes\node;$env:Path"
$env:GOPROXY = 'https://mirrors.aliyun.com/goproxy,direct'
$env:GOSUMDB = 'off'
$env:VERSION = '0.9.17'
Log "go: $(go version)"
Log "wails: $(wails3 version 2>&1 | Select-Object -First 1)"
Log "node: $(node --version) npm: $(npm.cmd --version)"

# 后台试装 Git（T03b 需要 git 才有伪版本；winget 慢，让它自己跑，不阻塞构建）。
Start-Job -Name 'git-install' -ScriptBlock {
  try {
    winget install --id Git.Git -e --silent --accept-source-agreements --accept-package-agreements --disable-interactivity 2>&1 | Out-File 'C:\temp\unbox0917\git-install.log' -Encoding utf8
    'winget exit: ' + $LASTEXITCODE | Out-File 'C:\temp\unbox0917\git-install.log' -Encoding utf8 -Append
  } catch {
    "winget failed: $_" | Out-File 'C:\temp\unbox0917\git-install.log' -Encoding utf8
  }
} | Out-Null
Log 'git install started in background (winget Git.Git)'

# 门禁：go test / go vet / gofmt（失败就停，不构建）。
go test ./... 2>&1 | Out-File (Join-Path $OutDir 'gotest.log') -Encoding utf8
if ($LASTEXITCODE -ne 0) { Log 'FAIL: go test'; Get-Content (Join-Path $OutDir 'gotest.log') -Tail 5 | ForEach-Object { Log $_ }; exit 1 }
Log 'go test OK'
go vet ./... 2>&1 | Out-File (Join-Path $OutDir 'govet.log') -Encoding utf8
if ($LASTEXITCODE -ne 0) { Log 'FAIL: go vet'; exit 1 }
Log 'go vet OK'
$gofmt = gofmt -l cmd internal
if ($gofmt) { Log "FAIL: gofmt dirty: $gofmt"; exit 1 }
Log 'gofmt OK'

# 构建（wails3 task windows:build，VERSION 经环境变量注入）。
wails3 task windows:build 2>&1 | Out-File (Join-Path $OutDir 'build.log') -Encoding utf8
if ($LASTEXITCODE -ne 0) { Log 'FAIL: wails build'; Get-Content (Join-Path $OutDir 'build.log') -Tail 10 | ForEach-Object { Log $_ }; exit 1 }
$exe = Join-Path $RepoRoot 'bin\UnBox.exe'
$size = (Get-Item $exe).Length
Log "build OK size=$size"
Copy-Item $exe (Join-Path $OutDir 'UnBox-0.9.17-test.exe') -Force
Copy-Item (Join-Path $RepoRoot 'test\e2e-cloudpc\UnBox.E2E.ps1') (Join-Path $OutDir 'UnBox.E2E.ps1') -Force
Copy-Item (Join-Path $RepoRoot 'test\e2e-cloudpc\UnBox.Uia.ps1') (Join-Path $OutDir 'UnBox.Uia.ps1') -Force
Log "copied exe + E2E scripts to $OutDir"
Log "git install job state: $((Get-Job -Name 'git-install').State)"
Log 'build done'
