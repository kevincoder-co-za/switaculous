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
    if (!response.ok) {
      const error = new Error(data.error || 'Request failed');
      error.status = response.status;
      throw error;
    }
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
      phrases: { '(Use /re/ syntax for regexp search)': FIND_HINT },
      gutters: ['CodeMirror-linenumbers', 'error-gutter'],
      extraKeys: {
        'Ctrl-F': 'findPersistent',
        'Cmd-F': 'findPersistent',
        F3: 'findNext',
        'Shift-F3': 'findPrev',
        F12: () => goToDefinition(wordAt(editor.getCursor()), { quiet: false }),
        'Ctrl-S': () => { formatCurrentFile(); saveCurrentFile(); },
        'Cmd-S': () => { formatCurrentFile(); saveCurrentFile(); },
        'Shift-Ctrl-S': () => menuCommands['save-as'](),
        'Shift-Cmd-S': () => menuCommands['save-as'](),
        'Shift-Alt-F': () => formatCurrentFile(),
        'Ctrl-/': 'toggleComment',
        'Cmd-/': 'toggleComment',
        'Ctrl-Space': () => showCompletions(true),
        'Ctrl-Shift-Space': () => { const t = editor.getTokenAt(editor.getCursor()); const d = docsFor(t.string); if (d.length) showDocTip(d.map((x) => x.sig).join('\n'), d[0].doc, null, d[0].example); },
        Tab: (cm) => {
          if (cm.somethingSelected()) cm.indentSelection('add');
          else cm.replaceSelection('    ', 'end');
        },
        'Shift-Tab': (cm) => cm.indentSelection('subtract'),
      },
    });
    editor.on('change', onEditorChange);
    const openDialog = editor.openDialog;
    editor.openDialog = function (template, callback, options) {
      const close = openDialog.call(this, template, callback, options);
      showFindMatchCount();
      return close;
    };
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

  // Re-indents the open file with 4 spaces per brace level, the way Xcode's
  // Re-Indent does, as one undo step. Only the whitespace at the ends of each
  // line changes, so the caret stays where it was.
  function formatCurrentFile() {
    if (!openFiles.has(currentPath) || !currentPath.endsWith('.swift')) return;
    const cursorLine = editor.getCursor().line;
    const oldLines = editor.getValue().split('\n');
    const newLines = window.SwiftFormat.reindent(oldLines, cursorLine);
    editor.operation(() => {
      newLines.forEach((line, index) => {
        const old = oldLines[index];
        if (line === old) return;
        const oldText = old.trim();
        const oldStart = old.length - old.trimStart().length;
        const newStart = line.length - line.trimStart().length;
        if (!oldText) {
          editor.replaceRange('', { line: index, ch: 0 }, { line: index, ch: old.length }, '+format');
          return;
        }
        const oldEnd = oldStart + oldText.length;
        if (line.length - newStart === oldText.length && old.length !== oldEnd) {
          editor.replaceRange('', { line: index, ch: oldEnd }, { line: index, ch: old.length }, '+format');
        }
        if (old.slice(0, oldStart) !== line.slice(0, newStart)) {
          editor.replaceRange(line.slice(0, newStart), { line: index, ch: 0 }, { line: index, ch: oldStart }, '+format');
        }
      });
    });
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
    localStorage.setItem('switaculous.file.' + projectDir, path);
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
    jumpToLocation(error);
  }

  function jumpToLocation(location) {
    const goToLine = () => {
      editor.setCursor({ line: location.line - 1, ch: Math.max(0, location.col - 1) });
      editor.scrollIntoView(null, 120);
      editor.focus();
    };
    if (location.path === currentPath) goToLine();
    else openFile(location.path).then(goToLine);
  }

  // ---------- go to definition ----------

  // The identifier around the cursor, found by scanning the line in both directions so
  // the cursor can sit at the start, the middle or the end of the word.
  function wordAt(position) {
    const lineText = editor.getLine(position.line) || '';
    let start = position.ch;
    let end = position.ch;
    while (start > 0 && /\w/.test(lineText[start - 1])) start--;
    while (end < lineText.length && /\w/.test(lineText[end])) end++;
    const word = lineText.slice(start, end);
    return /^[A-Za-z_]\w*$/.test(word) ? word : '';
  }

  async function goToDefinition(name, options) {
    if (!name) return;
    let sites = [];
    try {
      const response = await fetch('/api/definition?name=' + encodeURIComponent(name));
      sites = (await response.json()).sites || [];
    } catch (error) {
      showToast('Could not look up ' + name, true);
      return;
    }
    // The declaration the cursor is already on is skipped, so jumping again from
    // there moves on to the next place the name is declared (an extension, say).
    const cursorLine = editor.getCursor().line + 1;
    const elsewhere = sites.filter((site) => !(site.path === currentPath && site.line === cursorLine));
    if (elsewhere.length) {
      jumpToLocation(elsewhere[0]);
      return;
    }
    if (options && options.quiet) return;
    showToast(sites.length ? `${name} is declared right here` : `No declaration of ${name} in this project`);
  }

  function isWithinSelection(position) {
    return CodeMirror.cmpPos(position, editor.getCursor('from')) >= 0 && CodeMirror.cmpPos(position, editor.getCursor('to')) <= 0;
  }

  function wireEditorContextMenu() {
    const menu = byId('editormenu');
    const wrapper = editor.getWrapperElement();
    let wordUnderPointer = '';
    const closeMenu = () => menu.classList.remove('show');
    const setItemState = (command, label, enabled) => {
      const item = menu.querySelector(`[data-command="${command}"]`);
      if (label) item.querySelector('.menu-label').textContent = label;
      item.classList.toggle('disabled', !enabled);
    };

    wrapper.addEventListener('dblclick', (event) => {
      if (!event.target.closest('.CodeMirror-lines')) return;
      const position = editor.coordsChar({ left: event.clientX, top: event.clientY }, 'window');
      goToDefinition(wordAt(position), { quiet: true });
    });

    wrapper.addEventListener('contextmenu', (event) => {
      if (event.target.closest('.CodeMirror-dialog')) return;
      event.preventDefault();
      const position = editor.coordsChar({ left: event.clientX, top: event.clientY }, 'window');
      if (!(editor.somethingSelected() && isWithinSelection(position))) editor.setCursor(position);
      wordUnderPointer = wordAt(position);
      setItemState('definition', wordUnderPointer ? `Go to Definition of ${wordUnderPointer}` : 'Go to Definition', Boolean(wordUnderPointer));
      setItemState('cut', null, editor.somethingSelected());
      setItemState('copy', null, editor.somethingSelected());
      menu.classList.add('show');
      menu.style.left = Math.min(event.clientX, window.innerWidth - menu.offsetWidth - 8) + 'px';
      menu.style.top = Math.min(event.clientY, window.innerHeight - menu.offsetHeight - 8) + 'px';
    });

    const commands = {
      definition: () => goToDefinition(wordUnderPointer, { quiet: false }),
      find: () => editor.execCommand('findPersistent'),
      cut: async () => {
        await navigator.clipboard.writeText(editor.getSelection());
        editor.replaceSelection('');
      },
      copy: () => navigator.clipboard.writeText(editor.getSelection()),
      paste: async () => editor.replaceSelection(await navigator.clipboard.readText()),
      format: () => formatCurrentFile(),
    };
    menu.addEventListener('click', async (event) => {
      const item = event.target.closest('[data-command]');
      if (!item) return;
      closeMenu();
      editor.focus();
      try {
        await commands[item.dataset.command]();
      } catch (error) {
        showToast('The browser did not allow clipboard access', true);
      }
    });
    document.addEventListener('pointerdown', (event) => {
      if (!event.target.closest('#editormenu')) closeMenu();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') closeMenu();
    });
  }

  // ---------- find ----------

  const FIND_HINT = 'Enter: next · Shift+Enter: previous · Esc: close';

  // Shows "3 of 12" in the find bar, kept current as the query is typed and as Enter moves between matches.
  function showFindMatchCount() {
    const wrapper = editor.getWrapperElement();
    const field = wrapper.querySelector('.CodeMirror-dialog .CodeMirror-search-field');
    const hint = wrapper.querySelector('.CodeMirror-dialog .CodeMirror-search-hint');
    if (!field || !hint) return;
    const update = () => { hint.textContent = describeFindMatches(field.value); };
    field.addEventListener('input', update);
    field.addEventListener('keyup', update);
    update();
  }

  function describeFindMatches(queryText) {
    if (!queryText) return FIND_HINT;
    let query = queryText;
    const regexSyntax = queryText.match(/^\/(.*)\/([a-z]*)$/);
    if (regexSyntax) {
      try { query = new RegExp(regexSyntax[1], regexSyntax[2].includes('i') ? 'i' : ''); } catch (error) { query = queryText; }
    }
    const caseFold = typeof query === 'string' && query === query.toLowerCase();
    const selectionStart = editor.getCursor('from');
    const cursor = editor.getSearchCursor(query, { line: 0, ch: 0 }, { caseFold, multiline: true });
    let total = 0;
    let current = 0;
    while (cursor.findNext()) {
      total++;
      if (CodeMirror.cmpPos(cursor.from(), selectionStart) === 0) current = total;
    }
    if (!total) return 'No matches · ' + FIND_HINT;
    return `${current || '–'} of ${total} · ${FIND_HINT}`;
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
          const result = {
            list: items.map((item) => completionEntry(item)),
            from: CodeMirror.Pos(cursor.line, start),
            to: cursor,
          };
          CodeMirror.on(result, 'select', (completion, node) => showCompletionInfo(completion.item, node));
          CodeMirror.on(result, 'close', () => { if (byId('doctip').dataset.mode === 'completion') hideDocTip(); });
          resolve(result);
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
    if (item.kind === 'method' || item.kind === 'modifier' || item.kind === 'view') showDocTip(item.detail, item.doc, null, item.example);
  }

  // While the completion list is open, the highlighted entry is explained in a
  // panel beside it: what it does, an example, and what its parameter types are.
  function showCompletionInfo(item, node) {
    if (!item || !node) return;
    const signature = item.kind === 'field' && item.detail ? item.text + ': ' + item.detail : item.detail;
    if (!fillDocTip(signature, item.doc, item.example)) { hideDocTip(); return; }
    const tip = byId('doctip');
    tip.dataset.mode = 'completion';
    clearTimeout(docTipTimer);
    const list = node.closest('.CodeMirror-hints').getBoundingClientRect();
    const row = node.getBoundingClientRect();
    tip.classList.add('show');
    const width = tip.offsetWidth;
    const left = list.right + 6 + width <= window.innerWidth ? list.right + 6 : Math.max(4, list.left - 6 - width);
    tip.style.left = left + 'px';
    tip.style.top = Math.max(4, Math.min(row.top, window.innerHeight - tip.offsetHeight - 4)) + 'px';
  }

  // ---------- hover help ----------

  let apiDocs = { modifiers: {}, views: {}, types: {} };
  let docTipTimer = null;

  async function loadDocs() {
    try {
      apiDocs = await (await fetch('/api/docs')).json();
    } catch (error) {
      apiDocs = { modifiers: {}, views: {}, types: {} };
    }
  }

  function docsFor(word) {
    return (apiDocs.views[word] || []).concat(apiDocs.modifiers[word] || []);
  }

  // The parameter types in a signature that have an explanation, so
  // "accentColor(_ accentColor: Color?)" also says what a Color can be.
  function typeNotesFor(signature) {
    const types = apiDocs.types || {};
    const open = signature.indexOf('(');
    const params = open >= 0 ? signature.slice(open) : signature;
    const names = params.match(/[A-Z]\w*(?:\.[A-Z]\w*)*/g) || [];
    return [...new Set(names)].filter((name) => types[name]).slice(0, 4).map((name) => ({ name, doc: types[name] }));
  }

  function hideDocTip() {
    byId('doctip').classList.remove('show');
  }

  function fillDocTip(signature, summary, example) {
    const tip = byId('doctip');
    if (!signature && !summary && !example) return false;
    tip.innerHTML = '';
    if (signature) tip.appendChild(createElement('div', 'doctip-sig', signature));
    if (summary) tip.appendChild(createElement('div', 'doctip-doc', summary));
    if (example) {
      tip.appendChild(createElement('div', 'doctip-label', 'Example'));
      tip.appendChild(createElement('div', 'doctip-example', example));
    }
    for (const note of typeNotesFor(signature || '')) {
      const row = createElement('div', 'doctip-type');
      row.append(createElement('span', 'doctip-type-name', note.name), createElement('span', '', note.doc));
      tip.appendChild(row);
    }
    return true;
  }

  function showDocTip(signature, summary, anchor, example) {
    const tip = byId('doctip');
    if (!fillDocTip(signature, summary, example)) { hideDocTip(); return; }
    tip.dataset.mode = 'tip';
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
      showDocTip(docs.map((d) => d.sig).join('\n'), docs[0].doc, coords, docs[0].example);
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
    const remembered = localStorage.getItem('switaculous.file.' + projectDir);
    if (remembered && paths.includes(remembered)) return remembered;
    if (focusedFile && paths.includes(focusedFile)) return focusedFile;
    return paths.find((path) => /ContentView\.swift$/.test(path)) || paths[0] || '';
  }

  // ---------- project ----------

  async function showProjectPathDialog() {
    const info = await (await fetch('/api/project')).json();
    const input = byId('projectinput');
    input.value = info.dir;
    byId('projectdialog').classList.add('show');
    input.focus();
    input.select();
  }

  async function openProject(dir) {
    try {
      await saveCurrentFile();
      await postJSON('/api/project', { dir });
      byId('projectdialog').classList.remove('show');
      byId('welcomedialog').classList.remove('show');
    } catch (error) {
      showToast(error.message, true);
    }
  }

  // Open Folder and New Project show the operating system's own dialog. When
  // there isn't one (a Linux desktop without zenity or kdialog) the user types
  // the path instead.
  async function runProjectRequest(url, body) {
    try {
      await saveCurrentFile();
      await postJSON(url, body);
    } catch (error) {
      if (error.status === 501) showProjectPathDialog();
      else showToast(error.message, true);
    }
  }

  // ---------- welcome ----------

  // Shown when Switaculous starts: the projects used before, or a new project
  // or a demo. New projects and demo copies both ask where to save them. It can
  // only be closed once a project is open.
  async function showWelcome() {
    const [project, demos] = await Promise.all([
      fetch('/api/project').then((response) => response.json()),
      fetch('/api/demos').then((response) => response.json()),
    ]);
    const recent = project.recent || [];
    byId('welcomehint').textContent = recent.length
      ? 'Open one of your projects, or start a new one.'
      : 'Start a new project, or learn from one of the demos.';
    byId('welcomerecent').classList.toggle('hidden', !recent.length);
    fillWelcomeList('welcomerecentlist', recent.map((dir) => ({
      label: dir.split(/[\\/]/).pop() || dir,
      detail: dir,
      choose: () => chooseFromWelcome('/api/project', { dir }),
    })));
    fillWelcomeList('welcomedemolist', demos.map((name) => ({
      label: name,
      choose: () => chooseFromWelcome('/api/demos/open', { name }, `Choose where to save your copy of ${name} in the window that opens…`),
    })));
    byId('welcomedemos').classList.add('hidden');
    byId('welcomeclose').classList.toggle('hidden', !project.dir);
    byId('welcomedialog').classList.add('show');
  }

  function fillWelcomeList(id, items) {
    const list = byId(id);
    list.replaceChildren();
    for (const item of items) {
      const button = createElement('button', '');
      button.appendChild(createElement('span', '', item.label));
      if (item.detail) button.appendChild(createElement('span', 'welcome-path', item.detail));
      button.title = item.detail || item.label;
      button.addEventListener('click', item.choose);
      list.appendChild(button);
    }
  }

  // Leaves the welcome screen up when the folder dialog is cancelled.
  async function chooseFromWelcome(url, body, message) {
    if (message) showToast(message);
    try {
      await saveCurrentFile();
      const result = await postJSON(url, body);
      if (!result.cancelled) byId('welcomedialog').classList.remove('show');
    } catch (error) {
      if (error.status === 501) showProjectPathDialog();
      else showToast(error.message, true);
    }
  }

  function closeWelcome() {
    if (!projectDir) return;
    byId('welcomedialog').classList.remove('show');
    fetch('/api/project/welcome-done', { method: 'POST' });
  }

  function wireWelcome() {
    const actions = {
      'new-project': () => chooseFromWelcome('/api/project/new', null, 'Name the project and choose where to save it in the window that opens…'),
      'open-folder': () => chooseFromWelcome('/api/project/open-folder', null, 'Choose a folder in the window that opens…'),
      demos: () => byId('welcomedemos').classList.toggle('hidden'),
    };
    byId('welcomedialog').addEventListener('click', (event) => {
      const action = event.target.closest('[data-welcome]');
      if (action) actions[action.dataset.welcome]();
      else if (event.target === byId('welcomedialog')) closeWelcome();
    });
    byId('welcomeclose').addEventListener('click', closeWelcome);
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape' && byId('welcomedialog').classList.contains('show')) closeWelcome();
    });
    fetch('/api/project').then((response) => response.json()).then((project) => {
      if (project.welcome || !project.dir) showWelcome();
    });
  }

  // ---------- menu ----------

  const menuCommands = {
    'open-folder': () => {
      showToast('Choose a folder in the window that opens…');
      runProjectRequest('/api/project/open-folder');
    },
    'new-project': () => {
      showToast('Name the project and choose where to save it in the window that opens…');
      runProjectRequest('/api/project/new');
    },
    'save-as': async () => {
      showToast('Name the copy and choose where to save it in the window that opens…');
      try {
        await saveCurrentFile();
        const result = await postJSON('/api/project/save-as');
        if (!result.cancelled) showToast('Saved a copy to ' + result.dir + ' and opened it');
      } catch (error) {
        showToast(error.message, true);
      }
    },
    welcome: () => showWelcome(),
    'connect-mcp': () => window.showConnectDialog('mcp'),
    'connect-phone': () => window.showConnectDialog('phone'),
    quit: async () => {
      await saveCurrentFile();
      await fetch('/api/quit', { method: 'POST' });
      document.body.classList.add('quit');
    },
  };

  function closeMainMenu() {
    byId('mainmenu').classList.remove('show');
    byId('menubtn').classList.remove('active');
  }

  async function toggleMainMenu() {
    const isOpen = byId('mainmenu').classList.toggle('show');
    byId('menubtn').classList.toggle('active', isOpen);
    if (!isOpen) return;
    const [demos, project] = await Promise.all([
      fetch('/api/demos').then((response) => response.json()),
      fetch('/api/project').then((response) => response.json()),
    ]);
    fillSubmenu('demosmenu', demos.map((name) => ({
      label: name,
      open: () => {
        showToast(`Choose where to save your copy of ${name} in the window that opens…`);
        runProjectRequest('/api/demos/open', { name });
      },
    })), 'No demos');
    fillSubmenu('recentmenu', (project.recent || []).map((dir) => ({
      label: dir.split(/[\\/]/).pop() || dir,
      detail: dir,
      current: dir === project.dir,
      open: () => runProjectRequest('/api/project', { dir }),
    })), 'No recent projects yet');
  }

  function fillSubmenu(id, items, emptyText) {
    const submenu = byId(id);
    submenu.replaceChildren();
    if (!items.length) {
      submenu.appendChild(createElement('div', 'menu-empty', emptyText));
      return;
    }
    for (const item of items) {
      const button = createElement('button', 'menu-item' + (item.current ? ' current' : ''), item.label);
      if (item.detail) {
        button.title = item.detail;
        button.appendChild(createElement('span', 'menu-detail', item.detail));
      }
      button.addEventListener('click', () => {
        closeMainMenu();
        item.open();
      });
      submenu.appendChild(button);
    }
  }

  function wireMainMenu() {
    byId('menubtn').addEventListener('click', toggleMainMenu);
    byId('mainmenu').addEventListener('click', (event) => {
      const command = event.target.closest('[data-command]');
      if (!command) return;
      closeMainMenu();
      menuCommands[command.dataset.command]();
    });
    document.addEventListener('pointerdown', (event) => {
      if (!event.target.closest('.menu-anchor')) closeMainMenu();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') closeMainMenu();
    });
  }

  // ---------- layout ----------

  function makePaneResizable(splitterId, paneId, storageKey, minWidth) {
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
        const delta = moveEvent.clientX - startX;
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
    window.dispatchEvent(new CustomEvent('switaculous-assets', { detail: { images: projectAssets.images, colors: payload.assetColors || {} } }));
  }

  function init() {
    createEditor();
    wireHoverHelp();
    wireEditorContextMenu();
    loadDocs();
    makePaneResizable('split-explorer', 'explorer', 'switaculous.explorerWidth', 140);

    byId('newfile').addEventListener('click', createFile);
    byId('filetree').addEventListener('click', (event) => {
      const row = event.target.closest('.tree-file');
      if (!row) return;
      const action = event.target.closest('[data-action]');
      if (action && action.dataset.action === 'rename') renameFile(row.dataset.path);
      else if (action && action.dataset.action === 'delete') deleteFile(row.dataset.path);
      else openFile(row.dataset.path);
    });
    wireMainMenu();
    wireWelcome();
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
    byId('errorbox').addEventListener('click', () => jumpToError(lastErrors[0]));
    window.addEventListener('switaculous-payload', (event) => onPayload(event.detail));
    window.addEventListener('beforeunload', () => saveCurrentFile());
  }

  // The asset and icon panels live in library.js and use these to talk to the editor.
  window.SwitaculousEditor = {
    jumpTo: jumpToError,
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
