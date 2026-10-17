package switaculous

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ImageAsset struct {
	Name      string `json:"name"`
	InCatalog bool   `json:"inCatalog"`
}

var assetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]*$`)

func (rt *Runtime) imageAssets() []ImageAsset {
	list := make([]ImageAsset, 0, len(rt.assets))
	for name, path := range rt.assets {
		list = append(list, ImageAsset{Name: name, InCatalog: strings.Contains(path, ".imageset")})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func (rt *Runtime) catalogFolder() string {
	existing := ""
	filepath.Walk(rt.dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || existing != "" {
			return nil
		}
		if info.IsDir() && strings.HasSuffix(info.Name(), ".xcassets") {
			existing = p
			return filepath.SkipDir
		}
		if info.IsDir() && p != rt.dir && skipDir(info.Name()) {
			return filepath.SkipDir
		}
		return nil
	})
	if existing != "" {
		return existing
	}
	return filepath.Join(rt.dir, "Assets.xcassets")
}

func (rt *Runtime) registerAssetAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/asset/image", rt.handleAddImageAsset)
	mux.HandleFunc("/api/asset/color", rt.handleAddColorAsset)
	mux.HandleFunc("/api/asset/delete", rt.handleDeleteAsset)
}

func (rt *Runtime) handleAddImageAsset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	name := strings.TrimSpace(request.Name)
	if !assetNamePattern.MatchString(name) {
		writeJSONError(w, http.StatusBadRequest, "asset names can contain letters, numbers, spaces, dots, dashes and underscores")
		return
	}
	bytes, mimeType, ok := decodeDataURL(request.Data)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "the image could not be read")
		return
	}
	extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/svg+xml": "svg", "image/heic": "heic"}[mimeType]
	if extension == "" {
		writeJSONError(w, http.StatusBadRequest, "only PNG, JPEG, GIF, WebP and SVG images can be added")
		return
	}
	rt.mu.Lock()
	folder := filepath.Join(rt.catalogFolder(), name+".imageset")
	rt.mu.Unlock()
	if err := writeImageSet(folder, name, extension, bytes); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.refreshAssets()
	writeJSON(w, map[string]string{"name": name})
}

// Xcode's image set layout: the file plus a Contents.json listing it at 1x.
func writeImageSet(folder, name, extension string, bytes []byte) error {
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}
	fileName := name + "." + extension
	if err := os.WriteFile(filepath.Join(folder, fileName), bytes, 0644); err != nil {
		return err
	}
	contents := map[string]interface{}{
		"images": []map[string]string{
			{"filename": fileName, "idiom": "universal", "scale": "1x"},
			{"idiom": "universal", "scale": "2x"},
			{"idiom": "universal", "scale": "3x"},
		},
		"info": map[string]interface{}{"author": "xcode", "version": 1},
	}
	encoded, _ := json.MarshalIndent(contents, "", "  ")
	return os.WriteFile(filepath.Join(folder, "Contents.json"), encoded, 0644)
}

func (rt *Runtime) handleAddColorAsset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name  string `json:"name"`
		Light string `json:"light"`
		Dark  string `json:"dark"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	name := strings.TrimSpace(request.Name)
	if !assetNamePattern.MatchString(name) {
		writeJSONError(w, http.StatusBadRequest, "colour names can contain letters, numbers, spaces, dots, dashes and underscores")
		return
	}
	light, err := hexComponents(request.Light)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries := []map[string]interface{}{{"color": map[string]interface{}{"color-space": "srgb", "components": light}, "idiom": "universal"}}
	if request.Dark != "" {
		dark, err := hexComponents(request.Dark)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		entries = append(entries, map[string]interface{}{
			"appearances": []map[string]string{{"appearance": "luminosity", "value": "dark"}},
			"color":       map[string]interface{}{"color-space": "srgb", "components": dark},
			"idiom":       "universal",
		})
	}
	rt.mu.Lock()
	folder := filepath.Join(rt.catalogFolder(), name+".colorset")
	rt.mu.Unlock()
	if err := os.MkdirAll(folder, 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	contents := map[string]interface{}{"colors": entries, "info": map[string]interface{}{"author": "xcode", "version": 1}}
	encoded, _ := json.MarshalIndent(contents, "", "  ")
	if err := os.WriteFile(filepath.Join(folder, "Contents.json"), encoded, 0644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.refreshAssets()
	writeJSON(w, map[string]string{"name": name})
}

func hexComponents(hex string) (map[string]string, error) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return nil, errors.New("colours must be written as #RRGGBB")
	}
	components := map[string]string{"alpha": "1.000"}
	for i, key := range []string{"red", "green", "blue"} {
		n, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, errors.New("colours must be written as #RRGGBB")
		}
		components[key] = fmt.Sprintf("0x%02X", n)
	}
	return components, nil
}

func (rt *Runtime) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	json.NewDecoder(r.Body).Decode(&request)
	suffix := map[string]string{"image": ".imageset", "color": ".colorset"}[request.Kind]
	if suffix == "" || !assetNamePattern.MatchString(request.Name) {
		writeJSONError(w, http.StatusBadRequest, "unknown asset")
		return
	}
	rt.mu.Lock()
	folder := filepath.Join(rt.catalogFolder(), request.Name+suffix)
	rt.mu.Unlock()
	if _, err := os.Stat(filepath.Join(folder, "Contents.json")); err != nil {
		writeJSONError(w, http.StatusNotFound, "only assets inside Assets.xcassets can be deleted here")
		return
	}
	if err := os.RemoveAll(folder); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rt.refreshAssets()
	writeJSON(w, map[string]bool{"deleted": true})
}

func (rt *Runtime) refreshAssets() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.scan()
	rt.renderLocked()
}
