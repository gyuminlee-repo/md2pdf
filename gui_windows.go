//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
)

// ── Win32 DLL imports ────────────────────────────────────────

var (
	comdlg32         = syscall.NewLazyDLL("comdlg32.dll")
	pGetOpenFileName = comdlg32.NewProc("GetOpenFileNameW")

	shell32              = syscall.NewLazyDLL("shell32.dll")
	pSHBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	pDragQueryFileW      = shell32.NewProc("DragQueryFileW")

	ole32               = syscall.NewLazyDLL("ole32.dll")
	pCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree      = ole32.NewProc("CoTaskMemFree")
	pOleInitialize      = ole32.NewProc("OleInitialize")
	pRegisterDragDrop   = ole32.NewProc("RegisterDragDrop")
	pRevokeDragDrop     = ole32.NewProc("RevokeDragDrop")
	pReleaseStgMedium   = ole32.NewProc("ReleaseStgMedium")

	user32Lib         = syscall.NewLazyDLL("user32.dll")
	pEnumChildWindows = user32Lib.NewProc("EnumChildWindows")
)

// ── Win32 types for file dialog ──────────────────────────────

type openFileName struct {
	StructSize      uint32
	Owner           syscall.Handle
	Instance        syscall.Handle
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

const (
	ofnExplorer         = 0x00080000
	ofnFileMustExist    = 0x00001000
	ofnAllowMultiSelect = 0x00000200
)

// ── Win32 types for folder dialog ─────────────────────────────

type browseInfo struct {
	Owner        syscall.Handle
	Root         uintptr
	DisplayName  *uint16
	Title        *uint16
	Flags        uint32
	CallbackFunc uintptr
	LParam       uintptr
	Image        int32
}

const (
	bifReturnonlyfsdirs     = 0x00000001
	bifNewdialogstyle       = 0x00000040
	coinitApartmentthreaded = 0x2
)

// ── OLE IDropTarget COM types ─────────────────────────────────

const (
	cfHDROP         = 15
	dvaspectContent = 1
	tymedHGlobal    = 1
	dropEffectCopy  = 1
	sOK             = 0
)

// FORMATETC matches the C layout on AMD64.
type formatETC struct {
	cfFormat uint16
	ptd      uintptr
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// STGMEDIUM matches the C layout on AMD64.
type stgMedium struct {
	tymed          uint32
	unionMember    uintptr // hGlobal for TYMED_HGLOBAL
	pUnkForRelease uintptr
}

// iDropTargetVtbl is the COM vtable for IDropTarget.
type iDropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

// myDropTarget implements IDropTarget. The first field must be the vtable pointer.
type myDropTarget struct {
	lpVtbl uintptr
	ref    int32
}

var (
	dtVtbl    iDropTargetVtbl
	dtVtblPtr uintptr // set once in initDropTarget
)

func initDropTarget() *myDropTarget {
	dtVtbl = iDropTargetVtbl{
		QueryInterface: syscall.NewCallback(dtQueryInterface),
		AddRef:         syscall.NewCallback(dtAddRef),
		Release:        syscall.NewCallback(dtRelease),
		DragEnter:      syscall.NewCallback(dtDragEnter),
		DragOver:       syscall.NewCallback(dtDragOver),
		DragLeave:      syscall.NewCallback(dtDragLeave),
		Drop:           syscall.NewCallback(dtDrop),
	}
	dtVtblPtr = uintptr(unsafe.Pointer(&dtVtbl))

	dt := &myDropTarget{
		lpVtbl: dtVtblPtr,
		ref:    1,
	}
	return dt
}

func dtQueryInterface(this, riid, ppvObject uintptr) uintptr {
	// Accept any interface query — we only implement IDropTarget.
	*(*uintptr)(unsafe.Pointer(ppvObject)) = this
	dtAddRef(this)
	return sOK
}

func dtAddRef(this uintptr) uintptr {
	dt := (*myDropTarget)(unsafe.Pointer(this))
	dt.ref++
	return uintptr(dt.ref)
}

func dtRelease(this uintptr) uintptr {
	dt := (*myDropTarget)(unsafe.Pointer(this))
	dt.ref--
	return uintptr(dt.ref)
}

func dtDragEnter(this, pDataObj, grfKeyState, pt, pdwEffect uintptr) uintptr {
	*(*uint32)(unsafe.Pointer(pdwEffect)) = dropEffectCopy
	if globalWebView != nil {
		globalWebView.Dispatch(func() {
			globalWebView.Eval("document.getElementById('dropzone').classList.add('drag-over')")
		})
	}
	return sOK
}

func dtDragOver(this, grfKeyState, pt, pdwEffect uintptr) uintptr {
	*(*uint32)(unsafe.Pointer(pdwEffect)) = dropEffectCopy
	return sOK
}

func dtDragLeave(this uintptr) uintptr {
	if globalWebView != nil {
		globalWebView.Dispatch(func() {
			globalWebView.Eval("document.getElementById('dropzone').classList.remove('drag-over')")
		})
	}
	return sOK
}

func dtDrop(this, pDataObj, grfKeyState, pt, pdwEffect uintptr) uintptr {
	*(*uint32)(unsafe.Pointer(pdwEffect)) = dropEffectCopy

	if globalWebView != nil {
		globalWebView.Dispatch(func() {
			globalWebView.Eval("document.getElementById('dropzone').classList.remove('drag-over')")
		})
	}

	// Call IDataObject::GetData(CF_HDROP) to get file paths.
	fmtetc := formatETC{
		cfFormat: cfHDROP,
		dwAspect: dvaspectContent,
		lindex:   -1,
		tymed:    tymedHGlobal,
	}
	var stgmed stgMedium

	// IDataObject vtable: [0]QI [1]AddRef [2]Release [3]GetData ...
	vtblPtr := *(*uintptr)(unsafe.Pointer(pDataObj))
	getDataFn := *(*uintptr)(unsafe.Pointer(vtblPtr + 3*unsafe.Sizeof(uintptr(0))))

	ret, _, _ := syscall.SyscallN(getDataFn, pDataObj,
		uintptr(unsafe.Pointer(&fmtetc)),
		uintptr(unsafe.Pointer(&stgmed)))

	if ret != 0 {
		return sOK
	}

	// Extract file paths from HDROP (= HGLOBAL containing DROPFILES struct)
	hDrop := stgmed.unionMember
	extractDroppedFiles(hDrop)

	// Release the storage medium (do NOT call DragFinish — that's for WM_DROPFILES only)
	pReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stgmed)))

	return sOK
}

// extractDroppedFiles reads file paths from an HDROP handle and adds them to the queue.
func extractDroppedFiles(hDrop uintptr) {
	count, _, _ := pDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)

	added := 0
	for i := uintptr(0); i < count; i++ {
		nameLen, _, _ := pDragQueryFileW.Call(hDrop, i, 0, 0)
		if nameLen == 0 {
			continue
		}

		buf := make([]uint16, nameLen+1)
		pDragQueryFileW.Call(hDrop, i, uintptr(unsafe.Pointer(&buf[0])), nameLen+1)
		runtime.KeepAlive(buf)

		path := syscall.UTF16ToString(buf)
		if isMarkdownFile(path) {
			if addToQueue(path) {
				added++
			}
		}
	}

	if globalWebView != nil && added > 0 {
		jsonBytes, err := json.Marshal(fileQueue)
		if err != nil {
			return
		}
		js := fmt.Sprintf(
			"files = %s; renderFiles(); setStatus(files.length + '개 파일 준비됨');",
			string(jsonBytes),
		)
		globalWebView.Dispatch(func() {
			globalWebView.Eval(js)
		})
	}
}

// registerDropTargetOnChildren replaces WebView2's OLE drop targets with ours.
func registerDropTargetOnChildren(parent uintptr, dt *myDropTarget) {
	dtPtr := uintptr(unsafe.Pointer(dt))

	enumCb := syscall.NewCallback(func(child, lParam uintptr) uintptr {
		// Revoke WebView2's built-in OLE drop target
		pRevokeDragDrop.Call(child)
		// Register our IDropTarget that extracts real file paths
		pRegisterDragDrop.Call(child, dtPtr)
		return 1 // continue
	})
	pEnumChildWindows.Call(parent, enumCb, 0)

	// Also register on parent window itself
	pRevokeDragDrop.Call(parent)
	pRegisterDragDrop.Call(parent, dtPtr)
}

// ── Global state ─────────────────────────────────────────────

var (
	fileQueue     []string
	fileQueueSet  map[string]bool
	lastBrowseDir string
	outputDir     string
	dropTempDir   string // temp dir for JS drag-drop fallback

	globalWebView webview2.WebView
)

// ── Helpers ──────────────────────────────────────────────────

func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func toPDFName(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + ".pdf"
}

func addToQueue(path string) bool {
	lookupKey := strings.ToLower(path)
	if fileQueueSet[lookupKey] {
		return false
	}
	fileQueueSet[lookupKey] = true
	fileQueue = append(fileQueue, path)
	return true
}

func removeFromQueue(path string) {
	lookupKey := strings.ToLower(path)
	delete(fileQueueSet, lookupKey)
	for i, q := range fileQueue {
		if q == path {
			fileQueue = append(fileQueue[:i], fileQueue[i+1:]...)
			break
		}
	}
}

func clearQueue() {
	fileQueue = nil
	fileQueueSet = make(map[string]bool)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ── Folder dialog (Win32 SHBrowseForFolder) ──────────────────

func browseFolderDialog(title string) string {
	pCoInitializeEx.Call(0, uintptr(coinitApartmentthreaded))

	displayName := make([]uint16, syscall.MAX_PATH)
	titlePtr := syscall.StringToUTF16Ptr(title)

	bi := browseInfo{
		DisplayName: &displayName[0],
		Title:       titlePtr,
		Flags:       bifReturnonlyfsdirs | bifNewdialogstyle,
	}

	pidl, _, _ := pSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(bi)
	runtime.KeepAlive(displayName)
	runtime.KeepAlive(titlePtr)

	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)

	pathBuf := make([]uint16, syscall.MAX_PATH)
	ok, _, _ := pSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	runtime.KeepAlive(pathBuf)

	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(pathBuf)
}

// ── Entry point ──────────────────────────────────────────────

func runGUI() {
	runtime.LockOSThread()

	// Initialize OLE (required for RegisterDragDrop). Must be called before
	// WebView2 creation so it sets COM to STA mode.
	pOleInitialize.Call(0)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "md2pdf",
			Width:  1040,
			Height: 1120,
			Center: true,
		},
	})
	if w == nil {
		fmt.Fprintln(os.Stderr, "WebView2 runtime not found. Please install Microsoft Edge WebView2 Runtime.")
		os.Exit(1)
	}
	defer w.Destroy()

	w.SetSize(1040, 1120, webview2.HintFixed)

	fileQueueSet = make(map[string]bool)
	globalWebView = w

	// Bind Go functions to JS
	w.Bind("_browseFiles", func() []string {
		return handleBrowse()
	})

	w.Bind("_convertFiles", func(paths []string, theme string) map[string]interface{} {
		return handleConvert(paths, theme, w)
	})

	w.Bind("_getVersion", func() string {
		return version
	})

	w.Bind("_getThemes", func() []ThemeEntry {
		return ThemeList()
	})

	w.Bind("_removeFile", func(key string) []string {
		removeFromQueue(key)
		return fileQueue
	})

	w.Bind("_clearFiles", func() {
		clearQueue()
	})

	w.Bind("_getFileQueue", func() []string {
		return fileQueue
	})

	// JS drag-drop fallback: if native IDropTarget fails, JS reads file
	// content and Go saves it to a temp directory.
	w.Bind("_addDroppedFile", func(name, content string) map[string]string {
		if dropTempDir == "" {
			dir, err := os.MkdirTemp("", "md2pdf-drop-*")
			if err != nil {
				return map[string]string{"status": "error", "msg": err.Error()}
			}
			dropTempDir = dir
		}

		path := filepath.Join(dropTempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return map[string]string{"status": "error", "msg": err.Error()}
		}

		if addToQueue(path) {
			return map[string]string{"status": "added", "path": path}
		}
		return map[string]string{"status": "duplicate", "path": path}
	})

	// Output directory bindings
	w.Bind("_browseOutputDir", func() string {
		dir := browseFolderDialog("PDF 출력 폴더를 선택하세요")
		if dir != "" {
			outputDir = dir
		}
		return outputDir
	})

	w.Bind("_clearOutputDir", func() {
		outputDir = ""
	})

	w.Bind("_getOutputDir", func() string {
		return outputDir
	})

	// Create our IDropTarget COM object
	dt := initDropTarget()

	// Bind a function that JS calls after the page loads, so we can
	// replace WebView2's drop targets with ours (child windows exist by then).
	w.Bind("_registerNativeDrop", func() {
		hwnd := uintptr(w.Window())
		registerDropTargetOnChildren(hwnd, dt)
	})

	// Pre-load CLI args (drag onto exe)
	var initialFiles []string
	if len(os.Args) > 1 {
		for _, a := range os.Args[1:] {
			if isMarkdownFile(a) {
				if addToQueue(a) {
					initialFiles = append(initialFiles, a)
				}
			}
		}
	}

	// Register init scripts BEFORE SetHtml
	if len(initialFiles) > 0 {
		jsonBytes, err := json.Marshal(initialFiles)
		if err == nil {
			w.Init(fmt.Sprintf("window._initialFiles = %s;", string(jsonBytes)))
		}
	}

	// Delay native drop registration until WebView2 child windows are created
	w.Init("setTimeout(function(){ try { _registerNativeDrop(); } catch(e) {} }, 800);")

	// Set HTML UI
	w.SetHtml(buildHTML())

	w.Run()
}

// ── File dialog (Win32) ──────────────────────────────────────

func handleBrowse() []string {
	filter := buildFilter(
		"Markdown", "*.md;*.markdown;*.txt",
		"All Files", "*.*",
	)

	fileBuf := make([]uint16, 65536)
	title := syscall.StringToUTF16Ptr("Markdown 파일 선택")

	ofn := openFileName{
		StructSize: uint32(unsafe.Sizeof(openFileName{})),
		Filter:     &filter[0],
		File:       &fileBuf[0],
		MaxFile:    uint32(len(fileBuf)),
		Title:      title,
		Flags:      ofnExplorer | ofnFileMustExist | ofnAllowMultiSelect,
	}

	ret, _, _ := pGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(fileBuf)
	runtime.KeepAlive(title)

	if ret == 0 {
		return nil
	}

	parts := splitNulls(fileBuf)
	var paths []string
	if len(parts) == 1 {
		paths = append(paths, parts[0])
	} else if len(parts) > 1 {
		dir := parts[0]
		for _, name := range parts[1:] {
			paths = append(paths, filepath.Join(dir, name))
		}
	}

	if len(paths) > 0 {
		lastBrowseDir = filepath.Dir(paths[0])
	}

	for _, p := range paths {
		addToQueue(p)
	}

	return fileQueue
}

// ── Convert handler ──────────────────────────────────────────

func handleConvert(paths []string, theme string, w webview2.WebView) map[string]interface{} {
	if len(paths) == 0 {
		return map[string]interface{}{
			"ok":   0,
			"fail": 0,
			"msg":  "변환할 파일이 없습니다.",
		}
	}

	ok, fail := 0, 0

	for i, in := range paths {
		displayName := filepath.Base(in)
		idx, total, dn := i+1, len(paths), displayName
		w.Dispatch(func() {
			w.Eval(fmt.Sprintf(
				"window._updateProgress(%d, %d, %s)",
				idx, total, jsonString(dn),
			))
		})

		out := toPDFName(in)
		if outputDir != "" {
			out = filepath.Join(outputDir, filepath.Base(out))
		}
		if err := ConvertFile(in, out, theme); err != nil {
			fail++
		} else {
			ok++
		}
	}

	msg := fmt.Sprintf("%d개 변환 완료", ok)
	if fail > 0 {
		msg += fmt.Sprintf(", %d개 실패", fail)
	}

	return map[string]interface{}{
		"ok":   ok,
		"fail": fail,
		"msg":  msg,
	}
}

// ── Win32 helpers ────────────────────────────────────────────

func buildFilter(pairs ...string) []uint16 {
	var buf []uint16
	for _, s := range pairs {
		for _, r := range s {
			buf = append(buf, uint16(r))
		}
		buf = append(buf, 0)
	}
	buf = append(buf, 0)
	return buf
}

func splitNulls(buf []uint16) []string {
	var parts []string
	start := 0
	for i, v := range buf {
		if v == 0 {
			if i == start {
				break
			}
			parts = append(parts, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	return parts
}

// ── HTML/CSS/JS UI ───────────────────────────────────────────

func buildHTML() string {
	return `<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }

  body {
    font-family: 'Segoe UI', -apple-system, sans-serif;
    background: #f8fafc;
    color: #0f172a;
    padding: 32px 36px 16px;
    user-select: none;
    overflow: hidden;
    height: 100vh;
    display: flex;
    flex-direction: column;
  }

  .header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
  }
  .header h1 {
    font-size: 42px;
    font-weight: 700;
    color: #0f172a;
    letter-spacing: -0.3px;
  }
  .header .badge {
    font-size: 20px;
    font-weight: 500;
    color: #64748b;
    background: #f1f5f9;
    border: 1px solid #e2e8f0;
    padding: 3px 10px;
    border-radius: 9999px;
  }

  .dropzone {
    border: 2px dashed #cbd5e1;
    border-radius: 12px;
    padding: 36px 24px;
    text-align: center;
    background: #fff;
    transition: all 0.2s ease;
    cursor: pointer;
    margin-bottom: 20px;
  }
  .dropzone:hover, .dropzone.drag-over {
    border-color: #3b82f6;
    background: #eff6ff;
  }
  .dropzone .icon {
    font-size: 60px;
    margin-bottom: 10px;
    opacity: 0.6;
  }
  .dropzone p {
    font-size: 24px;
    color: #64748b;
    line-height: 1.6;
  }
  .dropzone p strong {
    color: #3b82f6;
    cursor: pointer;
  }

  .file-list {
    flex: 1;
    overflow-y: auto;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    background: #fff;
    margin-bottom: 16px;
    min-height: 0;
  }
  .file-list-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: #94a3b8;
    font-size: 24px;
    padding: 40px;
  }
  .file-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 1px solid #f1f5f9;
    transition: background 0.15s;
  }
  .file-item:last-child { border-bottom: none; }
  .file-item:hover { background: #f8fafc; }
  .file-item .file-icon {
    width: 48px; height: 48px;
    background: #f1f5f9;
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    font-size: 22px;
    flex-shrink: 0;
  }
  .file-item .file-info {
    flex: 1;
    min-width: 0;
  }
  .file-item .file-name {
    font-size: 22px;
    font-weight: 500;
    color: #1e293b;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .file-item .file-path {
    font-size: 18px;
    color: #94a3b8;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin-top: 2px;
  }
  .file-item .file-status {
    font-size: 26px;
    flex-shrink: 0;
  }
  .file-item .remove-btn {
    background: none;
    border: none;
    color: #94a3b8;
    cursor: pointer;
    font-size: 26px;
    padding: 6px;
    border-radius: 4px;
    transition: all 0.15s;
    flex-shrink: 0;
  }
  .file-item .remove-btn:hover {
    color: #ef4444;
    background: #fef2f2;
  }

  .actions {
    display: flex;
    gap: 10px;
    margin-bottom: 8px;
  }
  .btn {
    flex: 1;
    padding: 14px 20px;
    border: none;
    border-radius: 8px;
    font-size: 24px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
    font-family: inherit;
  }
  .btn:active { transform: scale(0.98); }
  .btn-primary {
    background: #3b82f6;
    color: #fff;
  }
  .btn-primary:hover { background: #2563eb; }
  .btn-primary:disabled {
    background: #93c5fd;
    cursor: not-allowed;
    transform: none;
  }
  .btn-secondary {
    background: #f1f5f9;
    color: #475569;
    border: 1px solid #e2e8f0;
  }
  .btn-secondary:hover { background: #e2e8f0; }
  .btn-danger {
    background: #fff;
    color: #ef4444;
    border: 1px solid #fecaca;
  }
  .btn-danger:hover { background: #fef2f2; }
  .btn-danger:disabled {
    color: #fca5a5;
    border-color: #fef2f2;
    cursor: not-allowed;
    transform: none;
  }

  .status {
    font-size: 20px;
    color: #64748b;
    text-align: center;
    padding: 6px;
  }
  .status.success { color: #16a34a; font-weight: 500; }
  .status.error { color: #ef4444; font-weight: 500; }
  .status.progress { color: #3b82f6; font-weight: 500; }

  .theme-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: #f1f5f9;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    margin-bottom: 8px;
    font-size: 19px;
    color: #475569;
  }
  .theme-bar .theme-label {
    flex-shrink: 0;
    font-weight: 500;
  }
  .theme-bar select {
    flex: 1;
    padding: 4px 8px;
    border: 1px solid #cbd5e1;
    border-radius: 5px;
    font-size: 18px;
    font-family: inherit;
    background: #fff;
    color: #1e293b;
    cursor: pointer;
  }
  .theme-bar select:focus {
    outline: none;
    border-color: #3b82f6;
  }

  .output-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: #f1f5f9;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    margin-bottom: 8px;
    font-size: 19px;
    color: #475569;
  }
  .output-bar .output-label {
    flex-shrink: 0;
    font-weight: 500;
  }
  .output-bar .output-path {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: #1e293b;
  }
  .output-bar .output-btn {
    flex-shrink: 0;
    background: none;
    border: 1px solid #cbd5e1;
    border-radius: 5px;
    padding: 3px 10px;
    font-size: 18px;
    color: #3b82f6;
    cursor: pointer;
    font-family: inherit;
    transition: all 0.15s;
  }
  .output-bar .output-btn:hover {
    background: #dbeafe;
    border-color: #93c5fd;
  }

  .file-list::-webkit-scrollbar { width: 6px; }
  .file-list::-webkit-scrollbar-track { background: transparent; }
  .file-list::-webkit-scrollbar-thumb {
    background: #cbd5e1;
    border-radius: 3px;
  }
  .file-list::-webkit-scrollbar-thumb:hover { background: #94a3b8; }
</style>
</head>
<body>

<div class="header">
  <h1>md2pdf</h1>
  <span class="badge" id="versionBadge">v</span>
</div>

<div class="dropzone" id="dropzone" onclick="onBrowse()">
  <div class="icon">` + "\U0001F4C4" + `</div>
  <p>Markdown 파일을 이 창에 끌어다 놓거나<br><strong>파일 추가</strong> 버튼을 클릭하세요</p>
</div>

<div class="file-list" id="fileList">
  <div class="file-list-empty">
    파일을 추가하면 여기에 표시됩니다
  </div>
</div>

<div class="theme-bar">
  <span class="theme-label">테마:</span>
  <select id="themeSelect"></select>
</div>

<div class="output-bar" id="outputBar">
  <span class="output-label">출력 폴더:</span>
  <span class="output-path" id="outputPath">(원본 위치)</span>
  <button class="output-btn" id="outputBtn" onclick="onToggleOutputDir()">변경</button>
</div>

<div class="actions">
  <button class="btn btn-secondary" onclick="onBrowse()">파일 추가</button>
  <button class="btn btn-primary" id="btnConvert" onclick="onConvert()" disabled>PDF 변환</button>
  <button class="btn btn-danger" id="btnClear" onclick="onClear()" disabled>비우기</button>
</div>

<div class="status" id="status">파일을 추가하면 변환할 수 있습니다.</div>

<script>
  // DPI correction
  (function() {
    var dpr = window.devicePixelRatio || 1;
    if (dpr !== 1) {
      document.body.style.zoom = (1 / dpr);
      document.body.style.height = (100 * dpr) + 'vh';
    }
  })();

  const $fileList = document.getElementById('fileList');
  const $btnConvert = document.getElementById('btnConvert');
  const $btnClear = document.getElementById('btnClear');
  const $status = document.getElementById('status');
  const $dropzone = document.getElementById('dropzone');

  let files = [];
  let converting = false;

  // Init
  (async () => {
    const ver = await _getVersion();
    document.getElementById('versionBadge').textContent = 'v' + ver;

    const themes = await _getThemes();
    const sel = document.getElementById('themeSelect');
    themes.forEach(t => {
      const opt = document.createElement('option');
      opt.value = t.Name;
      opt.textContent = t.Name + ' — ' + t.Description;
      sel.appendChild(opt);
    });

    if (window._initialFiles && window._initialFiles.length > 0) {
      files = [...window._initialFiles];
      renderFiles();
    }
  })();

  // Drag-drop: Native IDropTarget (Go side) is primary — gets real file paths.
  // JS handler below is a fallback if native registration failed.
  document.addEventListener('dragover', (e) => {
    e.preventDefault();
    $dropzone.classList.add('drag-over');
  });
  document.addEventListener('dragleave', (e) => {
    if (!e.relatedTarget || e.relatedTarget === document.documentElement) {
      $dropzone.classList.remove('drag-over');
    }
  });
  document.addEventListener('drop', async (e) => {
    e.preventDefault();
    $dropzone.classList.remove('drag-over');

    // Wait briefly for native IDropTarget handler (if active, it adds files directly)
    const queueBefore = files.length;
    await new Promise(r => setTimeout(r, 300));
    const freshQueue = await _getFileQueue();
    if (freshQueue.length > queueBefore) {
      files = freshQueue;
      renderFiles();
      setStatus(files.length + '개 파일 준비됨');
      return;
    }

    // Fallback: read via JS and save to temp dir
    let added = 0, dupes = 0, errors = [];
    for (const file of e.dataTransfer.files) {
      if (/\.(md|markdown|txt)$/i.test(file.name)) {
        try {
          const content = await file.text();
          const res = await _addDroppedFile(file.name, content);
          if (res.status === 'added') added++;
          else if (res.status === 'duplicate') dupes++;
          else errors.push(res.msg);
        } catch (err) {
          errors.push(file.name + ': ' + err.message);
        }
      }
    }

    files = await _getFileQueue();
    renderFiles();

    if (errors.length > 0) {
      setStatus(errors.join('; '), 'error');
    } else if (dupes > 0 && added === 0) {
      setStatus('이미 목록에 있는 파일입니다', 'error');
    } else if (added > 0) {
      setStatus(files.length + '개 파일 준비됨');
    }
  });

  async function onBrowse() {
    if (converting) return;
    const result = await _browseFiles();
    if (result && result.length > 0) {
      files = result;
      renderFiles();
      setStatus(files.length + '개 파일 준비됨');
    }
  }

  async function onConvert() {
    if (converting || files.length === 0) return;
    converting = true;
    updateButtons();
    setStatus('변환 준비 중...', 'progress');

    const theme = document.getElementById('themeSelect').value;
    const result = await _convertFiles(files, theme);
    converting = false;

    if (result.fail > 0) {
      setStatus(result.msg, 'error');
    } else {
      setStatus(result.msg, 'success');
      document.querySelectorAll('.file-status').forEach(el => { el.textContent = '✅'; });
    }
    updateButtons();
  }

  async function onClear() {
    if (converting) return;
    files = [];
    await _clearFiles();
    renderFiles();
    setStatus('목록을 비웠습니다.');
  }

  async function onRemove(index) {
    if (converting || index < 0 || index >= files.length) return;
    files = await _removeFile(files[index]);
    renderFiles();
    setStatus(files.length === 0
      ? '파일을 추가하면 변환할 수 있습니다.'
      : files.length + '개 파일 준비됨');
  }

  let currentOutputDir = '';

  async function onToggleOutputDir() {
    if (currentOutputDir) {
      await _clearOutputDir();
      currentOutputDir = '';
    } else {
      const dir = await _browseOutputDir();
      if (dir) currentOutputDir = dir;
    }
    renderOutputBar();
  }

  function renderOutputBar() {
    const $path = document.getElementById('outputPath');
    const $btn = document.getElementById('outputBtn');
    if (currentOutputDir) {
      const short = currentOutputDir.length > 50
        ? '...' + currentOutputDir.substring(currentOutputDir.length - 47)
        : currentOutputDir;
      $path.textContent = short;
      $path.title = currentOutputDir;
      $btn.textContent = '초기화';
    } else {
      $path.textContent = '(원본 위치)';
      $path.title = '';
      $btn.textContent = '변경';
    }
  }

  window._updateProgress = function(current, total, filename) {
    setStatus('변환 중 (' + current + '/' + total + '): ' + filename, 'progress');
  };

  function renderFiles() {
    if (files.length === 0) {
      $fileList.innerHTML = '<div class="file-list-empty">파일을 추가하면 여기에 표시됩니다</div>';
      updateButtons();
      return;
    }

    let html = '';
    files.forEach((f, i) => {
      const name = f.split('\\').pop();
      const dir = f.substring(0, f.length - name.length - 1);
      const shortDir = dir.length > 45 ? '...' + dir.substring(dir.length - 42) : dir;

      html += '<div class="file-item">' +
        '<div class="file-icon">` + "\U0001F4DD" + `</div>' +
        '<div class="file-info">' +
          '<div class="file-name">' + escapeHtml(name) + '</div>' +
          '<div class="file-path" title="' + escapeAttr(f) + '">' + escapeHtml(shortDir) + '</div>' +
        '</div>' +
        '<span class="file-status"></span>' +
        '<button class="remove-btn" onclick="onRemove(' + i + ')" title="제거">✕</button>' +
      '</div>';
    });

    $fileList.innerHTML = html;
    updateButtons();
  }

  function updateButtons() {
    $btnConvert.disabled = files.length === 0 || converting;
    $btnClear.disabled = files.length === 0 || converting;
  }

  function setStatus(text, type) {
    $status.textContent = text;
    $status.className = 'status' + (type ? ' ' + type : '');
  }

  function escapeHtml(s) {
    const d = document.createElement('div');
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  function escapeAttr(s) {
    return s.replace(/&/g,'&amp;').replace(/'/g,'&#39;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
</script>
</body>
</html>`
}
