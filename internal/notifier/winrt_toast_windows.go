//go:build windows

// ABOUTME: Direct WinRT COM bindings for Windows Toast notifications.
// ABOUTME: Eliminates PowerShell process spawn (~300ms) by calling WinRT APIs in-process via go-ole.
package notifier

// Interface definitions are derived from go-toast's code-generated winrt wrappers
// (git.sr.ht/~jackmordaunt/go-toast, MIT license) and Windows SDK IDL headers.
// Only the toast display pipeline is included — no CoRegisterClassObject or
// CustomActivator, which would break protocol activation.
//
// Uses IToastNotificationManagerStatics (the same interface PowerShell uses)
// rather than the newer IToastNotificationManagerStatics5.GetDefault() path,
// to ensure identical activation behavior.

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"

	"github.com/777genius/claude-notifications/internal/logging"
)

var procWindowsCreateString = windows.NewLazySystemDLL("combase.dll").NewProc("WindowsCreateString")

// newHStringUTF16 creates an HString with correct UTF-16 length.
// go-ole's NewHString uses utf8.RuneCountInString (rune count) as the length
// parameter, but WindowsCreateString expects UTF-16 code unit count. For
// supplementary plane characters (emoji like 📝 U+1F4DD), the rune count is
// less than the UTF-16 code unit count (surrogate pairs = 2 code units per rune),
// causing the HString to be truncated and XML parsing to fail.
func newHStringUTF16(s string) (ole.HString, error) {
	u16 := windows.StringToUTF16(s)
	// StringToUTF16 appends a null terminator; length excludes it
	u16Len := len(u16) - 1
	if u16Len < 0 {
		u16Len = 0
	}
	var h ole.HString
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(u16Len),
		uintptr(unsafe.Pointer(&h)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return h, nil
}

// WinRT COM interface GUIDs from Windows SDK IDL.
const (
	guidIXmlDocumentIO                    = "6cd0e74e-ee65-4489-9ebf-ca43e87ba637"
	guidIToastNotificationManagerStatics  = "50ac103f-d235-4598-bbef-98fe4d1a3ad4"
	guidIToastNotificationFactory         = "04124b20-82c6-4229-b109-fd9ed4662b53"
	guidIToastNotifier                    = "75927b93-03f3-41ec-91d3-6e5bac1b38e7"
)

// ---------- XmlDocument ----------

type xmlDocument struct{ ole.IUnknown }

func newXmlDocument() (*xmlDocument, error) {
	inspectable, err := ole.RoActivateInstance("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return nil, err
	}
	return (*xmlDocument)(unsafe.Pointer(inspectable)), nil
}

func (d *xmlDocument) loadXml(xml string) error {
	itf := d.MustQueryInterface(ole.NewGUID(guidIXmlDocumentIO))
	defer itf.Release()
	v := (*iXmlDocumentIO)(unsafe.Pointer(itf))
	return v.loadXml(xml)
}

type iXmlDocumentIO struct{ ole.IInspectable }
type iXmlDocumentIOVtbl struct {
	ole.IInspectableVtbl
	LoadXml             uintptr
	LoadXmlWithSettings uintptr
	SaveToFileAsync     uintptr
}

func (v *iXmlDocumentIO) vt() *iXmlDocumentIOVtbl {
	return (*iXmlDocumentIOVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iXmlDocumentIO) loadXml(xml string) error {
	h, err := newHStringUTF16(xml)
	if err != nil {
		return err
	}
	defer ole.DeleteHString(h)
	hr, _, _ := syscall.SyscallN(v.vt().LoadXml,
		uintptr(unsafe.Pointer(v)),
		uintptr(h),
	)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

// ---------- ToastNotificationManager (static API, same as PowerShell) ----------

// IToastNotificationManagerStatics — the original static interface used by
// [ToastNotificationManager]::CreateToastNotifier($appID) in PowerShell.
// This is preferred over IToastNotificationManagerStatics5.GetDefault() to
// ensure identical activation behavior with protocol activation.
type iToastNotificationManagerStatics struct{ ole.IInspectable }
type iToastNotificationManagerStaticsVtbl struct {
	ole.IInspectableVtbl
	CreateToastNotifier       uintptr // slot 6: CreateToastNotifier() -> ToastNotifier
	CreateToastNotifierWithId uintptr // slot 7: CreateToastNotifier(appID) -> ToastNotifier
	GetTemplateContent        uintptr // slot 8
}

func (v *iToastNotificationManagerStatics) vt() *iToastNotificationManagerStaticsVtbl {
	return (*iToastNotificationManagerStaticsVtbl)(unsafe.Pointer(v.RawVTable))
}

// createToastNotifierForApp mirrors PowerShell's
// [ToastNotificationManager]::CreateToastNotifier($appID)
func createToastNotifierForApp(appID string) (*toastNotifier, error) {
	inspectable, err := ole.RoGetActivationFactory(
		"Windows.UI.Notifications.ToastNotificationManager",
		ole.NewGUID(guidIToastNotificationManagerStatics),
	)
	if err != nil {
		return nil, err
	}
	defer inspectable.Release()
	v := (*iToastNotificationManagerStatics)(unsafe.Pointer(inspectable))

	h, err := newHStringUTF16(appID)
	if err != nil {
		return nil, err
	}
	defer ole.DeleteHString(h)

	var out *toastNotifier
	hr, _, _ := syscall.SyscallN(v.vt().CreateToastNotifierWithId,
		uintptr(unsafe.Pointer(v)),
		uintptr(h),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ---------- ToastNotification ----------

type toastNotification struct{ ole.IUnknown }

func createToastNotification(doc *xmlDocument) (*toastNotification, error) {
	inspectable, err := ole.RoGetActivationFactory(
		"Windows.UI.Notifications.ToastNotification",
		ole.NewGUID(guidIToastNotificationFactory),
	)
	if err != nil {
		return nil, err
	}
	defer inspectable.Release()
	v := (*iToastNotificationFactory)(unsafe.Pointer(inspectable))

	var out *toastNotification
	hr, _, _ := syscall.SyscallN(v.vt().CreateToastNotification,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(doc)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

type iToastNotificationFactory struct{ ole.IInspectable }
type iToastNotificationFactoryVtbl struct {
	ole.IInspectableVtbl
	CreateToastNotification uintptr
}

func (v *iToastNotificationFactory) vt() *iToastNotificationFactoryVtbl {
	return (*iToastNotificationFactoryVtbl)(unsafe.Pointer(v.RawVTable))
}

// ---------- ToastNotifier ----------

type toastNotifier struct{ ole.IUnknown }

func (n *toastNotifier) show(toast *toastNotification) error {
	itf := n.MustQueryInterface(ole.NewGUID(guidIToastNotifier))
	defer itf.Release()
	v := (*iToastNotifier)(unsafe.Pointer(itf))
	return v.show(toast)
}

type iToastNotifier struct{ ole.IInspectable }
type iToastNotifierVtbl struct {
	ole.IInspectableVtbl
	Show                           uintptr
	Hide                           uintptr
	GetSetting                     uintptr
	AddToSchedule                  uintptr
	RemoveFromSchedule             uintptr
	GetScheduledToastNotifications uintptr
}

func (v *iToastNotifier) vt() *iToastNotifierVtbl {
	return (*iToastNotifierVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iToastNotifier) show(notification *toastNotification) error {
	hr, _, _ := syscall.SyscallN(v.vt().Show,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(notification)),
	)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

// ---------- pushToastCOM ----------

var (
	comOnce    sync.Once
	comInitErr error
)

// pushToastCOM sends a toast notification via direct WinRT COM calls.
// No PowerShell process spawn, no CoRegisterClassObject, no CustomActivator —
// protocol activation remains intact.
//
// Uses the same IToastNotificationManagerStatics.CreateToastNotifier(appID)
// code path as PowerShell to ensure identical activation behavior.
//
// Designed for single-goroutine hook usage. RoInitialize(MTA) is process-wide
// so concurrent goroutines would also be safe, but this is not the expected
// usage pattern.
func pushToastCOM(toastXML, appID string) (err error) {
	// Recover from panics (Windows 7 WinRT stubs can panic via MustQueryInterface)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("WinRT COM panic: %v", r)
		}
	}()

	comOnce.Do(func() {
		comInitErr = ole.RoInitialize(1)
	})
	if comInitErr != nil {
		return fmt.Errorf("RoInitialize: %w", comInitErr)
	}

	doc, err := newXmlDocument()
	if err != nil {
		return fmt.Errorf("XmlDocument activate: %w", err)
	}
	defer doc.Release()

	if err := doc.loadXml(toastXML); err != nil {
		return fmt.Errorf("XmlDocument LoadXml: %w", err)
	}

	notifier, err := createToastNotifierForApp(appID)
	if err != nil {
		return fmt.Errorf("CreateToastNotifier(%q): %w", appID, err)
	}
	defer notifier.Release()

	toast, err := createToastNotification(doc)
	if err != nil {
		return fmt.Errorf("CreateToastNotification: %w", err)
	}
	defer toast.Release()

	if err := notifier.show(toast); err != nil {
		return fmt.Errorf("ToastNotifier.Show: %w", err)
	}

	logging.Debug("Toast sent via WinRT COM (appID=%s)", appID)
	return nil
}
