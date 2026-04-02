//go:build windows

// ABOUTME: Windows UI Automation bindings for tab switching in Windows Terminal.
// ABOUTME: Uses UIA COM to enumerate tabs and call SelectionItemPattern.Select().
package notifier

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"

	"github.com/777genius/claude-notifications/internal/logging"
)

// UIA COM GUIDs
var (
	clsidCUIAutomation = ole.NewGUID("FF48DBA4-60EF-4201-AA87-54103EEF594E")
	iidIUIAutomation   = ole.NewGUID("30CBE57D-D9D0-452A-AB13-7AC5AC4825EE")
)

// UIA ControlType ID for TabItem
const uiaControlTypeTabItem = 50019

// UIA property IDs
const (
	uiaPropertyControlType = 30003
)

// UIA TreeScope
const uiaScopeDescendants = 4

// ---------- IUIAutomation ----------

type iUIAutomation struct{ ole.IUnknown }
type iUIAutomationVtbl struct {
	ole.IUnknownVtbl
	// IUIAutomation has many methods; we only need a few.
	// Vtable slots (0-based after IUnknown's 3):
	CompareElements                         uintptr // 3
	CompareRuntimeIds                       uintptr // 4
	GetRootElement                          uintptr // 5
	ElementFromHandle                       uintptr // 6
	ElementFromPoint                        uintptr // 7
	GetFocusedElement                       uintptr // 8
	CreateTreeWalker                        uintptr // 9
	ControlViewWalker                       uintptr // 10
	ContentViewWalker                       uintptr // 11
	RawViewWalker                           uintptr // 12
	RawViewCondition                        uintptr // 13
	ControlViewCondition                    uintptr // 14
	ContentViewCondition                    uintptr // 15
	CreateCacheRequest                      uintptr // 16
	CreateTrueCondition                     uintptr // 17
	CreateFalseCondition                    uintptr // 18
	CreatePropertyCondition                 uintptr // 19
	CreatePropertyConditionEx               uintptr // 20
	CreateAndCondition                      uintptr // 21
	CreateAndConditionFromArray             uintptr // 22
	CreateAndConditionFromNativeArray       uintptr // 23
	CreateOrCondition                       uintptr // 24
	CreateOrConditionFromArray              uintptr // 25
	CreateOrConditionFromNativeArray        uintptr // 26
	CreateNotCondition                      uintptr // 27
	AddAutomationEventHandler               uintptr // 28
	RemoveAutomationEventHandler            uintptr // 29
	AddPropertyChangedEventHandlerNativeArr uintptr // 30
	RemovePropertyChangedEventHandler       uintptr // 31
	AddStructureChangedEventHandler         uintptr // 32
	RemoveStructureChangedEventHandler      uintptr // 33
	AddFocusChangedEventHandler             uintptr // 34
	RemoveFocusChangedEventHandler          uintptr // 35
	RemoveAllEventHandlers                  uintptr // 36
	IntNativeArrayToSafeArray               uintptr // 37
	IntSafeArrayToNativeArray               uintptr // 38
	RectToVariant                           uintptr // 39
	VariantToRect                           uintptr // 40
	SafeArrayToRectNativeArray              uintptr // 41
	CreateProxyFactoryEntry                 uintptr // 42
	ProxyFactoryMapping                     uintptr // 43
	GetPropertyProgrammaticName             uintptr // 44
	GetPatternProgrammaticName              uintptr // 45
	PollForPotentialSupportedPatterns       uintptr // 46
	PollForPotentialSupportedProperties     uintptr // 47
	CheckNotSupported                       uintptr // 48
	ReservedNotSupportedValue               uintptr // 49
	ReservedMixedAttributeValue             uintptr // 50
	// ... IUIAutomation continues with more methods but we don't need them
}

func (v *iUIAutomation) vt() *iUIAutomationVtbl {
	return (*iUIAutomationVtbl)(unsafe.Pointer(v.RawVTable))
}

var (
	procCoCreateInstance  = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procCoSetProxyBlanket = windows.NewLazySystemDLL("ole32.dll").NewProc("CoSetProxyBlanket")
)

func newUIAutomation() (*iUIAutomation, error) {
	// Prefer MTA — cross-process UIA marshaling requires either a message pump
	// (STA) or MTA. The focus binary has no message loop, so MTA is required.
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		oleErr, ok := err.(*ole.OleError)
		if ok && (oleErr.Code() == 1 || oleErr.Code() == 0x80010106) {
			logging.Debug("UIA: COM already initialized (code=0x%X), proceeding", oleErr.Code())
		} else {
			return nil, fmt.Errorf("CoInitializeEx: %w", err)
		}
	}
	var uia *iUIAutomation
	logging.Debug("_probe_ UIA: CoCreateInstance(CUIAutomation) calling...")
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsidCUIAutomation)),
		0,
		1|4, // CLSCTX_INPROC_SERVER | CLSCTX_LOCAL_SERVER
		uintptr(unsafe.Pointer(iidIUIAutomation)),
		uintptr(unsafe.Pointer(&uia)),
	)
	if hr != 0 {
		logging.Debug("_probe_ UIA: CoCreateInstance failed: HRESULT=0x%X", hr)
		return nil, ole.NewError(hr)
	}
	logging.Debug("_probe_ UIA: CoCreateInstance succeeded, uia=%p", uia)

	// Set impersonation level on the UIA proxy. When launched via ShellExecute
	// (protocol activation), implicit CoInitializeSecurity may use
	// RPC_C_IMP_LEVEL_IDENTIFY which prevents UIA from enumerating XAML Islands
	// content cross-process. IMPERSONATE allows the WT UIA provider to fully
	// service requests. CoSetProxyBlanket works even if CoInitializeSecurity
	// was already called (RPC_E_TOO_LATE).
	const (
		rpcCAuthnDefault       = 0xFFFFFFFF // RPC_C_AUTHN_DEFAULT
		rpcCAuthzDefault       = 0xFFFFFFFF // RPC_C_AUTHZ_DEFAULT
		rpcCAuthnLevelDefault  = 0
		rpcCImpLevelImpersonate = 3
		eoacNone               = 0
	)
	proxyHr, _, _ := procCoSetProxyBlanket.Call(
		uintptr(unsafe.Pointer(uia)),       // pProxy
		uintptr(rpcCAuthnDefault),          // dwAuthnSvc
		uintptr(rpcCAuthzDefault),          // dwAuthzSvc
		0,                                  // pServerPrincName (COLE_DEFAULT_PRINCIPAL)
		uintptr(rpcCAuthnLevelDefault),     // dwAuthnLevel
		uintptr(rpcCImpLevelImpersonate),   // dwImpLevel
		0,                                  // pAuthInfo
		uintptr(eoacNone),                  // dwCapabilities
	)
	if proxyHr != 0 {
		logging.Debug("_probe_ UIA: CoSetProxyBlanket HRESULT=0x%X (non-fatal)", proxyHr)
	} else {
		logging.Debug("_probe_ UIA: CoSetProxyBlanket OK (IMPERSONATE)")
	}

	return uia, nil
}

func (v *iUIAutomation) elementFromHandle(hwnd uintptr) (*iUIAutomationElement, error) {
	logging.Debug("_probe_ UIA: ElementFromHandle(0x%X) calling...", hwnd)
	var elem *iUIAutomationElement
	hr, _, _ := syscall.SyscallN(v.vt().ElementFromHandle,
		uintptr(unsafe.Pointer(v)),
		hwnd,
		uintptr(unsafe.Pointer(&elem)),
	)
	if hr != 0 {
		logging.Debug("_probe_ UIA: ElementFromHandle failed: HRESULT=0x%X", hr)
		return nil, ole.NewError(hr)
	}
	logging.Debug("_probe_ UIA: ElementFromHandle succeeded, elem=%p", elem)
	return elem, nil
}

func (v *iUIAutomation) createTrueCondition() (*iUIAutomationCondition, error) {
	var cond *iUIAutomationCondition
	hr, _, _ := syscall.SyscallN(v.vt().CreateTrueCondition,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&cond)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return cond, nil
}

// UIA property IDs for reading element properties
const uiaPropertyName = 30005

func (v *iUIAutomationElement) getCurrentPropertyValue(propertyId int32) (int32, error) {
	// Returns VARIANT; we only support VT_I4 for ControlType
	var variant [24]byte
	hr, _, _ := syscall.SyscallN(v.vt().GetCurrentPropertyValue,
		uintptr(unsafe.Pointer(v)),
		uintptr(propertyId),
		uintptr(unsafe.Pointer(&variant[0])),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	// VT_I4: type at offset 0, value at offset 8
	return *(*int32)(unsafe.Pointer(&variant[8])), nil
}

// ---------- IUIAutomationElement ----------

type iUIAutomationElement struct{ ole.IUnknown }
type iUIAutomationElementVtbl struct {
	ole.IUnknownVtbl
	SetFocus                        uintptr // 3
	GetRuntimeId                    uintptr // 4
	FindFirst                       uintptr // 5
	FindAll                         uintptr // 6
	FindFirstBuildCache             uintptr // 7
	FindAllBuildCache               uintptr // 8
	BuildUpdatedCache               uintptr // 9
	GetCurrentPropertyValue         uintptr // 10
	GetCurrentPropertyValueEx       uintptr // 11
	GetCachedPropertyValue          uintptr // 12
	GetCachedPropertyValueEx        uintptr // 13
	GetCurrentPatternAs             uintptr // 14
	GetCachedPatternAs              uintptr // 15
	GetCurrentPattern               uintptr // 16
	GetCachedPattern                uintptr // 17
	GetCurrentParent                uintptr // 18 (deprecated)
	GetCurrentChildren              uintptr // 19 (deprecated)
}

func (v *iUIAutomationElement) vt() *iUIAutomationElementVtbl {
	return (*iUIAutomationElementVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iUIAutomationElement) findAll(scope int32, condition *iUIAutomationCondition) (*iUIAutomationElementArray, error) {
	var arr *iUIAutomationElementArray
	hr, _, _ := syscall.SyscallN(v.vt().FindAll,
		uintptr(unsafe.Pointer(v)),
		uintptr(scope),
		uintptr(unsafe.Pointer(condition)),
		uintptr(unsafe.Pointer(&arr)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return arr, nil
}

// ISelectionItemPattern GUID
var iidISelectionItemPattern = ole.NewGUID("A8EFA66A-0FDA-421A-9194-38021F3578EA")

func (v *iUIAutomationElement) getCurrentPatternAsSelectionItem() (*iSelectionItemPattern, error) {
	var pat *iSelectionItemPattern
	hr, _, _ := syscall.SyscallN(v.vt().GetCurrentPatternAs,
		uintptr(unsafe.Pointer(v)),
		10010, // UIA_SelectionItemPatternId
		uintptr(unsafe.Pointer(iidISelectionItemPattern)),
		uintptr(unsafe.Pointer(&pat)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return pat, nil
}

// ---------- IUIAutomationElementArray ----------

type iUIAutomationElementArray struct{ ole.IUnknown }
type iUIAutomationElementArrayVtbl struct {
	ole.IUnknownVtbl
	GetLength  uintptr // 3 (property getter)
	GetElement uintptr // 4
}

func (v *iUIAutomationElementArray) vt() *iUIAutomationElementArrayVtbl {
	return (*iUIAutomationElementArrayVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iUIAutomationElementArray) length() (int32, error) {
	var length int32
	hr, _, _ := syscall.SyscallN(v.vt().GetLength,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&length)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return length, nil
}

func (v *iUIAutomationElementArray) getElement(index int32) (*iUIAutomationElement, error) {
	var elem *iUIAutomationElement
	hr, _, _ := syscall.SyscallN(v.vt().GetElement,
		uintptr(unsafe.Pointer(v)),
		uintptr(index),
		uintptr(unsafe.Pointer(&elem)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return elem, nil
}

// ---------- IUIAutomationCondition ----------

type iUIAutomationCondition struct{ ole.IUnknown }

// ---------- ISelectionItemPattern ----------

type iSelectionItemPattern struct{ ole.IUnknown }
type iSelectionItemPatternVtbl struct {
	ole.IUnknownVtbl
	Select              uintptr // 3
	AddToSelection      uintptr // 4
	RemoveFromSelection uintptr // 5
	GetIsSelected       uintptr // 6 (property getter: get_CurrentIsSelected)
}

func (v *iSelectionItemPattern) vt() *iSelectionItemPatternVtbl {
	return (*iSelectionItemPatternVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iSelectionItemPattern) selectItem() error {
	hr, _, _ := syscall.SyscallN(v.vt().Select,
		uintptr(unsafe.Pointer(v)),
	)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

func (v *iSelectionItemPattern) isSelected() (bool, error) {
	var selected int32 // BOOL
	hr, _, _ := syscall.SyscallN(v.vt().GetIsSelected,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&selected)),
	)
	if hr != 0 {
		return false, ole.NewError(hr)
	}
	return selected != 0, nil
}

// ---------- Public API ----------

// findTabItems enumerates all TabItem descendants of the given UIA element.
// Uses CreateTrueCondition + manual ControlType filtering (avoids VARIANT
// marshalling issues with CreatePropertyCondition over SyscallN).
func findTabItems(uia *iUIAutomation, root *iUIAutomationElement) ([]*iUIAutomationElement, error) {
	cond, err := uia.createTrueCondition()
	if err != nil {
		return nil, fmt.Errorf("CreateTrueCondition: %w", err)
	}
	defer cond.Release()

	arr, err := root.findAll(uiaScopeDescendants, cond)
	if err != nil {
		return nil, fmt.Errorf("FindAll: %w", err)
	}
	defer arr.Release()

	count, _ := arr.length()
	logging.Debug("_probe_ UIA: FindAll(Descendants, TrueCondition) returned %d elements", count)
	var tabs []*iUIAutomationElement
	var _probe_ctSamples []int32
	for i := int32(0); i < count; i++ {
		elem, err := arr.getElement(i)
		if err != nil {
			logging.Debug("_probe_ UIA: GetElement(%d) failed: %v", i, err)
			continue
		}
		ct, err := elem.getCurrentPropertyValue(uiaPropertyControlType)
		if err != nil {
			logging.Debug("_probe_ UIA: GetCurrentPropertyValue(ControlType) failed for elem[%d]: %v", i, err)
			elem.Release()
			continue
		}
		if len(_probe_ctSamples) < 10 {
			_probe_ctSamples = append(_probe_ctSamples, ct)
		}
		if ct == uiaControlTypeTabItem {
			tabs = append(tabs, elem)
		} else {
			elem.Release()
		}
	}
	logging.Debug("_probe_ UIA: found %d TabItem (id=%d) out of %d descendants. First ControlTypes: %v", len(tabs), uiaControlTypeTabItem, count, _probe_ctSamples)
	return tabs, nil
}

// getSelectedTabIndex returns the 0-based index of the currently selected tab
// in the Windows Terminal window identified by hwnd. Returns -1 if tabs cannot
// be enumerated (e.g., non-WT window, single-tab window with no tab bar).
func getSelectedTabIndex(hwnd uintptr) int {
	uia, err := newUIAutomation()
	if err != nil {
		logging.Debug("UIA init failed: %v", err)
		return -1
	}
	defer uia.Release()

	elem, err := uia.elementFromHandle(hwnd)
	if err != nil {
		logging.Debug("UIA ElementFromHandle failed: %v", err)
		return -1
	}
	defer elem.Release()

	tabs, err := findTabItems(uia, elem)
	if err != nil {
		logging.Debug("UIA findTabItems failed: %v", err)
		return -1
	}
	defer func() {
		for _, t := range tabs {
			t.Release()
		}
	}()

	for i, tab := range tabs {
		pat, err := tab.getCurrentPatternAsSelectionItem()
		if err != nil {
			continue
		}
		selected, err := pat.isSelected()
		pat.Release()
		if err == nil && selected {
			logging.Debug("UIA selected tab index: %d (of %d)", i, len(tabs))
			return i
		}
	}

	logging.Debug("UIA no selected tab found (count=%d)", len(tabs))
	return -1
}

// selectTab switches to the tab at the given index in the Windows Terminal
// window identified by hwnd. Returns an error if the tab cannot be selected.
func selectTab(hwnd uintptr, tabIndex int) error {
	logging.Debug("_probe_ selectTab: hwnd=0x%X tabIndex=%d, locking OS thread", hwnd, tabIndex)
	// Lock to OS thread for COM apartment safety in GUI subsystem binary
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	uia, err := newUIAutomation()
	if err != nil {
		return fmt.Errorf("UIA init: %w", err)
	}
	defer uia.Release()

	elem, err := uia.elementFromHandle(hwnd)
	if err != nil {
		return fmt.Errorf("UIA ElementFromHandle: %w", err)
	}
	defer elem.Release()

	tabs, err := findTabItems(uia, elem)
	if err != nil {
		return fmt.Errorf("UIA findTabItems: %w", err)
	}
	defer func() {
		for _, t := range tabs {
			t.Release()
		}
	}()

	if tabIndex >= len(tabs) {
		return fmt.Errorf("tab index %d out of range (count=%d)", tabIndex, len(tabs))
	}

	pat, err := tabs[tabIndex].getCurrentPatternAsSelectionItem()
	if err != nil {
		return fmt.Errorf("UIA SelectionItemPattern: %w", err)
	}
	defer pat.Release()

	if err := pat.selectItem(); err != nil {
		return fmt.Errorf("UIA Select: %w", err)
	}

	logging.Debug("UIA tab switched to index %d (of %d)", tabIndex, len(tabs))
	return nil
}
