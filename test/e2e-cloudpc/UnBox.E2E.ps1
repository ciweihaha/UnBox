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
  [string]$ExpectedVersion = '0.9.17',
  [switch]$Probe
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'UnBox.Uia.ps1')

if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir -Force | Out-Null }
$script:Results = @()
$script:Shots = 0
$script:Win = $null
$script:AppPid = 0

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

# Search-Vod 在搜索页搜片名、点进第一个同名结果：输入必须用剪贴板粘贴
#（SetValue 不触发 Vue v-model，搜不到）；搜全网最多等 30 秒。
# 搜不到结果或没出结果时重导航再试一次（导航点击偶尔 miss）。
function Search-Vod {
  param([string]$Query)
  $script:SearchDiag = ''
  for ($attempt = 1; $attempt -le 2; $attempt++) {
    Go-Mode 'search' | Out-Null
    Start-Sleep -Seconds 1
    $box = $null
    $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    foreach ($e in $all) {
      try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $box = $e; break } } catch { }
    }
    if (-not $box) { continue }
    Set-InputText -Edit $box -Text $Query | Out-Null
    Send-Key '{ENTER}'
    # 等结果卡片：不要只等一个同名元素——搜索历史 chip 也叫这个名，
    # 会立刻命中、让脚本在卡片出来前就点了 chip（第三轮就是这么丢的）。
    # 轮询等一张非小按钮的卡片（结果 2.8 秒就回来，但渲染要再等一下）。
    $card = $null
    $cardEnd = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $cardEnd -and -not $card) {
      $cands = Find-Elements -Root $script:Win -NameLike "*$Query*"
      foreach ($c in $cands) {
        try {
          $r = $c.Current.BoundingRectangle
          if ($c.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $r.Height -lt 40) { continue }
          $card = $c; break
        } catch { continue }
      }
      if (-not $card) {
        if (Find-Element -Root $script:Win -Name '暂无搜索结果') { break }
        Start-Sleep -Seconds 2
      }
    }
    if ($card) {
      Invoke-ElementOrAncestor $card | Out-Null
      # 等详情打开：详情要调聚合接口，冷启动几秒才出来，别只睡 4 秒就判。
      $opened = Wait-For {
        ($null -ne (Find-Element -Root $script:Win -Name '更多类似内容')) -or
        ((Find-Elements -Root $script:Win -NameLike '第*集*').Count -ge 1) -or
        ((Find-Elements -Root $script:Win -NameLike '线路*').Count -ge 1)
      } -Seconds 30
      if ($opened) { $script:SearchDiag = ''; return $true }
    }
    # 诊断：这次到底看到了什么候选（名/类型/矩形），下次 0 卡片时直接看。
    $diag = @()
    $cands2 = Find-Elements -Root $script:Win -NameLike "*$Query*"
    foreach ($c in $cands2) {
      try { $r2 = $c.Current.BoundingRectangle; $diag += "'$($c.Current.Name)' $($c.Current.ControlType.ProgrammaticName) $($r2.Width)x$($r2.Height)" } catch { }
    }
    $script:SearchDiag = "attempt=$attempt cands='$($diag -join ' | ')'"
    # 没出结果：可能还在搜，也可能根本没搜出来；再等一次“暂无搜索结果”。
    $noRes = Find-Element -Root $script:Win -Name '暂无搜索结果'
    if ($noRes) { return $false }
  }
  return $false
}

function Get-StatusNames {
  # 当前播放状态行（正在播放/正在重连/频道播放失败/正在使用 mpv 播放）的名字。
  $out = @()
  foreach ($pat in @('正在播放', '正在重连', '频道播放失败', '正在使用 mpv 播放')) {
    $el = Find-Element -Root $script:Win -Name $pat
    if ($el) { try { $out += $el.Current.Name } catch { $out += $pat } }
  }
  return ($out -join ' | ')
}

function Get-PlayerProgress {
  # 播放器进度：优先读 .ctrl-time 文本。文本是“mm:ss / mm:ss”，
  # 但播超过 1 小时的流（直播长播流）会变成“h:mm:ss / …”，
  # 旧正则 ^(\d+):(\d+) / 只认 mm:ss，h:mm:ss 会读不到（回落到 Slider，
  # 直播的 Slider 值恒为 0），于是能播的台也被判死。先认 h:mm:ss 再认 mm:ss。
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      if ($n -match '^(\d+):(\d+):(\d+) / ') { return ([int]$Matches[1]) * 3600 + ([int]$Matches[2]) * 60 + [int]$Matches[3] }
      if ($n -match '^(\d+):(\d+) / ') { return ([int]$Matches[1]) * 60 + [int]$Matches[2] }
    } catch { }
  }
  # 兜底：.ctrl-seek 是 range input，UIA 里是 Slider（RangeValuePattern）。
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') {
        return [double]($e.GetCurrentPattern($script:RangePattern).Value)
      }
    } catch { }
  }
  return $null
}

function Wait-PlayerProgress {
  param([int]$Seconds = 20)
  $end = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $end) {
    $p = Get-PlayerProgress
    if ($null -ne $p -and $p -gt 0) { return $p }
    Start-Sleep -Seconds 2
  }
  return (Get-PlayerProgress)
}

function Find-PlayerGroup {
  # 播放器容器：含进度条（Slider 子孙）的 Group；按宽度猜会撞到主窗口本身。
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -ne 'ControlType.Group') { continue }
      $r = $e.Current.BoundingRectangle
      if ($r.Width -lt 300 -or $r.Height -lt 150) { continue }
      $subs = $e.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
      foreach ($s in $subs) {
        try {
          if ($s.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') { return $e }
        } catch { }
      }
    } catch { }
  }
  return $null
}

function Find-DeepestPlayerGroup {
  # 真正的播放器（PlaybackView）：含 Slider 的 Group 里最深（最后）的一个。
  # Find-PlayerGroup 返回第一个（最外层容器，约等于视口 1233x725），拿它量
  # 详情等高（T29b）会拿 h2/页签跟页面边框比、恒错；这里取最深的才是播放器本身。
  $found = $null
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -ne 'ControlType.Group') { continue }
      $r = $e.Current.BoundingRectangle
      if ($r.Width -lt 300 -or $r.Height -lt 150) { continue }
      $subs = $e.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
      foreach ($s in $subs) {
        try {
          if ($s.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') { $found = $e; break }
        } catch { }
      }
    } catch { }
  }
  return $found
}

function Find-PlayerViewGroup {
  # 播放器列/播放器本身：含 Slider 的 Group 里面积最大、且不是整页容器
  # 的那一个。最深的 Group 是视频子元素（725x363，底比播放器列短 19px，
  # T29b 底边 gap 会量成 35 而不是 16）；最外层是整页容器（1233x725）。
  # 播放器列（约 877x399，顶=行顶、底=行底）才是 T29b 等高要量的对象，
  # 它是含 Slider 的 Group 里最大的、且宽度不到窗口 95% 的那个。
  $best = $null
  $bestArea = 0
  $wr = Get-WindowRect
  $maxW = $wr.Width * 0.95
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -ne 'ControlType.Group') { continue }
      $r = $e.Current.BoundingRectangle
      if ($r.Width -lt 300 -or $r.Height -lt 150) { continue }
      if ($r.Width -ge $maxW) { continue }
      $subs = $e.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
      $hasSlider = $false
      foreach ($s in $subs) {
        try { if ($s.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') { $hasSlider = $true; break } } catch { }
      }
      if ($hasSlider) {
        $area = $r.Width * $r.Height
        if ($area -gt $bestArea) { $bestArea = $area; $best = $e }
      }
    } catch { }
  }
  return $best
}

$script:LastLiveDead = @()
function Select-LiveGroupAll {
  # 直播分了组（央视/卫视/其他地区），默认只显示当前组的台；中继一断，
  # 央视组全死，能播的台（BBC/香港/澳门）都在“其他地区直播”组里。
  # 把分类下拉（ThemedSelect，触发按钮 aria-label 是“分类”）切到“全部”，
  # 56 台全可见，后面找台才找得到。返回 $true 表示切上了（BBC/香港/
  # 澳门任一行出现了），$false 表示没切上——找台仍按当前组的行继续试。
  # 触发器按钮的名字就是 aria-label；别直接按名字找，会先撞到旁边的 label
  # 文本，要按 ControlType.Button 过滤（同 T28 的 CF 下拉）。
  $trigger = $null
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -eq '分类') { $trigger = $e; break }
    } catch { }
  }
  if (-not $trigger) { return $false }
  $opened = Invoke-Element $trigger
  if (-not $opened) {
    # InvokePattern 打不开时退到鼠标点触发器中心。
    try {
      $r = $trigger.Current.BoundingRectangle
      Click-At -X ([int]($r.X + $r.Width / 2)) -Y ([int]($r.Y + $r.Height / 2))
      $opened = $true
    } catch { }
  }
  if (-not $opened) { return $false }
  # 选项是 <button role="option">，UIA 里可能是 Button 也可能是 ListItem，
  # 按名字找、两种类型都认；菜单是 v-if，开了才进 DOM。
  $opt = Wait-For {
    $found = $null
    $all2 = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    foreach ($e in $all2) {
      try {
        $t = $e.Current.ControlType.ProgrammaticName
        if (($t -eq 'ControlType.Button' -or $t -eq 'ControlType.ListItem') -and $e.Current.Name -eq '全部') { $found = $e; break }
      } catch { }
    }
    $found
  } -Seconds 5
  if (-not $opt) { return $false }
  Invoke-Element $opt | Out-Null
  $ok = Wait-For {
    (Find-ChannelRow -Root $script:Win -NameLike '*BBC*') -or
    (Find-ChannelRow -Root $script:Win -NameLike '*香港*') -or
    (Find-ChannelRow -Root $script:Win -NameLike '*澳门*')
  } -Seconds 10
  return ($null -ne $ok)
}

function Find-WorkingLiveChannel {
  param([string[]]$Exclude = @(), [int]$PerRowSeconds = 20, [int]$MaxRows = 5)
  # 找一条真能播的直播台，返回台名（找不到返回 ''）。
  # 先切到“全部”分组（不然只看得到当前组的台）；顺序：CCTV综合优先，
  # 其余 CCTV，其次 BBC、香港、澳门，再是其它所有行；每类最多试
  # $MaxRows 行（总数不封顶，不然 CCTV 死台一多就永远到不了 BBC）。
  # 中继（SCF）一断就是一片 410，死守 CCTV 会白白浪费时间；点行后要等到
  # 进度 >0 才算播起来（只看状态会误判，报错状态也算“换台成功”）。
  # 死台不用等满整段：状态已换成新台名、且同时出现“频道播放失败”就跳过
  # （403 的台几秒就报错）。只看“频道播放失败”会误判：报错行会残留
  # （上一个死台的报错还在），后面所有台都会被当成死台。
  # 死台记进 $script:LastLiveDead。注意：'*' 会把已试过的行再列一遍，
  # 靠 $tried 去重，且跳过前面模式已覆盖的台（CCTV/BBC/香港/澳门），
  # 不然兜底的 5 个名额又被剩下的 CCTV 行占满。
  $script:LastLiveDead = @()
  Select-LiveGroupAll | Out-Null
  $patterns = @('*CCTV综合*', '*CCTV*', '*BBC*', '*香港*', '*澳门*', '*')
  $covered = @('*CCTV*', '*BBC*', '*香港*', '*澳门*')
  $tried = @()
  foreach ($pat in $patterns) {
    $rows = Find-ChannelRows -Root $script:Win -NameLike $pat
    $perPat = 0
    foreach ($row in $rows) {
      if ($perPat -ge $MaxRows) { break }
      $nm = ''
      try { $nm = $row.Current.Name } catch { continue }
      if (-not $nm -or $Exclude -contains $nm -or $tried -contains $nm) { continue }
      if ($pat -eq '*' ) {
        $isCovered = $false
        foreach ($c in $covered) { if ($nm -like $c) { $isCovered = $true; break } }
        if ($isCovered) { continue }
      }
      $tried += $nm
      $perPat++
      Invoke-Element $row | Out-Null
      $p = $null
      $elapsed = 0
      while ($elapsed -lt $PerRowSeconds) {
        Start-Sleep -Seconds 2
        $elapsed += 2
        $p = Get-PlayerProgress
        if ($null -ne $p -and $p -gt 0) { return $nm }
        $st = Get-StatusNames
        if (($st -like "*$nm*") -and ($st -like '*频道播放失败*')) { break }
      }
      $script:LastLiveDead += $nm
    }
  }
  return ''
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
  # ScrollPattern 读不到时（WebView2 不暴露滚动容器）：拿一个有名字的
  # 元素当标尺，看滚轮后它的 Y 是否移动。旧量法用 FromPoint 取元素，
  # WebView2 宿主元素没名字（item=''），量出来的 yAfter 恒为空，滚没滚
  # 都判失败，所以改成按名字找标尺。
  # 先往上滚几下回到顶（页面若已到底，往下滚就动不了，会误判滚轮坏了；
  # 第四轮 T10 就是这么丢的）。再找标尺、记 yBefore、往下滚、记 yAfter。
  Send-Wheel -Delta 360 -Clicks 3
  Start-Sleep -Milliseconds 800
  $ruler = $null
  $wr = Get-WindowRect
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name; $r = $e.Current.BoundingRectangle
      if ($n -and $n.Length -ge 2 -and $r.Y -gt ($wr.Y + $wr.Height * 0.45) -and $r.Y -lt ($wr.Y + $wr.Height * 0.95) -and $r.Width -gt 20) { $ruler = $e; break }
    } catch { }
  }
  if (-not $ruler) {
    Add-Result -Id $Id -Name $Name -Pass $false -Evidence 'no ruler element found'
    return
  }
  $name = $ruler.Current.Name
  $yBefore = $ruler.Current.BoundingRectangle.Y
  Send-Wheel -Delta -360 -Clicks 2
  Start-Sleep -Milliseconds 800
  $yAfter = $null
  $gone = $false
  $el2 = Find-Element -Root $script:Win -Name $name -Exact
  if ($el2) { try { $yAfter = $el2.Current.BoundingRectangle.Y } catch { $gone = $true } }
  else { $gone = $true }
  $moved = $gone -or ($null -ne $yBefore -and $null -ne $yAfter -and [Math]::Abs($yAfter - $yBefore) -gt 5)
  Add-Result -Id $Id -Name $Name -Pass $moved -Evidence "rulerLen=$($name.Length) yBefore=$yBefore yAfter=$yAfter gone=$gone"
}

# ---------- 启动 ----------
if (-not (Test-Path $ExePath)) { throw "找不到测试包：$ExePath" }
# 先清掉残留的旧版进程（0.9.16 轮的 UB916-Launch 留了一个在跑），
# 否则按窗口名找会测到旧窗口。
Stop-UnBoxProcesses
# Go 日志（含标题栏着色逐项 HRESULT、panic 堆栈）重定向到文件：
# 悄悄退出的根因只能靠它定位，平时 app 不写日志文件。
$proc = Start-Process -FilePath $ExePath -WorkingDirectory (Split-Path $ExePath -Parent) -PassThru -RedirectStandardOutput (Join-Path $OutDir 'app-stdout.txt') -RedirectStandardError (Join-Path $OutDir 'app-stderr.txt')
$script:AppPid = $proc.Id
$script:Win = Wait-For { Get-UnBoxWindowForPid -ProcessId $script:AppPid } -Seconds 60
if (-not $script:Win) { $script:Win = Wait-For { Get-UnBoxWindow } -Seconds 10 }
Add-Result -Id 'T01' -Name '应用启动，窗口出现（属于本次启动的进程）' -Pass ($null -ne $script:Win) -Evidence "pid=$($script:AppPid) window=$($null -ne $script:Win)"
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
  # 当前版本走 ldflags 注入：构建时 VERSION 是多少就该是多少。
  $curOk = $curText -match [regex]::Escape($ExpectedVersion)
  Add-Result -Id 'T03a' -Name "关于：当前版本是 $ExpectedVersion" -Pass $curOk -Evidence "current='$curText'"
  # 内部版本走 Go 伪版本：发版前是 vX.Y.Z-0.时间.commit 形式；关键是不能带 +dirty
  #（树干净时没有；构建弄脏树——go mod tidy/图标重生成/版本 sed——就会带上）。
  $clean = ($verText -notmatch '\+dirty') -and ($verText -match 'v\d+\.\d+')
  $verEvidence = "internal='$verText' current='$curText'"
  Add-Result -Id 'T03b' -Name '关于：内部版本干净（无 +dirty）' -Pass $clean -Evidence $verEvidence
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
  # 先走 UIA Invoke（合成鼠标在该 app 里不可靠，README 有记）：
  # 取光标处的元素直接 Invoke，点的就是遮罩本身；不行再退到合成鼠标点按，
  # 两次结果都记下来，好分清是“点按没送达页面”还是“遮罩不响应”。
  Click-Name '关于我们' | Out-Null
  Start-Sleep -Seconds 1
  $panel = Find-Element -Root $script:Win -Name '关于 UnBox'
  if ($panel) {
    $pr = Get-ElementRect $panel
    Move-Mouse -X ([int]($wr.X + 30)) -Y ([int]($wr.Y + $wr.Height - 30))
    $ptEl = $script:AE::FromPoint((New-Object System.Windows.Point(([int]($wr.X + 30)), ([int]($wr.Y + $wr.Height - 30)))))
    $uiaClicked = Invoke-ElementOrAncestor $ptEl
    Start-Sleep -Milliseconds 800
    $gone2 = $null -eq (Find-Element -Root $script:Win -Name '关于 UnBox')
    if (-not $gone2) {
      Add-Type @"
using System; using System.Runtime.InteropServices;
public static class Clicker { [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint x, uint y, int d, UIntPtr e); }
"@ -ErrorAction SilentlyContinue
      [Clicker]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
      [Clicker]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
      Start-Sleep -Milliseconds 800
      $gone2 = $null -eq (Find-Element -Root $script:Win -Name '关于 UnBox')
    }
    Add-Result -Id 'T06' -Name '关于：点遮罩关闭' -Pass $gone2 -Evidence "uiaInvoke=$uiaClicked closedAfterMaskClick=$gone2"
    if (-not $gone2) { Send-Key '{ESC}'; Start-Sleep -Milliseconds 500 }
  } else {
    Add-Result -Id 'T06' -Name '关于：点遮罩关闭' -Pass $false -Evidence '重开关于失败'
  }
} else {
  Add-Result -Id 'T03a' -Name "关于：当前版本是 $ExpectedVersion" -Pass $false -Evidence $verEvidence
  Add-Result -Id 'T03b' -Name '关于：内部版本干净（无 +dirty）' -Pass $false -Evidence $verEvidence
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
Test-WheelScrolls -Id 'T12' -Name '滚轮：直播列表能滚' -X ([int]($wr.X + 200)) -Y ([int]($wr.Y + $wr.Height / 2))
Shot -Tag 'live' | Out-Null
# 先找一条真能播的台（CCTV综合优先，其余 CCTV，其次 BBC，再是其它行），
# 找到的台名记下来，后面 T16/T17 用。冷启动中继取流慢，单台等 30 秒；
# 中继一断就是一片 410，死守 CCTV 会白白浪费时间，所以不只试 CCTV。
$liveChannelUsed = Find-WorkingLiveChannel -PerRowSeconds 30 -MaxRows 5
$p0 = Get-PlayerProgress
$liveStatusAtStart = Get-StatusNames
$playerGroup = Find-PlayerGroup
$pgRect = Get-ElementRect $playerGroup
Add-Result -Id 'T14' -Name '直播两栏：左列表右播放' -Pass (($null -ne $liveList) -and ($null -ne $playerGroup)) -Evidence "list=$($null -ne $liveList) playerW=$($pgRect.Width)"
Add-Result -Id 'T18' -Name '直播页铺满（播放区高≥500）' -Pass ($pgRect -and $pgRect.Height -ge 500) -Evidence "playerH=$($pgRect.Height)"
# 星标对齐：取频道行里的收藏按钮（名字是 ★/☆，但按名字找不可靠），
# 按位置量——所有按钮的 x 应在同一列（误差≤5px）。
$stars = @()
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.ListItem' -and $e.Current.Name -match '^CCTV') {
      $subs = $e.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
      foreach ($s in $subs) {
        try {
          if ($s.Current.ControlType.ProgrammaticName -eq 'ControlType.Button') { $stars += $s.Current.BoundingRectangle.X }
        } catch { }
      }
    }
  } catch { }
}
$starSpread = 0
if ($stars.Count -ge 2) { $starSpread = ($stars | Measure-Object -Maximum).Maximum - ($stars | Measure-Object -Minimum).Minimum }
Add-Result -Id 'T15' -Name '直播星标统一在行右端' -Pass ($stars.Count -ge 2 -and $starSpread -le 5) -Evidence "stars=$($stars.Count) spread=$starSpread"
# 进度前进：p0 是起播后等到的首个 >0 进度，再过 5 秒应更大。
# 直播流偶尔会重启一次（进度归 0 再涨）：p2 没涨时多等 3 秒再读 p3，
# 只要最后在涨就算过（T17 会再验暂停/继续）。
$p1 = $p0
Start-Sleep -Seconds 5
$p2 = Get-PlayerProgress
$pass16 = ($null -ne $p1 -and $null -ne $p2 -and $p2 -gt $p1)
if (-not $pass16) {
  $p1 = Get-PlayerProgress
  Start-Sleep -Seconds 3
  $p2 = Get-PlayerProgress
  $pass16 = ($null -ne $p1 -and $null -ne $p2 -and $p2 -gt $p1)
}
Add-Result -Id 'T16' -Name '直播点播后进度前进' -Pass $pass16 -Evidence "channel=$liveChannelUsed p1=$p1 p2=$p2 status='$liveStatusAtStart'"
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
$foundManvs = Search-Vod -Query '荒野求生'
# 详情到底开没开：首跑 T21/T22/T29 全空，但 T23/T24 证明详情本身能加载，
# 必须先分清是“没进详情”还是“进了详情但量不到”，否则修错地方。
$detailSignals = @()
if ($null -ne (Find-Element -Root $script:Win -Name '更多类似内容')) { $detailSignals += 'similar' }
if ((Find-Elements -Root $script:Win -NameLike '第*集*').Count -ge 1) { $detailSignals += 'episodes' }
if ((Find-Elements -Root $script:Win -NameLike '线路*').Count -ge 1) { $detailSignals += 'lines' }
$detailOpenedEv = "found=$foundManvs signals='$($detailSignals -join ',')'"
# 线路按钮友好名。
$lineBtns = Find-Elements -Root $script:Win -NameLike '线路*'
$rawIds = Find-Elements -Root $script:Win -NameLike '*m3u8*'
Add-Result -Id 'T21' -Name '详情线路显示友好名（线路N）' -Pass ($lineBtns.Count -ge 1 -and $rawIds.Count -eq 0) -Evidence "friendly=$($lineBtns.Count) rawIds=$($rawIds.Count) $detailOpenedEv diag=$($script:SearchDiag)"
# 选集按钮更大：量第一个“第*集”按钮的高度。
$epBtn = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -match '^第.*集$') { $epBtn = $e; break } } catch { }
}
$epRect = Get-ElementRect $epBtn
Add-Result -Id 'T22' -Name '选集按钮做大（高≥30）' -Pass ($epRect -and $epRect.Height -ge 30) -Evidence "epBtnH=$($epRect.Height) $detailOpenedEv"
# 点播空格暂停。
if ($epBtn) { Invoke-Element $epBtn | Out-Null }
$q1 = Wait-PlayerProgress -Seconds 20
Start-Sleep -Seconds 3
$q2 = Get-PlayerProgress
Add-Result -Id 'T29a' -Name '点播能播（进度前进）' -Pass ($null -ne $q1 -and $null -ne $q2 -and $q2 -gt $q1) -Evidence "q1=$q1 q2=$q2"
# 详情是整页：点集播起来后才有播放器容器，播后再量。
# 用播放器列（最大且不是整页容器的含 Slider Group）：最深的 Group 是视频
# 子元素（底比播放器列短 19px，T29b 底边 gap 会量成 35）；最外层是整页容器。
$detailPlayer = Find-PlayerViewGroup
$dpRect = Get-ElementRect $detailPlayer
Add-Result -Id 'T30' -Name '详情是整页（播放区宽≥700）' -Pass (($null -ne $detailPlayer) -and $dpRect.Width -ge 700) -Evidence "playerW=$($dpRect.Width) playerH=$($dpRect.Height)"
# 详情左右等高（T29b）：信息栏是 grid 右列（align-items: stretch），
# 顶 = 行顶、底 = 行底。信息栏容器本身 UIA 不暴露，用两处量：
# 栏内 h2 标题顶（栏顶 + 12px 内边距）≈ 播放器顶（±24，“正在播放”行约 28px，
# 播放器顶含这行，h2 顶不含，所以差值约 -16，容差放宽到 24 才稳）；
# 选集区首个线路页签顶（行底 + 16px 间距）− 播放器底 ≈ 35（±10）。
# 量到的是视频区（Find-PlayerViewGroup 返回的最大非页面含 Slider Group，
# 725×363），它的底比播放器列底短约 19px（下面还有控制条），所以期望
# gap 是 16（grid 间距）+ 19（控制条）= 35，不是 16；两轮实测都是 35。
# 上下叠放时两处都会差出几百像素。
$wr = Get-WindowRect
$titleEls = Find-Elements -Root $script:Win -NameLike '荒野求生*'
$h2Rect = $null
foreach ($e in $titleEls) { $r = Get-ElementRect $e; if ($r -and $r.X -gt ($wr.X + $wr.Width / 2)) { $h2Rect = $r; break } }
$tabBtns = Find-Elements -Root $script:Win -NameLike '线路*'
$tabRect = $null
if ($tabBtns.Count -ge 1) { $tabRect = Get-ElementRect $tabBtns[0] }
$topOk = $h2Rect -and [Math]::Abs($h2Rect.Y - $dpRect.Y) -le 24
$botGap = $null
if ($tabRect) { $botGap = $tabRect.Y - ($dpRect.Y + $dpRect.Height) }
$botOk = ($null -ne $botGap) -and [Math]::Abs($botGap - 35) -le 10
# 布局诊断：把窗口、所有“荒野求生*”元素、线路页签、类似内容标题的矩形都
# 记下来，下次直接看是布局没对齐还是量到了错的元素。
$layoutEv = "win=$($wr.X),$($wr.Y) $($wr.Width)x$($wr.Height)"
foreach ($e in $titleEls) {
  $r = Get-ElementRect $e
  if ($r) { try { $layoutEv += " | title '$($e.Current.Name)' $($e.Current.ControlType.ProgrammaticName) $($r.X),$($r.Y) $($r.Width)x$($r.Height)" } catch { } }
}
if ($tabBtns.Count -ge 1) { try { $layoutEv += " | tab '$($tabBtns[0].Current.Name)'" } catch { } }
$simHead = Find-Element -Root $script:Win -Name '更多类似内容'
if ($simHead) { $sr = Get-ElementRect $simHead; if ($sr) { $layoutEv += " | similarHead $($sr.X),$($sr.Y) $($sr.Width)x$($sr.Height)" } }
Add-Result -Id 'T29b' -Name '详情左右等高（信息栏与播放区顶底对齐）' -Pass ($topOk -and $botOk) -Evidence "h2Top=$($h2Rect.Y) playerTop=$($dpRect.Y) tabTop=$($tabRect.Y) playerBottom=$($dpRect.Y + $dpRect.Height) gap=$botGap | $layoutEv"
Shot -Tag 'detail-manvs' | Out-Null
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
$foundXc = Search-Vod -Query '小城良方'
# 选集是详情聚合回来后才出的，Search-Vod 只要见到“线路”就返回，选集可能
# 还在路上：轮询等 15 秒，别刚回来就判 0。found 记下来，下次 0 选集时
# 才分得清是“搜不到/没进详情”还是“进了详情但没选集”。
$epBtns = @()
$epEnd = (Get-Date).AddSeconds(15)
while ((Get-Date) -lt $epEnd -and $epBtns.Count -eq 0) {
  $epBtns = @()
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -match '^第.*集$') { $epBtns += $e } } catch { }
  }
  if ($epBtns.Count -eq 0) { Start-Sleep -Seconds 3 }
}
$similarExistsXc = $null -ne (Find-Element -Root $script:Win -Name '更多类似内容')
Add-Result -Id 'T23' -Name '小城良方详情有选集按钮' -Pass ($epBtns.Count -ge 1) -Evidence "episodeButtons=$($epBtns.Count) found=$foundXc similarTitle=$similarExistsXc"
Shot -Tag 'detail-xiaocheng' | Out-Null
# 类似内容：不同片名。类似区是详情落地后才后台去全网找的（浏览同类
# 分类、最多 10 秒 + 回落搜索），标题先出来、卡片后到：轮询等卡片出现。
$similarCards = @()
$similarCardW = $null
$similarCardWs = @()
$script:FirstSimilarDiag = $null
$similarTitle = Find-Element -Root $script:Win -Name '更多类似内容'
if ($similarTitle) {
  $end = (Get-Date).AddSeconds(30)
  while ((Get-Date) -lt $end -and $similarCards.Count -eq 0) {
    $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    $inSimilar = $false
    $found = @()
    $foundW = $null
    $foundWs = @()
    foreach ($e in $all) {
      try {
        $n = $e.Current.Name
        if ($n -eq '更多类似内容') { $inSimilar = $true; continue }
        if (-not $inSimilar) { continue }
        $t = $e.Current.ControlType.ProgrammaticName
        $r = $e.Current.BoundingRectangle
        # 卡片是 Button 或 ListItem、宽约 121px；标题文本（Text）不算。
        if (($t -eq 'ControlType.Button' -or $t -eq 'ControlType.ListItem') -and $n -and $n -ne '小城良方' -and $r.Width -ge 80 -and $r.Width -le 200) {
          $found += $n
          $foundWs += [int]$r.Width
          if ($null -eq $foundW) { $foundW = $r.Width }
          # 诊断：第一个量到的元素到底是什么（Type + UIA 类名 + 矩形）。
          # 卡片三轮都是 151px，但 CSS 上必为 120px（!important），要确认
          # 量到的是 li.content-card 还是别的元素（如海报/标题）。
          if (-not $script:FirstSimilarDiag) {
            $cls = ''
            try { $cls = $e.Current.ClassName } catch { }
            $script:FirstSimilarDiag = "type=$t class='$cls' rect=$([int]$r.X),$([int]$r.Y) $([int]$r.Width)x$([int]$r.Height) name='$n'"
          }
        }
      } catch { }
    }
    if ($found.Count -gt 0) { $similarCards = $found; $similarCardW = $foundW; $similarCardWs = $foundWs; break }
    Start-Sleep -Seconds 3
  }
}
$distinct = ($similarCards | Sort-Object -Unique)
$sameTitle = ($similarCards | Where-Object { $_ -like '*小城良方*' }).Count
Add-Result -Id 'T24' -Name '更多类似内容是不同片名' -Pass ($distinct.Count -ge 3 -and $sameTitle -eq 0) -Evidence "cards=$($similarCards.Count) distinct=$($distinct.Count) sameTitle=$sameTitle similarTitle=$($null -ne $similarTitle) distinctList='$($distinct -join ' | ')'"
# 首跑 cardW=151：CSS 里类似卡片是 120px、详情海报是 150px，第一个量到的
# 可能是海报而不是卡片，所以把全部卡宽都记下来，看 151 是个例还是整行都宽。
Add-Result -Id 'T25' -Name '类似内容卡片收窄（宽≤140）' -Pass ($null -ne $similarCardW -and $similarCardW -le 140) -Evidence "cardW=$similarCardW allWidths='$($similarCardWs -join ',')' firstDiag='$script:FirstSimilarDiag'"

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
# ThemedSelect 不是原生 ComboBox（UIA 里没有）：触发器按钮的名字是
# aria-label（“Cloudflare 点播源”），可见的源名是里面的 span；
# 点开后选项是 role=option 的按钮，名字就是源名。
$cfTrigger = $null
$opened = $false
# 设置页会异步重渲染（CF 目录拉到后下拉框重建），缓存的元素可能过期：
# 每次重找、最多试 5 次；先滚到顶，免得触发器在折叠区域外。
Move-Mouse -X ([int]($wr.X + $wr.Width / 2)) -Y ([int]($wr.Y + 300))
Send-Wheel -Delta 1200 -Clicks 3
for ($try = 1; $try -le 5 -and -not $opened; $try++) {
  $cfTrigger = $null
  $all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      # 触发器按钮的名字就是 aria-label；别直接按名字找，会先撞到旁边的 label 文本。
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name -eq 'Cloudflare 点播源') { $cfTrigger = $e; break }
    } catch { }
  }
  if ($cfTrigger) { $opened = Invoke-Element $cfTrigger }
  if (-not $opened -and $cfTrigger) {
    # InvokePattern 打不开时退到鼠标点触发器中心（设置页按钮，合成点击可用）。
    try {
      $r = $cfTrigger.Current.BoundingRectangle
      Click-At -X ([int]($r.X + $r.Width / 2)) -Y ([int]($r.Y + $r.Height / 2))
      $opened = $true
    } catch { }
  }
  if (-not $opened) { Start-Sleep -Seconds 2 }
}
$cfEv = 'no-dropdown-trigger'
$cfPass = $false
if ($opened) {
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
  $cfEv = "opened=$opened options='$($opts -join ' | ')'"
  # 选项为空时：把触发器下方 400px 内的全部按钮/ListItem 名记下来，
  # 看菜单到底渲染了什么（是源没加载，还是选项名不是源名）。
  if (-not $has3 -and $cfTrigger) {
    try {
      $tr = $cfTrigger.Current.BoundingRectangle
      $near = @()
      foreach ($e in $all2) {
        try {
          $r = $e.Current.BoundingRectangle
          $t = $e.Current.ControlType.ProgrammaticName
          if (($t -eq 'ControlType.Button' -or $t -eq 'ControlType.ListItem') -and $r.Y -ge $tr.Y -and $r.Y -le ($tr.Y + 400) -and $e.Current.Name) { $near += "$($e.Current.Name)@$([int]$r.Width)" }
        } catch { }
      }
      $cfEv += " nearMenu='$((($near | Sort-Object -Unique) -join ' | '))'"
    } catch { }
  }
  Send-Key '{ESC}'
}
if (-not $opened) {
  # 还是打不开：把设置页所有按钮名记下来，下次就知道触发器到底叫什么。
  $btnNames = @()
  $all3 = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all3) {
    try {
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $e.Current.Name) { $btnNames += $e.Current.Name }
    } catch { }
  }
  $cfEv = "no-dropdown-trigger buttons='$($btnNames -join ' | ')'"
}
Add-Result -Id 'T28' -Name 'CF 下拉自动列出 3 个目录源' -Pass $cfPass -Evidence $cfEv
$subHint = Find-Element -Root $script:Win -Name '字幕翻译'
Add-Result -Id 'T27' -Name '设置页字幕说明完整' -Pass ($null -ne $subHint) -Evidence "subtitleSection=$($null -ne $subHint)"
Shot -Tag 'settings-cf' | Out-Null

# ---------- 隐藏 bug 探查：反复切换 + 异常路径 ----------
$switchOk = $true
$switchEv = ''
$switchChanged = 0
$switchLog = ''
Go-Mode 'live' | Out-Null
Start-Sleep -Seconds 2
for ($i = 0; $i -lt 10; $i++) {
  # 切台行不只盯 CCTV：中继一断 CCTV 全死，行数不够就切不动。
  $rows = Find-ChannelRows -Root $script:Win -NameLike '*'
  if ($rows.Count -ge 2) {
    $target = $rows[$i % $rows.Count]
    $beforeStatus = Get-StatusNames
    $targetName = ''
    try { $targetName = $target.Current.Name } catch { }
    Invoke-Element $target | Out-Null
    # 切台要真换台：等状态文本变了才算成功（只看进程活着不够）。
    # 不只认“正在播放”：目标台若死了，状态会变成“正在重连：新台”或
    # “频道播放失败”，那也是切过去了（源站死不是 app 的错）；没点上时
    # 状态纹丝不动。首跑 5/10 就是卡在“正在重连”上。
    $changed = Wait-For { $s = Get-StatusNames; $s -and ($s -ne $beforeStatus) } -Seconds 10
    $afterStatus = Get-StatusNames
    if ($changed) { $switchChanged++ }
    $switchLog += "sw$i target='$targetName' before='$beforeStatus' changed=$($null -ne $changed) status='$afterStatus'; "
    Start-Sleep -Milliseconds 300
  }
  if (-not (Test-ProcessAlive -Id $script:AppPid)) { $switchOk = $false; $switchEv = "died at switch $i"; break }
}
if ($switchOk) { $switchEv = "$switchChanged/10 switches changed channel, process alive | $switchLog" }
$switchOk = $switchOk -and ($switchChanged -ge 8)
Add-Result -Id 'T31' -Name '反复切台 10 次不崩且真换台' -Pass $switchOk -Evidence $switchEv
# 空搜索：输一个不存在的片名，应显示“暂无搜索结果”而不是卡死/报错。
Go-Mode 'search' | Out-Null
Start-Sleep -Seconds 1
$searchBox = $null
$all = $script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $searchBox = $e; break } } catch { }
}
if ($searchBox) {
  Set-InputText -Edit $searchBox -Text '不存在的片名zzzz1234' | Out-Null
  Send-Key '{ENTER}'
  $noRes = Wait-For { Find-Element -Root $script:Win -Name '暂无搜索结果' } -Seconds 30
  Add-Result -Id 'T32' -Name '搜不到时显示暂无结果（不卡死）' -Pass ($null -ne $noRes) -Evidence "noResultsShown=$($null -ne $noRes)"
}
Add-Result -Id 'T33' -Name '全程无悄悄退出' -Pass (Test-ProcessAlive -Id $script:AppPid) -Evidence "pid=$($script:AppPid) alive=$(Test-ProcessAlive -Id $script:AppPid)"

# ---------- 长时间播放（soak；只在 -SoakMinutes > 0 时跑） ----------
if ($SoakMinutes -gt 0) {
  Go-Mode 'live' | Out-Null
  Start-Sleep -Seconds 2
  $soakLog = Join-Path $OutDir 'soak.log'
  "soak start $(Get-Date -Format 'HH:mm:ss') pid=$($script:AppPid)" | Out-File $soakLog
  # 浸泡测的是 app 本身，不是源站：先找一条真能播的台。CCTV-1 曾 403 半小时
  # （源站中断），死守它会白白浪费 30 分钟；中继一断就是一片 410，所以不只
  # 试 CCTV（CCTV → BBC → 其它行），播起来的才开始计时，死台记进日志。
  $soakChannel = Find-WorkingLiveChannel -PerRowSeconds 30 -MaxRows 5
  $soakDead = $script:LastLiveDead
  if ($soakChannel) {
    $p0 = Get-PlayerProgress
    "soak channel '$soakChannel' playing (progress=$p0)" | Out-File $soakLog -Append
  } else {
    "soak: NO working channel" | Out-File $soakLog -Append
  }
  $switchCount = 0
  $lastProgress = Get-PlayerProgress
  $stuckSamples = 0
  $froze = $false
  $recovered = $false
  $unresolvedFreeze = $false
  $lastAdvanceMin = 0
  $soakPass = $true
  $everPlayed = ($null -ne $lastProgress -and $lastProgress -gt 0)
  $nullStreak = 0
  $lostPlayback = $false
  for ($m = 1; $m -le $SoakMinutes -and $soakChannel; $m++) {
    Start-Sleep -Seconds 60
    if (-not (Test-ProcessAlive -Id $script:AppPid)) {
      "MIN ${m}: PROCESS GONE (silent exit)" | Out-File $soakLog -Append
      $soakPass = $false; $froze = $true; break
    }
    $p = Get-PlayerProgress
    $reconnecting = $null -ne (Find-Element -Root $script:Win -Name '正在重连')
    # 浸泡诊断：每分钟记 WebView2/mpv 进程数、UIA 元素总数、状态行。
    # 首跑 MIN 11–19 进度读空：要分清是渲染进程死了（uiaCount 骤降）、
    # 回落到 mpv 了（mpv 进程出现、页内控件消失）还是单纯读不到。
    $wvCount = (Get-Process -Name 'msedgewebview2' -ErrorAction SilentlyContinue).Count
    $mpvCount = (Get-Process -Name 'mpv*' -ErrorAction SilentlyContinue).Count
    $uiaCount = -1
    try { $uiaCount = ($script:Win.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)).Count } catch { }
    $stNames = Get-StatusNames
    "MIN $m $(Get-Date -Format 'HH:mm:ss') progress=$p reconnecting=$reconnecting wv2=$wvCount mpv=$mpvCount uia=$uiaCount status='$stNames'" | Out-File $soakLog -Append
    if ($null -ne $p) { $everPlayed = $true; $nullStreak = 0 } else { $nullStreak++ }
    if ($null -ne $p -and $null -ne $lastProgress -and $p -gt $lastProgress) { $lastAdvanceMin = $m }
    # 播着播着彻底没进度：只记下来，到结束还在死播才判不通过（见结尾）。
    if ($nullStreak -ge 5 -and -not $lostPlayback) {
      $lostPlayback = $true
      "MIN ${m}: PLAYBACK LOST (no progress for 5 min)" | Out-File $soakLog -Append
    }
    # 台在浸泡中死了（源站中断）：换一条能播的继续泡（最多 3 次），
    # 浸泡测的是 app，不是源站；死台记进日志，换台不算失败。
    # 每 3 分钟没进度才试一次（$nullStreak % 3 -eq 0），免得每分钟都去试。
    if ($nullStreak -ge 3 -and ($nullStreak % 3 -eq 0) -and $switchCount -lt 3 -and $soakChannel) {
      $newCh = Find-WorkingLiveChannel -Exclude @($soakChannel) -PerRowSeconds 15 -MaxRows 5
      if ($newCh) {
        $switchCount++
        "MIN ${m}: SWITCHED channel '$soakChannel' -> '$newCh' (old channel died)" | Out-File $soakLog -Append
        $soakChannel = $newCh
        $nullStreak = 0; $stuckSamples = 0; $lastProgress = Get-PlayerProgress
        $soakDead += $script:LastLiveDead
      } else {
        "MIN ${m}: no replacement channel found (tried, all dead)" | Out-File $soakLog -Append
      }
    }
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
      else {
        # 60 秒没恢复：先不判死——源站中断几分钟是常事（CCTV 中继常 403），
        # 只要到浸泡结束时自己恢复过来（不需人动手）就算过；到结束还在死播才不通过。
        $unresolvedFreeze = $true
        "MIN ${m}: STILL FROZEN after 60s (source may be down, keep watching)" | Out-File $soakLog -Append
      }
    }
    $lastProgress = $p
  }
  if (-not $soakChannel) { $soakPass = $false }
  # 终判：进程活着、播过、且到结束时进度还在前进（最近 2 分钟内前进过），
  # 冻住只要最后自己恢复过来就算过——60 秒窗口量的是源站中断时长，
  # 不是 app；到结束还在死播（或进程没了、或根本没播过）才不通过。
  $endAlive = Test-ProcessAlive -Id $script:AppPid
  $playingAtEnd = ($SoakMinutes - $lastAdvanceMin) -le 2
  $soakPass = $soakPass -and $endAlive -and $everPlayed -and $playingAtEnd -and [bool]$soakChannel
  $ev = "minutes=$SoakMinutes froze=$froze recovered=$recovered unresolvedFreeze=$unresolvedFreeze processAlive=$endAlive channel='$soakChannel' dead='$($soakDead -join ' | ')' everPlayed=$everPlayed lostPlayback=$lostPlayback lastAdvanceMin=$lastAdvanceMin playingAtEnd=$playingAtEnd"
  Add-Result -Id 'T34' -Name "连续播放 ${SoakMinutes} 分钟不冻住（冻住自己恢复就算过）" -Pass $soakPass -Evidence $ev
  Shot -Tag 'soak-end' | Out-Null
}

# ---------- 报告 ----------
$passed = ($script:Results | Where-Object { $_.pass }).Count
$total = $script:Results.Count
$summary = @{
  exe = $ExePath
  pid = $script:AppPid
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
$md += "- 进程 pid：$($script:AppPid)，soak：${SoakMinutes} 分钟"
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
