# Probe4：查三件事——① CCTV-1/2/3 各播 25 秒看进度；② 搜索粘贴与点卡片全过程；
# ③ 设置页 CF 下拉触发器到底长什么样。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe4.txt'
"probe4 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id)" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -ProcessId $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }

function Get-Progress {
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      if ($n -match '^(\d+):(\d+) / ') { return ([int]$Matches[1]) * 60 + [int]$Matches[2] }
    } catch { }
  }
  return $null
}

# ① 直播三台各播 25 秒。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '直播') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 3
foreach ($ch in @('CCTV-1', 'CCTV-2', 'CCTV-3')) {
  $row = Find-Element -Root $win -Name $ch
  if (-not $row) { "$ch : row not found" | Out-File $out -Append; continue }
  Invoke-Element $row | Out-Null
  $samples = @()
  for ($i = 0; $i -lt 5; $i++) { Start-Sleep -Seconds 5; $samples += (Get-Progress) }
  $reconnecting = $null -ne (Find-Element -Root $win -Name '正在重连')
  $err = Find-Element -Root $win -Name '频道播放失败'
  "$ch : progress samples=($($samples -join ',')) reconnecting=$reconnecting errorShown=$($null -ne $err)" | Out-File $out -Append
}

# ② 搜索：粘贴、结果、点卡片。
$nav = Find-Element -Root $win -Name '搜索'
if ($nav) { Invoke-Element $nav | Out-Null }
Start-Sleep -Seconds 2
$box = $null
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $box = $e; break } } catch { } }
"search box: $($null -ne $box)" | Out-File $out -Append
if ($box) {
  "paste: $(Set-InputText -Edit $box -Text '荒野求生')" | Out-File $out -Append
  try { "box value: '$($box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Value)'" | Out-File $out -Append } catch { }
  [System.Windows.Forms.SendKeys]::SendWait('{ENTER}')
  Start-Sleep -Seconds 15
  $cands = Find-Elements -Root $win -NameLike '*荒野求生*'
  "candidates named *荒野求生*: $($cands.Count)" | Out-File $out -Append
  foreach ($c in $cands) {
    try {
      $r = $c.Current.BoundingRectangle
      "  cand type=$($c.Current.ControlType.ProgrammaticName) name='$($c.Current.Name)' rect=$([int]$r.X),$([int]$r.Y),$([int]$r.Width)x$([int]$r.Height)" | Out-File $out -Append
    } catch { }
  }
  $card = $null
  foreach ($c in $cands) {
    try {
      $r = $c.Current.BoundingRectangle
      if ($c.Current.ControlType.ProgrammaticName -eq 'ControlType.Button' -and $r.Height -lt 40) { continue }
      $card = $c; break
    } catch { continue }
  }
  "card picked: $($null -ne $card)" | Out-File $out -Append
  if ($card) {
    "card invoke-or-ancestor: $(Invoke-ElementOrAncestor $card)" | Out-File $out -Append
    Start-Sleep -Seconds 5
    $similar = Find-Element -Root $win -Name '更多类似内容'
    $eps = Find-Elements -Root $win -NameLike '第*集*'
    $lines = Find-Elements -Root $win -NameLike '线路*'
    "after click: similar=$($null -ne $similar) epButtons=$($eps.Count) lineBtns=$($lines.Count)" | Out-File $out -Append
  }
}

# ③ 设置页 CF 下拉触发器。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '设置') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 2
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) {
  try {
    $n = $e.Current.Name
    if ($n -match 'Cloudflare|点播源') {
      $r = $e.Current.BoundingRectangle
      "  cf-elem type=$($e.Current.ControlType.ProgrammaticName) name='$n' rect=$([int]$r.X),$([int]$r.Y),$([int]$r.Width)x$([int]$r.Height)" | Out-File $out -Append
    }
  } catch { }
}
"probe4 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
