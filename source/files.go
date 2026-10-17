package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type FileRef struct {
	Path    string `json:"path"`
	Version string `json:"ver"`
}

var swiftTypeNamePattern = regexp.MustCompile(`[^A-Za-z0-9_]`)

func (rt *Runtime) rel(absPath string) string {
	if absPath == "" {
		return ""
	}
	relPath, err := filepath.Rel(rt.dir, absPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relPath)
}

func (rt *Runtime) fileRefs() []FileRef {
	refs := make([]FileRef, 0, len(rt.files))
	for absPath, version := range rt.files {
		refs = append(refs, FileRef{Path: rt.rel(absPath), Version: version})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs
}

func (rt *Runtime) projectFilePath(relPath string) (string, error) {
	if relPath == "" {
		return "", errors.New("missing file path")
	}
	cleaned := filepath.Clean(filepath.FromSlash(relPath))
	// The editor must never be able to read or overwrite files outside the project folder.
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", errors.New("file must be inside the project folder")
	}
	return filepath.Join(rt.dir, cleaned), nil
}

func newViewSource(fileName string) string {
	typeName := strings.TrimSuffix(fileName, ".swift")
	typeName = swiftTypeNamePattern.ReplaceAllString(typeName, "")
	if typeName == "" || (typeName[0] >= '0' && typeName[0] <= '9') {
		typeName = "NewView"
	}
	return fmt.Sprintf(`import SwiftUI

struct %s: View {
    var body: some View {
        Text("Hello, World!")
    }
}

#Preview {
    %s()
}
`, typeName, typeName)
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (rt *Runtime) reloadAfterFileChange(changedFile string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.files, _ = rt.scan()
	if changedFile != "" {
		rt.lastEdited = changedFile
	}
	rt.reloadLocked()
}

func (rt *Runtime) registerFileAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/file", rt.handleFile)
	mux.HandleFunc("/api/file/create", rt.handleCreateFile)
	mux.HandleFunc("/api/file/rename", rt.handleRenameFile)
	mux.HandleFunc("/api/file/delete", rt.handleDeleteFile)
	mux.HandleFunc("/api/focus", rt.handleFocusFile)
	mux.HandleFunc("/api/project", rt.handleProject)
	mux.HandleFunc("/api/open-in-vscode", rt.handleOpenInVSCode)
	mux.HandleFunc("/api/show-in-explorer", rt.handleShowInExplorer)
	mux.HandleFunc("/api/complete", rt.handleComplete)
	mux.HandleFunc("/api/docs", rt.handleDocs)
}

func (rt *Runtime) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rt.mu.Lock()
		absPath, err := rt.projectFilePath(r.URL.Query().Get("path"))
		rt.mu.Unlock()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		writeJSON(w, map[string]string{"content": string(content)})
		return
	}

	var request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	rt.mu.Lock()
	absPath, err := rt.projectFilePath(request.Path)
	rt.mu.Unlock()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(absPath, []byte(request.Content), 0644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.reloadAfterFileChange(absPath)
	writeJSON(w, map[string]bool{"saved": true})
}

func (rt *Runtime) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	if !strings.HasSuffix(strings.ToLower(request.Path), ".swift") {
		request.Path += ".swift"
	}
	rt.mu.Lock()
	absPath, err := rt.projectFilePath(request.Path)
	rt.mu.Unlock()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(absPath); err == nil {
		writeJSONError(w, http.StatusConflict, "a file with that name already exists")
		return
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(absPath, []byte(newViewSource(filepath.Base(absPath))), 0644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.reloadAfterFileChange(absPath)
	writeJSON(w, map[string]string{"path": filepath.ToSlash(filepath.Clean(request.Path))})
}

func (rt *Runtime) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	if !strings.HasSuffix(strings.ToLower(request.To), ".swift") {
		request.To += ".swift"
	}
	rt.mu.Lock()
	fromPath, fromErr := rt.projectFilePath(request.From)
	toPath, toErr := rt.projectFilePath(request.To)
	rt.mu.Unlock()
	if fromErr != nil || toErr != nil {
		writeJSONError(w, http.StatusBadRequest, "file must be inside the project folder")
		return
	}
	if _, err := os.Stat(toPath); err == nil {
		writeJSONError(w, http.StatusConflict, "a file with that name already exists")
		return
	}
	if err := os.Rename(fromPath, toPath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.reloadAfterFileChange(toPath)
	writeJSON(w, map[string]string{"path": filepath.ToSlash(filepath.Clean(request.To))})
}

func (rt *Runtime) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	rt.mu.Lock()
	absPath, err := rt.projectFilePath(request.Path)
	rt.mu.Unlock()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Remove(absPath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.reloadAfterFileChange("")
	writeJSON(w, map[string]bool{"deleted": true})
}

// Switching files in the editor makes the "Auto" preview follow that file,
// the same way Xcode's canvas shows the preview of the file being edited.
func (rt *Runtime) handleFocusFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	absPath, err := rt.projectFilePath(request.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rt.lastEdited != absPath {
		rt.lastEdited = absPath
		rt.renderLocked()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Runtime) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rt.mu.Lock()
		currentDir := rt.dir
		rt.mu.Unlock()
		writeJSON(w, map[string]interface{}{"dir": currentDir, "recent": loadRecentProjects()})
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

func recentProjectsFile() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "swiftui-tutor", "recent-projects.json")
}

func loadRecentProjects() []string {
	recent := []string{}
	content, err := os.ReadFile(recentProjectsFile())
	if err == nil {
		json.Unmarshal(content, &recent)
	}
	return recent
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

func (rt *Runtime) handleOpenInVSCode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	rt.mu.Lock()
	projectDir := rt.dir
	filePath, fileErr := rt.projectFilePath(request.Path)
	rt.mu.Unlock()

	arguments := []string{projectDir}
	if fileErr == nil {
		location := filePath
		if request.Line > 0 {
			location = fmt.Sprintf("%s:%d", filePath, request.Line)
		}
		arguments = append(arguments, "--goto", location)
	}
	vsCodePath := findVSCodeCommand()
	if vsCodePath == "" {
		writeJSONError(w, http.StatusNotFound,
			"Couldn't find VS Code. Install it from code.visualstudio.com, then try again.")
		return
	}
	command := exec.Command(vsCodePath, arguments...)
	if err := command.Start(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Couldn't start VS Code: "+err.Error())
		return
	}
	go command.Wait()
	writeJSON(w, map[string]bool{"opened": true})
}

func findVSCodeCommand() string {
	if found, err := exec.LookPath("code"); err == nil {
		return found
	}
	// VS Code adds itself to PATH on install, but programs that were already
	// running (or started from an old terminal) don't see that until a restart.
	var installLocations []string
	switch runtime.GOOS {
	case "windows":
		installLocations = []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "bin", "code.cmd"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "bin", "code.cmd"),
		}
	case "darwin":
		installLocations = []string{"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"}
	default:
		installLocations = []string{"/usr/bin/code", "/snap/bin/code"}
	}
	for _, location := range installLocations {
		if _, err := os.Stat(location); err == nil {
			return location
		}
	}
	return ""
}

func (rt *Runtime) handleShowInExplorer(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	projectDir := rt.dir
	rt.mu.Unlock()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer", projectDir)
	case "darwin":
		command = exec.Command("open", projectDir)
	default:
		command = exec.Command("xdg-open", projectDir)
	}
	command.Start()
	go command.Wait()
	writeJSON(w, map[string]bool{"opened": true})
}
