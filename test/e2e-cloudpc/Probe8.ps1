# Probe8：ValuePattern.SetValue 到底能不能驱动搜索。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe8.txt'
"probe8 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id)" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -ProcessId $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }
$script:Win = $win

function Find-SearchBox {
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { return $e } } catch { } }
  return $null
}

$nav = Find-Element -Root $win -Name '搜索'
if ($nav) { Invoke-Element $nav | Out-Null }
Start-Sleep -Seconds 2
$box = Find-SearchBox
"box: $($null -ne $box) enabled=$($box.Current.IsEnabled)" | Out-File $out -Append
if ($box) {
  Set-ActiveWindow -Window $win
  try {
    $box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('荒野求生')
    "setvalue: no exception" | Out-File $out -Append
  } catch { "setvalue FAILED: $_" | Out-File $out -Append }
  Start-Sleep -Milliseconds 800
  try { "value after setvalue: '$($box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Value)'" | Out-File $out -Append } catch { }
  [System.Windows.Forms.SendKeys]::SendWait('{ENTER}')
  "enter sent" | Out-File $out -Append
  for ($i = 1; $i -le 6; $i++) {
    Start-Sleep -Seconds 5
    $cards = @(Find-Elements -Root $win -NameLike '*荒野求生*')
    $cardLike = @($cards | Where-Object { try { $_.Current.BoundingRectangle.Height -ge 100 } catch { $false } })
    $noRes = Find-Element -Root $win -Name '暂无搜索结果'
    "t=$($i * 5)s matches=$($cards.Count) cardLike=$($cardLike.Count) noRes=$($null -ne $noRes)" | Out-File $out -Append
    if ($cardLike.Count -ge 1) { break }
  }
}
"probe8 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
