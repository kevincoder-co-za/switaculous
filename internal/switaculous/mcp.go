package switaculous

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Switaculous can act as an MCP server (JSON-RPC over stdio) for Claude Code
// and similar tools. The MCP process is a thin bridge to the running preview
// window, so everything the assistant does shows up live in the editor.

const serverInfoFile = "server.json"

type runningServerInfo struct {
	Port int    `json:"port"`
	Dir  string `json:"dir"`
	PID  int    `json:"pid"`
}

func serverInfoPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "switaculous", serverInfoFile)
}

func writeServerInfo(port int, dir string) {
	path := serverInfoPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	content, _ := json.Marshal(runningServerInfo{Port: port, Dir: dir, PID: os.Getpid()})
	os.WriteFile(path, content, 0644)
}

func readServerInfo() (runningServerInfo, bool) {
	var info runningServerInfo
	content, err := os.ReadFile(serverInfoPath())
	if err != nil || json.Unmarshal(content, &info) != nil || info.Port == 0 {
		return info, false
	}
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/project", info.Port))
	if err != nil {
		return info, false
	}
	response.Body.Close()
	return info, response.StatusCode == http.StatusOK
}

type mcpServer struct {
	baseURL string
	out     *bufio.Writer
}

type jsonRPCRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

func stringParam(description string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description}
}

func boolParam(description string) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": description}
}

func schema(required []string, properties map[string]interface{}) map[string]interface{} {
	s := map[string]interface{}{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var mcpTools = []mcpTool{
	{"project_info", "The open project: folder, Swift files, available previews, the selected device, and current build or runtime errors.", schema(nil, map[string]interface{}{})},
	{"read_swift_file", "Read a Swift file from the project. Paths are relative to the project folder, e.g. \"Views/ContentView.swift\".", schema([]string{"path"}, map[string]interface{}{"path": stringParam("Relative path of the file")})},
	{"write_swift_file", "Write (or create) a Swift file with the given content. The preview rebuilds immediately; check preview_status afterwards for errors.", schema([]string{"path", "content"}, map[string]interface{}{"path": stringParam("Relative path, e.g. \"Views/DetailView.swift\""), "content": stringParam("The complete file content")})},
	{"delete_swift_file", "Delete a Swift file from the project.", schema([]string{"path"}, map[string]interface{}{"path": stringParam("Relative path of the file")})},
	{"preview_status", "What the preview shows right now: build and runtime errors with line numbers, warnings, recent console output, and an outline of the rendered screen (every view with its text). Call this after writing code to verify it works.", schema(nil, map[string]interface{}{})},
	{"select_preview", "Choose which #Preview or App to show. Use the id or name from project_info, or \"auto\" to follow the file being edited.", schema([]string{"preview"}, map[string]interface{}{"preview": stringParam("Preview id, name, or \"auto\"")})},
	{"set_device", "Change the simulated device, orientation, dark mode or iOS style.", schema(nil, map[string]interface{}{
		"device":    stringParam("iphone16, iphone16pro, iphone16promax, iphonese, ipadmini, ipad11 or ipadpro13"),
		"landscape": boolParam("Turn an iPad sideways"),
		"dark":      boolParam("Dark mode on or off"),
	})},
	{"tap", "Tap a button, link, row, toggle or tab in the preview by its visible text, as a user would. Returns the screen outline afterwards.", schema([]string{"text"}, map[string]interface{}{"text": stringParam("The visible text of the control, e.g. \"Add Item\" or \"Settings\"")})},
	{"type_text", "Type into a TextField, SecureField or TextEditor found by its placeholder, label or current text.", schema([]string{"field", "text"}, map[string]interface{}{"field": stringParam("The field's placeholder, label or current text"), "text": stringParam("The new text")})},
	{"reset_state", "Reset all @State to its initial values, as if the app was relaunched. Saved SwiftData is kept.", schema(nil, map[string]interface{}{})},
	{"list_assets", "Images, colour sets and system icon names the project can use with Image(\"name\"), Color(\"name\") and Image(systemName:).", schema(nil, map[string]interface{}{})},
	{"search_api", "Look up SwiftUI views and modifiers the preview supports: signatures and a one-line description. Use it to check a modifier exists before using it.", schema([]string{"query"}, map[string]interface{}{"query": stringParam("Part of a view or modifier name, e.g. \"navigation\" or \"padding\"")})},
}

func runMCPServer(projectDir string, port int, noOpen bool) {
	info, running := readServerInfo()
	baseURL := ""
	if running && (projectDir == "" || sameFolder(info.Dir, projectDir)) {
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", info.Port)
	} else {
		baseURL = startEmbeddedServer(projectDir, port, noOpen)
	}
	server := &mcpServer{baseURL: baseURL, out: bufio.NewWriter(os.Stdout)}
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			server.handleLine(bytes.TrimSpace(line))
		}
		if err != nil {
			return
		}
	}
}

func sameFolder(a, b string) bool {
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

// Without a Switaculous window open, the MCP process runs the preview server
// itself (and opens the browser), so the assistant still has a live project.
func startEmbeddedServer(projectDir string, port int, noOpen bool) string {
	if projectDir == "" {
		projectDir = "MyApp"
	}
	ready := make(chan string, 1)
	go serveProject(projectDir, port, noOpen, true, ready)
	return <-ready
}

func (s *mcpServer) handleLine(line []byte) {
	var request jsonRPCRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return
	}
	if response, hasResponse := s.handle(request); hasResponse {
		s.send(response)
	}
}

// Answers one JSON-RPC message. Notifications produce no response.
func (s *mcpServer) handle(request jsonRPCRequest) (interface{}, bool) {
	switch request.Method {
	case "initialize":
		return s.result(request.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "switaculous", "version": version},
			"instructions": "Switaculous runs the user's SwiftUI project in a live preview. Write Swift files with write_swift_file, then call preview_status " +
				"to see errors and the rendered screen, and tap/type_text to try the UI. The preview supports standard SwiftUI, SwiftData and PhotosPicker; " +
				"search_api tells you which views and modifiers are available.",
		})
	case "notifications/initialized", "notifications/cancelled":
		return nil, false
	case "ping":
		return s.result(request.ID, map[string]interface{}{})
	case "tools/list":
		return s.result(request.ID, map[string]interface{}{"tools": mcpTools})
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		json.Unmarshal(request.Params, &params)
		text, err := s.callTool(params.Name, params.Arguments)
		if err != nil {
			return s.result(request.ID, map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Error: " + err.Error()}}, "isError": true})
		}
		return s.result(request.ID, map[string]interface{}{"content": []map[string]string{{"type": "text", "text": text}}})
	}
	if len(request.ID) == 0 {
		return nil, false
	}
	return map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32601, "message": "unknown method " + request.Method}}, true
}

func (s *mcpServer) result(id json.RawMessage, result interface{}) (interface{}, bool) {
	if len(id) == 0 {
		return nil, false
	}
	return map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result}, true
}

// The same server over Streamable HTTP, served by the preview window itself at
// /mcp. Claude Code connects with: claude mcp add --transport http switaculous http://127.0.0.1:<port>/mcp
func (s *mcpServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "The MCP endpoint is only available on this computer.", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodPost:
	case http.MethodGet:
		http.Error(w, "This server does not open notification streams.", http.StatusMethodNotAllowed)
		return
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var responses []interface{}
	batch := bytes.HasPrefix(bytes.TrimSpace(body), []byte("["))
	var requests []jsonRPCRequest
	if batch {
		json.Unmarshal(body, &requests)
	} else {
		var single jsonRPCRequest
		if json.Unmarshal(body, &single) != nil {
			http.Error(w, "invalid JSON-RPC message", http.StatusBadRequest)
			return
		}
		requests = []jsonRPCRequest{single}
	}
	for _, request := range requests {
		if response, hasResponse := s.handle(request); hasResponse {
			responses = append(responses, response)
		}
	}
	w.Header().Set("Mcp-Session-Id", "switaculous")
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if batch {
		json.NewEncoder(w).Encode(responses)
		return
	}
	json.NewEncoder(w).Encode(responses[0])
}

func newHTTPMCPServer() *mcpServer {
	return &mcpServer{}
}

func (s *mcpServer) send(message interface{}) {
	encoded, _ := json.Marshal(message)
	s.out.Write(encoded)
	s.out.WriteByte('\n')
	s.out.Flush()
}

// ---------- tools ----------

func argString(arguments map[string]interface{}, name string) string {
	value, _ := arguments[name].(string)
	return value
}

func (s *mcpServer) callTool(name string, arguments map[string]interface{}) (string, error) {
	switch name {
	case "project_info":
		return s.projectInfo()
	case "read_swift_file":
		var response struct {
			Content string `json:"content"`
		}
		if err := s.get("/api/file?path="+escapeQuery(argString(arguments, "path")), &response); err != nil {
			return "", err
		}
		return response.Content, nil
	case "write_swift_file":
		path := argString(arguments, "path")
		if !strings.HasSuffix(strings.ToLower(path), ".swift") {
			return "", fmt.Errorf("only .swift files can be written")
		}
		if err := s.post("/api/file", map[string]string{"path": path, "content": argString(arguments, "content")}, nil); err != nil {
			if err := s.post("/api/file/create", map[string]string{"path": path, "content": argString(arguments, "content")}, nil); err != nil {
				return "", err
			}
		}
		time.Sleep(400 * time.Millisecond)
		status, err := s.previewStatus()
		return "Saved " + path + ".\n\n" + status, err
	case "delete_swift_file":
		if err := s.post("/api/file/delete", map[string]string{"path": argString(arguments, "path")}, nil); err != nil {
			return "", err
		}
		return "Deleted " + argString(arguments, "path"), nil
	case "preview_status":
		return s.previewStatus()
	case "select_preview":
		return s.selectPreview(argString(arguments, "preview"))
	case "set_device":
		settings := map[string]interface{}{}
		for _, key := range []string{"device", "landscape", "dark"} {
			if value, ok := arguments[key]; ok {
				settings[key] = value
			}
		}
		if err := s.post("/api/settings", settings, nil); err != nil {
			return "", err
		}
		return s.previewStatus()
	case "tap":
		return s.interact("tap", argString(arguments, "text"), "")
	case "type_text":
		return s.interact("type", argString(arguments, "field"), argString(arguments, "text"))
	case "reset_state":
		if err := s.post("/api/settings", map[string]bool{"reset": true}, nil); err != nil {
			return "", err
		}
		return s.previewStatus()
	case "list_assets":
		return s.listAssets()
	case "search_api":
		return searchAPIDocs(argString(arguments, "query")), nil
	}
	return "", fmt.Errorf("unknown tool %s", name)
}

func escapeQuery(value string) string {
	return strings.NewReplacer("%", "%25", "&", "%26", "#", "%23", "+", "%2B", " ", "%20").Replace(value)
}

func (s *mcpServer) get(path string, into interface{}) error {
	response, err := http.Get(s.baseURL + path)
	if err != nil {
		return fmt.Errorf("Switaculous is not reachable: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		return errorFromBody(body)
	}
	if into != nil {
		return json.Unmarshal(body, into)
	}
	return nil
}

func (s *mcpServer) post(path string, payload interface{}, into interface{}) error {
	encoded, _ := json.Marshal(payload)
	response, err := http.Post(s.baseURL+path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("Switaculous is not reachable: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		return errorFromBody(body)
	}
	if into != nil && len(body) > 0 {
		return json.Unmarshal(body, into)
	}
	return nil
}

func errorFromBody(body []byte) error {
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return fmt.Errorf("%s", parsed.Error)
	}
	return fmt.Errorf("%s", strings.TrimSpace(string(body)))
}

type previewState struct {
	Dir      string        `json:"dir"`
	Files    []FileRef     `json:"files"`
	Previews []PreviewRef  `json:"previews"`
	Current  string        `json:"current"`
	Pinned   string        `json:"pinned"`
	Errors   []ErrInfo     `json:"errors"`
	Warnings []string      `json:"warnings"`
	Console  []ConsoleLine `json:"console"`
	Device   Device        `json:"device"`
	DeviceID string        `json:"deviceId"`
	Dark     bool          `json:"dark"`
	Outline  string        `json:"outline"`
	Images   []ImageAsset  `json:"images"`
	Colors   []string      `json:"colors"`
}

func (s *mcpServer) state() (previewState, error) {
	var state previewState
	err := s.get("/api/state", &state)
	return state, err
}

func (s *mcpServer) projectInfo() (string, error) {
	state, err := s.state()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Project folder: %s\n\nSwift files:\n", state.Dir)
	for _, file := range state.Files {
		fmt.Fprintf(&out, "  %s\n", file.Path)
	}
	out.WriteString("\nPreviews (id — name):\n")
	for _, preview := range state.Previews {
		marker := "  "
		if preview.ID == state.Current {
			marker = "▶ "
		}
		fmt.Fprintf(&out, "%s%s — %s\n", marker, preview.ID, preview.Name)
	}
	fmt.Fprintf(&out, "\nDevice: %s (%s)%s, dark mode %v\n", state.Device.Name, state.DeviceID, orientationNote(state.Device), state.Dark)
	out.WriteString(describeErrors(state))
	return out.String(), nil
}

func orientationNote(device Device) string {
	if device.Landscape {
		return ", landscape"
	}
	return ""
}

func describeErrors(state previewState) string {
	if len(state.Errors) == 0 {
		return "\nNo errors.\n"
	}
	var out strings.Builder
	out.WriteString("\nErrors:\n")
	for _, e := range state.Errors {
		fmt.Fprintf(&out, "  %s:%d:%d: %s error: %s\n", e.File, e.Line, e.Col, e.Kind, e.Msg)
		if e.Snippet != "" {
			out.WriteString(indent(e.Snippet, "    ") + "\n")
		}
	}
	return out.String()
}

func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func (s *mcpServer) previewStatus() (string, error) {
	state, err := s.state()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	current := state.Current
	for _, preview := range state.Previews {
		if preview.ID == state.Current {
			current = preview.Name
		}
	}
	fmt.Fprintf(&out, "Showing: %s on %s%s\n", current, state.Device.Name, orientationNote(state.Device))
	out.WriteString(describeErrors(state))
	if len(state.Warnings) > 0 {
		out.WriteString("\nWarnings:\n")
		for _, warning := range state.Warnings {
			fmt.Fprintf(&out, "  %s\n", warning)
		}
	}
	recent := state.Console
	if len(recent) > 15 {
		recent = recent[len(recent)-15:]
	}
	if len(recent) > 0 {
		out.WriteString("\nConsole (latest):\n")
		for _, line := range recent {
			fmt.Fprintf(&out, "  [%s] %s\n", line.Kind, line.Msg)
		}
	}
	if state.Outline != "" {
		heading := "\nScreen:\n"
		if len(state.Errors) > 0 {
			heading = "\nScreen (last successful render, before the error):\n"
		}
		out.WriteString(heading + indent(state.Outline, "  ") + "\n")
	}
	return out.String(), nil
}

func (s *mcpServer) selectPreview(wanted string) (string, error) {
	state, err := s.state()
	if err != nil {
		return "", err
	}
	id := wanted
	if wanted != "auto" {
		id = ""
		for _, preview := range state.Previews {
			if preview.ID == wanted || strings.EqualFold(preview.Name, wanted) {
				id = preview.ID
			}
		}
		if id == "" {
			return "", fmt.Errorf("no preview called %q; project_info lists the available ones", wanted)
		}
	}
	if err := s.post("/api/settings", map[string]string{"preview": id}, nil); err != nil {
		return "", err
	}
	return s.previewStatus()
}

func (s *mcpServer) interact(action, text, value string) (string, error) {
	var response struct {
		Found bool   `json:"found"`
		Hint  string `json:"hint"`
	}
	if err := s.post("/api/interact", map[string]string{"action": action, "text": text, "value": value}, &response); err != nil {
		return "", err
	}
	if !response.Found {
		return "", fmt.Errorf("%s", response.Hint)
	}
	time.Sleep(300 * time.Millisecond)
	return s.previewStatus()
}

func (s *mcpServer) listAssets() (string, error) {
	state, err := s.state()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("Images (use Image(\"name\")):\n")
	for _, image := range state.Images {
		fmt.Fprintf(&out, "  %s\n", image.Name)
	}
	out.WriteString("\nColour sets (use Color(\"name\")):\n")
	for _, color := range state.Colors {
		fmt.Fprintf(&out, "  %s\n", color)
	}
	out.WriteString("\nSystem icons: any SF Symbol name works in Xcode. The preview draws about 700 common ones, for example " +
		"star, star.fill, heart, plus, minus, trash, pencil, gear, gearshape, magnifyingglass, person, person.circle, house, bell, " +
		"calendar, clock, camera, photo, map, cart, creditcard, chevron.right, chevron.left, arrow.up, arrow.down, checkmark, xmark, " +
		"square.and.arrow.up, ellipsis, list.bullet, folder, doc, envelope, phone, lock, bookmark, flag, tag, paperplane, sun.max, moon.\n")
	return out.String(), nil
}

func searchAPIDocs(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	var lines []string
	collect := func(docs map[string][]apiDoc, kind string) {
		for name, entries := range docs {
			if query != "" && !strings.Contains(strings.ToLower(name), query) {
				continue
			}
			for _, doc := range entries {
				lines = append(lines, fmt.Sprintf("%s %s — %s", kind, doc.Signature, doc.Summary))
			}
		}
	}
	collect(viewDocs, "view")
	collect(modifierDocs, "modifier")
	sort.Strings(lines)
	if len(lines) == 0 {
		var others []string
		for name := range knownModifiers {
			if strings.Contains(strings.ToLower(name), query) {
				others = append(others, name)
			}
		}
		sort.Strings(others)
		if len(others) == 0 {
			return "Nothing matches " + query + ". It may still work in Xcode; the preview reports unknown modifiers as errors."
		}
		return "Supported modifiers matching " + query + " (no description yet): " + strings.Join(others, ", ")
	}
	if len(lines) > 60 {
		lines = lines[:60]
	}
	return strings.Join(lines, "\n")
}

// ---------- HTTP side ----------

func (rt *Runtime) registerAssistantAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", rt.handleState)
	mux.HandleFunc("/api/interact", rt.handleInteract)
}

func (rt *Runtime) handleState(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	colors := make([]string, 0, len(rt.colorAssets))
	for name := range rt.colorAssets {
		colors = append(colors, name)
	}
	sort.Strings(colors)
	state := previewState{
		Dir: rt.dir, Files: rt.fileRefs(), Previews: rt.previews, Current: rt.current, Pinned: rt.pinned,
		Errors: rt.errs, Warnings: rt.warnList, Console: rt.console, Device: rt.store.currentDevice(), DeviceID: rt.store.device,
		Dark: rt.store.dark, Images: rt.imageAssets(), Colors: colors,
	}
	if rt.lastResult != nil {
		state.Outline = outlineOf(rt.lastResult.Root, 0)
		for _, overlay := range rt.lastResult.Overlays {
			state.Outline += "\n" + outlineOf(overlay, 0)
		}
	}
	rt.mu.Unlock()
	writeJSON(w, state)
}

// A readable tree of the rendered screen, so an assistant can "see" the UI.
func outlineOf(node *RNode, depth int) string {
	if node == nil {
		return ""
	}
	var out strings.Builder
	label := node.T
	switch node.T {
	case "root", "lbicon", "lbtitle", "tuple":
		label = ""
	case "text":
		label = fmt.Sprintf("text %q", describeProp(node.P, "s"))
	case "image":
		if sys := describeProp(node.P, "sys"); sys != "" {
			label = "icon " + sys
		} else {
			label = "image " + describeProp(node.P, "asset")
		}
	case "textfield":
		label = fmt.Sprintf("textfield placeholder=%q value=%q", describeProp(node.P, "ph"), describeProp(node.P, "v"))
	case "toggle":
		label = fmt.Sprintf("toggle on=%v", node.P["on"] == true)
	case "page":
		label = fmt.Sprintf("page title=%q", describeProp(node.P, "title"))
	case "tabitem":
		label = "tab"
	case "menu":
		label = "menu"
	case "list", "section":
		label = node.T
	case "sheet", "alert":
		label = node.T + " " + describeProp(node.P, "title")
	}
	if label != "" {
		out.WriteString(strings.Repeat("  ", depth) + label + "\n")
		depth++
	}
	for _, child := range childNodesOf(node) {
		out.WriteString(outlineOf(child, depth))
	}
	return out.String()
}

// Toolbar items, section headers and menu entries are stored in a node's
// properties rather than its children; this walks all of them.
func childNodesOf(node *RNode) []*RNode {
	children := append([]*RNode(nil), node.C...)
	for key, value := range node.P {
		switch nodes := value.(type) {
		case []*RNode:
			children = append(children, nodes...)
		case *RNode:
			children = append(children, nodes)
		case []map[string]interface{}:
			if key == "items" {
				for _, item := range nodes {
					title, _ := item["t"].(string)
					id, _ := item["id"].(string)
					children = append(children, &RNode{T: "tabitem", P: map[string]interface{}{"id": id}, C: []*RNode{{T: "text", P: map[string]interface{}{"s": title}}}})
				}
			}
		}
	}
	return children
}

func describeProp(props map[string]interface{}, key string) string {
	if props == nil {
		return ""
	}
	if value, ok := props[key].(string); ok {
		return value
	}
	return ""
}

// Finds a control by the text shown on it and fires its handler, so an
// assistant can walk through the app without a browser.
func (rt *Runtime) handleInteract(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Action string `json:"action"`
		Text   string `json:"text"`
		Value  string `json:"value"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	rt.mu.Lock()
	var handlerID string
	if rt.lastResult != nil {
		nodes := append([]*RNode{rt.lastResult.Root}, rt.lastResult.Overlays...)
		for _, node := range nodes {
			if handlerID == "" {
				handlerID = findHandlerByText(node, request.Action, strings.ToLower(strings.TrimSpace(request.Text)))
			}
		}
	}
	rt.mu.Unlock()
	if handlerID == "" {
		writeJSON(w, map[string]interface{}{"found": false, "hint": fmt.Sprintf("nothing on screen matches %q; preview_status shows what is visible", request.Text)})
		return
	}
	var value interface{}
	if request.Action == "type" {
		value = request.Value
	}
	rt.HandleEvent(handlerID, value)
	writeJSON(w, map[string]interface{}{"found": true})
}

func isTappable(node *RNode) bool {
	switch node.T {
	case "button", "navlink", "toggle", "tap", "link", "disclosure", "tabitem", "menuitem":
		return true
	}
	return false
}

func nodeText(node *RNode) string {
	var parts []string
	var walk func(n *RNode)
	walk = func(n *RNode) {
		if n.T == "text" {
			parts = append(parts, describeProp(n.P, "s"))
		}
		for _, child := range n.C {
			walk(child)
		}
	}
	walk(node)
	return strings.ToLower(strings.TrimSpace(strings.Join(parts, " ")))
}

func findHandlerByText(node *RNode, action, wanted string) string {
	if node == nil {
		return ""
	}
	if action == "type" && node.T == "textfield" {
		placeholder := strings.ToLower(describeProp(node.P, "ph"))
		current := strings.ToLower(describeProp(node.P, "v"))
		if id, ok := node.P["id"].(string); ok && (placeholder == wanted || current == wanted || strings.Contains(placeholder, wanted)) {
			return id
		}
	}
	if action == "tap" {
		if id, ok := node.P["id"].(string); ok && id != "" && isTappable(node) {
			text := nodeText(node)
			if text == wanted || (wanted != "" && strings.Contains(text, wanted)) {
				return id
			}
		}
	}
	for _, child := range childNodesOf(node) {
		if id := findHandlerByText(child, action, wanted); id != "" {
			return id
		}
	}
	return ""
}
