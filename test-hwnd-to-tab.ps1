# Test: use HWND to find the exact WT window, then get selected tab index
param([long]$HWND = 0xC02C4)  # from getTerminalHWND() log: 787140 = 0xC02C4

Add-Type -AssemblyName UIAutomationClient

Write-Host "Target HWND: 0x$($HWND.ToString('X'))"

# Step 1: Get UIA element from HWND
try {
    $hwndPtr = [IntPtr]$HWND
    $wtWin = [System.Windows.Automation.AutomationElement]::FromHandle($hwndPtr)
    Write-Host "UIA window: '$($wtWin.Current.Name)' PID=$($wtWin.Current.ProcessId)"
} catch {
    Write-Host "ERROR: FromHandle failed: $_"
    exit 1
}

# Step 2: Enumerate tabs
$tabs = $wtWin.FindAll('Descendants',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
        [System.Windows.Automation.ControlType]::TabItem)))

Write-Host "Tab count: $($tabs.Count)"
$selectedIndex = -1
for ($i = 0; $i -lt $tabs.Count; $i++) {
    $sel = $tabs[$i].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
    $marker = ""
    if ($sel.Current.IsSelected) {
        $selectedIndex = $i
        $marker = " <-- SELECTED"
    }
    Write-Host "  [$i] '$($tabs[$i].Current.Name)'$marker"
}
Write-Host "`nSelected tab index: $selectedIndex"

# Step 3: Test switching - go to another tab and back
if ($tabs.Count -gt 1 -and $selectedIndex -ge 0) {
    $otherIdx = if ($selectedIndex -eq 0) { 1 } else { 0 }
    Write-Host "`nSwitching to tab[$otherIdx]..."
    $tabs[$otherIdx].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Start-Sleep -Milliseconds 500

    Write-Host "Switching back to tab[$selectedIndex]..."
    $tabs[$selectedIndex].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Write-Host "Done - round-trip switch complete"
}
