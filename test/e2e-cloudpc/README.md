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
- **搜索框填字用 ValuePattern.SetValue**：Chromium 的 SetValue 会发 input
  事件，Vue 的 v-model 能收到（实测：设完 5 秒出 26 张卡）。设完读回经常
  是空（元素过期），不要拿读回值验，拿搜索结果验。不要用剪贴板粘贴/
  键盘打字：这台云电脑上 SendKeys 发不到输入框里（窗口前台、焦点、
  激活都试过，全都不进；0.9.16 轮的粘贴法在这里行不通）。
- **点台/点卡片要拿对元素**：行里的台名文本和行同名，Find-Element 会先
  撞到文本（点它没用）；点台用 ListItem 行，点卡片跳过 26px 高的历史
  小按钮、从标题文本往上找卡片容器；点卡后等详情打开（最多 30 秒）。
- **进度读 `.ctrl-time` 文本**：直播的 range input 值恒为 0，读 slider
  永远是 0；文本“mm:ss / mm:ss”才是真实进度。
- **星标按位置找，不要按名字**：★/☆ 按钮的名字在 UIA 里不稳定，
  按频道行里的按钮 x 坐标量对齐。
- **ThemedSelect 不是原生 ComboBox**：触发器是个 Button（名字是当前源），
  点开后选项在弹窗里；别找 ControlType.ComboBox。
- **先播再量播放器**：播放器容器、进度条都是播后才出现，没点台/没点集
  时 Find-PlayerGroup 必然是空。
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

## 经验（0.9.17 台式机首跑 15/33 后补的诊断）

- **Go 日志要重定向**：app 平时不写日志文件，标题栏 HRESULT、panic
  堆栈只在 stderr。E2E 启动时 `-RedirectStandardOutput/Error` 到
  `app-stdout.txt` / `app-stderr.txt`，悄悄退出的根因靠它定位。
- **滚轮量法（T10/T12）**：WebView2 不暴露 ScrollPattern，旧兜底用
  FromPoint 取元素——宿主元素没名字（item=''），yAfter 恒为空，滚没滚
  都判失败。改成按名字找标尺元素，看滚轮后 Y 是否移动。
- **遮罩点按（T06）**：先走 UIA Invoke（取光标处元素直接 Invoke），
  不行再退到合成鼠标点按，两次结果都记下来，分清是“点按没送达页面”
  还是“遮罩不响应”。
- **首播要等够（T16）**：冷启动中继取流慢，首播等 45 秒；频道行名是
  “CCTV-2 财经”这种，兜底别写 `*CCTV财经*`（永远匹配不到），写 `*财经*`。
- **详情页量不到先分清“没进详情”还是“进了量不到”（T21/T22/T29 组）**：
  搜完把 found + similar/episodes/lines 三个信号都记进 evidence。
- **类似卡宽（T25）**：CSS 卡片 120px、详情海报 150px，第一个量到的
  可能是海报；把整行卡宽都记下来再判。
- **浸泡（T34）**：每分钟除了进度，还记 WebView2/mpv 进程数、UIA 元素
  总数、状态行——分清渲染进程死、回落 mpv、单纯读不到三者。
- **无 git 构建的内部版本是 `(devel)`（T03b）**：不是 app bug，是构建
  环境没有 .git；不要为此改 app 代码造假版本。台式机装 Git 或以 CI
  发版构建（走 git tag）为准。
