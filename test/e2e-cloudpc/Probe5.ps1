# Probe5：CCTV-1 播 90 秒，每 5 秒采样：进度 / 正在重连 / 报错 / 进程。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe5.txt'
"probe5 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
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

$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.Name -eq '直播') { Invoke-Element $e | Out-Null; break } } catch { } }
Start-Sleep -Seconds 5
$row = $null
$end = (Get-Date).AddSeconds(20)
while ((Get-Date) -lt $end -and -not $row) {
  $row = Find-Element -Root $win -Name 'CCTV综合'
  if (-not $row) { Start-Sleep -Seconds 2 }
}
"row found: $($null -ne $row)" | Out-File $out -Append
if ($row) { Invoke-Element $row | Out-Null }
for ($i = 1; $i -le 18; $i++) {
  Start-Sleep -Seconds 5
  $p = Get-Progress
  $re = $null -ne (Find-Element -Root $win -Name '正在重连')
  $err = Find-Element -Root $win -Name '频道播放失败'
  $now = Find-Element -Root $win -Name '正在播放'
  $nowTxt = ''
  if ($now) { $nowTxt = $now.Current.Name }
  "t=$($i * 5)s progress=$p reconnecting=$re error=$($null -ne $err) status='$nowTxt'" | Out-File $out -Append
}
"probe5 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
