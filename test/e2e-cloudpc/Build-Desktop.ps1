# Build-Desktop.ps1 — 台式机同步 + 构建 UnBox 测试包（0.9.17 轮）。
#
# 放在仓库里就是为了能经 codeload ZIP 同步到台式机（台式机无 git，
# 不能直接拉代码；WinRM 单条命令又有长度限制，大脚本只能走 ZIP）。
# 在台式机上经计划任务 UB-BUILD 跑（构建不需要桌面交互，session 0 即可）：
#   powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\temp\unbox-src\UnBox-master\test\e2e-cloudpc\Build-Desktop.ps1
# 前置：台式机已装 Go 1.27.4、Wails v3.1.0、Node 24（TeleAgent 自带）。
# 产物：C:\temp\unbox0917\UnBox-0.9.17-test.exe；日志：sync0917.log。
#
# 注意：不要对原生命令用 `2>&1`（`$ErrorActionPreference='Stop'` 时，
# 原生命令写 stderr——wails3 version 的版本号就走 stderr——会被当成
# 终止错误，脚本没跑到构建就死掉；2026-11-20 01:14 构建就是这么死的）。
# 原生命令一律 stdout/stderr 分别写文件，再查 $LASTEXITCODE。
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
trap { Log "UNEXPECTED ERROR: $_"; exit 1 }
Set-Location $RepoRoot

# 原生命令：stdout/stderr 分写文件，返回退出码（不走 2>&1，见文件头注释）。
# 参数别叫 $Args：它和自动变量 $Args 冲突，@Args 展开为空，命令会无参运行
#（wails3 出 help、go 出 usage——2026-11-20 01:26 构建就是这么死的）。
# 调用期间 $ErrorActionPreference 临时改回 Continue：这台 PS 5.1 上即使
# stderr 已重定向到文件，原生命令的 stderr 文本仍会变成终止错误
#（wails3 version 的 "v3.1.0" 就走 stderr——2026-11-20 01:34 构建死在这里）；
# 改回 Continue 后只记一条非终止错误，退出码照常可用。
function Invoke-Native {
  param([string]$Exe, [string[]]$CmdArgs, [string]$OutLog)
  $errLog = "$OutLog.err"
  $oldEap = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    & $Exe @CmdArgs > $OutLog 2> $errLog
  } finally {
    $ErrorActionPreference = $oldEap
  }
  $code = $LASTEXITCODE
  if ((Test-Path $errLog) -and (Get-Item $errLog).Length -gt 0) {
    Get-Content $errLog -Raw | Out-File $OutLog -Encoding utf8 -Append
  }
  return $code
}

# Go / Node / wails 进 PATH（台式机固定位置）。
$env:Path = "C:\Program Files\Go\bin;$env:USERPROFILE\go\bin;$env:USERPROFILE\.local\share\TeleAgent\runtimes\node;$env:Path"
$env:GOPROXY = 'https://mirrors.aliyun.com/goproxy,direct'
$env:GOSUMDB = 'off'
$env:VERSION = '0.9.17'
Log "go: $(go version)"
$wailsOut = Join-Path $OutDir 'wails-version.log'
[void](Invoke-Native 'wails3' @('version') $wailsOut)
Log "wails: $((Get-Content $wailsOut -Raw).Trim())"
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
$code = Invoke-Native 'go' @('test', './...') (Join-Path $OutDir 'gotest.log')
if ($code -ne 0) { Log 'FAIL: go test'; Get-Content (Join-Path $OutDir 'gotest.log') -Tail 5 | ForEach-Object { Log $_ }; exit 1 }
Log 'go test OK'
$code = Invoke-Native 'go' @('vet', './...') (Join-Path $OutDir 'govet.log')
if ($code -ne 0) { Log 'FAIL: go vet'; exit 1 }
Log 'go vet OK'
$gofmt = gofmt -l cmd internal
if ($gofmt) { Log "FAIL: gofmt dirty: $gofmt"; exit 1 }
Log 'gofmt OK'

# 构建（wails3 task windows:build，VERSION 经环境变量注入）。
$code = Invoke-Native 'wails3' @('task', 'windows:build') (Join-Path $OutDir 'build.log')
if ($code -ne 0) { Log 'FAIL: wails build'; Get-Content (Join-Path $OutDir 'build.log') -Tail 10 | ForEach-Object { Log $_ }; exit 1 }
$exe = Join-Path $RepoRoot 'bin\UnBox.exe'
$size = (Get-Item $exe).Length
Log "build OK size=$size"
Copy-Item $exe (Join-Path $OutDir 'UnBox-0.9.17-test.exe') -Force
Copy-Item (Join-Path $RepoRoot 'test\e2e-cloudpc\UnBox.E2E.ps1') (Join-Path $OutDir 'UnBox.E2E.ps1') -Force
Copy-Item (Join-Path $RepoRoot 'test\e2e-cloudpc\UnBox.Uia.ps1') (Join-Path $OutDir 'UnBox.Uia.ps1') -Force
Log "copied exe + E2E scripts to $OutDir"
Log "git install job state: $((Get-Job -Name 'git-install').State)"
Log 'build done'
