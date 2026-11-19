# Probe7：搜索框粘贴到底行不行——三种聚焦方式 × 粘贴/打字，每次重找框读值。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe7.txt'
"probe7 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id)" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -ProcessId $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }
$script:Win = $win
Set-ActiveWindow -Window $win
"activated window" | Out-File $out -Append

function Find-SearchBox {
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { return $e } } catch { } }
  return $null
}
function Read-BoxValue {
  $b = Find-SearchBox
  if (-not $b) { return '<no-box>' }
  try { return $b.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Value } catch { return "<read-failed: $_>" }
}

$nav = Find-Element -Root $win -Name '搜索'
if ($nav) { Invoke-Element $nav | Out-Null }
Start-Sleep -Seconds 2
"box at start: '$(Read-BoxValue)'" | Out-File $out -Append

# 方式1：鼠标点框 + 粘贴。
$box = Find-SearchBox
if ($box) {
  $r = $box.Current.BoundingRectangle
  "box rect: $([int]$r.X),$([int]$r.Y) $([int]$r.Width)x$([int]$r.Height)" | Out-File $out -Append
  Click-At -X ([int]($r.X + $r.Width / 2)) -Y ([int]($r.Y + $r.Height / 2))
  try { [System.Windows.Forms.Clipboard]::SetText('荒野求生'); "clipboard set ok" | Out-File $out -Append } catch { "clipboard set FAILED: $_" | Out-File $out -Append }
  [System.Windows.Forms.SendKeys]::SendWait('^a')
  [System.Windows.Forms.SendKeys]::SendWait('^v')
  Start-Sleep -Milliseconds 800
  "method1 mouse-click+paste value: '$(Read-BoxValue)'" | Out-File $out -Append
  "clipboard now: '$([System.Windows.Forms.Clipboard]::GetText())'" | Out-File $out -Append
}
# 方式2：UIA SetFocus + 粘贴。
$box = Find-SearchBox
if ($box) {
  try { $box.SetFocus(); "setfocus ok" | Out-File $out -Append } catch { "setfocus FAILED: $_" | Out-File $out -Append }
  [System.Windows.Forms.SendKeys]::SendWait('^a')
  [System.Windows.Forms.SendKeys]::SendWait('^v')
  Start-Sleep -Milliseconds 800
  "method2 setfocus+paste value: '$(Read-BoxValue)'" | Out-File $out -Append
}
# 方式3：键盘直接打字。
$box = Find-SearchBox
if ($box) {
  try { $box.SetFocus() } catch { }
  [System.Windows.Forms.SendKeys]::SendWait('^a')
  [System.Windows.Forms.SendKeys]::SendWait('小城良方')
  Start-Sleep -Milliseconds 800
  "method3 typing value: '$(Read-BoxValue)'" | Out-File $out -Append
  $focused = [System.Windows.Automation.AutomationElement]::FocusedElement
  try { "focused element: type=$($focused.Current.ControlType.ProgrammaticName) name='$($focused.Current.Name)'" | Out-File $out -Append } catch { }
}
"probe7 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
