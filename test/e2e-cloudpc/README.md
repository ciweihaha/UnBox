# UnBox 云电脑端到端测试（test/e2e-cloudpc）

每轮发版前在云电脑上跑的完整测试脚本（用户 2026-11-19 立的规矩：先测后发）。
脚本全过才打 tag；发版后用同一脚本再验一遍安装包。

## 文件

- `UnBox.Uia.ps1` — UIA 助手（找窗口/元素、InvokePattern 点按、滚轮、截图、取像素）。
- `UnBox.E2E.ps1` — 主脚本：v0.9.16 的 17 项 + 本轮修复 + 隐藏 bug 探查
  （反复切台、空搜索、坏源报错、长时间播放、悄悄退出监视）。

## 在云电脑上跑

前置：云电脑上有 Git、Go 1.27、Node 22+、wails3 CLI；仓库在 `C:\unbox`。

```powershell
# 1. 拉最新代码，本地构建（不要直接打 tag 发版）
cd C:\unbox; git pull
$env:VERSION = '0.9.17'
wails3 task windows:build          # 产物 C:\unbox\bin\UnBox.exe（便携版）
Copy-Item C:\unbox\bin\UnBox.exe C:\temp\unbox0917\UnBox-0.9.17-test.exe

# 2. 经计划任务在 session 1（交互会话）里跑测试
#    WinRM session 0 看不到桌面，UIA/截图/合成输入必须走 session 1。
$act = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument '-NoProfile -ExecutionPolicy Bypass -File C:\unbox\test\e2e-cloudpc\UnBox.E2E.ps1 -ExePath C:\temp\unbox0917\UnBox-0.9.17-test.exe -OutDir C:\temp\unbox0917'
$prin = New-ScheduledTaskPrincipal -UserId 'Administrator' -LogonType Interactive -RunLevel Highest
Register-ScheduledTask -TaskName 'UB-E2E' -Action $act -Principal $prin -Force
Start-ScheduledTask -TaskName 'UB-E2E'

# 3. 看结果：C:\temp\unbox0917\report.md / report.json；截图同目录。
#    长时间播放单跑：加 -SoakMinutes 30（播 CCTV-1 满 30 分钟，每分钟采样；
#    冻住时 60 秒内自动恢复算通过，卡死算失败）。
```

## 经验（0.9.16 轮踩过的坑）

- 合成鼠标点击在该 app 里不可靠，点按一律用 UIA InvokePattern。
- 滚轮测试必须先把光标移到列表上方再滚，否则测不到真滚动。
- 中文输出经 WinRM 会变 `??`：元素名按原文匹配，结果写进报告文件再拉回来看。
- 播放器 Group 的无障碍名是误导性的“无法播放按钮2”——判断卡住只看进度条。
- 进度用 `.ctrl-seek` 的 RangeValuePattern 读；读不到时退到 `.ctrl-time` 文本。
- 报错 toast 的触发路径：设置页填一个连不上的点播源地址再点导入。
- 测的时候留意“悄悄退出”：每步都查进程在不在，soak 每分钟记一次。
- **旧进程污染（0.9.17 轮踩到）**：上一轮的 UB916-Launch 任务把 0.9.16 包
  留在后台跑，脚本按窗口名找会测到旧窗口。脚本启动前先杀掉所有
  `UnBox*` 进程，并按窗口句柄反查 pid、只认本次启动的窗口；
  `Get-Process UnBox` 是精确匹配，查不到改名后的包，一定要用通配。
- **+dirty 的深层根因（0.9.17 轮查到）**：仓库的 go.mod 不是 tidy 状态、
  图标是旧版 wails 生成的，每次构建 `go mod tidy` 和 `generate:icons`
  都会弄脏树，内部版本永远带 +dirty。修法：提交 tidy 后的 go.mod/go.sum
  和 wails 3.1.0 生成的图标（已验证两台机器字节一致），以后构建是复写。
- **浅克隆没有 tag**：`git clone --depth 1` 拉不到 tag，Go 伪版本会变成
  `v0.0.0-时间-commit`；云电脑的克隆要 `git fetch --unshallow --tags`。
- **脚本编码**：.ps1 必须是 UTF-8 带 BOM，否则云电脑 PowerShell 5.1
  按 ANSI 解码，中文/★☆ 会解析失败。另：`$PID` 是只读自动变量，
  别拿它当变量名；`"MIN $m: ..."` 里 `$m:` 会被当成带盘符的变量，要写 `${m}`。
- **云电脑构建**：`proxy.golang.org` 连不上，构建脚本里设
  `GOPROXY=https://mirrors.aliyun.com/goproxy,direct` + `GOSUMDB=off`；
  GitHub 直链慢，下载走 `gh-proxy.com`（ghfast.top 从云电脑不通）。
