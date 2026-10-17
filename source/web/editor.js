(() => {
  'use strict';

  const AUTOSAVE_DELAY_MS = 600;
  const openFiles = new Map();
  let editor = null;
  let currentPath = '';
  let projectDir = '';
  let knownFiles = [];
  let errorMarks = [];
  let saveTimer = null;
  let ignoreNextChange = false;
  let lastErrors = [];

  const byId = (id) => document.getElementById(id);
  const createElement = (tag, className, text) => {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text !== undefined) element.textContent = text;
    return element;
  };

  async function postJSON(url, body) {
    const response = await fetch(url, { method: 'POST', body: JSON.stringify(body || {}) });
    const text = await response.text();
    const data = text ? JSON.parse(text) : {};
    if (!response.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  function showToast(message, isError) {
    const toast = byId('toast');
    toast.textContent = message;
    toast.className = 'show' + (isError ? ' error' : '');
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => { toast.className = ''; }, 3500);
  }

  function setSaveStatus(text) {
    byId('savestatus').textContent = text;
  }

  // ---------- editor ----------

  function createEditor() {
    editor = CodeMirror(byId('editor'), {
      value: '',
      mode: 'swift',
      lineNumbers: true,
      indentUnit: 4,
      tabSize: 4,
      indentWithTabs: false,
      matchBrackets: true,
      autoCloseBrackets: true,
      styleActiveLine: true,
      gutters: ['CodeMirror-linenumbers', 'error-gutter'],
      extraKeys: {
        'Ctrl-S': () => saveCurrentFile(),
        'Cmd-S': () => saveCurrentFile(),
        'Ctrl-/': 'toggleComment',
        'Cmd-/': 'toggleComment',
        'Ctrl-Space': () => showCompletions(true),
        'Ctrl-Shift-Space': () => { const t = editor.getTokenAt(editor.getCursor()); const d = docsFor(t.string); if (d.length) showDocTip(d.map((x) => x.sig).join('\n'), d[0].doc); },
        Tab: (cm) => {
          if (cm.somethingSelected()) cm.indentSelection('add');
          else cm.replaceSelection('    ', 'end');
        },
        'Shift-Tab': (cm) => cm.indentSelection('subtract'),
      },
    });
    editor.on('change', onEditorChange);
    editor.on('inputRead', (cm, change) => {
      const typed = change.text.join('');
      if (typed === '.' || typed === '"' || /^[A-Za-z$]$/.test(typed)) showCompletions(false);
    });
  }

  function onEditorChange() {
    if (ignoreNextChange) {
      ignoreNextChange = false;
      return;
    }
    const entry = openFiles.get(currentPath);
    if (!entry) return;
    entry.dirty = true;
    setSaveStatus('Edited');
    renderFileTree();
    scheduleAutosave();
  }

  // Saving while the completion list is open would build half-typed code, so
  // the save waits until the list closes.
  function scheduleAutosave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      if (editor.state.completionActive) {
        scheduleAutosave();
        return;
      }
      saveCurrentFile();
    }, AUTOSAVE_DELAY_MS);
  }

  async function saveCurrentFile() {
    clearTimeout(saveTimer);
    const entry = openFiles.get(currentPath);
    if (!entry || !entry.dirty) return;
    const path = currentPath;
    const content = entry.doc.getValue();
    entry.saving = true;
    setSaveStatus('Saving…');
    try {
      await postJSON('/api/file', { path, content });
      entry.savedText = content;
      entry.dirty = entry.doc.getValue() !== content;
      setSaveStatus(entry.dirty ? 'Edited' : 'Saved');
    } catch (error) {
      setSaveStatus('Not saved');
      showToast('Could not save: ' + error.message, true);
    } finally {
      entry.saving = false;
      renderFileTree();
    }
  }

  async function openFile(path) {
    if (!path) return;
    if (currentPath && currentPath !== path) await saveCurrentFile();
    let entry = openFiles.get(path);
    if (!entry) {
      let content = '';
      try {
        const response = await fetch('/api/file?path=' + encodeURIComponent(path));
        content = (await response.json()).content || '';
      } catch (error) {
        showToast('Could not open ' + path, true);
        return;
      }
      entry = { doc: CodeMirror.Doc(content, 'swift'), savedText: content, dirty: false, saving: false, version: versionOf(path) };
      openFiles.set(path, entry);
    }
    currentPath = path;
    ignoreNextChange = false;
    editor.swapDoc(entry.doc);
    editor.focus();
    byId('filename').textContent = path;
    setSaveStatus(entry.dirty ? 'Edited' : 'Saved');
    localStorage.setItem('swiftuitutor.file.' + projectDir, path);
    renderFileTree();
    showErrorMarks(lastErrors);
    fetch('/api/focus', { method: 'POST', body: JSON.stringify({ path }) });
  }

  function versionOf(path) {
    const file = knownFiles.find((f) => f.path === path);
    return file ? file.ver : '';
  }

  // Files edited outside the browser (for example in VS Code) are reloaded here,
  // unless the same file has unsaved edits in this editor.
  async function reloadFilesChangedOnDisk() {
    for (const [path, entry] of openFiles) {
      const diskVersion = versionOf(path);
      if (!diskVersion) {
        openFiles.delete(path);
        continue;
      }
      if (diskVersion === entry.version) continue;
      entry.version = diskVersion;
      if (entry.dirty || entry.saving) continue;
      const response = await fetch('/api/file?path=' + encodeURIComponent(path));
      const content = (await response.json()).content || '';
      if (content === entry.doc.getValue()) continue;
      const cursor = entry.doc.getCursor();
      const scroll = path === currentPath ? editor.getScrollInfo() : null;
      if (path === currentPath) ignoreNextChange = true;
      entry.doc.setValue(content);
      entry.doc.setCursor(cursor);
      entry.savedText = content;
      if (scroll) editor.scrollTo(scroll.left, scroll.top);
    }
  }

  // ---------- errors ----------

  function showErrorMarks(errors) {
    lastErrors = errors || [];
    for (const mark of errorMarks) mark.clear();
    errorMarks = [];
    editor.clearGutter('error-gutter');
    for (const error of lastErrors) {
      if (error.path !== currentPath || !error.line) continue;
      const lineIndex = error.line - 1;
      const marker = createElement('div', 'error-marker', '●');
      marker.title = error.msg;
      editor.setGutterMarker(lineIndex, 'error-gutter', marker);
      const lineHandle = editor.addLineClass(lineIndex, 'background', 'error-line');
      const message = createElement('div', 'error-inline', error.msg);
      const widget = editor.addLineWidget(lineIndex, message, { coverGutter: false, noHScroll: true });
      errorMarks.push({ clear: () => { editor.removeLineClass(lineHandle, 'background', 'error-line'); widget.clear(); } });
    }
  }

  function jumpToError(error) {
    if (!error || !error.path) return;
    const goToLine = () => {
      editor.setCursor({ line: error.line - 1, ch: Math.max(0, error.col - 1) });
      editor.scrollIntoView(null, 120);
      editor.focus();
    };
    if (error.path === currentPath) goToLine();
    else openFile(error.path).then(goToLine);
  }

  // ---------- autocomplete ----------

  const CURSOR_MARKER = '\u0000';

  function showCompletions(explicit) {
    editor.showHint({
      completeSingle: false,
      closeCharacters: /[\s()\[\]{};>,]/,
      hint: (cm) => new Promise((resolve) => {
        const cursor = cm.getCursor();
        const lineText = cm.getLine(cursor.line);
        let start = cursor.ch;
        while (start > 0 && /[\w$]/.test(lineText[start - 1])) start--;
        const prefix = lineText.slice(start, cursor.ch);
        const afterDot = lineText[start - 1] === '.';
        if (!explicit && !afterDot && prefix.length < 2 && !insideStringAt(lineText, start)) { resolve(null); return; }
        requestCompletions(cm.getValue(), cursor.line, cursor.ch).then((response) => {
          let items = response.items || [];
          if (response.context === 'string') items = stringCompletions(lineText, start, response.prefix);
          if (!items.length) { resolve(null); return; }
          resolve({
            list: items.map((item) => completionEntry(item)),
            from: CodeMirror.Pos(cursor.line, start),
            to: cursor,
          });
        });
      }),
    });
  }

  function insideStringAt(lineText, position) {
    let inString = false;
    for (let i = 0; i < position; i++) {
      if (lineText[i] === '\\') i++;
      else if (lineText[i] === '"') inString = !inString;
    }
    return inString;
  }

  let completionRequestTimer = null;
  function requestCompletions(text, line, ch) {
    return new Promise((resolve) => {
      clearTimeout(completionRequestTimer);
      completionRequestTimer = setTimeout(async () => {
        try {
          resolve(await postJSON('/api/complete', { text, line, ch }));
        } catch (error) {
          resolve({ items: [] });
        }
      }, 60);
    });
  }

  // Inside Image(systemName: "…") the names of the icon set are offered, and
  // inside Image("…") or Color("…") the project's asset names.
  function stringCompletions(lineText, start, prefix) {
    const head = lineText.slice(0, start);
    const lowerPrefix = prefix.toLowerCase();
    const matches = (names, kind) => names.filter((name) => name.toLowerCase().includes(lowerPrefix))
      .sort((a, b) => (a.startsWith(lowerPrefix) === b.startsWith(lowerPrefix) ? a.localeCompare(b) : a.startsWith(lowerPrefix) ? -1 : 1))
      .slice(0, 60).map((name) => ({ text: name, insert: name, detail: '', doc: '', kind }));
    if (/systemImage:\s*"[\w.]*$|systemName:\s*"[\w.]*$/.test(head)) return matches(window.SFSymbolNames || [], 'symbol');
    if (/Image\(\s*"[\w ]*$/.test(head)) return matches(projectAssets.images || [], 'image');
    if (/Color\(\s*"[\w ]*$/.test(head)) return matches(projectAssets.colors || [], 'color');
    return [];
  }

  function completionEntry(item) {
    return {
      text: item.text,
      item,
      render: (element) => {
        const kind = document.createElement('span');
        kind.className = 'hint-kind kind-' + item.kind;
        kind.textContent = kindGlyph(item.kind);
        if (item.kind === 'symbol' && window.SFSymbol) { kind.innerHTML = window.SFSymbol(item.text).svg; kind.classList.add('hint-symbol'); }
        const name = document.createElement('span');
        name.className = 'hint-name';
        name.textContent = item.text;
        const detail = document.createElement('span');
        detail.className = 'hint-detail';
        detail.textContent = item.detail && item.detail !== item.text ? item.detail : '';
        element.append(kind, name, detail);
        if (item.doc) element.title = item.doc;
      },
      hint: (cm, data, completion) => insertCompletion(cm, data, completion.item),
    };
  }

  function kindGlyph(kind) {
    return { field: 'p', method: 'f', modifier: 'm', view: 'V', type: 'T', case: 'c', static: 's', keyword: 'k', local: 'v', param: 'v', function: 'f', global: 'g', symbol: '', image: 'i', color: '■' }[kind] || '•';
  }

  function insertCompletion(cm, data, item) {
    const insert = item.insert || item.text;
    const caretOffset = insert.indexOf(CURSOR_MARKER);
    const text = insert.replace(CURSOR_MARKER, '');
    cm.replaceRange(text, data.from, data.to, 'complete');
    if (caretOffset >= 0) cm.setCursor(CodeMirror.Pos(data.from.line, data.from.ch + caretOffset));
    if (item.kind === 'method' || item.kind === 'modifier' || item.kind === 'view') showDocTip(item.detail, item.doc);
  }

  // ---------- hover help ----------

  let apiDocs = { modifiers: {}, views: {} };
  let docTipTimer = null;

  async function loadDocs() {
    try {
      apiDocs = await (await fetch('/api/docs')).json();
    } catch (error) {
      apiDocs = { modifiers: {}, views: {} };
    }
  }

  function docsFor(word) {
    return (apiDocs.views[word] || []).concat(apiDocs.modifiers[word] || []);
  }

  function showDocTip(signature, summary, anchor) {
    const tip = byId('doctip');
    if (!signature && !summary) { tip.classList.remove('show'); return; }
    tip.innerHTML = '';
    const sig = document.createElement('div');
    sig.className = 'doctip-sig';
    sig.textContent = signature;
    const doc = document.createElement('div');
    doc.className = 'doctip-doc';
    doc.textContent = summary;
    tip.append(sig, doc);
    const coords = anchor || editor.cursorCoords(true, 'page');
    tip.style.left = Math.min(coords.left, window.innerWidth - 440) + 'px';
    tip.style.top = (coords.bottom + 6) + 'px';
    tip.classList.add('show');
    clearTimeout(docTipTimer);
    docTipTimer = setTimeout(() => tip.classList.remove('show'), 6000);
  }

  function wireHoverHelp() {
    const wrapper = editor.getWrapperElement();
    let lastWord = '';
    wrapper.addEventListener('mousemove', (event) => {
      const position = editor.coordsChar({ left: event.clientX, top: event.clientY }, 'window');
      const token = editor.getTokenAt(position);
      const word = token && /^[A-Za-z_]\w*$/.test(token.string) ? token.string : '';
      if (word === lastWord) return;
      lastWord = word;
      const docs = docsFor(word);
      if (!docs.length) { byId('doctip').classList.remove('show'); return; }
      const coords = editor.charCoords({ line: position.line, ch: token.start }, 'page');
      showDocTip(docs.map((d) => d.sig).join('\n'), docs[0].doc, coords);
    });
    wrapper.addEventListener('mouseleave', () => { lastWord = ''; byId('doctip').classList.remove('show'); });
  }

  let projectAssets = { images: [], colors: [] };

  // ---------- file explorer ----------

  function buildFolderTree(paths) {
    const root = { folders: {}, files: [] };
    for (const path of paths) {
      const parts = path.split('/');
      let folder = root;
      for (const part of parts.slice(0, -1)) {
        if (!folder.folders[part]) folder.folders[part] = { folders: {}, files: [] };
        folder = folder.folders[part];
      }
      folder.files.push(path);
    }
    return root;
  }

  function renderFileTree() {
    const list = byId('filetree');
    list.innerHTML = '';
    const errorPaths = new Set(lastErrors.map((error) => error.path));
    const tree = buildFolderTree(knownFiles.map((file) => file.path));

    const addFolder = (folder, depth) => {
      for (const name of Object.keys(folder.folders).sort()) {
        const row = createElement('div', 'tree-folder');
        row.style.paddingLeft = (10 + depth * 14) + 'px';
        row.innerHTML = '<span class="folder-icon">▾</span>';
        row.appendChild(createElement('span', '', name));
        list.appendChild(row);
        addFolder(folder.folders[name], depth + 1);
      }
      for (const path of folder.files.sort()) {
        const entry = openFiles.get(path);
        const row = createElement('div', 'tree-file' + (path === currentPath ? ' selected' : ''));
        row.style.paddingLeft = (12 + depth * 14) + 'px';
        row.dataset.path = path;
        row.appendChild(createElement('span', 'swift-icon', 'S'));
        row.appendChild(createElement('span', 'file-name', path.split('/').pop()));
        if (entry && entry.dirty) row.appendChild(createElement('span', 'dirty-dot', '●'));
        if (errorPaths.has(path)) row.appendChild(createElement('span', 'error-dot', '!'));
        const actions = createElement('span', 'file-actions');
        const renameButton = createElement('button', 'icon-button', '✎');
        renameButton.title = 'Rename';
        renameButton.dataset.action = 'rename';
        const deleteButton = createElement('button', 'icon-button', '🗑');
        deleteButton.title = 'Delete';
        deleteButton.dataset.action = 'delete';
        actions.append(renameButton, deleteButton);
        row.appendChild(actions);
        list.appendChild(row);
      }
    };
    addFolder(tree, 0);
    if (!knownFiles.length) list.appendChild(createElement('div', 'tree-empty', 'No .swift files yet. Click + to add one.'));
  }

  function askForText(title, initialValue, confirmLabel) {
    return new Promise((resolve) => {
      const dialog = byId('dialog');
      byId('dialogtitle').textContent = title;
      const input = byId('dialoginput');
      input.style.display = '';
      input.value = initialValue;
      byId('dialogok').textContent = confirmLabel;
      dialog.classList.add('show');
      input.focus();
      input.setSelectionRange(0, initialValue.replace(/\.swift$/, '').length);
      const close = (value) => {
        dialog.classList.remove('show');
        byId('dialogok').onclick = null;
        byId('dialogcancel').onclick = null;
        input.onkeydown = null;
        resolve(value);
      };
      byId('dialogok').onclick = () => close(input.value.trim());
      byId('dialogcancel').onclick = () => close(null);
      input.onkeydown = (event) => {
        if (event.key === 'Enter') close(input.value.trim());
        if (event.key === 'Escape') close(null);
      };
    });
  }

  function askToConfirm(title, confirmLabel) {
    return new Promise((resolve) => {
      const dialog = byId('dialog');
      byId('dialogtitle').textContent = title;
      byId('dialoginput').style.display = 'none';
      byId('dialogok').textContent = confirmLabel;
      dialog.classList.add('show');
      const close = (answer) => {
        dialog.classList.remove('show');
        byId('dialogok').onclick = null;
        byId('dialogcancel').onclick = null;
        resolve(answer);
      };
      byId('dialogok').onclick = () => close(true);
      byId('dialogcancel').onclick = () => close(false);
    });
  }

  async function createFile() {
    const name = await askForText('New SwiftUI file', 'NewView.swift', 'Create');
    if (!name) return;
    try {
      const result = await postJSON('/api/file/create', { path: name });
      await refreshFileList();
      openFile(result.path);
    } catch (error) {
      showToast(error.message, true);
    }
  }

  async function renameFile(path) {
    const newName = await askForText('Rename file', path, 'Rename');
    if (!newName || newName === path) return;
    await saveCurrentFile();
    try {
      const result = await postJSON('/api/file/rename', { from: path, to: newName });
      const entry = openFiles.get(path);
      openFiles.delete(path);
      if (entry) openFiles.set(result.path, entry);
      if (currentPath === path) currentPath = result.path;
      await refreshFileList();
      openFile(currentPath);
    } catch (error) {
      showToast(error.message, true);
    }
  }

  async function deleteFile(path) {
    const confirmed = await askToConfirm(`Delete ${path}? This can't be undone.`, 'Delete');
    if (!confirmed) return;
    try {
      await postJSON('/api/file/delete', { path });
      openFiles.delete(path);
      await refreshFileList();
      if (currentPath === path) {
        currentPath = '';
        openFile(pickStartingFile(''));
      }
    } catch (error) {
      showToast(error.message, true);
    }
  }

  async function refreshFileList() {
    await new Promise((resolve) => setTimeout(resolve, 150));
    renderFileTree();
  }

  function pickStartingFile(focusedFile) {
    const paths = knownFiles.map((file) => file.path);
    const remembered = localStorage.getItem('swiftuitutor.file.' + projectDir);
    if (remembered && paths.includes(remembered)) return remembered;
    if (focusedFile && paths.includes(focusedFile)) return focusedFile;
    return paths.find((path) => /ContentView\.swift$/.test(path)) || paths[0] || '';
  }

  // ---------- project ----------

  async function showProjectDialog() {
    const info = await (await fetch('/api/project')).json();
    const dialog = byId('projectdialog');
    const input = byId('projectinput');
    input.value = info.dir;
    const recentList = byId('recentprojects');
    recentList.innerHTML = '';
    for (const dir of info.recent || []) {
      const row = createElement('div', 'recent-project', dir);
      row.onclick = () => { input.value = dir; };
      row.ondblclick = () => openProject(dir);
      recentList.appendChild(row);
    }
    dialog.classList.add('show');
    input.focus();
    input.select();
  }

  async function openProject(dir) {
    try {
      await saveCurrentFile();
      await postJSON('/api/project', { dir });
      byId('projectdialog').classList.remove('show');
    } catch (error) {
      showToast(error.message, true);
    }
  }

  // ---------- layout ----------

  function makePaneResizable(splitterId, paneId, storageKey, minWidth, fromRight) {
    const splitter = byId(splitterId);
    const pane = byId(paneId);
    const savedWidth = localStorage.getItem(storageKey);
    if (savedWidth) pane.style.width = savedWidth + 'px';
    splitter.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      splitter.setPointerCapture(event.pointerId);
      const startX = event.clientX;
      const startWidth = pane.offsetWidth;
      const onMove = (moveEvent) => {
        const delta = fromRight ? startX - moveEvent.clientX : moveEvent.clientX - startX;
        const width = Math.max(minWidth, startWidth + delta);
        pane.style.width = width + 'px';
        if (window.fitPreviewToStage) window.fitPreviewToStage();
      };
      const onUp = () => {
        splitter.removeEventListener('pointermove', onMove);
        splitter.removeEventListener('pointerup', onUp);
        localStorage.setItem(storageKey, pane.offsetWidth);
        editor.refresh();
      };
      splitter.addEventListener('pointermove', onMove);
      splitter.addEventListener('pointerup', onUp);
    });
  }

  // ---------- wiring ----------

  async function onPayload(payload) {
    if (payload.dir !== projectDir) {
      projectDir = payload.dir;
      openFiles.clear();
      currentPath = '';
      byId('projectname').textContent = projectDir.split(/[\\/]/).pop() || projectDir;
      byId('projectname').title = projectDir;
      knownFiles = payload.files || [];
      await openFile(pickStartingFile(payload.focus));
    }
    const filesChanged = JSON.stringify(payload.files || []) !== JSON.stringify(knownFiles);
    knownFiles = payload.files || [];
    if (filesChanged) {
      await reloadFilesChangedOnDisk();
      if (currentPath && !knownFiles.some((file) => file.path === currentPath)) {
        currentPath = '';
        await openFile(pickStartingFile(payload.focus));
      }
    }
    showErrorMarks(payload.errors);
    renderFileTree();
    projectAssets = { images: payload.images || [], colors: Object.keys(payload.assetColors || {}) };
    window.dispatchEvent(new CustomEvent('tutor-assets', { detail: { images: projectAssets.images, colors: payload.assetColors || {} } }));
  }

  function init() {
    createEditor();
    wireHoverHelp();
    loadDocs();
    makePaneResizable('split-explorer', 'explorer', 'swiftuitutor.explorerWidth', 140, false);
    makePaneResizable('split-preview', 'stage', 'swiftuitutor.previewWidth', 320, true);

    byId('newfile').addEventListener('click', createFile);
    byId('filetree').addEventListener('click', (event) => {
      const row = event.target.closest('.tree-file');
      if (!row) return;
      const action = event.target.closest('[data-action]');
      if (action && action.dataset.action === 'rename') renameFile(row.dataset.path);
      else if (action && action.dataset.action === 'delete') deleteFile(row.dataset.path);
      else openFile(row.dataset.path);
    });
    byId('projectbtn').addEventListener('click', showProjectDialog);
    byId('projectopen').addEventListener('click', () => openProject(byId('projectinput').value));
    byId('projectcancel').addEventListener('click', () => byId('projectdialog').classList.remove('show'));
    byId('projectinput').addEventListener('keydown', (event) => {
      if (event.key === 'Enter') openProject(byId('projectinput').value);
      if (event.key === 'Escape') byId('projectdialog').classList.remove('show');
    });
    byId('vscodebtn').addEventListener('click', async () => {
      try {
        await saveCurrentFile();
        await postJSON('/api/open-in-vscode', { path: currentPath, line: editor.getCursor().line + 1 });
        showToast('Opening in VS Code… edits there will show up here automatically.');
      } catch (error) {
        showToast(error.message, true);
      }
    });
    byId('explorerbtn').addEventListener('click', () => postJSON('/api/show-in-explorer'));
    byId('errorbox').addEventListener('click', () => jumpToError(lastErrors[0]));
    window.addEventListener('tutor-payload', (event) => onPayload(event.detail));
    window.addEventListener('beforeunload', () => saveCurrentFile());
  }

  // The asset and icon panels live in library.js and use these to talk to the editor.
  window.TutorEditor = {
    postJSON,
    showToast,
    askToConfirm,
    insertAtCursor: (text) => {
      editor.replaceSelection(text);
      editor.focus();
    },
    cursorIsInString: () => {
      const cursor = editor.getCursor();
      return insideStringAt(editor.getLine(cursor.line), cursor.ch);
    },
  };

  if (document.body.dataset.view === 'editor') init();
})();
