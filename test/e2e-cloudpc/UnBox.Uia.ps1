# UnBox.Uia.ps1 — UnBox 云电脑端到端测试的 UIA 助手。
#
# 用法：. .\UnBox.Uia.ps1
# 在云电脑 session 1（交互会话）里跑：WinRM session 0 看不到桌面，
# 所以测试脚本要经计划任务（LogonType Interactive）在 session 1 里执行。
# 中文元素名在脚本内直接用原文匹配；WinRM 回显会变 ??，不要靠回显读中文，
# 一律把结果写进报告文件再拉回来看。
# 鼠标合成输入在该 app 里不可靠：点按一律用 UIA InvokePattern；
# 滚轮测试必须先把光标移到列表上方再滚（0.9.16 的教训）。

Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class NativeInput {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, int dwData, UIntPtr dwExtraInfo);
  public const uint MOUSEEVENTF_WHEEL = 0x0800;
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);
}
"@

$script:AE = [System.Windows.Automation.AutomationElement]
$script:TreeScope = [System.Windows.Automation.TreeScope]
$script:NameProp = [System.Windows.Automation.AutomationElement]::NameProperty
$script:InvokePattern = [System.Windows.Automation.InvokePattern]::Pattern
$script:ValuePattern = [System.Windows.Automation.ValuePattern]::Pattern
$script:RangePattern = [System.Windows.Automation.RangeValuePattern]::Pattern
$script:ScrollPattern = [System.Windows.Automation.ScrollPattern]::Pattern

function Get-UnBoxWindow {
  $cond = New-Object System.Windows.Automation.PropertyCondition($script:NameProp, 'UnBox')
  return $script:AE::RootElement.FindFirst($script:TreeScope::Children, $cond)
}

function Get-WindowOwnerPid {
  param([IntPtr]$Handle)
  $pidOut = 0
  [void][NativeInput]::GetWindowThreadProcessId($Handle, [ref]$pidOut)
  return $pidOut
}

# Get-UnBoxWindowForPid 只认属于指定进程的 UnBox 窗口：云电脑上可能残留
# 旧版进程（0.9.16 轮的 UB916-Launch 就留了一个），按名字找会测到旧窗口。
function Get-UnBoxWindowForPid {
  param([int]$ProcessId)
  $cond = New-Object System.Windows.Automation.PropertyCondition($script:NameProp, 'UnBox')
  $wins = $script:AE::RootElement.FindAll($script:TreeScope::Children, $cond)
  foreach ($w in $wins) {
    try {
      $h = $w.Current.NativeWindowHandle
      if ($h -and (Get-WindowOwnerPid -Handle ([IntPtr]$h)) -eq $ProcessId) { return $w }
    } catch { }
  }
  return $null
}

function Stop-UnBoxProcesses {
  Get-Process | Where-Object { $_.Name -like 'UnBox*' } | ForEach-Object {
    try { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue } catch { }
  }
  Start-Sleep -Seconds 2
}

function Find-Element {
  param(
    [System.Windows.Automation.AutomationElement]$Root,
    [string]$Name,
    [switch]$Exact
  )
  if (-not $Root) { return $null }
  $cond = New-Object System.Windows.Automation.PropertyCondition($script:NameProp, $Name)
  $el = $Root.FindFirst($script:TreeScope::Descendants, $cond)
  if ($el -or $Exact) { return $el }
  # 模糊匹配：名字包含给定文本的第一个元素。
  $all = $Root.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try { if ($e.Current.Name -like "*$Name*") { return $e } } catch { }
  }
  return $null
}

function Find-Elements {
  param(
    [System.Windows.Automation.AutomationElement]$Root,
    [string]$NameLike
  )
  if (-not $Root) { return @() }
  $out = @()
  $all = $Root.FindAll($script:TreeScope::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($e in $all) {
    try { if ($e.Current.Name -like $NameLike) { $out += $e } } catch { }
  }
  return $out
}

function Get-ElementRect {
  param([System.Windows.Automation.AutomationElement]$El)
  if (-not $El) { return $null }
  try { return $El.Current.BoundingRectangle } catch { return $null }
}

function Invoke-Element {
  param([System.Windows.Automation.AutomationElement]$El)
  if (-not $El) { return $false }
  try {
    $p = $El.GetCurrentPattern($script:InvokePattern)
    $p.Invoke()
    return $true
  } catch { return $false }
}

# Invoke-ElementOrAncestor：卡片标题是纯文本（点不动），往上找可点的祖先再点。
function Invoke-ElementOrAncestor {
  param([System.Windows.Automation.AutomationElement]$El)
  if (-not $El) { return $false }
  $cur = $El
  for ($i = 0; $i -lt 4 -and $cur; $i++) {
    if (Invoke-Element $cur) { return $true }
    try { $cur = [System.Windows.Automation.TreeWalker]::RawViewWalker.GetParent($cur) } catch { $cur = $null }
  }
  return $false
}

function Expand-Element {
  param([System.Windows.Automation.AutomationElement]$El)
  if (-not $El) { return $false }
  try {
    $p = $El.GetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern)
    $p.Expand()
    return $true
  } catch { }
  return (Invoke-Element $El)
}

function Get-ElementText {
  param([System.Windows.Automation.AutomationElement]$El)
  if (-not $El) { return '' }
  try {
    $p = $El.GetCurrentPattern($script:ValuePattern)
    return [string]$p.Value
  } catch { }
  try { return [string]$El.Current.Name } catch { return '' }
}

function Send-Key {
  param([string]$Keys)
  [System.Windows.Forms.SendKeys]::SendWait($Keys)
  Start-Sleep -Milliseconds 300
}

# Set-InputText 往输入框填中文：不要用 ValuePattern.SetValue——它不触发
# Vue 的 v-model，框里有字、app 里还是空；剪贴板粘贴（0.9.16 轮验证过）才行。
function Set-InputText {
  param(
    [System.Windows.Automation.AutomationElement]$Edit,
    [string]$Text
  )
  if (-not $Edit) { return $false }
  try { $Edit.SetFocus() } catch { }
  Start-Sleep -Milliseconds 300
  [System.Windows.Forms.Clipboard]::SetText($Text)
  [System.Windows.Forms.SendKeys]::SendWait('^a')
  [System.Windows.Forms.SendKeys]::SendWait('^v')
  Start-Sleep -Milliseconds 500
  try {
    $v = $Edit.GetCurrentPattern($script:ValuePattern).Value
    return ($v -eq $Text)
  } catch { return $true }
}

function Move-Mouse {
  param([int]$X, [int]$Y)
  [NativeInput]::SetCursorPos($X, $Y) | Out-Null
  Start-Sleep -Milliseconds 200
}

function Send-Wheel {
  param([int]$Delta = -120, [int]$Clicks = 1)
  for ($i = 0; $i -lt $Clicks; $i++) {
    [NativeInput]::mouse_event([NativeInput]::MOUSEEVENTF_WHEEL, 0, 0, $Delta, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 150
  }
}

function Save-Screenshot {
  param([string]$Path, [int]$X = 0, [int]$Y = 0, [int]$Width = 0, [int]$Height = 0)
  if ($Width -le 0) { $Width = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width }
  if ($Height -le 0) { $Height = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Height }
  $bmp = New-Object System.Drawing.Bitmap($Width, $Height)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($X, $Y, 0, 0, $bmp.Size)
  $g.Dispose()
  $dir = Split-Path $Path -Parent
  if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
  $bmp.Save($Path, [System.Drawing.Imaging.ImageFormat]::Jpeg)
  $bmp.Dispose()
  return $Path
}

function Get-ScreenPixel {
  param([int]$X, [int]$Y)
  $bmp = New-Object System.Drawing.Bitmap(1, 1)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($X, $Y, 0, 0, $bmp.Size)
  $g.Dispose()
  $c = $bmp.GetPixel(0, 0)
  $bmp.Dispose()
  return $c
}

function Test-ProcessAlive {
  param([int]$Id)
  return $null -ne (Get-Process -Id $Id -ErrorAction SilentlyContinue)
}
