# Probe2：dump 直播/搜索/设置三页的 UIA 树（名字 + 类型 + 矩形），定位 E2E 脚本问题。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe2.txt'
"probe2 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id)" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -Pid $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }

function Dump-Tree {
  param([string]$Tag)
  "=== $Tag ===" | Out-File $out -Append
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      $t = $e.Current.ControlType.ProgrammaticName
      $r = $e.Current.BoundingRectangle
      if ($n -or $t -match 'Slider|ComboBox|Edit|Button') {
        "[$t] name='$n' rect=$([int]$r.X),$([int]$r.Y),$([int]$r.Width)x$([int]$r.Height)" | Out-File $out -Append
      }
    } catch { }
  }
}

# 直播页。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '直播') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 3
Dump-Tree -Tag 'LIVE-BEFORE-PLAY'
# 点第一行 CCTV。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -like 'CCTV*') { "clicking row: $($e.Current.Name)" | Out-File $out -Append; Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 10
Dump-Tree -Tag 'LIVE-AFTER-PLAY-10S'
# 搜索页：搜荒野求生。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '搜索') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 2
$box = $null
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $box = $e; break } } catch { } }
"search-box: $($null -ne $box) name='$($box.Current.Name)'" | Out-File $out -Append
if ($box) {
  try { $box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('荒野求生') } catch { "setvalue failed: $_" | Out-File $out -Append }
  Start-Sleep -Seconds 1
  "box value after set: '$($box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Value)'" | Out-File $out -Append
  [System.Windows.Forms.SendKeys]::SendWait('{ENTER}')
  Start-Sleep -Seconds 8
}
Dump-Tree -Tag 'SEARCH-RESULTS-8S'
# 设置页。
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '设置') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 2
Dump-Tree -Tag 'SETTINGS'
"probe2 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
