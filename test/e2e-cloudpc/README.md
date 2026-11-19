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
