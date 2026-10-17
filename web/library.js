(() => {
  'use strict';

  const byId = (id) => document.getElementById(id);
  const editorBridge = () => window.SwitaculousEditor;

  let images = [];
  let colors = {};
  let symbolsRendered = false;

  function showPanel(panelId) {
    for (const tab of document.querySelectorAll('.sidebar-tab')) tab.classList.toggle('active', tab.dataset.panel === panelId);
    for (const panel of document.querySelectorAll('.sidebar-panel')) panel.classList.toggle('hidden', panel.id !== panelId);
    try { localStorage.setItem('switaculous.sidebarPanel', panelId); } catch (error) { /* private mode */ }
    if (panelId === 'symbolspanel' && !symbolsRendered) renderSymbols('');
  }

  // ---------- assets ----------

  function renderAssets() {
    const list = byId('assetlist');
    list.innerHTML = '';
    if (!images.length && !Object.keys(colors).length) {
      const empty = document.createElement('div');
      empty.className = 'tree-empty';
      empty.textContent = 'No assets yet. Add images or colours and they are saved to Assets.xcassets, ready for Xcode.';
      list.appendChild(empty);
      return;
    }
    if (images.length) list.appendChild(sectionTitle('Images'));
    for (const image of images) {
      const row = assetRow(image.name, `Image("${image.name}")`, image.inCatalog ? 'image' : '');
      const thumb = document.createElement('img');
      thumb.className = 'asset-thumb';
      thumb.src = '/asset/' + encodeURIComponent(image.name);
      thumb.alt = '';
      row.prepend(thumb);
      list.appendChild(row);
    }
    const colorNames = Object.keys(colors).sort();
    if (colorNames.length) list.appendChild(sectionTitle('Colours'));
    for (const name of colorNames) {
      const row = assetRow(name, `Color("${name}")`, 'color');
      const swatch = document.createElement('span');
      swatch.className = 'asset-swatch';
      swatch.style.background = `linear-gradient(135deg, ${colors[name].light} 50%, ${colors[name].dark} 50%)`;
      swatch.title = `Light ${colors[name].light}\nDark ${colors[name].dark}`;
      row.prepend(swatch);
      list.appendChild(row);
    }
  }

  function sectionTitle(text) {
    const title = document.createElement('div');
    title.className = 'tree-folder';
    title.textContent = text;
    return title;
  }

  function assetRow(name, snippet, deletableKind) {
    const row = document.createElement('div');
    row.className = 'tree-file asset-row';
    row.title = 'Click to insert ' + snippet;
    const label = document.createElement('span');
    label.className = 'file-name';
    label.textContent = name;
    row.appendChild(label);
    const actions = document.createElement('span');
    actions.className = 'file-actions';
    const copyButton = document.createElement('button');
    copyButton.className = 'icon-button';
    copyButton.title = 'Copy the name';
    copyButton.textContent = '⧉';
    copyButton.addEventListener('click', (event) => {
      event.stopPropagation();
      navigator.clipboard.writeText(name).then(() => editorBridge().showToast('Copied "' + name + '"'));
    });
    actions.appendChild(copyButton);
    if (deletableKind) {
      const deleteButton = document.createElement('button');
      deleteButton.className = 'icon-button';
      deleteButton.title = 'Delete from Assets.xcassets';
      deleteButton.textContent = '🗑';
      deleteButton.addEventListener('click', async (event) => {
        event.stopPropagation();
        if (!(await editorBridge().askToConfirm(`Delete "${name}" from Assets.xcassets?`, 'Delete'))) return;
        try {
          await editorBridge().postJSON('/api/asset/delete', { name, kind: deletableKind });
        } catch (error) {
          editorBridge().showToast(error.message, true);
        }
      });
      actions.appendChild(deleteButton);
    }
    row.appendChild(actions);
    row.addEventListener('click', () => editorBridge().insertAtCursor(editorBridge().cursorIsInString() ? name : snippet));
    return row;
  }

  function readFileAsDataURL(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    });
  }

  function addImages() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/png,image/jpeg,image/gif,image/webp,image/svg+xml';
    input.multiple = true;
    input.addEventListener('change', async () => {
      for (const file of input.files) {
        const name = file.name.replace(/\.[^.]+$/, '').replace(/@[23]x$/, '');
        try {
          await editorBridge().postJSON('/api/asset/image', { name, data: await readFileAsDataURL(file) });
          editorBridge().showToast(`Added "${name}" — use Image("${name}")`);
        } catch (error) {
          editorBridge().showToast(error.message, true);
        }
      }
    });
    input.click();
  }

  function addColor() {
    const dialog = byId('colordialog');
    const nameInput = byId('colorname');
    const hasDark = byId('colorhasdark');
    nameInput.value = '';
    dialog.classList.add('show');
    nameInput.focus();
    const close = () => { dialog.classList.remove('show'); byId('colorok').onclick = null; byId('colorcancel').onclick = null; };
    byId('colorcancel').onclick = close;
    byId('colorok').onclick = async () => {
      try {
        await editorBridge().postJSON('/api/asset/color', {
          name: nameInput.value.trim(),
          light: byId('colorlight').value,
          dark: hasDark.checked ? byId('colordark').value : '',
        });
        editorBridge().showToast(`Added "${nameInput.value.trim()}" — use Color("${nameInput.value.trim()}")`);
        close();
      } catch (error) {
        editorBridge().showToast(error.message, true);
      }
    };
    nameInput.onkeydown = (event) => { if (event.key === 'Enter') byId('colorok').onclick(); if (event.key === 'Escape') close(); };
  }

  // ---------- system icons ----------

  function renderSymbols(query) {
    symbolsRendered = true;
    const grid = byId('symbolgrid');
    grid.innerHTML = '';
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    const names = (window.SFSymbolNames || []).filter((name) => words.every((word) => name.includes(word)));
    for (const name of names.slice(0, 400)) {
      const cell = document.createElement('button');
      cell.className = 'symbol-cell';
      cell.title = name + '\nClick to insert Image(systemName: "' + name + '")';
      cell.innerHTML = window.SFSymbol(name).svg;
      const label = document.createElement('span');
      label.textContent = name;
      cell.appendChild(label);
      cell.addEventListener('click', () => editorBridge().insertAtCursor(editorBridge().cursorIsInString() ? name : `Image(systemName: "${name}")`));
      grid.appendChild(cell);
    }
    if (!names.length) {
      const empty = document.createElement('div');
      empty.className = 'tree-empty';
      empty.textContent = 'No icons match. Any SF Symbol name still works in Xcode; unknown ones show a placeholder here.';
      grid.appendChild(empty);
    } else if (names.length > 400) {
      const more = document.createElement('div');
      more.className = 'tree-empty';
      more.textContent = `${names.length - 400} more — type to narrow the list`;
      grid.appendChild(more);
    }
  }

  function init() {
    for (const tab of document.querySelectorAll('.sidebar-tab')) tab.addEventListener('click', () => showPanel(tab.dataset.panel));
    byId('addimage').addEventListener('click', addImages);
    byId('addcolor').addEventListener('click', addColor);
    byId('colorhasdark').addEventListener('change', () => { byId('colordark').disabled = !byId('colorhasdark').checked; });
    byId('symbolsearch').addEventListener('input', () => renderSymbols(byId('symbolsearch').value));
    window.addEventListener('switaculous-assets', (event) => {
      images = event.detail.images || [];
      colors = event.detail.colors || {};
      renderAssets();
    });
    let remembered = 'filespanel';
    try { remembered = localStorage.getItem('switaculous.sidebarPanel') || 'filespanel'; } catch (error) { /* private mode */ }
    showPanel(remembered);
  }

  if (document.body.dataset.view === 'editor') init();
})();
