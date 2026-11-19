# UnBox.E2E.ps1 — UnBox 云电脑端到端测试（可复用，每轮发版前跑）。
#
# 覆盖：v0.9.16 的 17 项 + v0.9.17 的冻住/滚轮/标题栏/关于/类似内容/
# 美化 8 条 + 隐藏 bug 探查（边界、异常路径、长时间播、反复切换）。
# 先测后发：脚本全过才打 tag；发版后用同一脚本再验一遍安装包。
#
# 运行方式（在云电脑 session 1 里，经计划任务）：
#   powershell -ExecutionPolicy Bypass -File C:\unbox\test\e2e-cloudpc\UnBox.E2E.ps1 `
#     -ExePath C:\temp\unbox0917\UnBox-0.9.17-test.exe -OutDir C:\temp\unbox0917
#   长时间播放：-SoakMinutes 30（播 CCTV-1 满 30 分钟，每分钟采样进度；
#   冻住时看 60 秒内是否自动恢复——能恢复算通过，卡死算失败）。
# 结果：$OutDir\report.json（机器可读）+ report.md（人读）+ 截图。
# 注意：不要在台式机/平板上跑——只在云电脑上测；不要装安装版，只跑便携版。

param(
  [string]$ExePath = 'C:\temp\unbox0917\UnBox-0.9.17-test.exe',
  [string]$OutDir = 'C:\temp\unbox0917',
  [int]$SoakMinutes = 0,
  [switch]$Probe
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'UnBox.Uia.ps1')

if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir -Force | Out-Null }
$script:Results = @()
$script:Shots = 0
$script:Win = $null
$script:Pid = 0

function Add-Result {
  param([string]$Id, [string]$Name, [bool]$Pass, [string]$Evidence, [string]$Note = '')
  $script:Results += [pscustomobject]@{
    id = $Id; name = $Name; pass = $Pass; evidence = $Evidence; note = $Note
  }
  Write-Output ("[{0}] {1}: {2} -- {3}" -f $Id, $(if ($Pass) { 'PASS' } else { 'FAIL' }), $Name, $Evidence)
}

function Shot {
  param([string]$Tag)
  $script:Shots++
  $p = Join-Path $OutDir ("{0:D2}-{1}.jpg" -f $script:Shots, $Tag)
  Save-Screenshot -Path $p | Out-Null
  return $p
}

function Wait-For {
  param([scriptblock]$Cond, [int]$Seconds = 15, [int]$StepMs = 500)
  $end = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $end) {
    try { $r = & $Cond; if ($r) { return $r } } catch { }
    Start-Sleep -Milliseconds $StepMs
  }
  return $null
}

function Click-Name {
  param([string]$Name, [int]$WaitSeconds = 10)
  $el = Wait-For { Find-Element -Root $script:Win -Name $Name } -Seconds $WaitSeconds
  if (-not $el) { return $false }
  return (Invoke-Element $el)
}

function Go-Mode {
  param([string]$Mode)
  # 顶栏导航：首页 / 直播 / 我的片单；设置是右上角齿轮（aria-label 设置）。
  switch ($Mode) {
    'home' { return (Click-Name '首页') }
    'live' { return (Click-Name '直播') }
    'favorites' { return (Click-Name '我的片单') }
    'settings' { return (Click-Name '设置') }
    'search' { return (Click-Name '搜索') }
  }
  return $false
}

function Get-WindowRect { return (Get-ElementRect $script:Win) }

function Get-PlayerProgress {
  # 播放器进度：.ctrl-seek 是 range input，UIA 里是 Slider（RangeValuePattern）；
  # 兜底读 .ctrl-time 文本“mm:ss / mm:ss”。
  $slider = $null
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') { $slider = $e; break }
    } catch { }
  }
  if ($slider) {
    try { return [double]($slider.GetCurrentPattern($script:RangePattern).Value) } catch { }
  }
  $t = Find-Element -Root $script:Win -Name ' / '
  if ($t) {
    $txt = Get-ElementText $t
    if ($txt -match '(\d+):(\d+)') { return ([int]$Matches[1]) * 60 + [int]$Matches[2] }
  }
  return $null
}

function Get-ScrollPercentAt {
  param([int]$X, [int]$Y)
  $pt = New-Object System.Windows.Point($X, $Y)
  $el = $script:AE::FromPoint($pt)
  if (-not $el) { return $null }
  $cur = $el
  for ($i = 0; $i -lt 8 -and $cur; $i++) {
    try {
      $sp = $cur.GetCurrentPattern($script:ScrollPattern)
      if ($sp -and $sp.VerticallyScrollable) { return $sp.VerticalScrollPercent }
    } catch { }
    try { $cur = [System.Windows.Automation.TreeWalker]::RawViewWalker.GetParent($cur) } catch { $cur = $null }
  }
  return $null
}

function Test-WheelScrolls {
  param([string]$Id, [string]$Name, [int]$X, [int]$Y)
  Move-Mouse -X $X -Y $Y
  $before = Get-ScrollPercentAt -X $X -Y $Y
  Send-Wheel -Delta -360 -Clicks 1
  Start-Sleep -Milliseconds 800
  $after = Get-ScrollPercentAt -X $X -Y $Y
  if ($null -ne $before -and $null -ne $after) {
    Add-Result -Id $Id -Name $Name -Pass ($after -ne $before) -Evidence "scrollpercent before=$before after=$after"
    return
  }
  # ScrollPattern 读不到时：看光标处条目的位置是否移动（滚下去后 Y 变小或滚出视图）。
  $el = $script:AE::FromPoint((New-Object System.Windows.Point($X, $Y)))
  $name = ''
  $yBefore = $null
  if ($el) {
    try { $name = $el.Current.Name; $yBefore = $el.Current.BoundingRectangle.Y } catch { }
  }
  Send-Wheel -Delta -360 -Clicks 2
  Start-Sleep -Milliseconds 800
  $yAfter = $null
  $gone = $false
  if ($name) {
    $el2 = Find-Element -Root $script:Win -Name $name -Exact
    if ($el2) { try { $yAfter = $el2.Current.BoundingRectangle.Y } catch { $gone = $true } }
    else { $gone = $true }
  }
  $moved = $gone -or ($null -ne $yBefore -and $null -ne $yAfter -and [Math]::Abs($yAfter - $yBefore) -gt 5)
  Add-Result -Id $Id -Name $Name -Pass $moved -Evidence "item='$name' yBefore=$yBefore yAfter=$yAfter gone=$gone"
}

# ---------- 启动 ----------
if (-not (Test-Path $ExePath)) { throw "找不到测试包：$ExePath" }
$proc = Start-Process -FilePath $ExePath -WorkingDirectory (Split-Path $ExePath -Parent) -PassThru
$script:Pid = $proc.Id
$script:Win = Wait-For { Get-UnBoxWindow } -Seconds 60
Add-Result -Id 'T01' -Name '应用启动，窗口出现' -Pass ($null -ne $script:Win) -Evidence "pid=$($script:Pid) window=$($null -ne $script:Win)"
if (-not $script:Win) { throw '窗口 60 秒内没出现' }
Start-Sleep -Seconds 3
$wr = Get-WindowRect
Shot -Tag 'home' | Out-Null

if ($Probe) {
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  $names = @()
  foreach ($e in $all) { try { $names += $e.Current.Name } catch { } }
  ($names | Sort-Object -Unique) -join "`n" | Out-File (Join-Path $OutDir 'probe-names.txt')
  Write-Output 'Probe: 元素名已写 probe-names.txt'
}

# ---------- 标题栏 ----------
$cx = [int]($wr.X + $wr.Width / 2)
$pixTop = Get-ScreenPixel -X $cx -Y ([int]$wr.Y + 10)
$pixLeft = Get-ScreenPixel -X ([int]$wr.X + 100) -Y ([int]$wr.Y + 10)
$isWhite = ($pixTop.R -gt 240 -and $pixTop.G -gt 240 -and $pixTop.B -gt 240)
Add-Result -Id 'T02' -Name '标题栏不是白色（Server 2022）' -Pass (-not $isWhite) -Evidence "center=($($pixTop.R),$($pixTop.G),$($pixTop.B)) left=($($pixLeft.R),$($pixLeft.G),$($pixLeft.B))"

# ---------- 关于 ----------
Go-Mode 'settings' | Out-Null
Start-Sleep -Seconds 1
Click-Name '关于我们' | Out-Null
$aboutTitle = Wait-For { Find-Element -Root $script:Win -Name '关于 UnBox' } -Seconds 10
$aboutPass = $null -ne $aboutTitle
$verEvidence = 'dialog=missing'
if ($aboutTitle) {
  $verEl = Find-Element -Root $script:Win -Name '内部版本'
  $verText = if ($verEl) { Get-ElementText $verEl } else { '' }
  $curEl = Find-Element -Root $script:Win -Name '当前版本'
  $curText = if ($curEl) { Get-ElementText $curEl } else { '' }
  $clean = ($verText -match 'v\d+\.\d+\.\d+$') -and ($verText -notmatch '\+dirty')
  $verEvidence = "internal='$verText' current='$curText'"
  Add-Result -Id 'T03' -Name '关于：内部版本干净（无 +dirty）' -Pass $clean -Evidence $verEvidence
  # 关闭 × 位置：应在弹窗右上角（x 大于窗口中心）。
  $closeBtn = Find-Element -Root $script:Win -Name '✕'
  $closeRect = Get-ElementRect $closeBtn
  $onRight = $closeRect -and ($closeRect.X -gt ($wr.X + $wr.Width / 2))
  Add-Result -Id 'T04' -Name '关于：关闭 × 在右上角' -Pass $onRight -Evidence "closeX=$($closeRect.X) windowCenterX=$($wr.X + $wr.Width / 2)"
  Shot -Tag 'about' | Out-Null
  # Esc 关闭。
  Send-Key '{ESC}'
  Start-Sleep -Milliseconds 800
  $gone = $null -eq (Find-Element -Root $script:Win -Name '关于 UnBox')
  Add-Result -Id 'T05' -Name '关于：Esc 关闭' -Pass $gone -Evidence "closedAfterEsc=$gone"
  if (-not $gone) { Invoke-Element $closeBtn | Out-Null; Start-Sleep -Milliseconds 500 }
  # 遮罩点击关闭：重开后点弹窗外、窗口内的空白处。
  Click-Name '关于我们' | Out-Null
  Start-Sleep -Seconds 1
  $panel = Find-Element -Root $script:Win -Name '关于 UnBox'
  if ($panel) {
    $pr = Get-ElementRect $panel
    Move-Mouse -X ([int]($wr.X + 30)) -Y ([int]($wr.Y + $wr.Height - 30))
    Add-Type @"
using System; using System.Runtime.InteropServices;
public static class Clicker { [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint x, uint y, int d, UIntPtr e); }
"@ -ErrorAction SilentlyContinue
    [Clicker]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
    [Clicker]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 800
    $gone2 = $null -eq (Find-Element -Root $script:Win -Name '关于 UnBox')
    Add-Result -Id 'T06' -Name '关于：点遮罩关闭' -Pass $gone2 -Evidence "closedAfterMaskClick=$gone2"
    if (-not $gone2) { Send-Key '{ESC}'; Start-Sleep -Milliseconds 500 }
  } else {
    Add-Result -Id 'T06' -Name '关于：点遮罩关闭' -Pass $false -Evidence '重开关于失败'
  }
} else {
  Add-Result -Id 'T03' -Name '关于：内部版本干净（无 +dirty）' -Pass $false -Evidence $verEvidence
  Add-Result -Id 'T04' -Name '关于：关闭 × 在右上角' -Pass $false -Evidence $verEvidence
  Add-Result -Id 'T05' -Name '关于：Esc 关闭' -Pass $false -Evidence $verEvidence
  Add-Result -Id 'T06' -Name '关于：点遮罩关闭' -Pass $false -Evidence $verEvidence
}

# ---------- 首页 ----------
Go-Mode 'home' | Out-Null
Start-Sleep -Seconds 3
$wr = Get-WindowRect
$heroMeta = Find-Element -Root $script:Win -Name '聚合万能源'
$cardNames = (Find-Elements -Root $script:Win -NameLike '*聚合万能源*')
Add-Result -Id 'T07' -Name '首页不显示源名' -Pass (($null -eq $heroMeta) -and ($cardNames.Count -eq 0)) -Evidence "heroLeak=$($null -ne $heroMeta) cardLeaks=$($cardNames.Count)"
$combos = @()
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.ComboBox') { $combos += $e } } catch { } }
Add-Result -Id 'T08' -Name '首页无源下拉框' -Pass ($combos.Count -eq 0) -Evidence "comboboxes=$($combos.Count)"
Test-WheelScrolls -Id 'T10' -Name '滚轮：首页能滚' -X ([int]($wr.X + $wr.Width / 2)) -Y ([int]($wr.Y + $wr.Height / 2))
Shot -Tag 'home-after-wheel' | Out-Null

# ---------- 直播 ----------
Go-Mode 'live' | Out-Null
Start-Sleep -Seconds 3
$wr = Get-WindowRect
$liveList = Find-Element -Root $script:Win -Name 'CCTV'
$playerGroup = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Group' -and $e.Current.BoundingRectangle.Width -gt 800) { $playerGroup = $e; break }
  } catch { }
}
$pgRect = Get-ElementRect $playerGroup
Add-Result -Id 'T14' -Name '直播两栏：左列表右播放' -Pass (($null -ne $liveList) -and ($null -ne $playerGroup)) -Evidence "list=$($null -ne $liveList) playerW=$($pgRect.Width)"
Add-Result -Id 'T18' -Name '直播页铺满（播放区高≥500）' -Pass ($pgRect -and $pgRect.Height -ge 500) -Evidence "playerH=$($pgRect.Height)"
# 星标对齐：所有收藏星按钮 x 应在同一列（误差≤5px）。
$stars = @()
foreach ($e in $all) {
  try {
    if (($e.Current.Name -eq '★' -or $e.Current.Name -eq '☆') -and $e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button') {
      $r = $e.Current.BoundingRectangle
      if ($r.X -gt $wr.X -and $r.X -lt ($wr.X + 500)) { $stars += $r.X }
    }
  } catch { }
}
$starSpread = 0
if ($stars.Count -ge 2) { $starSpread = ($stars | Measure-Object -Maximum).Maximum - ($stars | Measure-Object -Minimum).Minimum }
Add-Result -Id 'T15' -Name '直播星标统一在行右端' -Pass ($stars.Count -ge 2 -and $starSpread -le 5) -Evidence "stars=$($stars.Count) spread=$starSpread"
Test-WheelScrolls -Id 'T12' -Name '滚轮：直播列表能滚' -X ([int]($wr.X + 200)) -Y ([int]($wr.Y + $wr.Height / 2))
Shot -Tag 'live' | Out-Null
# 点第一行播，进度 5 秒内前进。
$firstRow = Find-Element -Root $script:Win -Name 'CCTV-1'
if (-not $firstRow) { $firstRow = Find-Element -Root $script:Win -Name 'CCTV' }
if ($firstRow) { Invoke-Element $firstRow | Out-Null }
Start-Sleep -Seconds 3
$p1 = Get-PlayerProgress
Start-Sleep -Seconds 5
$p2 = Get-PlayerProgress
Add-Result -Id 'T16' -Name '直播点播后进度前进' -Pass ($null -ne $p1 -and $null -ne $p2 -and $p2 -gt $p1) -Evidence "p1=$p1 p2=$p2"
# 空格暂停/继续。
Send-Key ' '
Start-Sleep -Seconds 3
$pausedText = Find-Element -Root $script:Win -Name '已暂停'
$p3 = Get-PlayerProgress
Start-Sleep -Seconds 3
$p4 = Get-PlayerProgress
$pausedOk = ($null -ne $pausedText) -and ($null -ne $p3 -and $null -ne $p4 -and ($p4 - $p3) -lt 1.5)
Add-Result -Id 'T17' -Name '直播空格暂停（进度停住+已暂停）' -Pass $pausedOk -Evidence "pausedText=$($null -ne $pausedText) p3=$p3 p4=$p4"
Send-Key ' '
Start-Sleep -Seconds 3
$p5 = Get-PlayerProgress
Add-Result -Id 'T17b' -Name '直播空格继续（进度恢复）' -Pass ($null -ne $p4 -and $null -ne $p5 -and $p5 -gt $p4) -Evidence "p4=$p4 p5=$p5"
Shot -Tag 'live-playing' | Out-Null

# ---------- 点播：搜荒野求生进详情 ----------
Go-Mode 'search' | Out-Null
Start-Sleep -Seconds 1
$searchBox = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $searchBox = $e; break } } catch { }
}
if ($searchBox) {
  try { $searchBox.GetCurrentPattern($script:ValuePattern).SetValue('荒野求生') } catch { }
  Send-Key '{ENTER}'
  Start-Sleep -Seconds 5
  $hit = Wait-For { Find-Element -Root $script:Win -Name '荒野求生' } -Seconds 20
  if ($hit) { Invoke-Element $hit | Out-Null; Start-Sleep -Seconds 4 }
}
$detailPlayer = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Group' -and $e.Current.BoundingRectangle.Width -gt 700) { $detailPlayer = $e; break }
  } catch { }
}
$dpRect = Get-ElementRect $detailPlayer
Add-Result -Id 'T30' -Name '详情是整页（播放区宽≥700）' -Pass ($null -ne $detailPlayer) -Evidence "playerW=$($dpRect.Width) playerH=$($dpRect.Height)"
Shot -Tag 'detail-manvs' | Out-Null
# 线路按钮友好名。
$lineBtns = Find-Elements -Root $script:Win -NameLike '线路*'
$rawIds = Find-Elements -Root $script:Win -NameLike '*m3u8*'
Add-Result -Id 'T21' -Name '详情线路显示友好名（线路N）' -Pass ($lineBtns.Count -ge 1 -and $rawIds.Count -eq 0) -Evidence "friendly=$($lineBtns.Count) rawIds=$($rawIds.Count)"
# 选集按钮更大：量第一个“第*集”按钮的高度。
$epBtn = $null
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -match '^第.*集$') { $epBtn = $e; break } } catch { }
}
$epRect = Get-ElementRect $epBtn
Add-Result -Id 'T22' -Name '选集按钮做大（高≥30）' -Pass ($epRect -and $epRect.Height -ge 30) -Evidence "epBtnH=$($epRect.Height)"
# 点播空格暂停。
if ($epBtn) { Invoke-Element $epBtn | Out-Null; Start-Sleep -Seconds 4 }
$q1 = Get-PlayerProgress
Start-Sleep -Seconds 3
$q2 = Get-PlayerProgress
Add-Result -Id 'T29a' -Name '点播能播（进度前进）' -Pass ($null -ne $q1 -and $null -ne $q2 -and $q2 -gt $q1) -Evidence "q1=$q1 q2=$q2"
Send-Key ' '
Start-Sleep -Seconds 3
$pausedText2 = Find-Element -Root $script:Win -Name '已暂停'
$q3 = Get-PlayerProgress
Start-Sleep -Seconds 3
$q4 = Get-PlayerProgress
Add-Result -Id 'T29' -Name '点播空格暂停' -Pass (($null -ne $pausedText2) -and ($null -ne $q3 -and $null -ne $q4 -and ($q4 - $q3) -lt 1.5)) -Evidence "pausedText=$($null -ne $pausedText2) q3=$q3 q4=$q4"
Send-Key ' '
Start-Sleep -Seconds 2
Shot -Tag 'detail-playing' | Out-Null

# ---------- 小城良方：无选集回归 + 类似内容 ----------
Go-Mode 'search' | Out-Null
Start-Sleep -Seconds 1
$searchBox = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $searchBox = $e; break } } catch { }
}
if ($searchBox) {
  try { $searchBox.GetCurrentPattern($script:ValuePattern).SetValue('小城良方') } catch { }
  Send-Key '{ENTER}'
  Start-Sleep -Seconds 5
  $hit = Wait-For { Find-Element -Root $script:Win -Name '小城良方' } -Seconds 20
  if ($hit) { Invoke-Element $hit | Out-Null; Start-Sleep -Seconds 4 }
}
$epBtns = @()
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -match '^第.*集$') { $epBtns += $e } } catch { }
}
Add-Result -Id 'T23' -Name '小城良方详情有选集按钮' -Pass ($epBtns.Count -ge 1) -Evidence "episodeButtons=$($epBtns.Count)"
Shot -Tag 'detail-xiaocheng' | Out-Null
# 类似内容：不同片名。
$similarTitle = Find-Element -Root $script:Win -Name '更多类似内容'
$similarCards = @()
if ($similarTitle) {
  Start-Sleep -Seconds 5
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  # 类似区卡片：取“更多类似内容”标题之后、窗口底部之前的卡片标题（去重后数不同片名）。
  $inSimilar = $false
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      if ($n -eq '更多类似内容') { $inSimilar = $true; continue }
      if ($inSimilar -and $e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $n -and $n -ne '小城良方') { $similarCards += $n }
    } catch { }
  }
}
$distinct = ($similarCards | Sort-Object -Unique)
$sameTitle = ($similarCards | Where-Object { $_ -like '*小城良方*' }).Count
Add-Result -Id 'T24' -Name '更多类似内容是不同片名' -Pass ($distinct.Count -ge 3 -and $sameTitle -eq 0) -Evidence "cards=$($similarCards.Count) distinct=$($distinct.Count) sameTitle=$sameTitle distinctList='$($distinct -join ' | ')'"
# 类似卡片宽度≤140。
$cardW = $null
if ($similarTitle) {
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -and $e.Current.Name -ne '小城良方' -and $e.Current.BoundingRectangle.Width -ge 80 -and $e.Current.BoundingRectangle.Width -le 200 -and $e.Current.BoundingRectangle.Y -gt 500) { $cardW = $e.Current.BoundingRectangle.Width; break }
    } catch { }
  }
}
Add-Result -Id 'T25' -Name '类似内容卡片收窄（宽≤140）' -Pass ($null -ne $cardW -and $cardW -le 140) -Evidence "cardW=$cardW"

# ---------- 报错 toast 5 秒自消 ----------
Go-Mode 'settings' | Out-Null
Start-Sleep -Seconds 1
$vodUrlBox = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit' -and $e.Current.Name -match '点播源') { $vodUrlBox = $e; break }
  } catch { }
}
if (-not $vodUrlBox) {
  # 兜底：取设置页第一个输入框（点播源地址栏）。
  foreach ($e in $all) {
    try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $vodUrlBox = $e; break } } catch { }
  }
}
$toastPass = $false
$toastEv = 'no-input-box'
if ($vodUrlBox) {
  try { $vodUrlBox.GetCurrentPattern($script:ValuePattern).SetValue('https://127.0.0.1:9/vod.json') } catch { }
  $importBtn = Find-Element -Root $script:Win -Name '导入'
  if ($importBtn) { Invoke-Element $importBtn | Out-Null }
  $errBox = Wait-For { Find-Element -Root $script:Win -Name '127.0.0.1' } -Seconds 15
  if ($errBox) {
    $t0 = Get-Date
    $gone = Wait-For { $null -eq (Find-Element -Root $script:Win -Name '127.0.0.1') } -Seconds 8
    $dt = ((Get-Date) - $t0).TotalSeconds
    $toastPass = ($null -ne $gone)
    $toastEv = "shownThenGone=$($null -ne $gone) dismissSeconds=$([Math]::Round($dt, 1)) (expect ~5)"
  } else {
    $toastEv = 'error-box-never-appeared'
  }
}
Add-Result -Id 'T26' -Name '报错提示 5 秒自动消失' -Pass $toastPass -Evidence $toastEv
Shot -Tag 'settings' | Out-Null

# ---------- 设置：CF 下拉自动检测新源 + 字幕说明 ----------
$cfCombo = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.ComboBox') { $cfCombo = $e; break }
  } catch { }
}
$cfEv = 'no-combobox'
$cfPass = $false
if ($cfCombo) {
  Expand-Element $cfCombo | Out-Null
  Start-Sleep -Seconds 2
  $opts = @()
  $all2 = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all2) {
    try {
      $n = $e.Current.Name
      if ($n -match '聚合万能源|OrionTV|亚洲点播|正在用的源') { $opts += $n }
    } catch { }
  }
  $opts = $opts | Sort-Object -Unique
  $has3 = (($opts | Where-Object { $_ -match '聚合万能源' }).Count -ge 1) -and (($opts | Where-Object { $_ -match 'OrionTV' }).Count -ge 1) -and (($opts | Where-Object { $_ -match '亚洲点播' }).Count -ge 1)
  $cfPass = $has3
  $cfEv = "options='$($opts -join ' | ')'"
  Send-Key '{ESC}'
}
Add-Result -Id 'T28' -Name 'CF 下拉自动列出 3 个目录源' -Pass $cfPass -Evidence $cfEv
$subHint = Find-Element -Root $script:Win -Name '字幕翻译'
Add-Result -Id 'T27' -Name '设置页字幕说明完整' -Pass ($null -ne $subHint) -Evidence "subtitleSection=$($null -ne $subHint)"
Shot -Tag 'settings-cf' | Out-Null

# ---------- 隐藏 bug 探查：反复切换 + 异常路径 ----------
$switchOk = $true
$switchEv = ''
Go-Mode 'live' | Out-Null
Start-Sleep -Seconds 2
for ($i = 0; $i -lt 10; $i++) {
  $rows = @()
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try { if ($e.Current.Name -match '^CCTV') { $rows += $e } } catch { }
  }
  if ($rows.Count -ge 2) {
    Invoke-Element $rows[$i % $rows.Count] | Out-Null
    Start-Sleep -Milliseconds 700
  }
  if (-not (Test-ProcessAlive -Id $script:Pid)) { $switchOk = $false; $switchEv = "died at switch $i"; break }
}
if ($switchOk) { $switchEv = '10 switches, process alive' }
Add-Result -Id 'T31' -Name '反复切台 10 次不崩' -Pass $switchOk -Evidence $switchEv
# 空搜索：输一个不存在的片名，应显示“暂无搜索结果”而不是卡死/报错。
Go-Mode 'search' | Out-Null
Start-Sleep -Seconds 1
$searchBox = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $searchBox = $e; break } } catch { }
}
if ($searchBox) {
  try { $searchBox.GetCurrentPattern($script:ValuePattern).SetValue('不存在的片名zzzz1234') } catch { }
  Send-Key '{ENTER}'
  $noRes = Wait-For { Find-Element -Root $script:Win -Name '暂无搜索结果' } -Seconds 20
  Add-Result -Id 'T32' -Name '搜不到时显示暂无结果（不卡死）' -Pass ($null -ne $noRes) -Evidence "noResultsShown=$($null -ne $noRes)"
}
Add-Result -Id 'T33' -Name '全程无悄悄退出' -Pass (Test-ProcessAlive -Id $script:Pid) -Evidence "pid=$($script:Pid) alive=$(Test-ProcessAlive -Id $script:Pid)"

# ---------- 长时间播放（soak；只在 -SoakMinutes > 0 时跑） ----------
if ($SoakMinutes -gt 0) {
  Go-Mode 'live' | Out-Null
  Start-Sleep -Seconds 2
  $firstRow = Find-Element -Root $script:Win -Name 'CCTV-1'
  if (-not $firstRow) { $firstRow = Find-Element -Root $script:Win -Name 'CCTV' }
  if ($firstRow) { Invoke-Element $firstRow | Out-Null }
  Start-Sleep -Seconds 5
  $soakLog = Join-Path $OutDir 'soak.log'
  "soak start $(Get-Date -Format 'HH:mm:ss') pid=$($script:Pid)" | Out-File $soakLog
  $lastProgress = Get-PlayerProgress
  $stuckSamples = 0
  $froze = $false
  $recovered = $false
  $soakPass = $true
  for ($m = 1; $m -le $SoakMinutes; $m++) {
    Start-Sleep -Seconds 60
    if (-not (Test-ProcessAlive -Id $script:Pid)) {
      "MIN ${m}: PROCESS GONE (silent exit)" | Out-File $soakLog -Append
      $soakPass = $false; $froze = $true; break
    }
    $p = Get-PlayerProgress
    $reconnecting = $null -ne (Find-Element -Root $script:Win -Name '正在重连')
    "MIN $m $(Get-Date -Format 'HH:mm:ss') progress=$p reconnecting=$reconnecting" | Out-File $soakLog -Append
    if ($null -ne $p -and $null -ne $lastProgress -and $p -eq $lastProgress) { $stuckSamples++ } else { $stuckSamples = 0 }
    if ($stuckSamples -ge 2 -and -not $froze) {
      $froze = $true
      "MIN ${m}: FROZEN at $p, waiting 60s for auto-recovery" | Out-File $soakLog -Append
      Shot -Tag ("soak-frozen-min$m") | Out-Null
      Start-Sleep -Seconds 60
      $p2 = Get-PlayerProgress
      $reconnecting2 = $null -ne (Find-Element -Root $script:Win -Name '正在重连')
      "MIN ${m}: after 60s progress=$p2 reconnecting=$reconnecting2" | Out-File $soakLog -Append
      if ($null -ne $p2 -and $p2 -gt $p) { $recovered = $true; $stuckSamples = 0; $froze = $false }
      else { $soakPass = $false }
    }
    $lastProgress = $p
  }
  $ev = "minutes=$SoakMinutes froze=$froze recovered=$recovered processAlive=$(Test-ProcessAlive -Id $script:Pid)"
  Add-Result -Id 'T34' -Name "连续播放 ${SoakMinutes} 分钟不冻住（冻住 60 秒内自动恢复也算过）" -Pass $soakPass -Evidence $ev
  Shot -Tag 'soak-end' | Out-Null
}

# ---------- 报告 ----------
$passed = ($script:Results | Where-Object { $_.pass }).Count
$total = $script:Results.Count
$summary = @{
  exe = $ExePath
  pid = $script:Pid
  time = (Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
  soakMinutes = $SoakMinutes
  passed = $passed
  total = $total
  allPass = ($passed -eq $total)
  results = $script:Results
}
$summary | ConvertTo-Json -Depth 5 | Out-File (Join-Path $OutDir 'report.json')
$md = @()
$md += "# UnBox 端到端测试报告（$($summary.time)）"
$md += ""
$md += "- 测试包：$ExePath"
$md += "- 进程 pid：$($script:Pid)，soak：${SoakMinutes} 分钟"
$md += "- 结果：**$passed / $total 通过**"
$md += ""
$md += "| 编号 | 检查项 | 结果 | 证据 |"
$md += "|---|---|---|---|"
foreach ($r in $script:Results) {
  $md += "| $($r.id) | $($r.name) | $(if ($r.pass) { '通过' } else { '不通过' }) | $($r.evidence) |"
}
$md -join "`n" | Out-File (Join-Path $OutDir 'report.md')
Write-Output ("E2E DONE: $passed/$total passed")
if ($passed -ne $total) { exit 1 }
exit 0
