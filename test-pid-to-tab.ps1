# Test: given a known shell PID, find tab index and switch to it
param([int]$TargetShellPID = 51752)  # default: this session's outermost shell

Add-Type -AssemblyName UIAutomationClient

$wtPid = (Get-Process WindowsTerminal)[0].Id
$allProcs = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CreationDate

# Find all OpenConsole children of WT, sorted by creation time
$consoles = $allProcs | Where-Object {
    $_.ParentProcessId -eq $wtPid -and $_.Name -eq 'OpenConsole.exe'
} | Sort-Object CreationDate

# For each OpenConsole, check if the target shell is its child
$targetTabIndex = -1
for ($i = 0; $i -lt $consoles.Count; $i++) {
    $oc = $consoles[$i]
    $children = $allProcs | Where-Object { $_.ParentProcessId -eq $oc.ProcessId }
    foreach ($ch in $children) {
        if ($ch.ProcessId -eq $TargetShellPID) {
            $targetTabIndex = $i
            Write-Host "Shell PID $TargetShellPID is child of OpenConsole[$i] (PID=$($oc.ProcessId))"
            break
        }
    }
    if ($targetTabIndex -ge 0) { break }
}

if ($targetTabIndex -lt 0) {
    Write-Host "Shell PID $TargetShellPID not found as direct child of any OpenConsole"
    exit 1
}

# Get UIA tabs
$root = [System.Windows.Automation.AutomationElement]::RootElement
$wtWin = $root.FindFirst('Children',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ClassNameProperty,
        'CASCADIA_HOSTING_WINDOW_CLASS')))
$tabs = $wtWin.FindAll('Descendants',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
        [System.Windows.Automation.ControlType]::TabItem)))

Write-Host "UIA tab count: $($tabs.Count), target index: $targetTabIndex"

if ($targetTabIndex -ge $tabs.Count) {
    Write-Host "ERROR: tab index $targetTabIndex exceeds UIA tab count $($tabs.Count)"
    exit 1
}

$targetTab = $tabs[$targetTabIndex]
Write-Host "Target tab: '$($targetTab.Current.Name)'"

# Switch to a DIFFERENT tab first, then switch back to prove it works
$currentIdx = -1
for ($i = 0; $i -lt $tabs.Count; $i++) {
    $sel = $tabs[$i].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
    if ($sel.Current.IsSelected) { $currentIdx = $i }
}
Write-Host "Currently on tab[$currentIdx]"

if ($currentIdx -ne $targetTabIndex) {
    Write-Host "Switching to tab[$targetTabIndex]..."
    $sel = $targetTab.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
    $sel.Select()
    Write-Host "Done! Switched to: '$($targetTab.Current.Name)'"
} else {
    # Already on target, switch to another and back
    $otherIdx = if ($targetTabIndex -eq 0) { 1 } else { 0 }
    Write-Host "Already on target. Switching to tab[$otherIdx] then back..."
    $tabs[$otherIdx].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Start-Sleep -Milliseconds 300
    $targetTab.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Write-Host "Done! Switched away and back to: '$($targetTab.Current.Name)'"
}
