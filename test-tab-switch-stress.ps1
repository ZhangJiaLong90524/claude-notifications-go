# Stress test: switch to every tab and back, verify correctness each time
param([long]$HWND = 0xC02C4)

Add-Type -AssemblyName UIAutomationClient

$hwndPtr = [IntPtr]$HWND
$wtWin = [System.Windows.Automation.AutomationElement]::FromHandle($hwndPtr)
Write-Host "Window: '$($wtWin.Current.Name)'"

$tabCond = New-Object System.Windows.Automation.PropertyCondition(
    [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
    [System.Windows.Automation.ControlType]::TabItem)

function Get-Tabs { $wtWin.FindAll('Descendants', $tabCond) }
function Get-SelectedIndex($tabs) {
    for ($i = 0; $i -lt $tabs.Count; $i++) {
        $sel = $tabs[$i].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
        if ($sel.Current.IsSelected) { return $i }
    }
    return -1
}

$tabs = Get-Tabs
$tabCount = $tabs.Count
$originalIdx = Get-SelectedIndex $tabs
Write-Host "Tab count: $tabCount, starting at: [$originalIdx] '$($tabs[$originalIdx].Current.Name)'"
Write-Host ""

# Test 1: Visit every tab sequentially
Write-Host "=== Test 1: Sequential visit ==="
$pass = 0; $fail = 0
for ($target = 0; $target -lt $tabCount; $target++) {
    $tabs[$target].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Start-Sleep -Milliseconds 200
    $tabs2 = Get-Tabs
    $actual = Get-SelectedIndex $tabs2
    $ok = ($actual -eq $target)
    $status = if ($ok) { "OK"; $pass++ } else { "FAIL (got $actual)"; $fail++ }
    Write-Host "  Switch to [$target] '$($tabs2[$target].Current.Name)': $status"
}
Write-Host "  Result: $pass/$tabCount passed"
Write-Host ""

# Test 2: Random jumps
Write-Host "=== Test 2: Random jumps (20 iterations) ==="
$pass = 0; $fail = 0
$rng = New-Object System.Random
for ($iter = 0; $iter -lt 20; $iter++) {
    $target = $rng.Next($tabCount)
    $tabs = Get-Tabs
    $tabs[$target].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    Start-Sleep -Milliseconds 150
    $tabs2 = Get-Tabs
    $actual = Get-SelectedIndex $tabs2
    $ok = ($actual -eq $target)
    $status = if ($ok) { "OK" ; $pass++ } else { "FAIL (got $actual)"; $fail++ }
    Write-Host "  [$iter] -> tab[$target]: $status"
}
Write-Host "  Result: $pass/20 passed"
Write-Host ""

# Test 3: Rapid back-and-forth (no sleep)
Write-Host "=== Test 3: Rapid toggle 0<->last (10x, no delay) ==="
$pass = 0; $fail = 0
$lastIdx = $tabCount - 1
for ($iter = 0; $iter -lt 10; $iter++) {
    $target = if ($iter % 2 -eq 0) { $lastIdx } else { 0 }
    $tabs = Get-Tabs
    $tabs[$target].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
    $tabs2 = Get-Tabs
    $actual = Get-SelectedIndex $tabs2
    $ok = ($actual -eq $target)
    if ($ok) { $pass++ } else { $fail++ }
}
Write-Host "  Result: $pass/10 passed"
Write-Host ""

# Restore original tab
$tabs = Get-Tabs
$tabs[$originalIdx].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
Write-Host "Restored to tab[$originalIdx]"
Write-Host "=== All tests complete ==="
