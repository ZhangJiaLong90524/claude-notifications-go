# Dump complete WT process tree and correlate with UIA tabs
Add-Type -AssemblyName UIAutomationClient

$wtPid = (Get-Process WindowsTerminal)[0].Id
$allProcs = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CreationDate

# Recursive tree printer
function Show-Children($parentPid, $indent) {
    $children = $allProcs | Where-Object { $_.ParentProcessId -eq $parentPid } | Sort-Object CreationDate
    foreach ($c in $children) {
        Write-Host "$indent$($c.Name) PID=$($c.ProcessId) Created=$($c.CreationDate)"
        Show-Children $c.ProcessId "$indent  "
    }
}

Write-Host "=== WT Process Tree (PID $wtPid) ==="
Show-Children $wtPid "  "

# UIA tabs
Write-Host "`n=== UIA Tabs ==="
$root = [System.Windows.Automation.AutomationElement]::RootElement
$wtWin = $root.FindFirst('Children',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ClassNameProperty,
        'CASCADIA_HOSTING_WINDOW_CLASS')))
$tabs = $wtWin.FindAll('Descendants',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
        [System.Windows.Automation.ControlType]::TabItem)))
for ($i = 0; $i -lt $tabs.Count; $i++) {
    $sel = $tabs[$i].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
    Write-Host "  UIA[$i] '$($tabs[$i].Current.Name)' selected=$($sel.Current.IsSelected)"
}

# My own process chain
Write-Host "`n=== My Process Chain ==="
$pid2 = $PID
for ($depth = 0; $depth -lt 10; $depth++) {
    $proc = $allProcs | Where-Object { $_.ProcessId -eq $pid2 }
    if (-not $proc) { break }
    Write-Host "  $($proc.Name) PID=$($proc.ProcessId) Parent=$($proc.ParentProcessId)"
    if ($proc.ProcessId -eq $wtPid) { break }
    $pid2 = $proc.ParentProcessId
}
