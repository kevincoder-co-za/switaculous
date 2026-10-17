package switaculous

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

var errNoNativeDialog = errors.New("no native folder dialog is available on this computer")

func (rt *Runtime) registerProjectAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/project", rt.handleProject)
	mux.HandleFunc("/api/project/open-folder", rt.handleOpenFolder)
	mux.HandleFunc("/api/project/new", rt.handleNewProject)
	mux.HandleFunc("/api/demos", handleListDemos)
	mux.HandleFunc("/api/demos/open", rt.handleOpenDemo)
	mux.HandleFunc("/api/quit", handleQuit)
}

func handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
}

// The preview follows whichever file the user is working in. Picking one of a
// file's previews by hand only lasts until they move on to another file.
func (rt *Runtime) followFileLocked(absPath string) {
	if rt.lastEdited != absPath {
		rt.lastEdited = absPath
		rt.pinned = "auto"
	}
}

func (rt *Runtime) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rt.mu.Lock()
		currentDir := rt.dir
		rt.mu.Unlock()
		writeJSON(w, map[string]interface{}{"dir": currentDir, "recent": existingRecentProjects()})
		return
	}
	var request struct {
		Dir string `json:"dir"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	projectDir, err := filepath.Abs(strings.TrimSpace(request.Dir))
	if err != nil || request.Dir == "" {
		writeJSONError(w, http.StatusBadRequest, "please enter a folder path")
		return
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		writeJSONError(w, http.StatusBadRequest, "can't open or create that folder: "+err.Error())
		return
	}
	ensureStarter(projectDir)
	rt.openProject(projectDir)
	writeJSON(w, map[string]string{"dir": projectDir})
}

func (rt *Runtime) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	projectDir, err := showNativeFolderDialog(false, "Open a folder with Swift code", "")
	if err != nil {
		writeJSONError(w, http.StatusNotImplemented, err.Error())
		return
	}
	if projectDir == "" {
		writeJSON(w, map[string]bool{"cancelled": true})
		return
	}
	ensureStarter(projectDir)
	rt.openProject(projectDir)
	writeJSON(w, map[string]string{"dir": projectDir})
}

// Like Xcode's "Save As" step for a new project: the user names the project
// and picks where it goes, and that becomes the project folder.
func (rt *Runtime) handleNewProject(w http.ResponseWriter, r *http.Request) {
	projectDir, err := showNativeFolderDialog(true, "Name your new project and choose where to save it", "MyApp")
	if err != nil {
		writeJSONError(w, http.StatusNotImplemented, err.Error())
		return
	}
	if projectDir == "" {
		writeJSON(w, map[string]bool{"cancelled": true})
		return
	}
	if containsSwiftFiles(projectDir) {
		writeJSONError(w, http.StatusConflict, filepath.Base(projectDir)+" already has Swift files in it. Use Open Folder to open it, or pick another name.")
		return
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		writeJSONError(w, http.StatusBadRequest, "can't create that folder: "+err.Error())
		return
	}
	ensureStarter(projectDir)
	rt.openProject(projectDir)
	writeJSON(w, map[string]string{"dir": projectDir})
}

func (rt *Runtime) openProject(projectDir string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.closeModelStoresLocked()
	rt.userDefaultsLoaded = false
	rt.userDefaultsSaved = ""
	rt.dir = projectDir
	rt.files = map[string]string{}
	rt.lastEdited = ""
	rt.pinned = "auto"
	rt.current = ""
	freshStore := newStore()
	freshStore.dark, freshStore.device, freshStore.landscape = rt.store.dark, rt.store.device, rt.store.landscape
	rt.store = freshStore
	rt.files, _ = rt.scan()
	rt.reloadLocked()
	saveRecentProject(projectDir)
}

func containsSwiftFiles(dir string) bool {
	found := false
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".swift") {
			found = true
			return filepath.SkipAll
		}
		if err == nil && info.IsDir() && p != dir && skipDir(info.Name()) {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// ---------- recent projects ----------

func recentProjectsFile() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "switaculous", "recent-projects.json")
}

func loadRecentProjects() []string {
	recent := []string{}
	content, err := os.ReadFile(recentProjectsFile())
	if err == nil {
		json.Unmarshal(content, &recent)
	}
	return recent
}

func existingRecentProjects() []string {
	existing := []string{}
	for _, dir := range loadRecentProjects() {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			existing = append(existing, dir)
		}
	}
	return existing
}

func saveRecentProject(projectDir string) {
	recent := []string{projectDir}
	for _, previous := range loadRecentProjects() {
		if previous != projectDir && len(recent) < 10 {
			recent = append(recent, previous)
		}
	}
	settingsFile := recentProjectsFile()
	if settingsFile == "" {
		return
	}
	os.MkdirAll(filepath.Dir(settingsFile), 0755)
	content, _ := json.Marshal(recent)
	os.WriteFile(settingsFile, content, 0644)
}

// ---------- demos ----------

func demoNames() []string {
	entries, _ := fs.ReadDir(demoFiles, "examples")
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func handleListDemos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, demoNames())
}

// The demos ship inside the program, so they are copied to
// Documents/Switaculous/Demos the first time they are opened. After that the copy is
// theirs to change: opening the demo again keeps their edits.
func (rt *Runtime) handleOpenDemo(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	isKnownDemo := false
	for _, name := range demoNames() {
		if name == request.Name {
			isKnownDemo = true
		}
	}
	if !isKnownDemo {
		writeJSONError(w, http.StatusNotFound, "there is no demo called "+request.Name)
		return
	}
	demoDir := filepath.Join(switaculousDocumentsFolder(), "Demos", request.Name)
	if !containsSwiftFiles(demoDir) {
		if err := copyDemo(request.Name, demoDir); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "couldn't copy the demo: "+err.Error())
			return
		}
	}
	rt.openProject(demoDir)
	writeJSON(w, map[string]string{"dir": demoDir})
}

func copyDemo(name, targetDir string) error {
	sourceDir := path.Join("examples", name)
	return fs.WalkDir(demoFiles, sourceDir, func(sourcePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, filepath.FromSlash(strings.TrimPrefix(sourcePath, sourceDir)))
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		content, err := fs.ReadFile(demoFiles, sourcePath)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, content, 0644)
	})
}

// ---------- native dialogs ----------

// Shows the operating system's own folder picker. With save set it shows a
// save dialog instead, so the user can type a new project name. Returns ""
// when the user cancels.
func showNativeFolderDialog(save bool, title, defaultName string) (string, error) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := fmt.Sprintf(windowsDialogScript, powerShellString(title), powerShellString(defaultName), save)
		command = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(script))
		hideConsoleWindow(command)
	case "darwin":
		script := fmt.Sprintf(`POSIX path of (choose folder with prompt %q)`, title)
		if save {
			script = fmt.Sprintf(`POSIX path of (choose file name with prompt %q default name %q)`, title, defaultName)
		}
		command = exec.Command("osascript", "-e", script)
	default:
		if zenity, err := exec.LookPath("zenity"); err == nil {
			arguments := []string{"--file-selection", "--directory", "--title=" + title}
			if save {
				arguments = []string{"--file-selection", "--save", "--title=" + title, "--filename=" + defaultName}
			}
			command = exec.Command(zenity, arguments...)
		} else if kdialog, err := exec.LookPath("kdialog"); err == nil {
			home, _ := os.UserHomeDir()
			arguments := []string{"--title", title, "--getexistingdirectory", home}
			if save {
				arguments = []string{"--title", title, "--getsavefilename", filepath.Join(home, defaultName)}
			}
			command = exec.Command(kdialog, arguments...)
		} else {
			return "", errNoNativeDialog
		}
	}
	output, err := command.Output()
	// zenity, kdialog and osascript exit with status 1 when the user cancels.
	var exitError *exec.ExitError
	if runtime.GOOS != "windows" && errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", errNoNativeDialog, err)
	}
	chosen := strings.TrimSpace(string(output))
	if chosen == "" {
		return "", nil
	}
	return filepath.Clean(chosen), nil
}

func powerShellString(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	bytes := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(bytes[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

// Windows' modern Explorer-style picker (IFileOpenDialog / IFileSaveDialog) is
// only reachable through COM, so a few lines of C# are compiled on the fly.
// The dialog is owned by an invisible topmost window so it opens in front of
// the browser rather than behind it.
const windowsDialogScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;

[ComImport, Guid("42f85136-db7e-439c-85f1-e4075d135fc8"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IFileDialog {
    [PreserveSig] int Show(IntPtr parent);
    void SetFileTypes();
    void SetFileTypeIndex();
    void GetFileTypeIndex();
    void Advise();
    void Unadvise();
    void SetOptions(uint options);
    void GetOptions(out uint options);
    void SetDefaultFolder();
    void SetFolder();
    void GetFolder();
    void GetCurrentSelection();
    void SetFileName([MarshalAs(UnmanagedType.LPWStr)] string name);
    void GetFileName();
    void SetTitle([MarshalAs(UnmanagedType.LPWStr)] string title);
    void SetOkButtonLabel();
    void SetFileNameLabel();
    void GetResult(out IShellItem item);
}

[ComImport, Guid("43826D1E-E718-42EE-BC55-A1E261C37BFE"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IShellItem {
    void BindToHandler();
    void GetParent();
    void GetDisplayName(uint kind, [MarshalAs(UnmanagedType.LPWStr)] out string name);
}

public static class SwitaculousFolderDialog {
    public static string Show(IntPtr owner, string title, string defaultName, bool save) {
        Guid classId = new Guid(save ? "C0B4E2F3-BA21-4773-8DBA-335EC946EB8B" : "DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7");
        IFileDialog dialog = (IFileDialog)Activator.CreateInstance(Type.GetTypeFromCLSID(classId));
        uint options;
        dialog.GetOptions(out options);
        const uint forceFileSystem = 0x40, pickFolders = 0x20;
        options |= forceFileSystem;
        if (!save) options |= pickFolders;
        dialog.SetOptions(options);
        dialog.SetTitle(title);
        if (save) dialog.SetFileName(defaultName);
        if (dialog.Show(owner) != 0) return "";
        IShellItem item;
        dialog.GetResult(out item);
        string path;
        const uint fileSystemPath = 0x80058000;
        item.GetDisplayName(fileSystemPath, out path);
        return path;
    }
}
"@
$owner = New-Object System.Windows.Forms.Form
$owner.TopMost = $true
$owner.ShowInTaskbar = $false
$owner.FormBorderStyle = 'None'
$owner.Opacity = 0
$owner.StartPosition = 'CenterScreen'
$owner.Size = New-Object System.Drawing.Size(1, 1)
$owner.Show()
$owner.Activate()
$path = [SwitaculousFolderDialog]::Show($owner.Handle, %s, %s, $%t)
$owner.Close()
Write-Output $path
`
