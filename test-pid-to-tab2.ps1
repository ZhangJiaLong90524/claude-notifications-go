# Test: given a shell PID, find its WT window + tab index, then switch
param([int]$TargetShellPID = 51752)

Add-Type -AssemblyName UIAutomationClient

$allProcs = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CreationDate

# Step 1: Walk up from target shell to find which WT owns it
$pid2 = $TargetShellPID
$wtPid = -1
for ($depth = 0; $depth -lt 20; $depth++) {
    $proc = $allProcs | Where-Object { $_.ProcessId -eq $pid2 }
    if (-not $proc) { break }
    if ($proc.Name -eq 'WindowsTerminal.exe') {
        $wtPid = $proc.ProcessId
        break
    }
    $pid2 = $proc.ParentProcessId
}
if ($wtPid -lt 0) { Write-Host "ERROR: no WT ancestor for PID $TargetShellPID"; exit 1 }
Write-Host "Shell PID $TargetShellPID belongs to WT PID $wtPid"

# Step 2: Find the DIRECT shell child of WT that is our ancestor
# (our target might be pwsh, or it might be claude.exe under pwsh)
$pid2 = $TargetShellPID
$directChildPid = -1
for ($depth = 0; $depth -lt 20; $depth++) {
    $proc = $allProcs | Where-Object { $_.ProcessId -eq $pid2 }
    if (-not $proc) { break }
    if ($proc.ParentProcessId -eq $wtPid) {
        $directChildPid = $proc.ProcessId
        break
    }
    $pid2 = $proc.ParentProcessId
}
if ($directChildPid -lt 0) { Write-Host "ERROR: no direct WT child found"; exit 1 }
$directChild = $allProcs | Where-Object { $_.ProcessId -eq $directChildPid }
Write-Host "Direct WT child: $($directChild.Name) PID=$directChildPid"

# Step 3: List ALL direct shell children of this WT (not OpenConsole)
$shellNames = @('powershell.exe','pwsh.exe','cmd.exe','bash.exe','zsh.exe')
$shells = $allProcs | Where-Object {
    $_.ParentProcessId -eq $wtPid -and $_.Name -in $shellNames
} | Sort-Object CreationDate

Write-Host "`nAll shell children of WT $wtPid (sorted by creation):"
for ($i = 0; $i -lt $shells.Count; $i++) {
    $marker = if ($shells[$i].ProcessId -eq $directChildPid) { " <-- TARGET" } else { "" }
    Write-Host "  [$i] $($shells[$i].Name) PID=$($shells[$i].ProcessId) Created=$($shells[$i].CreationDate)$marker"
}

# Step 4: Find target's index
$tabIndex = -1
for ($i = 0; $i -lt $shells.Count; $i++) {
    if ($shells[$i].ProcessId -eq $directChildPid) { $tabIndex = $i; break }
}
if ($tabIndex -lt 0) { Write-Host "ERROR: target not in shell list"; exit 1 }
Write-Host "`nTarget tab index: $tabIndex"

# Step 5: Find UIA window for this specific WT PID
$root = [System.Windows.Automation.AutomationElement]::RootElement
$allWtWindows = $root.FindAll('Children',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ClassNameProperty,
        'CASCADIA_HOSTING_WINDOW_CLASS')))

Write-Host "`nWT UIA windows found: $($allWtWindows.Count)"
$targetWtWin = $null
foreach ($w in $allWtWindows) {
    $winPid = $w.Current.ProcessId
    Write-Host "  WT window PID=$winPid '$($w.Current.Name)'"
    if ($winPid -eq $wtPid) { $targetWtWin = $w }
}
if ($targetWtWin -eq $null) { Write-Host "ERROR: no UIA window for WT PID $wtPid"; exit 1 }

# Step 6: Enumerate tabs in that window
$tabs = $targetWtWin.FindAll('Descendants',
    (New-Object System.Windows.Automation.PropertyCondition(
        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
        [System.Windows.Automation.ControlType]::TabItem)))

Write-Host "`nUIA tabs in target window: $($tabs.Count)"
for ($i = 0; $i -lt $tabs.Count; $i++) {
    $sel = $tabs[$i].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern)
    Write-Host "  [$i] '$($tabs[$i].Current.Name)' selected=$($sel.Current.IsSelected)"
}

if ($tabIndex -ge $tabs.Count) {
    Write-Host "ERROR: tab index $tabIndex >= tab count $($tabs.Count)"
    exit 1
}

# Step 7: Switch to target tab
Write-Host "`nSwitching to tab[$tabIndex] '$($tabs[$tabIndex].Current.Name)'..."
$tabs[$tabIndex].GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select()
Write-Host "SUCCESS: tab switched"
