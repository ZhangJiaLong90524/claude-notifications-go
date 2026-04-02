# Test: match shell PID to UIA tab index
Add-Type -AssemblyName UIAutomationClient

# 1. Get WT process
$wtPid = (Get-Process WindowsTerminal)[0].Id
Write-Host "WT PID: $wtPid"

# 2. Build process tree: all processes with their parents
$allProcs = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CreationDate

# 3. Find OpenConsole children of WT, sorted by creation time
$consoles = $allProcs | Where-Object { $_.ParentProcessId -eq $wtPid -and $_.Name -eq 'OpenConsole.exe' } | Sort-Object CreationDate

# 4. For each OpenConsole, find shell children
Write-Host "`n=== Process Tree ==="
$tabIdx = 0
$pidToTab = @{}
foreach ($oc in $consoles) {
    $shells = $allProcs | Where-Object { $_.ParentProcessId -eq $oc.ProcessId }
    foreach ($sh in $shells) {
        Write-Host ("  Tab[$tabIdx] OC=$($oc.ProcessId) -> $($sh.Name) PID=$($sh.ProcessId)")
        $pidToTab[$sh.ProcessId] = $tabIdx

        # Also map grandchildren (e.g., bash -> claude)
        $grandchildren = $allProcs | Where-Object { $_.ParentProcessId -eq $sh.ProcessId }
        foreach ($gc in $grandchildren) {
            $pidToTab[$gc.ProcessId] = $tabIdx
        }
    }
    $tabIdx++
}

# 5. Get UIA tabs
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
    Write-Host ("  UIA[$i] '$($tabs[$i].Current.Name)' selected=$($sel.Current.IsSelected)")
}

# 6. Cross-reference: does process creation order match UIA tab order?
Write-Host "`n=== Match Test ==="
Write-Host "OpenConsole count: $($consoles.Count) vs UIA tab count: $($tabs.Count)"

# 7. Test with current shell PID
$myShellPid = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
Write-Host "`nMy PID: $PID, parent shell PID: $myShellPid"
if ($pidToTab.ContainsKey($myShellPid)) {
    Write-Host "  -> Mapped to Tab[$($pidToTab[$myShellPid])]"
} else {
    Write-Host "  -> No mapping found for shell PID $myShellPid"
    # Try going up one more level
    $grandparent = ($allProcs | Where-Object { $_.ProcessId -eq $myShellPid }).ParentProcessId
    Write-Host "  -> Trying grandparent PID: $grandparent"
    if ($pidToTab.ContainsKey($grandparent)) {
        Write-Host "  -> Mapped to Tab[$($pidToTab[$grandparent])]"
    }
}
