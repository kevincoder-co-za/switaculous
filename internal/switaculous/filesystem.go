package switaculous

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// Files an app writes in the preview land in a sandbox folder per project,
// standing in for the app container: FileManager.default.temporaryDirectory
// and the documents directory both live under it, and nothing outside it is
// ever written.
func (ip *Interp) sandboxRoot() string {
	key := "scratch"
	if ip.rt != nil {
		sum := sha1.Sum([]byte(ip.rt.dir))
		key = filepath.Base(ip.rt.dir) + "-" + hex.EncodeToString(sum[:4])
	}
	return filepath.Join(os.TempDir(), "switaculous-sandbox", key)
}

func (ip *Interp) sandboxDirectory(name string) *URLV {
	dir := filepath.Join(ip.sandboxRoot(), name)
	os.MkdirAll(dir, 0o755)
	return &URLV{S: dir}
}

func (ip *Interp) isInsideSandbox(path string) bool {
	rel, err := filepath.Rel(ip.sandboxRoot(), path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func urlPath(v Value) string {
	switch x := v.(type) {
	case *URLV:
		return strings.TrimPrefix(x.S, "file://")
	case string:
		return x
	}
	return ""
}

// Writes a file for Data.write(to:) and String.write(to:), refusing paths
// outside the sandbox so a preview can never touch the user's own files.
func (ip *Interp) writeSandboxFile(target Value, content []byte, pos Pos) {
	path := urlPath(target)
	if path == "" || !ip.isInsideSandbox(path) {
		panic(&ThrowSignal{V: newCodingError("CocoaError", "The preview can only write inside the app's own folders (temporaryDirectory or the documents directory): "+describe(target)), Pos: pos})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(&ThrowSignal{V: newCodingError("CocoaError", "The folder couldn't be created: "+err.Error()), Pos: pos})
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		panic(&ThrowSignal{V: newCodingError("CocoaError", "The file couldn't be saved: "+err.Error()), Pos: pos})
	}
}

func (ip *Interp) fileManagerProperty(name string) (Value, bool) {
	switch name {
	case "temporaryDirectory":
		return ip.sandboxDirectory("tmp"), true
	case "currentDirectoryPath":
		return ip.sandboxRoot(), true
	}
	return nil, false
}

func (ip *Interp) fileManagerMethod(name string, args []ArgV, pos Pos) (Value, bool) {
	switch name {
	case "urls":
		directory, _ := argNamed(args, "for")
		return &ArrayV{E: []Value{ip.searchPathDirectory(directory)}, Elem: &TypeRef{Name: "URL"}}, true
	case "url":
		directory, _ := argNamed(args, "for")
		return ip.searchPathDirectory(directory), true
	case "fileExists":
		path, _ := argNamed(args, "atPath")
		_, err := os.Stat(urlPath(path))
		return err == nil, true
	case "createDirectory":
		path, ok := argNamed(args, "at")
		if !ok {
			path, _ = argNamed(args, "atPath")
		}
		if dir := urlPath(path); ip.isInsideSandbox(dir) {
			os.MkdirAll(dir, 0o755)
		}
		return Void, true
	case "removeItem":
		path, ok := argNamed(args, "at")
		if !ok {
			path, _ = argNamed(args, "atPath")
		}
		if target := urlPath(path); ip.isInsideSandbox(target) {
			os.RemoveAll(target)
		}
		return Void, true
	case "copyItem", "moveItem":
		from, _ := argNamed(args, "at")
		to, _ := argNamed(args, "to")
		source, destination := urlPath(from), urlPath(to)
		if !ip.isInsideSandbox(destination) {
			return Void, true
		}
		content, err := os.ReadFile(source)
		if err != nil {
			panic(&ThrowSignal{V: newCodingError("CocoaError", "The file couldn't be opened because there is no such file: "+source), Pos: pos})
		}
		os.MkdirAll(filepath.Dir(destination), 0o755)
		os.WriteFile(destination, content, 0o644)
		if name == "moveItem" && ip.isInsideSandbox(source) {
			os.Remove(source)
		}
		return Void, true
	case "contentsOfDirectory":
		path, ok := argNamed(args, "at")
		if !ok {
			path, _ = argNamed(args, "atPath")
		}
		dir := urlPath(path)
		entries, err := os.ReadDir(dir)
		if err != nil {
			panic(&ThrowSignal{V: newCodingError("CocoaError", "The folder couldn't be opened because there is no such folder: "+dir), Pos: pos})
		}
		out := &ArrayV{Elem: &TypeRef{Name: "URL"}}
		for _, entry := range entries {
			if ok {
				out.E = append(out.E, &URLV{S: filepath.Join(dir, entry.Name())})
			} else {
				out.E = append(out.E, entry.Name())
			}
		}
		return out, true
	case "attributesOfItem":
		path, _ := argNamed(args, "atPath")
		info, err := os.Stat(urlPath(path))
		if err != nil {
			panic(&ThrowSignal{V: newCodingError("CocoaError", "The file couldn't be opened because there is no such file: "+describe(path)), Pos: pos})
		}
		attributes := newDict()
		attributes.Set("size", int(info.Size()))
		attributes.Set("modificationDate", &DateV{T: info.ModTime()})
		return attributes, true
	}
	return nil, false
}

func (ip *Interp) searchPathDirectory(directory Value) *URLV {
	switch symName(directory) {
	case "cachesDirectory":
		return ip.sandboxDirectory("Library/Caches")
	case "applicationSupportDirectory":
		return ip.sandboxDirectory("Library/Application Support")
	case "libraryDirectory":
		return ip.sandboxDirectory("Library")
	}
	return ip.sandboxDirectory("Documents")
}

func (ip *Interp) urlProperty(u *URLV, name string) (Value, bool) {
	path := urlPath(u)
	switch name {
	case "pathExtension":
		return strings.TrimPrefix(filepath.Ext(path), "."), true
	case "deletingLastPathComponent":
		return &URLV{S: filepath.Dir(path)}, true
	case "deletingPathExtension":
		return &URLV{S: strings.TrimSuffix(path, filepath.Ext(path))}, true
	case "isFileURL":
		return !strings.Contains(u.S, "://") || strings.HasPrefix(u.S, "file://"), true
	case "pathComponents":
		out := &ArrayV{Elem: &TypeRef{Name: "String"}}
		for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
			out.E = append(out.E, part)
		}
		return out, true
	case "scheme":
		if i := strings.Index(u.S, "://"); i >= 0 {
			return u.S[:i], true
		}
		return nil, true
	case "standardizedFileURL", "absoluteURL":
		return u, true
	}
	return nil, false
}

func (ip *Interp) urlMethod(u *URLV, name string, args []ArgV, pos Pos) (Value, bool) {
	path := urlPath(u)
	switch name {
	case "appending", "appendingPathComponent":
		component, ok := argNamed(args, "path")
		if !ok {
			component, ok = argNamed(args, "component")
		}
		if !ok {
			component = argAt(args, 0)
		}
		return &URLV{S: filepath.Join(path, describe(component))}, true
	case "appendingPathExtension":
		return &URLV{S: path + "." + strings.TrimPrefix(describe(argAt(args, 0)), ".")}, true
	case "deletingLastPathComponent":
		return &URLV{S: filepath.Dir(path)}, true
	case "deletingPathExtension":
		return &URLV{S: strings.TrimSuffix(path, filepath.Ext(path))}, true
	case "path":
		return path, true
	case "startAccessingSecurityScopedResource":
		return true, true
	case "stopAccessingSecurityScopedResource":
		return Void, true
	case "checkResourceIsReachable":
		_, err := os.Stat(path)
		return err == nil, true
	}
	return nil, false
}

var urlMethodNames = map[string]bool{
	"appending": true, "appendingPathComponent": true, "appendingPathExtension": true, "deletingLastPathComponent": true,
	"deletingPathExtension": true, "path": true, "startAccessingSecurityScopedResource": true,
	"stopAccessingSecurityScopedResource": true, "checkResourceIsReachable": true,
}
