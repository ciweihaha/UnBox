# 探测：只起 0.9.17 测试包，读 About 里的两个版本号。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe-version.txt'
Stop-ScheduledTask -TaskName 'UB916-Launch' -ErrorAction SilentlyContinue
Disable-ScheduledTask -TaskName 'UB916-Launch' -ErrorAction SilentlyContinue | Out-Null
Get-Process UnBox -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
"old-exe-exists: $(Test-Path 'C:\temp\UnBox-0.9.16-windows-amd64.exe')" | Out-File $out
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"test-exe-pid: $($proc.Id)" | Out-File $out -Append
Start-Sleep -Seconds 8
$win = Get-UnBoxWindow
"window-found: $($null -ne $win)" | Out-File $out -Append
if ($win) {
  $procs = Get-Process UnBox -ErrorAction SilentlyContinue | ForEach-Object { "$($_.Id):$($_.Path)" }
  "running-unbox: $($procs -join ' | ')" | Out-File $out -Append
  # 进设置开关于。
  $el = $null
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) { try { if ($e.Current.Name -eq '设置') { $el = $e; break } } catch { } }
  if ($el) { Invoke-Element $el | Out-Null; Start-Sleep -Seconds 1 }
  $about = $null
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) { try { if ($e.Current.Name -eq '关于我们') { $about = $e; break } } catch { } }
  if ($about) { Invoke-Element $about | Out-Null; Start-Sleep -Seconds 2 }
  $all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try {
      $n = $e.Current.Name
      if ($n -like '内部版本*' -or $n -like '当前版本*') { "version-line: $n" | Out-File $out -Append }
    } catch { }
  }
  Save-Screenshot -Path 'C:\temp\unbox0917\probe-about.jpg' | Out-Null
}
"done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
