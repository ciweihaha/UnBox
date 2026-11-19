# Probe3：专查搜索流程——粘贴后框里的值、结果出没出、点的是哪个元素。
$ErrorActionPreference = 'Stop'
. (Join-Path 'C:\unbox\test\e2e-cloudpc' 'UnBox.Uia.ps1')
$out = 'C:\temp\unbox0917\probe3.txt'
"probe3 start: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out
Stop-UnBoxProcesses
$proc = Start-Process -FilePath 'C:\temp\unbox0917\UnBox-0.9.17-test.exe' -WorkingDirectory 'C:\temp\unbox0917' -PassThru
"pid: $($proc.Id)" | Out-File $out -Append
$win = $null
$end = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $end -and -not $win) { $win = Get-UnBoxWindowForPid -ProcessId $proc.Id; Start-Sleep -Milliseconds 500 }
"window: $($null -ne $win)" | Out-File $out -Append
if (-not $win) { "no window" | Out-File $out -Append; exit 1 }
# 点顶栏搜索图标。
$nav = Find-Element -Root $win -Name '搜索'
"nav-search found: $($null -ne $nav)" | Out-File $out -Append
if ($nav) {
  $r = $nav.Current.BoundingRectangle
  "nav-search rect: $([int]$r.X),$([int]$r.Y) size $([int]$r.Width)x$([int]$r.Height) type=$($nav.Current.ControlType.ProgrammaticName)" | Out-File $out -Append
  "nav-search invoke: $(Invoke-Element $nav)" | Out-File $out -Append
}
Start-Sleep -Seconds 2
$box = $null
$all = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
foreach ($e in $all) { try { if ($e.Current.ControlType.ProgrammaticName -eq 'ControlType.Edit') { $box = $e; break } } catch { } }
"box found: $($null -ne $box)" | Out-File $out -Append
if ($box) {
  $r = $box.Current.BoundingRectangle
  "box rect: $([int]$r.X),$([int]$r.Y) name='$($box.Current.Name)'" | Out-File $out -Append
  "paste result: $(Set-InputText -Edit $box -Text '荒野求生')" | Out-File $out -Append
  try { "box value: '$($box.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Value)'" | Out-File $out -Append } catch { }
  [System.Windows.Forms.SendKeys]::SendWait('{ENTER}')
  "enter sent" | Out-File $out -Append
}
for ($i = 1; $i -le 6; $i++) {
  Start-Sleep -Seconds 5
  $hit = Find-Element -Root $win -Name '荒野求生'
  $searching = Find-Element -Root $win -Name '正在搜索'
  $noRes = Find-Element -Root $win -Name '暂无搜索结果'
  $count = Find-Element -Root $win -Name '找到'
  "t=$($i * 5)s hit=$($null -ne $hit) searching=$($null -ne $searching) noRes=$($null -ne $noRes) countText=$($null -ne $count)" | Out-File $out -Append
  if ($hit) {
    "hit name='$($hit.Current.Name)' type=$($hit.Current.ControlType.ProgrammaticName)" | Out-File $out -Append
    $rr = $hit.Current.BoundingRectangle
    "hit rect: $([int]$rr.X),$([int]$rr.Y) $([int]$rr.Width)x$([int]$rr.Height)" | Out-File $out -Append
    "invoke-or-ancestor: $(Invoke-ElementOrAncestor $hit)" | Out-File $out -Append
    Start-Sleep -Seconds 5
    $detail = Find-Element -Root $win -Name '更多类似内容'
    $eps = Find-Elements -Root $win -NameLike '第*集*'
    "after click: similar=$($null -ne $detail) epButtons=$($eps.Count)" | Out-File $out -Append
    break
  }
}
"probe3 done: $(Get-Date -Format 'HH:mm:ss')" | Out-File $out -Append
