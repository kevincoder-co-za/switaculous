package switaculous

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"strings"
)

type DataV struct {
	Bytes []byte
}

type UIImageV struct {
	Bytes      []byte
	MIMEType   string
	Width      int
	Height     int
	SystemName string
	key        string
}

type PhotosPickerItemV struct {
	Identifier  string
	Bytes       []byte
	ContentType string
}

type imageBlob struct {
	bytes    []byte
	mimeType string
}

func (img *UIImageV) blobKey() string {
	if img.key == "" {
		sum := sha1.Sum(img.Bytes)
		img.key = hex.EncodeToString(sum[:])
	}
	return img.key
}

func newUIImageFromData(data []byte) *UIImageV {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return &UIImageV{Bytes: data, MIMEType: "image/" + format, Width: config.Width, Height: config.Height}
}

func decodeDataURL(url string) ([]byte, string, bool) {
	header, payload, found := strings.Cut(url, ",")
	if !found || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return nil, "", false
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", false
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	return data, mimeType, true
}

func uniformTypeFor(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "public.jpeg"
	case "image/png":
		return "public.png"
	case "image/gif":
		return "com.compuserve.gif"
	case "image/heic":
		return "public.heic"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "public.movie"
	}
	return "public.data"
}

func (img *UIImageV) jpegData(quality float64) *DataV {
	if img.MIMEType == "image/jpeg" {
		return &DataV{Bytes: img.Bytes}
	}
	decoded, _, err := image.Decode(bytes.NewReader(img.Bytes))
	if err != nil {
		return nil
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, decoded, &jpeg.Options{Quality: int(quality*100 + 0.5)}); err != nil {
		return nil
	}
	return &DataV{Bytes: out.Bytes()}
}

func (img *UIImageV) pngData() *DataV {
	if img.MIMEType == "image/png" {
		return &DataV{Bytes: img.Bytes}
	}
	decoded, _, err := image.Decode(bytes.NewReader(img.Bytes))
	if err != nil {
		return nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, decoded); err != nil {
		return nil
	}
	return &DataV{Bytes: out.Bytes()}
}

// ---------- constructors and members ----------

func (ip *Interp) constructMediaType(t *TypeV, args []ArgV, pos Pos) (Value, bool) {
	switch t.Name {
	case "Data":
		if len(args) == 0 {
			return &DataV{}, true
		}
		if v, ok := argNamed(args, "base64Encoded"); ok {
			decoded, err := base64.StdEncoding.DecodeString(describe(v))
			if err != nil {
				return nil, true
			}
			return &DataV{Bytes: decoded}, true
		}
		if v, ok := argNamed(args, "count"); ok {
			n, _ := toInt(v)
			return &DataV{Bytes: make([]byte, n)}, true
		}
		if source, ok := argNamed(args, "contentsOf"); ok {
			return &DataV{Bytes: ip.readFileValue(source, pos)}, true
		}
		switch source := argAt(args, 0).(type) {
		case string:
			return &DataV{Bytes: []byte(source)}, true
		case *DataV:
			return &DataV{Bytes: source.Bytes}, true
		case *ArrayV:
			data := make([]byte, len(source.E))
			for i, e := range source.E {
				n, _ := toInt(e)
				data[i] = byte(n)
			}
			return &DataV{Bytes: data}, true
		}
		return &DataV{}, true
	case "UIImage", "NSImage":
		if v, ok := argNamed(args, "data"); ok {
			data, isData := v.(*DataV)
			if !isData {
				return nil, true
			}
			if img := newUIImageFromData(data.Bytes); img != nil {
				return img, true
			}
			return nil, true
		}
		if v, ok := argNamed(args, "systemName"); ok {
			return &UIImageV{SystemName: describe(v), Width: 24, Height: 24}, true
		}
		if v, ok := argNamed(args, "named"); ok {
			if ip.rt == nil {
				return nil, true
			}
			path, found := ip.rt.assets[describe(v)]
			if !found {
				return nil, true
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, true
			}
			return newUIImageFromData(data), true
		}
		return &UIImageV{}, true
	case "PhotosPickerItem":
		ip.fail(pos, "PhotosPickerItem values come from a PhotosPicker; they can't be created directly")
	case "UIImagePickerController":
		picker := newObj("UIImagePickerController")
		picker.Props["sourceType"] = sym("photoLibrary")
		picker.Props["delegate"] = nil
		picker.Props["allowsEditing"] = false
		picker.Props["cameraDevice"] = sym("rear")
		picker.Props["mediaTypes"] = &ArrayV{E: []Value{"public.image"}}
		return picker, true
	case "PHPickerConfiguration":
		config := newObj("PHPickerConfiguration")
		config.Props["filter"] = nil
		config.Props["selectionLimit"] = 1
		return config, true
	case "PHPickerViewController":
		picker := newObj("PHPickerViewController")
		picker.Props["configuration"] = argAt(args, 0)
		picker.Props["delegate"] = nil
		return picker, true
	case "String":
		if source, ok := argNamed(args, "contentsOf"); ok {
			return string(ip.readFileValue(source, pos)), true
		}
		if v, ok := argNamed(args, "data"); ok {
			if data, isData := v.(*DataV); isData {
				return string(data.Bytes), true
			}
			return nil, true
		}
		if v, ok := argNamed(args, "decoding"); ok {
			switch source := v.(type) {
			case *DataV:
				return string(source.Bytes), true
			case *ArrayV:
				bytes := make([]byte, len(source.E))
				for i, e := range source.E {
					n, _ := toInt(e)
					bytes[i] = byte(n)
				}
				return string(bytes), true
			}
			return "", true
		}
	}
	return nil, false
}

func (ip *Interp) mediaStatic(typeName, name string) (Value, bool) {
	switch typeName {
	case "UIImagePickerController":
		switch name {
		case "InfoKey", "SourceType", "CameraDevice", "CameraCaptureMode":
			return &TypeV{Name: "UIImagePickerController." + name}, true
		}
	}
	return nil, false
}

func (ip *Interp) mediaStaticCall(typeName, name string, args []ArgV) (Value, bool) {
	switch typeName + "." + name {
	case "UIImagePickerController.isSourceTypeAvailable", "UIImagePickerController.isCameraDeviceAvailable":
		return true, true
	case "UIImagePickerController.availableMediaTypes":
		return &ArrayV{E: []Value{"public.image"}}, true
	}
	return nil, false
}

func (ip *Interp) mediaProperty(v Value, name string) (Value, bool) {
	switch x := v.(type) {
	case *DataV:
		switch name {
		case "count":
			return len(x.Bytes), true
		case "isEmpty":
			return len(x.Bytes) == 0, true
		case "description", "debugDescription":
			return describe(x), true
		}
	case *UIImageV:
		switch name {
		case "size":
			return &SizeV{W: float64(x.Width), H: float64(x.Height)}, true
		case "scale":
			return 1.0, true
		case "imageOrientation":
			return sym("up"), true
		case "cgImage", "ciImage":
			return x, true
		}
	case *PhotosPickerItemV:
		switch name {
		case "itemIdentifier":
			return x.Identifier, true
		case "supportedContentTypes":
			return &ArrayV{E: []Value{x.ContentType}}, true
		}
	}
	return nil, false
}

func (ip *Interp) mediaMethod(v Value, name string, args []ArgV, pos Pos) (Value, bool) {
	switch x := v.(type) {
	case *DataV:
		switch name {
		case "base64EncodedString":
			return base64.StdEncoding.EncodeToString(x.Bytes), true
		case "write":
			ip.warn(pos, "Data.write(to:) does nothing in the preview")
			return Void, true
		}
	case *UIImageV:
		switch name {
		case "jpegData":
			quality, _ := toFloat(argAt(args, 0))
			if quality == 0 {
				quality = 0.8
			}
			if data := x.jpegData(quality); data != nil {
				return data, true
			}
			return nil, true
		case "pngData":
			if data := x.pngData(); data != nil {
				return data, true
			}
			return nil, true
		case "preparingThumbnail", "withRenderingMode", "withTintColor", "resizableImage", "imageFlippedForRightToLeftLayoutDirection":
			return x, true
		case "byPreparingThumbnail":
			return x, true
		}
	case *PhotosPickerItemV:
		if name == "loadTransferable" {
			return ip.loadTransferable(x, args, pos), true
		}
	}
	return nil, false
}

func (ip *Interp) loadTransferable(item *PhotosPickerItemV, args []ArgV, pos Pos) Value {
	requested, _ := argNamed(args, "type")
	switch modelTypeName(requested) {
	case "Data":
		return &DataV{Bytes: item.Bytes}
	case "Image":
		if img := newUIImageFromData(item.Bytes); img != nil {
			return &ViewV{Kind: "Image", Args: []ArgV{{Label: "uiImage", V: img}}, Pos: pos}
		}
		return nil
	case "UIImage":
		return newUIImageFromData(item.Bytes)
	case "String":
		return string(item.Bytes)
	}
	ip.warn(pos, "loadTransferable(type: %s.self) is not supported by the preview; use Data.self or Image.self", modelTypeName(requested))
	return nil
}

func (ip *Interp) mediaNativeObjMethod(o *NativeObj, name string, args []ArgV, pos Pos) (Value, bool) {
	switch o.Kind {
	case "NSItemProvider":
		switch name {
		case "canLoadObject":
			return true, true
		case "loadObject":
			handler, _ := argNamed(args, "completionHandler")
			if handler == nil {
				for _, a := range args {
					if a.Trailing {
						handler = a.V
					}
				}
			}
			if handler != nil {
				ip.callValue(handler, []ArgV{{V: o.Props["image"]}, {V: nil}}, pos, nil)
			}
			return newObj("Progress"), true
		case "hasItemConformingToTypeIdentifier":
			return true, true
		}
	case "UIImagePickerController", "PHPickerViewController":
		if name == "dismiss" {
			if dismiss, ok := o.Props["presentationDismiss"].(*NativeV); ok {
				dismiss.Fn(&CallCtx{ip: ip, Pos: pos})
			}
			return Void, true
		}
	}
	return nil, false
}

// ---------- serving picked images to the browser ----------

func (r *Render) imageURL(img *UIImageV) string {
	key := img.blobKey()
	r.imageBlobs[key] = imageBlob{bytes: img.Bytes, mimeType: img.MIMEType}
	return "/blob/" + key
}

func (rt *Runtime) keepImageBlobsLocked(current map[string]imageBlob) {
	rt.previousImageBlobs = rt.imageBlobs
	rt.imageBlobs = current
}

func (rt *Runtime) serveImageBlob(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/blob/")
	rt.mu.Lock()
	blob, ok := rt.imageBlobs[key]
	if !ok {
		blob, ok = rt.previousImageBlobs[key]
	}
	rt.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", blob.mimeType)
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Write(blob.bytes)
}

// ---------- views ----------

func (r *Render) uiImageNode(n *RNode, v Value) {
	img, ok := v.(*UIImageV)
	if !ok {
		r.fail(Pos{}, "Image(uiImage:) needs a UIImage, but got a value of type '%s'", typeName(v))
	}
	if img.SystemName != "" {
		n.P["sys"] = img.SystemName
		return
	}
	if len(img.Bytes) == 0 {
		return
	}
	n.P["url"] = r.imageURL(img)
	n.P["nw"] = img.Width
	n.P["nh"] = img.Height
}

func pickerAcceptFilter(filter Value) string {
	switch symName(filter) {
	case "videos", "screenRecordings", "slomoVideos", "cinematicVideos":
		return "video/*"
	case "any":
		return "image/*,video/*"
	}
	return "image/*"
}

func (r *Render) photoItemsFromEvent(value interface{}, path string) []Value {
	files, _ := value.([]interface{})
	var items []Value
	for i, f := range files {
		file, _ := f.(map[string]interface{})
		url, _ := file["data"].(string)
		data, mimeType, ok := decodeDataURL(url)
		if !ok {
			continue
		}
		name, _ := file["name"].(string)
		items = append(items, &PhotosPickerItemV{
			Identifier:  fmt.Sprintf("%s/%d/%s", path, i, name),
			Bytes:       data,
			ContentType: uniformTypeFor(mimeType),
		})
	}
	return items
}

func (r *Render) setPickedPhotos(selection *BindingV, items []Value) {
	if selection == nil || len(items) == 0 {
		return
	}
	if _, isArray := selection.Get().(*ArrayV); isArray {
		selection.Set(&ArrayV{E: items})
		return
	}
	selection.Set(items[0])
}

func (r *Render) photosPicker(v *ViewV, path string, env *REnv) *RNode {
	selectionValue, _ := v.arg("selection")
	selection, _ := selectionValue.(*BindingV)
	if selection == nil {
		r.fail(v.Pos, "PhotosPicker needs a binding, for example PhotosPicker(selection: $selectedItem, matching: .images)")
	}
	filter, _ := v.arg("matching")
	maxCount, hasMax := v.arg("maxSelectionCount")
	_, multiple := selection.Get().(*ArrayV)
	if hasMax {
		if n, ok := toInt(maxCount); ok && n != 1 {
			multiple = true
		}
	}
	label := r.labelFrom(v, path, env, "label")
	if label == nil {
		for _, a := range v.Args {
			if a.Trailing {
				label = r.resolve(r.callBuilder(a.V), path+"/l", env)
			}
		}
	}
	style := r.buttonStyleName(env)
	if style == "custom" {
		style = "automatic"
	}
	n := &RNode{T: "button", ID: path, P: map[string]interface{}{"style": style, "multi": multiple, "accept": pickerAcceptFilter(filter)}, C: label}
	if !env.disabled {
		n.P["photos"] = r.on(path+"#photos", func(value interface{}) {
			r.setPickedPhotos(selection, r.photoItemsFromEvent(value, path))
		})
	} else {
		n.P["dis"] = true
	}
	return n
}

func (r *Render) presentPhotosPicker(m *ModV, path string, env *REnv) {
	presentedValue, _ := argNamed(m.Args, "isPresented")
	presented, _ := presentedValue.(*BindingV)
	if presented == nil || presented.Get() != true || env.hidden {
		return
	}
	selectionValue, _ := argNamed(m.Args, "selection")
	selection, _ := selectionValue.(*BindingV)
	filter, _ := argNamed(m.Args, "matching")
	multiple := false
	if selection != nil {
		_, multiple = selection.Get().(*ArrayV)
	}
	key := path + "#photospicker@" + posKey(m.Pos)
	n := &RNode{T: "photopicker", ID: key, P: map[string]interface{}{"multi": multiple, "accept": pickerAcceptFilter(filter)}}
	n.P["pick"] = r.on(key+"#pick", func(value interface{}) {
		r.setPickedPhotos(selection, r.photoItemsFromEvent(value, key))
		presented.Set(false)
	})
	n.P["cancel"] = r.on(key+"#cancel", func(interface{}) { presented.Set(false) })
	r.overlays = append(r.overlays, n)
}

// UIKit pickers wrapped in UIViewControllerRepresentable are shown with the
// browser's camera or file picker, and report back through the same delegate
// methods UIKit would call.
func (r *Render) representable(sv *StructV, path string, env *REnv) []*RNode {
	ip := r.ip
	ti := sv.T
	p := path + ":" + ti.Name
	inst := r.prepareStruct(sv, p, env)
	protocol, makeName := "UIViewControllerRepresentable", "makeUIViewController"
	if !ti.conformsTo(protocol) {
		protocol, makeName = "UIViewRepresentable", "makeUIView"
	}
	makeMethods := ip.findMethods(ti, makeName)
	if makeMethods == nil {
		r.fail(Pos{}, "type '%s' does not conform to protocol '%s' (add func %s(context: Context))", ti.Name, protocol, makeName)
	}
	context := newObj("RepresentableContext")
	if coordinatorMethods := ip.findMethods(ti, "makeCoordinator"); coordinatorMethods != nil {
		context.Props["coordinator"] = ip.callValue(ip.bindMethods(coordinatorMethods, inst, ti, false), nil, Pos{}, nil)
	}
	controller := ip.callValue(ip.bindMethods(makeMethods, inst, ti, false), []ArgV{{Label: "context", V: context}}, Pos{}, nil)
	picker, _ := controller.(*NativeObj)
	if picker == nil || (picker.Kind != "UIImagePickerController" && picker.Kind != "PHPickerViewController") {
		return []*RNode{{T: "placeholder", ID: p, P: map[string]interface{}{"text": ti.Name + " (UIKit view)"}}}
	}
	dismiss, _ := env.envVals["dismiss"].(*NativeV)
	if dismiss != nil {
		picker.Props["presentationDismiss"] = dismiss
	}
	if updateMethods := ip.findMethods(ti, "updateUIViewController"); updateMethods != nil {
		ip.callValue(ip.bindMethods(updateMethods, inst, ti, false), []ArgV{{V: picker}, {Label: "context", V: context}}, Pos{}, nil)
	}
	source := "photoLibrary"
	front := false
	multiple := false
	if picker.Kind == "UIImagePickerController" {
		source = symName(picker.Props["sourceType"])
		front = symName(picker.Props["cameraDevice"]) == "front"
	} else if config, ok := picker.Props["configuration"].(*NativeObj); ok {
		limit, _ := toInt(config.Props["selectionLimit"])
		multiple = limit != 1
	}
	n := &RNode{T: "camera", ID: p, P: map[string]interface{}{"source": source, "front": front, "multi": multiple}}
	n.P["pick"] = r.on(p+"#pick", func(value interface{}) {
		items := r.photoItemsFromEvent(value, p)
		r.deliverPickedImages(picker, items, dismiss)
	})
	n.P["cancel"] = r.on(p+"#cancel", func(interface{}) {
		r.cancelPicker(picker, dismiss)
	})
	return []*RNode{n}
}

func (r *Render) deliverPickedImages(picker *NativeObj, items []Value, dismiss *NativeV) {
	ip := r.ip
	delegate := picker.Props["delegate"]
	if delegate == nil {
		if dismiss != nil {
			dismiss.Fn(&CallCtx{ip: ip})
		}
		return
	}
	if picker.Kind == "PHPickerViewController" {
		results := &ArrayV{}
		for _, item := range items {
			photo := item.(*PhotosPickerItemV)
			provider := newObj("NSItemProvider")
			provider.Props["image"] = newUIImageFromData(photo.Bytes)
			result := newObj("PHPickerResult")
			result.Props["itemProvider"] = provider
			result.Props["assetIdentifier"] = photo.Identifier
			results.E = append(results.E, result)
		}
		ip.callMethod(delegate, nil, "picker", []ArgV{{V: picker}, {Label: "didFinishPicking", V: results}}, Pos{}, nil)
		return
	}
	info := newDict()
	if len(items) > 0 {
		img := newUIImageFromData(items[0].(*PhotosPickerItemV).Bytes)
		info.Set(sym("originalImage"), img)
		if picker.Props["allowsEditing"] == true {
			info.Set(sym("editedImage"), img)
		}
		info.Set(sym("mediaType"), "public.image")
	}
	ip.callMethod(delegate, nil, "imagePickerController", []ArgV{{V: picker}, {Label: "didFinishPickingMediaWithInfo", V: info}}, Pos{}, nil)
}

// UIKit dismisses the picker itself on Cancel unless the delegate handles it.
func (r *Render) cancelPicker(picker *NativeObj, dismiss *NativeV) {
	ip := r.ip
	delegate := picker.Props["delegate"]
	if obj, ok := delegate.(*ObjectV); ok {
		cancelName := "imagePickerControllerDidCancel"
		if picker.Kind == "PHPickerViewController" {
			ip.callMethod(delegate, nil, "picker", []ArgV{{V: picker}, {Label: "didFinishPicking", V: &ArrayV{}}}, Pos{}, nil)
			return
		}
		if ip.findMethods(obj.T, cancelName) != nil {
			ip.callMethod(delegate, nil, cancelName, []ArgV{{V: picker}}, Pos{}, nil)
			return
		}
	}
	if dismiss != nil {
		dismiss.Fn(&CallCtx{ip: ip})
	}
}
