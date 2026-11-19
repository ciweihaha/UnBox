# Probe6：点台后到底发生了什么——点击返回值 + 页面所有状态文本 + 播放器在不在。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe6.txt'
"probe6 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id) exe: C:\temp\unbox0917\UnBox-0.9.17-test.exe" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -ProcessId $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }

$nav = Find-Element -Root $win -Name '直播'
"nav live invoke: $(if ($nav) { Invoke-Element $nav } else { 'nav-not-found' })" | Out-File $out -Append
$row = $null
$end = (Get-Date).AddSeconds(30)
while ((Get-Date) -lt $end -and -not $row) {
  $row = Find-Element -Root $win -Name 'CCTV综合'
  if (-not $row) { Start-Sleep -Seconds 2 }
}
"row found: $($null -ne $row)" | Out-File $out -Append
if ($row) {
  "row type: $($row.Current.ControlType.ProgrammaticName)" | Out-File $out -Append
  "row invoke: $(Invoke-Element $row)" | Out-File $out -Append
}
for ($i = 1; $i -le 12; $i++) {
  Start-Sleep -Seconds 5
  $texts = @()
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  $slider = $false
  $prog = $null
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      if ($n -match '正在|播放|失败|重连|加载|缓冲|暂停') { $texts += $n }
      if ($n -match '^(\d+):(\d+) / ') { $prog = ([int]$Matches[1]) * 60 + [int]$Matches[2] }
      if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Slider') { $slider = $true }
    } catch { }
  }
  "t=$($i * 5)s progress=$prog slider=$slider statusTexts='$($texts -join ' | ')'" | Out-File $out -Append
}
"probe6 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
