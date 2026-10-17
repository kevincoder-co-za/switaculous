(() => {
  'use strict';
  const $ = (s, el = document) => el.querySelector(s);
  const mk = (tag, cls) => { const e = document.createElement(tag); if (cls) e.className = cls; return e; };
  const isNum = (x) => typeof x === 'number' && isFinite(x);

  // The page can run as the full editor, as a popped-out preview window, or as
  // the bare device on a phone or tablet that scanned the QR code.
  const viewMode = new URLSearchParams(location.search).get('view') || 'editor';
  document.body.dataset.view = viewMode;

  const st = {
    payload: null, first: true, device: null, deviceId: '', scale: 1, dark: false, menuDefs: {}, warnedSyms: new Set(),
    consoleOpen: localStorage.getItem('switaculous.console') !== '0',
    geoCache: {}, connected: false, lastSeq: 0,
  };

  // ---------- networking ----------
  const queue = [];
  let inflight = false;
  function send(id, v) {
    if (!id) return;
    const last = queue[queue.length - 1];
    if (last && last.id === id && typeof v === 'string' && typeof last.v === 'string') last.v = v;
    else queue.push({ id, v: v === undefined ? null : v });
    pump();
  }
  function pump() {
    if (inflight || !queue.length) return;
    const item = queue.shift();
    inflight = true;
    fetch('/api/event', { method: 'POST', body: JSON.stringify(item) }).catch(() => {}).finally(() => { inflight = false; pump(); });
  }
  function settings(obj) { return fetch('/api/settings', { method: 'POST', body: JSON.stringify(obj) }); }

  function connect() {
    const es = new EventSource('/events');
    es.onopen = () => { st.connected = true; $('#disconnected').style.display = 'none'; dot(); };
    es.onmessage = (e) => { try { onPayload(JSON.parse(e.data)); } catch (err) { console.error(err); } };
    es.onerror = () => { st.connected = false; $('#disconnected').style.display = 'block'; dot(); };
  }
  function dot() {
    const d = $('#bar .dot');
    d.className = 'dot' + (st.connected ? (st.payload && st.payload.errors && st.payload.errors.length ? ' err' : ' on') : '');
  }

  // ---------- chrome ----------
  function initChrome() {
    $('#devsel').addEventListener('change', (e) => settings({ device: e.target.value }));
    $('#rotatebtn').addEventListener('click', () => settings({ landscape: !(st.payload && st.payload.device.landscape) }));
    if ($('#sharecancel')) $('#sharecancel').addEventListener('click', () => $('#sharedialog').classList.remove('show'));
    if ($('#copymcpcommand')) $('#copymcpcommand').addEventListener('click', (e) => copyText('mcpcommand', e.target));
    if ($('#copymcpjson')) $('#copymcpjson').addEventListener('click', (e) => copyText('mcpjson', e.target));
    $('#darkbtn').addEventListener('click', () => settings({ dark: !st.dark }));
    $('#resetbtn').addEventListener('click', () => settings({ reset: true }));
    $('#erasebtn').addEventListener('click', () => {
      if (confirm('Erase all SwiftData data saved by this app? This deletes the .swiftdata folder in the project and resets @State.')) settings({ eraseData: true });
    });
    $('#consolebtn').addEventListener('click', () => { st.consoleOpen = !st.consoleOpen; localStorage.setItem('switaculous.console', st.consoleOpen ? '1' : '0'); $('#consolepane').classList.toggle('hidden', !st.consoleOpen); });
    $('#clearbtn').addEventListener('click', () => { st.clearedAt = (st.payload && st.payload.console || []).length; renderConsole(st.payload); });
    $('#consolepane').classList.toggle('hidden', !st.consoleOpen);
    window.addEventListener('resize', fit);
    window.fitPreviewToStage = fit;
    window.showConnectDialog = showConnectDialog;
    if (viewMode === 'editor') initDocking();
  }

  function renderChrome(p) {
    $('#devsel').value = p.deviceId;
    $('#rotatebtn').style.display = p.device.kind === 'ipad' ? '' : 'none';
    $('#rotatebtn').classList.toggle('active', !!p.device.landscape);
    $('#darkbtn').classList.toggle('active', !!p.dark);
    $('#erasebtn').style.display = p.usesSwiftData ? '' : 'none';
    renderPreviewTabs(p);
    const wn = (p.warnings || []).length;
    $('#consolebtn').textContent = 'Console' + (wn ? ` (${wn} ⚠︎)` : '');
    dot();
  }

  // The preview follows the file open in the editor. When that file has more
  // than one #Preview, they show as tabs above the device, as in Xcode.
  function renderPreviewTabs(p) {
    const previews = p.previews || [];
    const current = previews.find((preview) => preview.id === p.current);
    const sameFile = current ? previews.filter((preview) => preview.file === current.file) : [];
    $('#status').textContent = current && sameFile.length < 2 ? current.name : '';
    const tabs = $('#previewtabs');
    const signature = JSON.stringify(sameFile) + p.current;
    if (tabs.dataset.signature === signature) return;
    tabs.dataset.signature = signature;
    tabs.replaceChildren();
    if (sameFile.length < 2) return;
    for (const preview of sameFile) {
      const tab = mk('button', 'preview-tab' + (preview.id === p.current ? ' active' : ''));
      tab.textContent = preview.name;
      tab.addEventListener('click', () => settings({ preview: preview.id }));
      tabs.appendChild(tab);
    }
  }

  function renderConsole(p) {
    const c = $('#console');
    const lines = (p && p.console) || [];
    const start = st.clearedAt && st.clearedAt <= lines.length ? st.clearedAt : 0;
    const atBottom = c.scrollTop + c.clientHeight >= c.scrollHeight - 8;
    c.innerHTML = '';
    for (const l of lines.slice(start)) {
      const d = mk('div', 'ln ' + l.k);
      const t = mk('span', 't'); t.textContent = l.t;
      d.appendChild(t); d.appendChild(document.createTextNode(l.m));
      c.appendChild(d);
    }
    if (atBottom) c.scrollTop = c.scrollHeight;
  }

  function renderErrors(p) {
    const box = $('#errorbox');
    const errs = p.errors || [];
    $('#stage').classList.toggle('stale', errs.some((e) => e.kind === 'compile'));
    if (!errs.length) { box.classList.remove('show'); return; }
    const e = errs[0];
    box.innerHTML = '';
    const k = mk('div', 'kind'); k.textContent = e.kind === 'compile' ? 'Build failed' : 'Runtime error';
    const m = mk('div', 'msg'); m.textContent = e.msg;
    box.append(k, m);
    if (e.file) { const l = mk('div', 'loc'); l.textContent = `${e.file}:${e.line}:${e.col}` + (errs.length > 1 ? `  ·  ${errs.length - 1} more` : ''); box.appendChild(l); }
    if (e.snippet) { const pre = mk('pre'); pre.textContent = e.snippet; box.appendChild(pre); }
    box.classList.add('show');
  }

  // ---------- device ----------
  function statusIcons() {
    return `<svg width="18" height="12" viewBox="0 0 18 12"><rect x="0" y="8" width="3" height="4" rx="1" fill="currentColor"/><rect x="5" y="5.5" width="3" height="6.5" rx="1" fill="currentColor"/><rect x="10" y="3" width="3" height="9" rx="1" fill="currentColor"/><rect x="15" y="0" width="3" height="12" rx="1" fill="currentColor"/></svg>` +
      `<svg width="16" height="12" viewBox="0 0 16 12"><path d="M8 2.2c2.3 0 4.4.9 6 2.4l1.1-1.2A10.2 10.2 0 0 0 8 .5C5.3.5 2.8 1.5.9 3.4L2 4.6c1.6-1.5 3.7-2.4 6-2.4zm0 3.4c1.4 0 2.6.5 3.6 1.4l1.1-1.2A7 7 0 0 0 8 3.9 7 7 0 0 0 3.3 5.8L4.4 7c1-.9 2.2-1.4 3.6-1.4zM8 9l2.3-2.4A3.3 3.3 0 0 0 8 5.7c-.9 0-1.7.3-2.3.9z" fill="currentColor"/></svg>` +
      `<svg width="27" height="13" viewBox="0 0 27 13"><rect x=".5" y=".5" width="22" height="12" rx="3.8" fill="none" stroke="currentColor" opacity=".4"/><rect x="2" y="2" width="19" height="9" rx="2.5" fill="currentColor"/><path d="M24 4.5v4c.8-.3 1.3-1.1 1.3-2s-.5-1.7-1.3-2z" fill="currentColor" opacity=".45"/></svg>`;
  }

  function buildDevice(dev, id) {
    const wrap = $('#device-wrap');
    wrap.innerHTML = '';
    const isPad = dev.kind === 'ipad';
    const d = mk('div', 'device' + (isPad ? ' ipad' : dev.island ? '' : ' se'));
    const scr = mk('div', 'screen' + (isPad ? ' ipad' : ''));
    scr.id = 'screen';
    scr.style.width = dev.w + 'px';
    scr.style.height = dev.h + 'px';
    if (!dev.island && !isPad) scr.style.borderRadius = '0';
    else scr.style.borderRadius = dev.radius + 'px';
    d.style.borderRadius = isPad ? (dev.radius + 18) + 'px' : dev.island ? (dev.radius + 12) + 'px' : '52px';
    const sb = mk('div', 'statusbar');
    sb.style.height = (isPad ? dev.top : dev.island ? dev.top - 5 : 20) + 'px';
    sb.innerHTML = `<span>${isPad ? '9:41 AM&nbsp; Mon 9 Jun' : '9:41'}</span><span class="icons">${statusIcons()}</span>`;
    scr.appendChild(sb);
    if (dev.island) scr.appendChild(mk('div', 'island'));
    const content = mk('div');
    content.id = 'content';
    content.style.position = 'absolute';
    content.style.inset = '0';
    scr.appendChild(content);
    if (dev.bottom > 0) scr.appendChild(mk('div', 'homeind'));
    d.appendChild(scr);
    wrap.appendChild(d);
    st.device = dev;
    st.deviceId = id;
    bindScreen(scr);
    fit();
  }

  function fit() {
    const stage = $('#stage');
    const dev = st.device;
    if (!dev) return;
    const isPad = dev.kind === 'ipad';
    const pad = isPad ? 40 : dev.island ? 24 : 28;
    const w = dev.w + pad + 20, h = dev.h + (isPad || dev.island ? pad : 160) + 20;
    const stageInsets = document.body.dataset.view === 'device' ? 0 : 70;
    const s = Math.min(document.body.dataset.view === 'device' ? 10 : 1, (stage.clientWidth - 30) / w, (stage.clientHeight - stageInsets) / h);
    st.scale = s;
    $('#device-wrap').style.transform = `scale(${s})`;
  }

  // Colour sets from Assets.xcassets arrive with the payload and become CSS
  // variables on the page, so Color("Brand") paints like any system colour.
  function applyAssetColors(assetColors, dark) {
    let style = document.getElementById('asset-colors');
    if (!style) { style = mk('style'); style.id = 'asset-colors'; document.head.appendChild(style); }
    const rules = [];
    for (const [name, color] of Object.entries(assetColors)) {
      const cssName = '--c-asset-' + name.replace(/[^\p{L}\p{N}]/gu, '-');
      rules.push(`${cssName}: ${dark ? color.dark : color.light};`);
      if (name === 'AccentColor') rules.push(`--tint: ${dark ? color.dark : color.light};`);
    }
    style.textContent = rules.length ? `.screen { ${rules.join(' ')} }` : '';
  }

  // ---------- preview docking ----------
  // Like browser dev tools, the preview can sit on any side of the editor or in
  // its own window for a second screen. Closing that window docks it back where
  // it was.
  const dockSides = ['left', 'right', 'top', 'bottom'];
  let dockSide = 'right';
  let previewWindow = null;

  function isVerticalDock() {
    return dockSide === 'top' || dockSide === 'bottom';
  }

  function initDocking() {
    const savedSide = localStorage.getItem('switaculous.dock');
    if (dockSides.includes(savedSide)) dockSide = savedSide;
    for (const button of document.querySelectorAll('.dock-button')) {
      button.addEventListener('click', () => dockPreview(button.dataset.dock));
    }
    makePreviewResizable();
    window.addEventListener('pagehide', () => { if (previewWindow) previewWindow.close(); });
    applyDock();
  }

  function dockPreview(side) {
    if (side === 'popout') {
      if (previewWindow) closePreviewWindow();
      else openPreviewWindow();
      return;
    }
    dockSide = side;
    localStorage.setItem('switaculous.dock', side);
    if (previewWindow) closePreviewWindow();
    else applyDock();
  }

  function applyDock() {
    const activeDock = previewWindow ? 'popout' : dockSide;
    $('#main').className = 'dock-' + activeDock;
    const stage = $('#stage');
    stage.style.width = isVerticalDock() ? '' : (localStorage.getItem('switaculous.previewWidth') || 470) + 'px';
    stage.style.height = isVerticalDock() ? (localStorage.getItem('switaculous.previewHeight') || 420) + 'px' : '';
    for (const button of document.querySelectorAll('.dock-button')) {
      button.classList.toggle('active', button.dataset.dock === activeDock);
    }
    // The code editor and the device both re-measure themselves on resize.
    window.dispatchEvent(new Event('resize'));
  }

  function openPreviewWindow() {
    previewWindow = window.open(location.origin + '/?view=preview', 'switaculous-preview', 'width=560,height=1000');
    if (!previewWindow) {
      alertBlockedPopup();
      return;
    }
    applyDock();
    const watcher = setInterval(() => {
      if (previewWindow && !previewWindow.closed) return;
      clearInterval(watcher);
      if (previewWindow) {
        previewWindow = null;
        applyDock();
      }
    }, 400);
  }

  function closePreviewWindow() {
    const openWindow = previewWindow;
    previewWindow = null;
    if (openWindow && !openWindow.closed) openWindow.close();
    applyDock();
  }

  function alertBlockedPopup() {
    const toast = $('#toast');
    toast.textContent = 'The browser blocked the preview window. Allow pop-ups for this page, then try again.';
    toast.className = 'show error';
    setTimeout(() => { toast.className = ''; }, 4000);
  }

  function makePreviewResizable() {
    const splitter = $('#split-preview');
    const stage = $('#stage');
    splitter.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      splitter.setPointerCapture(event.pointerId);
      const vertical = isVerticalDock();
      const startPosition = vertical ? event.clientY : event.clientX;
      const startSize = vertical ? stage.offsetHeight : stage.offsetWidth;
      // Dragging towards the editor makes the preview bigger, whichever side it is on.
      const direction = dockSide === 'right' || dockSide === 'bottom' ? -1 : 1;
      const onMove = (moveEvent) => {
        const position = vertical ? moveEvent.clientY : moveEvent.clientX;
        const size = Math.max(vertical ? 200 : 320, startSize + direction * (position - startPosition));
        if (vertical) stage.style.height = size + 'px';
        else stage.style.width = size + 'px';
        fit();
      };
      const onUp = () => {
        splitter.removeEventListener('pointermove', onMove);
        splitter.removeEventListener('pointerup', onUp);
        if (vertical) localStorage.setItem('switaculous.previewHeight', stage.offsetHeight);
        else localStorage.setItem('switaculous.previewWidth', stage.offsetWidth);
        window.dispatchEvent(new Event('resize'));
      };
      splitter.addEventListener('pointermove', onMove);
      splitter.addEventListener('pointerup', onUp);
    });
  }

  async function showConnectDialog(section) {
    const dialog = $('#sharedialog');
    $('#connecttitle').textContent = section === 'mcp' ? 'Connect Claude Code (MCP)' : 'Open on iPhone or iPad';
    for (const part of dialog.querySelectorAll('[data-connect]')) part.hidden = part.dataset.connect !== section;
    dialog.classList.add('show');
    try {
      const info = await (await fetch('/api/share')).json();
      $('#mcpcommand').textContent = info.mcpCommand || '';
      $('#mcpjson').textContent = info.mcpJSON || '';
      $('#mcpurl').textContent = info.mcpURL || '';
      $('#mcpstdio').textContent = info.mcpStdioCommand || '';
      if (!info.url) {
        $('#shareurl').textContent = 'No Wi-Fi or Ethernet address was found on this computer, or sharing is off (-local-only).';
        $('#shareqr').style.display = 'none';
        return;
      }
      $('#shareurl').textContent = info.url;
      $('#shareqr').style.display = '';
      $('#shareqr').src = '/api/share/qr.png?' + Date.now();
    } catch (error) {
      $('#shareurl').textContent = 'Could not reach the server.';
      $('#shareqr').style.display = 'none';
    }
  }

  function copyText(elementId, button) {
    navigator.clipboard.writeText($('#' + elementId).textContent).then(() => {
      const label = button.textContent;
      button.textContent = 'Copied';
      setTimeout(() => { button.textContent = label; }, 1500);
    });
  }

  // ---------- payload ----------
  function onPayload(p) {
    st.payload = p;
    renderChrome(p);
    renderConsole(p);
    renderErrors(p);
    const deviceKey = p.deviceId + (p.device.landscape ? ':landscape' : '');
    if (!st.device || st.deviceId !== deviceKey) buildDevice(p.device, deviceKey);
    if (p.tree) renderAll(p, false);
    window.dispatchEvent(new CustomEvent('switaculous-payload', { detail: p }));
  }

  function renderAll(p, force) {
    const scr = $('#screen');
    const tree = p.tree;
    if (!tree) return;
    const dark = tree.scheme ? tree.scheme === 'dark' : !!p.dark;
    st.dark = !!p.dark;
    scr.classList.toggle('dark', dark);
    applyAssetColors(p.assetColors || {}, dark);
    scr.classList.add('ios26');
    const dev = st.device;
    scr.style.setProperty('--sat', dev.top + 'px');
    scr.style.setProperty('--sab', dev.bottom + 'px');
    scr.style.setProperty('--sat0', dev.top + 'px');
    scr.style.setProperty('--sab0', dev.bottom + 'px');
    st.menuDefs = {};
    st.hideTabBar = !!tree.hideTabBar;
    const fresh = mk('div');
    fresh.id = 'content';
    fresh.style.position = 'absolute';
    fresh.style.inset = '0';
    fresh.appendChild(buildRoot(tree.root));
    const ov = mk('div', 'overlays');
    ov.dataset.k = '__overlays';
    for (const o of tree.overlays || []) ov.appendChild(renderOverlay(o));
    fresh.appendChild(ov);
    const live = $('#content');
    if (p.anim) { scr.classList.add('animating'); clearTimeout(st.animT); st.animT = setTimeout(() => scr.classList.remove('animating'), 450); }
    if (st.first || force) {
      live.replaceWith(fresh);
      st.first = false;
    } else {
      st.animateEnter = true;
      morph(live, fresh);
      st.animateEnter = false;
    }
    if (st.openMenu && !document.contains(st.openMenu.anchor)) closeMenu();
    requestAnimationFrame(() => postLayout(p));
  }

  // ---------- markdown ----------
  function esc(s) { return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }
  function md(s) {
    let h = esc(s);
    h = h.replace(/`([^`]+)`/g, '<code>$1</code>');
    h = h.replace(/\*\*\*([^*]+)\*\*\*/g, '<b><i>$1</i></b>');
    h = h.replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>');
    h = h.replace(/__([^_]+)__/g, '<b>$1</b>');
    h = h.replace(/(^|[^*\w])\*([^*\s][^*]*)\*/g, '$1<i>$2</i>');
    h = h.replace(/(^|[^_\w])_([^_\s][^_]*)_/g, '$1<i>$2</i>');
    h = h.replace(/~~([^~]+)~~/g, '<s>$1</s>');
    h = h.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a data-url="$2">$1</a>');
    return h;
  }

  // ---------- paint ----------
  function gradientCSS(g) {
    const stops = (g.stops || []).slice().sort((a, b) => a[1] - b[1]);
    if (g.g === 'linear') {
      const sp = g.sp || [0.5, 0], ep = g.ep || [0.5, 1];
      const ang = Math.atan2(ep[0] - sp[0], -(ep[1] - sp[1])) * 180 / Math.PI;
      const len = Math.hypot(ep[0] - sp[0], ep[1] - sp[1]) || 1;
      const s = stops.map((x) => `${x[0]} ${(x[1] * 100 * Math.min(1, len)).toFixed(1)}%`).join(', ');
      return `linear-gradient(${ang.toFixed(1)}deg, ${s || 'transparent, transparent'})`;
    }
    if (g.g === 'radial') {
      const c = g.c || [0.5, 0.5];
      const sr = g.sr || 0, er = g.er || 100;
      const s = stops.map((x) => `${x[0]} ${(sr + x[1] * (er - sr)).toFixed(1)}px`).join(', ');
      return `radial-gradient(circle at ${c[0] * 100}% ${c[1] * 100}%, ${s || 'transparent, transparent'})`;
    }
    if (g.g === 'angular') {
      const c = g.c || [0.5, 0.5];
      const sa = g.sa || 0, ea = g.ea == null ? 360 : g.ea;
      const s = stops.map((x) => `${x[0]} ${(x[1] * (ea - sa)).toFixed(1)}deg`).join(', ');
      return `conic-gradient(from ${sa + 90}deg at ${c[0] * 100}% ${c[1] * 100}%, ${s || 'transparent, transparent'})`;
    }
    return 'transparent';
  }
  const matBg = { ultraThinMaterial: 'var(--materialThin)', thinMaterial: 'var(--materialThin)', regularMaterial: 'var(--material)', material: 'var(--material)', thickMaterial: 'var(--materialThick)', ultraThickMaterial: 'var(--materialThick)', bar: 'var(--bar)' };
  function applyPaint(el, p, prop) {
    if (p == null) return;
    if (typeof p === 'string') { el.style.background = p; return; }
    if (p.mat) { el.style.background = matBg[p.mat] || 'var(--material)'; el.style.backdropFilter = 'blur(20px) saturate(1.8)'; el.style.webkitBackdropFilter = 'blur(20px) saturate(1.8)'; return; }
    if (p.g) { el.style.background = gradientCSS(p); }
  }
  function paintColor(p) {
    if (p == null) return '';
    if (typeof p === 'string') return p;
    if (p.g && p.stops && p.stops.length) return p.stops[0][0];
    if (p.mat) return 'var(--c-primary)';
    return '';
  }

  // ---------- layout helpers ----------
  const hAlign = { leading: 'flex-start', center: 'center', trailing: 'flex-end' };
  const vAlign = { top: 'flex-start', center: 'center', bottom: 'flex-end', firstTextBaseline: 'baseline', lastTextBaseline: 'last baseline' };
  function alignXY(al) {
    al = al || 'center';
    const m = {
      topLeading: ['flex-start', 'flex-start'], top: ['center', 'flex-start'], topTrailing: ['flex-end', 'flex-start'],
      leading: ['flex-start', 'center'], center: ['center', 'center'], trailing: ['flex-end', 'center'],
      bottomLeading: ['flex-start', 'flex-end'], bottom: ['center', 'flex-end'], bottomTrailing: ['flex-end', 'flex-end'],
      leadingFirstTextBaseline: ['flex-start', 'flex-start'], centerFirstTextBaseline: ['center', 'flex-start'],
    };
    return m[al] || ['center', 'center'];
  }
  function place(r, axis, al) {
    const s = r.el.style;
    if (axis === 'v') {
      s.flex = r.v ? '1 1 0px' : '0 0 auto';
      if (r.h) s.alignSelf = 'stretch';
      if (!r.h && !r.noShrink) s.maxWidth = '100%';
    } else if (axis === 'h') {
      s.flex = r.h ? '1 1 0px' : (r.noShrink ? '0 0 auto' : '0 1 auto');
      if (r.v) s.alignSelf = 'stretch';
      if (r.prio) s.flexShrink = '0';
    } else if (axis === 'z') {
      const [jx, jy] = alignXY(al);
      s.justifySelf = r.h ? 'stretch' : jx.replace('flex-', '');
      s.alignSelf = r.v ? 'stretch' : jy.replace('flex-', '');
      if (!r.h) s.maxWidth = '100%';
    } else if (axis === 'w') {
      s.flex = r.v ? '1 1 0px' : '0 0 auto';
      if (r.h) s.alignSelf = 'stretch';
      if (!r.h && !r.noShrink) s.maxWidth = '100%';
    } else if (axis === 'sv') {
      s.flex = '0 0 auto';
      if (r.h) s.alignSelf = 'stretch';
      if (!r.h) s.maxWidth = '100%';
    } else if (axis === 'sh') {
      s.flex = '0 0 auto';
      if (r.v) s.alignSelf = 'stretch';
    }
  }
  function key(el, n) { if (n && n.id) el.dataset.k = n.id; }

  function renderMany(nodes, ctx, axis, container, al) {
    let h = false, v = false;
    for (const c of nodes || []) {
      const r = render(c, Object.assign({}, ctx, { axis }));
      place(r, axis, al);
      container.appendChild(r.el);
      h = h || r.h; v = v || r.v;
    }
    return { h, v };
  }

  function renderGroup(nodes, ctx) {
    if (!nodes || !nodes.length) return { el: mk('div', 'n'), h: false, v: false };
    if (nodes.length === 1) return render(nodes[0], ctx);
    const el = mk('div', 'n vstack');
    el.style.gap = '8px';
    const f = renderMany(nodes, ctx, 'v', el);
    return { el, h: f.h, v: f.v };
  }

  function isBleed(n) {
    if (!n) return false;
    if (n.m && n.m.some((m) => m.n === 'pad' || m.n === 'frame')) return false;
    return n.t === 'nav' || n.t === 'split' || n.t === 'tabview' || n.t === 'list' || n.t === 'camera' || (n.t === 'scroll' && n.p && n.p.ax !== 'h');
  }

  function buildRoot(root) {
    const kids = (root && root.c) || [];
    const el = mk('div', 'root');
    el.dataset.k = '__root';
    const dev = st.device;
    if (kids.length === 1 && isBleed(kids[0])) {
      el.classList.add('bleed');
      const r = render(kids[0], { root: true, bleed: true });
      place(r, 'w');
      r.el.style.alignSelf = 'stretch';
      r.el.style.flex = '1 1 0px';
      el.appendChild(r.el);
      return el;
    }
    el.style.paddingTop = dev.top + 'px';
    el.style.paddingBottom = dev.bottom + 'px';
    el.style.setProperty('--sat', '0px');
    el.style.setProperty('--sab', '0px');
    if (kids.length && kids[0].t === 'empty') {
      const t = mk('div', 'n text'); t.style.color = 'var(--c-secondary)'; t.style.padding = '30px'; t.style.textAlign = 'center';
      t.textContent = kids[0].p.msg; el.appendChild(t); return el;
    }
    const r = renderGroup(kids, { root: true });
    place(r, 'w');
    el.appendChild(r.el);
    return el;
  }

  // ---------- node renderers ----------
  function render(n, ctx) {
    let r;
    const p = n.p || {};
    switch (n.t) {
      case 'text': r = rText(n, ctx); break;
      case 'textcat': r = rTextCat(n, ctx); break;
      case 'image': r = rImage(n, ctx); break;
      case 'label': r = rLabel(n, ctx); break;
      case 'vstack': case 'hstack': case 'zstack': r = rStack(n, ctx); break;
      case 'spacer': r = rSpacer(n, ctx); break;
      case 'divider': {
        const el = mk('div', 'n divider');
        r = ctx.axis === 'h' ? { el, v: true, h: false } : { el, h: true, v: false };
        break;
      }
      case 'color': { const el = mk('div', 'n color'); el.style.background = p.c; r = { el, h: true, v: true, kind: 'color' }; break; }
      case 'gradient': { const el = mk('div', 'n gradient'); applyPaint(el, p.g); r = { el, h: true, v: true, kind: 'color' }; break; }
      case 'shape': r = rShape(n, ctx); break;
      case 'scroll': r = rScroll(n, ctx); break;
      case 'list': r = rList(n, ctx); break;
      case 'section': r = rBareSection(n, ctx); break;
      case 'button': r = rButton(n, ctx); break;
      case 'tap': {
        const el = mk('div', 'n w tapable'); el.dataset.tap = p.id || '';
        const g = renderGroup(n.c, ctx); place(g, 'w'); el.appendChild(g.el);
        r = { el, h: g.h, v: g.v }; break;
      }
      case 'toggle': r = rToggle(n, ctx); break;
      case 'textfield': r = rTextField(n, ctx); break;
      case 'slider': r = rSlider(n, ctx); break;
      case 'stepper': r = rStepper(n, ctx); break;
      case 'picker': r = rPicker(n, ctx); break;
      case 'datepicker': r = rDatePicker(n, ctx); break;
      case 'colorpicker': r = rColorPicker(n, ctx); break;
      case 'progress': r = rProgress(n, ctx); break;
      case 'link': {
        const el = mk('div', 'n link hstack'); el.dataset.url = p.url || '';
        el.style.gap = '6px'; el.style.alignItems = 'center';
        renderMany(n.c, ctx, 'h', el); r = { el, h: false, v: false }; break;
      }
      case 'menu': r = rMenu(n, ctx); break;
      case 'navlink': r = rNavLink(n, ctx); break;
      case 'nav': r = rNav(n, ctx); break;
      case 'split': r = rSplit(n, ctx); break;
      case 'tabview': r = rTabView(n, ctx); break;
      case 'groupbox': r = rGroupBox(n, ctx); break;
      case 'disclosure': r = rDisclosure(n, ctx); break;
      case 'labeled': r = rLabeled(n, ctx); break;
      case 'unavailable': r = rUnavailable(n, ctx); break;
      case 'geo': {
        const el = mk('div', 'n geo'); el.dataset.gid = p.gid;
        for (const c of n.c || []) { const cr = render(c, Object.assign({}, ctx, { axis: 'g' })); cr.el.style.flex = cr.v ? '1 1 0px' : '0 0 auto'; if (cr.h) cr.el.style.alignSelf = 'stretch'; el.appendChild(cr.el); }
        r = { el, h: true, v: true }; break;
      }
      case 'vgrid': case 'hgrid': r = rGrid(n, ctx); break;
      case 'grid': r = rGridX(n, ctx); break;
      case 'gridrow': { const el = mk('div', 'n gridrow'); renderMany(n.c, ctx, 'z', el); r = { el, h: false, v: false }; break; }
      case 'placeholder': { const el = mk('div', 'n placeholder'); el.textContent = (p.map ? '🗺  ' : '') + p.text + ' (not available in preview)'; r = { el, h: true, v: !!p.map }; break; }
      case 'empty': { const el = mk('div', 'n text'); el.textContent = p.msg; r = { el }; break; }
      case 'camera': r = { el: mediaHost(n), h: true, v: true }; break;
      case 'lbicon': case 'lbtitle': case 'lbvalue': {
        const el = mk('div', 'n ' + n.t); renderMany(n.c, ctx, 'h', el); r = { el, h: false, v: false }; break;
      }
      case 'tabpage': { const el = mk('div', 'n tabpage'); const g = renderGroup(n.c, ctx); place(g, 'w'); el.appendChild(g.el); r = { el, h: true, v: true }; break; }
      default: {
        const el = mk('div', 'n'); el.textContent = '[' + n.t + ']'; r = { el };
      }
    }
    if (n.m && n.m.length) r = applyMods(r, n, ctx);
    key(r.el, n);
    if (p.sid) r.el.dataset.sid = p.sid;
    return r;
  }

  function rText(n) {
    const el = mk('div', 'n text');
    const p = n.p || {};
    if (p.md) el.innerHTML = md(p.s); else el.textContent = p.s;
    return { el, h: false, v: false, kind: 'text' };
  }
  function rTextCat(n, ctx) {
    const el = mk('div', 'n textcat');
    for (const seg of n.c || []) {
      const span = mk('span');
      if (seg.t === 'text') { if (seg.p && seg.p.md) span.innerHTML = md(seg.p.s); else span.textContent = (seg.p && seg.p.s) || ''; }
      else if (seg.t === 'image') { const r = rImage(seg, ctx); span.appendChild(r.el); }
      for (const m of seg.m || []) textStyle(span, m);
      el.appendChild(span);
    }
    return { el, h: false, v: false, kind: 'text' };
  }

  function rImage(n, ctx) {
    const p = n.p || {};
    if (p.sys) {
      const el = mk('div', 'n sym' + (p.rz ? ' rz' : ''));
      let name = p.sys;
      if ((p.var === 'fill' || ctx.symFill) && !/\.fill$/.test(name)) name += '.fill';
      const s = window.SFSymbol(name);
      el.innerHTML = s.svg;
      if (!s.known && !st.warnedSyms.has(p.sys)) { st.warnedSyms.add(p.sys); console.info('SF Symbol not in the Switaculous symbol set: ' + p.sys); }
      if (p.rz) {
        if (p.fit === 'fit' || p.fit === 'fill') { el.style.aspectRatio = '1'; return { el, h: true, v: false }; }
        return { el, h: true, v: true };
      }
      return { el, h: false, v: false, noShrink: true };
    }
    const box = mk('div', 'n imgbox');
    const img = mk('img', 'img');
    img.draggable = false;
    img.src = p.url ? p.url : '/asset/' + encodeURIComponent(p.asset || '');
    img.alt = p.asset || '';
    img.onerror = () => {
      const miss = mk('div', 'imgmissing');
      miss.innerHTML = window.SFSymbol('photo').svg + `<span>${esc(p.asset || 'image')}</span>`;
      miss.style.width = '100%'; miss.style.height = '100%';
      img.replaceWith(miss);
    };
    box.appendChild(img);
    if (p.rz) {
      box.classList.add('rz');
      if (p.fit) box.classList.add(p.fit);
      if (p.fit === 'fit' && !p.ar) {
        img.style.height = 'auto'; img.style.maxHeight = '100%';
        box.style.display = 'flex'; box.style.alignItems = 'center'; box.style.justifyContent = 'center';
        return { el: box, h: true, v: true, img: true };
      }
      if (p.ar) box.style.aspectRatio = p.ar;
      return { el: box, h: true, v: true, img: true };
    }
    return { el: box, h: false, v: false, noShrink: true, img: true };
  }

  function rLabel(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n label ' + (p.style || ''));
    for (const part of n.c || []) {
      const d = mk('div', part.t);
      for (const c of part.c || []) { const r = render(c, Object.assign({}, ctx, { axis: 'h' })); place(r, 'h'); d.appendChild(r.el); }
      el.appendChild(d);
    }
    return { el, h: false, v: false };
  }

  function rStack(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n ' + n.t);
    const axis = n.t === 'vstack' ? 'v' : n.t === 'hstack' ? 'h' : 'z';
    const sp = isNum(p.sp) ? p.sp : 8;
    if (axis !== 'z') el.style.gap = sp + 'px';
    if (axis === 'v') el.style.alignItems = hAlign[p.al] || 'center';
    if (axis === 'h') el.style.alignItems = vAlign[p.al] || 'center';
    const f = renderMany(n.c, ctx, axis, el, p.al);
    return { el, h: f.h, v: f.v };
  }

  function rSpacer(n, ctx) {
    const el = mk('div', 'n spacer');
    const min = isNum(n.p && n.p.min) ? n.p.min : 8;
    if (ctx.axis === 'v') { el.style.minHeight = min + 'px'; return { el, v: true, h: false }; }
    if (ctx.axis === 'h') { el.style.minWidth = min + 'px'; return { el, h: true, v: false }; }
    return { el, h: false, v: false };
  }

  function rShape(n) {
    const el = mk('div', 'n shape');
    el.dataset.shape = JSON.stringify(n.p || {});
    return { el, h: true, v: true, kind: 'shape' };
  }

  function drawShape(el) {
    let p;
    try { p = JSON.parse(el.dataset.shape); } catch (e) { return; }
    const w = el.offsetWidth, h = el.offsetHeight;
    if (el._drawn === el.dataset.shape + w + 'x' + h) return;
    el._drawn = el.dataset.shape + w + 'x' + h;
    const ns = 'http://www.w3.org/2000/svg';
    let svg = el.querySelector(':scope > svg');
    if (!svg) { svg = document.createElementNS(ns, 'svg'); el.appendChild(svg); }
    svg.setAttribute('width', w); svg.setAttribute('height', h);
    svg.innerHTML = '';
    const lw = p.lw || 0;
    const ins = (p.inner ? lw / 2 : 0) + (p.inset || 0);
    let x = ins, y = ins, W = Math.max(0, w - 2 * ins), H = Math.max(0, h - 2 * ins);
    let shape;
    const k = p.k || 'rect';
    if (k === 'circle') {
      const r = Math.min(W, H) / 2;
      shape = document.createElementNS(ns, 'circle');
      shape.setAttribute('cx', x + W / 2); shape.setAttribute('cy', y + H / 2); shape.setAttribute('r', r);
    } else if (k === 'ellipse') {
      shape = document.createElementNS(ns, 'ellipse');
      shape.setAttribute('cx', x + W / 2); shape.setAttribute('cy', y + H / 2); shape.setAttribute('rx', W / 2); shape.setAttribute('ry', H / 2);
    } else if (k === 'uneven') {
      const tl = p.topLeadingRadius || 0, tr = p.topTrailingRadius || 0, br = p.bottomTrailingRadius || 0, bl = p.bottomLeadingRadius || 0;
      shape = document.createElementNS(ns, 'path');
      shape.setAttribute('d', `M${x + tl},${y}H${x + W - tr}A${tr},${tr} 0 0 1 ${x + W},${y + tr}V${y + H - br}A${br},${br} 0 0 1 ${x + W - br},${y + H}H${x + bl}A${bl},${bl} 0 0 1 ${x},${y + H - bl}V${y + tl}A${tl},${tl} 0 0 1 ${x + tl},${y}Z`);
    } else {
      shape = document.createElementNS(ns, 'rect');
      shape.setAttribute('x', x); shape.setAttribute('y', y); shape.setAttribute('width', W); shape.setAttribute('height', H);
      let r = 0;
      if (k === 'capsule') r = Math.min(W, H) / 2;
      if (k === 'rrect') r = Math.min(p.r || 0, W / 2, H / 2);
      if (r) { shape.setAttribute('rx', r); shape.setAttribute('ry', r); }
    }
    const defs = document.createElementNS(ns, 'defs');
    let gid = 0;
    const paintAttr = (pt) => {
      if (pt == null) return 'currentColor';
      if (typeof pt === 'string') return pt;
      if (pt.g === 'linear') {
        const id = 'g' + Math.random().toString(36).slice(2) + (gid++);
        const lg = document.createElementNS(ns, 'linearGradient');
        lg.id = id;
        const sp = pt.sp || [0.5, 0], ep = pt.ep || [0.5, 1];
        lg.setAttribute('x1', sp[0]); lg.setAttribute('y1', sp[1]); lg.setAttribute('x2', ep[0]); lg.setAttribute('y2', ep[1]);
        for (const s of pt.stops || []) { const stp = document.createElementNS(ns, 'stop'); stp.setAttribute('offset', s[1]); stp.setAttribute('stop-color', s[0]); lg.appendChild(stp); }
        defs.appendChild(lg);
        return `url(#${id})`;
      }
      if (pt.g) {
        const id = 'g' + Math.random().toString(36).slice(2) + (gid++);
        const rg = document.createElementNS(ns, 'radialGradient');
        rg.id = id;
        for (const s of pt.stops || []) { const stp = document.createElementNS(ns, 'stop'); stp.setAttribute('offset', s[1]); stp.setAttribute('stop-color', s[0]); rg.appendChild(stp); }
        defs.appendChild(rg);
        return `url(#${id})`;
      }
      if (pt.mat) return 'var(--material)';
      return 'currentColor';
    };
    svg.appendChild(defs);
    if (p.stroke) {
      shape.setAttribute('fill', 'none');
      shape.setAttribute('stroke', paintAttr(p.stroke));
      shape.setAttribute('stroke-width', lw || 1);
      if (p.cap) shape.setAttribute('stroke-linecap', p.cap === 'round' ? 'round' : p.cap === 'square' ? 'square' : 'butt');
      if (p.dash) shape.setAttribute('stroke-dasharray', p.dash.join(' '));
    } else {
      shape.setAttribute('fill', paintAttr(p.fill));
    }
    if (p.trim) {
      shape.setAttribute('pathLength', '1000');
      const a = Math.max(0, Math.min(1, p.trim[0])) * 1000, b = Math.max(0, Math.min(1, p.trim[1])) * 1000;
      if (p.stroke) {
        shape.setAttribute('stroke-dasharray', `${Math.max(0, b - a)} 1000`);
        shape.setAttribute('stroke-dashoffset', -a);
      }
    }
    svg.appendChild(shape);
  }

  function rScroll(n, ctx) {
    const p = n.p || {};
    const ax = p.ax || 'v';
    const el = mk('div', 'n scroll');
    const inner = mk('div', 'scrollin ' + ax);
    if (ax === 'v') { el.style.overflowY = 'auto'; el.style.overflowX = 'hidden'; }
    else if (ax === 'h') { el.style.overflowX = 'auto'; el.style.overflowY = 'hidden'; }
    else { el.style.overflow = 'auto'; }
    if (ctx.bleed) { inner.style.paddingTop = 'var(--sat)'; inner.style.paddingBottom = 'var(--sab)'; }
    if (ctx.pageHeader) for (const h of ctx.pageHeader) inner.appendChild(h);
    const sub = Object.assign({}, ctx, { bleed: false, pageHeader: null, root: false });
    const kids = n.c || [];
    if (kids.length > 1) inner.style.gap = '8px';
    for (const c of kids) {
      const r = render(c, Object.assign({}, sub, { axis: ax === 'h' ? 'h' : 'v' }));
      place(r, ax === 'h' ? 'sh' : 'sv');
      inner.appendChild(r.el);
    }
    el.appendChild(inner);
    return ax === 'h' ? { el, h: true, v: false } : { el, h: true, v: true };
  }

  // ----- lists -----
  function rowMods(node) {
    const out = { rest: [] };
    for (const m of node.m || []) {
      if (m.n === 'rowbg' || m.n === 'rowinsets' || m.n === 'rowsep' || m.n === 'badge') out[m.n] = m.a || {};
      else out.rest.push(m);
    }
    return out;
  }

  function rList(n, ctx) {
    const p = n.p || {};
    let style = p.style || 'insetGrouped';
    if (style === 'automatic') style = 'insetGrouped';
    const el = mk('div', 'n list ' + style);
    if ((n.m || []).some((m) => m.n === 'scrollbg' && m.a && m.a.hidden)) el.classList.add('nobg');
    const top = mk('div', 'listtop');
    if (ctx.bleed) top.style.height = 'var(--sat)';
    el.appendChild(top);
    if (ctx.pageHeader) for (const h of ctx.pageHeader) { h.style.alignSelf = 'stretch'; el.appendChild(h); }
    const sub = Object.assign({}, ctx, { bleed: false, pageHeader: null, inList: true, root: false });
    for (const sec of n.c || []) {
      const s = mk('div', 'lsec');
      key(s, sec);
      const sp = sec.p || {};
      const head = mk('div', 'lhead');
      if (sp.h && sp.h.length) {
        const g = renderGroup(sp.h, Object.assign({}, sub, { inList: false }));
        head.appendChild(g.el);
      } else head.classList.add('nohead');
      s.appendChild(head);
      const rows = mk('div', 'lrows');
      let hideNextSep = false;
      for (const rn of sec.c || []) {
        for (const row of rRows(rn, sub)) {
          if (hideNextSep) row.classList.add('nosep');
          hideNextSep = row._hideSepAfter;
          rows.appendChild(row);
        }
      }
      if (rows.childNodes.length) s.appendChild(rows);
      if (sp.f && sp.f.length) {
        const foot = mk('div', 'lfoot');
        foot.appendChild(renderGroup(sp.f, Object.assign({}, sub, { inList: false })).el);
        s.appendChild(foot);
      }
      el.appendChild(s);
    }
    el.appendChild(mk('div', 'listbottom'));
    return { el, h: true, v: true };
  }

  function rRows(node, ctx) {
    if (node.t === 'picker' && node.p && node.p.style === 'inline') {
      const out = [];
      if (node.p.label && node.p.label.length) {
        const hr = mk('div', 'row'); const hc = mk('div', 'rowc'); hc.appendChild(renderGroup(node.p.label, ctx).el); hr.appendChild(hc); out.push(hr);
      }
      for (const o of node.p.opts || []) {
        const row = mk('div', 'row tap'); const c = mk('div', 'rowc');
        row.dataset.tap = node.p.id; row.dataset.val = o.k; row.dataset.k = node.id + '/' + o.k;
        const r = render(o.n, Object.assign({}, ctx, { axis: 'h' })); place(r, 'h'); c.appendChild(r.el);
        if (o.k === node.p.sel) { const ck = mk('div', 'chev'); ck.style.color = 'var(--tint)'; ck.innerHTML = window.SFSymbol('checkmark').svg; c.appendChild(ck); }
        row.appendChild(c); out.push(row);
      }
      return out;
    }
    return [rRow(node, ctx)];
  }

  function rRow(node, ctx) {
    const rm = rowMods(node);
    const nn = Object.assign({}, node, { m: rm.rest });
    const row = mk('div', 'row');
    const c = mk('div', 'rowc');
    const p = nn.p || {};
    let r;
    if (nn.t === 'navlink' && !p.dis) {
      row.classList.add('tap');
      row.dataset.tap = p.id;
      r = renderGroup(nn.c, Object.assign({}, ctx, { axis: 'h' }));
      if (nn.m.length) r = applyMods(r, nn, ctx);
      place(r, 'h');
      c.appendChild(r.el);
      if (r.h) r.el.style.flex = '1 1 0px';
      const ch = mk('div', 'chev'); ch.innerHTML = window.SFSymbol('chevron.right').svg; c.appendChild(ch);
    } else {
      r = render(nn, Object.assign({}, ctx, { axis: 'h' }));
      place(r, 'h');
      if (nn.t === 'button' && p.id) {
        row.classList.add('tap'); row.dataset.tap = p.id;
        const keepsOwnWidth = /\b(bordered|borderedProminent|glass|glassProminent)\b/.test(r.el.className);
        r.el.style.flex = keepsOwnWidth ? '0 0 auto' : '1 1 auto';
      }
      const inner = findNavLink(nn);
      if (inner && inner.p && inner.p.id && !inner.p.dis) {
        row.classList.add('tap');
        row.dataset.tap = inner.p.id;
        c.appendChild(r.el);
        if (!r.h) { const sp = mk('div'); sp.style.flex = '1'; c.appendChild(sp); }
        const ch = mk('div', 'chev'); ch.innerHTML = window.SFSymbol('chevron.right').svg; c.appendChild(ch);
        r = null;
      }
      if (r) {
        if (nn.t === 'picker' && (p.style === 'menu' || p.style === 'navigationLink')) row.classList.add('tap');
        c.appendChild(r.el);
      }
    }
    if (rm.badge && rm.badge.v != null) { const b = mk('div', 'rowbadge'); b.textContent = rm.badge.v; c.appendChild(b); }
    if (rm.rowinsets) { const a = rm.rowinsets; c.style.padding = `${a.t || 0}px ${a.r || 0}px ${a.b || 0}px ${a.l || 0}px`; c.style.margin = '-11px -16px'; }
    if (rm.rowbg) {
      if (rm.rowbg.paint != null) { const col = typeof rm.rowbg.paint === 'string' ? rm.rowbg.paint : null; if (col) c.style.setProperty('--rowbg', col); else applyPaint(c, rm.rowbg.paint); }
      if (rm.rowbg.node) { c.style.setProperty('--rowbg', 'transparent'); const lay = mk('div', 'layer bglayer'); lay.style.zIndex = '0'; const g = renderGroup(rm.rowbg.node, ctx); place(g, 'w'); lay.appendChild(g.el); c.prepend(lay); c.classList.add('iso'); }
    }
    if (rm.rowsep && rm.rowsep.hidden) { row.classList.add('nosep'); row._hideSepAfter = true; }
    if (nn.t === 'label' || (nn.t === 'navlink' && nn.c && nn.c[0] && nn.c[0].t === 'label')) row.style.setProperty('--sepl', '60px');
    row.appendChild(c);
    const actions = [];
    if (p.swipe) for (const it of flattenItems(p.swipe)) actions.push(it);
    else if (p.del) actions.push({ t: 'Delete', role: 'destructive', id: p.del });
    if (actions.length) {
      const sw = mk('div', 'swipe' + (p.swipeL ? ' left' : ''));
      for (const a of actions) {
        const b = mk('div', a.role === 'destructive' ? 'destructive' : '');
        b.textContent = a.t || (a.sys ? '' : 'Action');
        if (a.sys && !a.t) b.innerHTML = window.SFSymbol(a.sys).svg;
        if (a.tint) b.style.background = a.tint;
        b.dataset.tap = a.id;
        sw.appendChild(b);
      }
      row.prepend(sw);
      row.dataset.swipe = p.swipeL ? 'l' : 'r';
    }
    if (p.ctxm) { const id = (node.id || 'row') + '#ctx'; st.menuDefs[id] = { kind: 'items', items: p.ctxm }; row.dataset.ctx = id; }
    key(row, node);
    return row;
  }

  function findNavLink(n) {
    if (!n || n.t === 'list' || n.t === 'nav') return null;
    if (n.t === 'navlink') return n;
    for (const c of n.c || []) { const f = findNavLink(c); if (f) return f; }
    return null;
  }

  function flattenItems(nodes) {
    const out = [];
    const walk = (ns) => {
      for (const n of ns || []) {
        if (n.t === 'button' || n.t === 'navlink') {
          const tint = (n.m || []).find((m) => m.n === 'tint');
          out.push({ t: plainText(n.c), sys: firstSys(n.c), role: n.p && n.p.role, id: n.p && n.p.id, tint: tint && paintColor(tint.a.c) });
        } else if (n.t === 'tap') {
          out.push({ t: plainText(n.c), id: n.p && n.p.id });
        } else walk(n.c);
      }
    };
    walk(nodes);
    return out;
  }
  function plainText(nodes) {
    let s = '';
    for (const n of nodes || []) {
      if (n.t === 'text') s += n.p.s;
      else if (n.t === 'textcat') s += plainText(n.c);
      else if (n.t !== 'image' && n.t !== 'lbicon') s += plainText(n.c);
    }
    return s;
  }
  function firstSys(nodes) {
    for (const n of nodes || []) {
      if (n.t === 'image' && n.p && n.p.sys) return n.p.sys;
      const s = firstSys(n.c); if (s) return s;
    }
    return '';
  }

  function rBareSection(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n vstack');
    el.style.alignItems = 'flex-start';
    el.style.gap = '8px';
    if (p.h) { const h = renderGroup(p.h, ctx); h.el.style.color = 'var(--c-secondary)'; h.el.style.fontSize = '13px'; h.el.style.textTransform = 'uppercase'; el.appendChild(h.el); }
    const f = renderMany(n.c, ctx, 'v', el);
    if (p.f) { const ft = renderGroup(p.f, ctx); ft.el.style.color = 'var(--c-secondary)'; ft.el.style.fontSize = '13px'; el.appendChild(ft.el); }
    return { el, h: f.h, v: f.v };
  }

  // ----- controls -----
  function rButton(n, ctx) {
    const p = n.p || {};
    let style = p.style || 'automatic';
    if (style === 'automatic' || style === 'borderless' || style === 'link') style = ctx.inList ? 'listbtn' : 'borderless';
    const el = mk('div', 'n btn ' + style + (p.role ? ' role-' + p.role : '') + (p.dis ? ' dis' : ''));
    if (p.cs) el.classList.add('cs-' + p.cs);
    if (p.id) el.dataset.tap = p.id;
    if (p.photos) { el.dataset.photos = p.photos; el.dataset.multi = p.multi ? '1' : ''; el.dataset.accept = p.accept || 'image/*'; }
    el.style.gap = '8px';
    let h = false, v = false;
    for (const c of n.c || []) {
      const r = render(c, Object.assign({}, ctx, { axis: 'h' }));
      place(r, 'h');
      el.appendChild(r.el);
      h = h || r.h; v = v || r.v;
    }
    if (ctx.inList && style === 'listbtn') { el.style.justifyContent = 'flex-start'; }
    return { el, h, v };
  }

  function rToggle(n, ctx) {
    const p = n.p || {};
    const btn = p.style === 'button';
    const el = mk('div', 'n toggle' + (p.on ? ' on' : '') + (p.dis ? ' dis' : '') + (btn ? ' btnstyle' : ''));
    const lab = mk('div', 'tlabel');
    const hasLabel = n.c && n.c.length;
    if (hasLabel) { renderMany(n.c, ctx, 'h', lab); lab.style.display = 'flex'; lab.style.alignItems = 'center'; lab.style.gap = '8px'; el.appendChild(lab); }
    const sw = mk('div', 'switch' + (p.on ? ' on' : ''));
    if (p.id) { if (btn) el.dataset.tap = p.id; else sw.dataset.tap = p.id; }
    el.appendChild(sw);
    return { el, h: !!hasLabel && !btn, v: false, noShrink: !hasLabel };
  }

  function rTextField(n, ctx) {
    const p = n.p || {};
    const style = p.style || 'automatic';
    const el = mk('div', 'n tf ' + style + (p.editor ? ' editor' : ''));
    let input;
    if (p.multi) { input = mk('textarea'); input.rows = 1; if (p.editor) input.rows = 3; }
    else { input = mk('input'); input.type = p.sec ? 'password' : (p.num ? 'text' : 'text'); if (p.num) input.inputMode = 'decimal'; }
    input.value = p.v == null ? '' : p.v;
    input.setAttribute('value', input.value);
    if (p.ph) input.placeholder = p.ph;
    input.spellcheck = false;
    input.autocomplete = 'off';
    if (p.id) input.dataset.input = p.id;
    if (p.sub) input.dataset.sub = p.sub;
    if (p.fid) input.dataset.fid = p.fid;
    if (p.foc) input.dataset.autofocus = '1';
    if (p.dis) input.disabled = true;
    el.appendChild(input);
    return { el, h: true, v: !!p.editor };
  }

  function rSlider(n) {
    const p = n.p || {};
    const el = mk('div', 'n slider' + (p.dis ? ' dis' : ''));
    if (p.minL) el.appendChild(renderGroup(p.minL, {}).el);
    const inp = mk('input'); inp.type = 'range';
    inp.min = p.min; inp.max = p.max; inp.step = p.step || 'any';
    inp.value = p.v;
    inp.setAttribute('value', p.v);
    const pct = p.max > p.min ? ((p.v - p.min) / (p.max - p.min)) * 100 : 0;
    inp.style.setProperty('--pct', pct + '%');
    if (p.id) inp.dataset.slide = p.id;
    el.appendChild(inp);
    if (p.maxL) el.appendChild(renderGroup(p.maxL, {}).el);
    return { el, h: true, v: false };
  }

  function rStepper(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n stepper');
    if (n.c && n.c.length) { const lab = mk('div', 'slabel'); renderMany(n.c, ctx, 'h', lab); lab.style.display = 'flex'; lab.style.gap = '8px'; el.appendChild(lab); }
    const ctl = mk('div', 'stepctl');
    const dec = mk('div', p.dec ? '' : 'off'); dec.textContent = '−'; if (p.dec) dec.dataset.tap = p.dec;
    const inc = mk('div', p.inc ? '' : 'off'); inc.textContent = '+'; if (p.inc) inc.dataset.tap = p.inc;
    ctl.append(dec, mk('span', 'sep'), inc);
    el.appendChild(ctl);
    return { el, h: !!(n.c && n.c.length), v: false };
  }

  function optLabelEl(o, ctx) {
    if (!o) return mk('span');
    if (o.n) { const r = render(o.n, Object.assign({}, ctx, { axis: 'h', inList: false })); r.el.removeAttribute('data-k'); return r.el; }
    const s = mk('span'); s.textContent = o.t; return s;
  }

  function rPicker(n, ctx) {
    const p = n.p || {};
    const opts = p.opts || [];
    const sel = opts.find((o) => o.k === p.sel);
    const style = p.style || 'menu';
    if (style === 'segmented' || style === 'palette') {
      const el = mk('div', 'n seg');
      for (const o of opts) {
        const d = mk('div', o.k === p.sel ? 'sel' : '');
        d.dataset.tap = p.id; d.dataset.val = o.k;
        if (o.t) d.textContent = o.t; else if (o.sys) d.innerHTML = window.SFSymbol(o.sys).svg;
        el.appendChild(d);
      }
      return { el, h: true, v: false };
    }
    if (style === 'wheel') {
      const el = mk('div', 'n wheel');
      for (const o of opts) { const d = mk('div', o.k === p.sel ? 'sel' : ''); d.textContent = o.t; d.dataset.tap = p.id; d.dataset.val = o.k; el.appendChild(d); }
      return { el, h: true, v: false };
    }
    const mid = n.id + '#menu';
    st.menuDefs[mid] = { kind: 'picker', opts, sel: p.sel, id: p.id };
    if (ctx.inList) {
      const el = mk('div', 'n pickrow');
      el.dataset.menu = mid;
      const lab = mk('div', 'plabel');
      if (p.label) lab.appendChild(renderGroup(p.label, ctx).el);
      const val = mk('div', 'pval');
      val.textContent = sel ? sel.t : '';
      const ch = mk('span', 'chev');
      ch.innerHTML = window.SFSymbol(style === 'navigationLink' ? 'chevron.right' : 'chevron.up.chevron.down').svg;
      ch.style.fontSize = '13px';
      val.appendChild(ch);
      el.append(lab, val);
      return { el, h: true, v: false };
    }
    const el = mk('div', 'n pickbtn');
    el.dataset.menu = mid;
    const t = mk('span'); t.textContent = sel ? sel.t : (p.label ? plainText(p.label) : '');
    const ch = mk('span', 'chev'); ch.innerHTML = window.SFSymbol('chevron.up.chevron.down').svg;
    el.append(t, ch);
    return { el, h: false, v: false };
  }

  function rDatePicker(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n datepick');
    if (n.c && n.c.length) { const lab = mk('div', 'dlabel'); lab.appendChild(renderGroup(n.c, ctx).el); el.appendChild(lab); }
    const pills = mk('div', 'pills');
    const v = p.v || '';
    const mkPill = (type, val, text) => {
      const pill = mk('div', 'pill');
      const span = mk('span'); span.textContent = text;
      const inp = mk('input'); inp.type = type; inp.value = val; inp.setAttribute('value', val); inp.dataset.change = p.id;
      pill.append(span, inp);
      return pill;
    };
    const disp = p.disp || {};
    if (p.comps === 'date' || p.comps === 'both') pills.appendChild(mkPill('date', v.slice(0, 10), disp.date || v.slice(0, 10)));
    if (p.comps === 'hourAndMinute' || p.comps === 'both') pills.appendChild(mkPill('time', v.slice(11, 16), disp.hourAndMinute || v.slice(11, 16)));
    el.appendChild(pills);
    return { el, h: !!(n.c && n.c.length), v: false };
  }

  function rColorPicker(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n colorpick');
    if (n.c && n.c.length) { const lab = mk('div', 'clabel'); lab.appendChild(renderGroup(n.c, ctx).el); el.appendChild(lab); }
    const well = mk('div', 'well');
    well.style.background = p.v || '#000';
    const inp = mk('input'); inp.type = 'color'; inp.value = p.v || '#000000'; inp.dataset.change = p.id;
    well.appendChild(inp);
    el.appendChild(well);
    return { el, h: !!(n.c && n.c.length), v: false };
  }

  function rProgress(n, ctx) {
    const p = n.p || {};
    if (p.v == null) {
      const el = mk('div', 'n progress indeterminate');
      const sp = mk('div', 'spinner');
      for (let i = 0; i < 8; i++) { const b = mk('i'); b.style.transform = `rotate(${i * 45}deg)`; b.style.animationDelay = (-(8 - i) / 8) + 's'; sp.appendChild(b); }
      el.appendChild(sp);
      if (n.c && n.c.length) { const lab = renderGroup(n.c, ctx).el; lab.style.color = 'var(--c-secondary)'; el.appendChild(lab); }
      return { el, h: false, v: false };
    }
    const el = mk('div', 'n progress' + (p.gauge ? ' gauge' : ''));
    if (n.c && n.c.length) el.appendChild(renderGroup(n.c, ctx).el);
    const bar = mk('div', 'bar'); const fill = mk('div'); fill.style.width = (p.v * 100) + '%'; bar.appendChild(fill);
    el.appendChild(bar);
    if (p.cur) { const c = renderGroup(p.cur, ctx).el; c.style.color = 'var(--c-secondary)'; c.style.fontSize = '15px'; el.appendChild(c); }
    return { el, h: true, v: false };
  }

  function rMenu(n, ctx) {
    const p = n.p || {};
    let style = p.style || 'automatic';
    if (style === 'automatic' || style === 'borderless') style = '';
    const el = mk('div', 'n menubtn btn ' + style);
    el.style.gap = '6px';
    el.dataset.menu = p.mid;
    st.menuDefs[p.mid] = { kind: 'items', items: p.items || [] };
    renderMany(n.c, ctx, 'h', el);
    return { el, h: false, v: false };
  }

  function rNavLink(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n btn navlink' + (p.dis ? ' dis' : '') + (ctx.inList ? ' plain' : ''));
    if (p.id && !p.dis) el.dataset.tap = p.id;
    el.style.gap = '8px';
    const f = renderMany(n.c, ctx, 'h', el);
    return { el, h: f.h, v: f.v };
  }

  function rGroupBox(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n groupbox');
    if (p.label) { const l = renderGroup(p.label, ctx).el; l.classList.add('gblabel'); el.appendChild(l); }
    renderMany(n.c, ctx, 'v', el);
    return { el, h: true, v: false };
  }

  function rDisclosure(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n disclosure' + (p.open ? ' open' : ''));
    const head = mk('div', 'dhead'); head.dataset.tap = p.id;
    const dl = mk('div', 'dl'); if (p.label) dl.appendChild(renderGroup(p.label, ctx).el);
    const ch = mk('div', 'chev'); ch.innerHTML = window.SFSymbol('chevron.right').svg;
    head.append(dl, ch);
    el.appendChild(head);
    if (p.open) { const body = mk('div', 'dbody'); renderMany(n.c, Object.assign({}, ctx, { inList: false }), 'v', body); el.appendChild(body); }
    return { el, h: true, v: false };
  }

  function rLabeled(n, ctx) {
    const el = mk('div', 'n labeled');
    for (const part of n.c || []) {
      const d = mk('div', part.t);
      d.appendChild(renderGroup(part.c, ctx).el);
      el.appendChild(d);
    }
    return { el, h: true, v: false };
  }

  function rUnavailable(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n unavailable');
    if (p.lbl) el.appendChild(renderGroup(p.lbl, ctx).el);
    if (p.sys) { const ic = mk('div', 'uicon sym'); ic.innerHTML = window.SFSymbol(p.sys).svg; el.appendChild(ic); }
    if (p.title) { const t = mk('div', 'utitle'); t.textContent = p.title; el.appendChild(t); }
    if (p.desc) { const d = mk('div', 'udesc'); d.appendChild(renderGroup(p.desc, ctx).el); el.appendChild(d); }
    if (n.c && n.c.length) { const a = mk('div', 'uact'); renderMany(n.c, ctx, 'v', a); el.appendChild(a); }
    return { el, h: true, v: true };
  }

  function rGrid(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n ' + n.t);
    const cols = (p.cols || []).map((c) => {
      if (c.k === 'fixed') return `${c.size || 50}px`;
      if (c.k === 'adaptive') return `repeat(auto-fill, minmax(${c.min || 50}px, ${c.max ? c.max + 'px' : '1fr'}))`;
      return `minmax(${c.min || 0}px, ${c.max ? c.max + 'px' : '1fr'})`;
    });
    const gap = isNum(p.sp) ? p.sp : 8;
    const colGap = (p.cols && p.cols[0] && isNum(p.cols[0].sp)) ? p.cols[0].sp : 8;
    if (n.t === 'vgrid') {
      el.style.gridTemplateColumns = cols.join(' ') || '1fr';
      el.style.rowGap = gap + 'px';
      el.style.columnGap = colGap + 'px';
    } else {
      el.style.gridTemplateRows = (p.cols || []).map((c) => c.k === 'fixed' ? `${c.size || 50}px` : 'auto').join(' ');
      el.style.columnGap = gap + 'px';
      el.style.rowGap = colGap + 'px';
    }
    for (const c of n.c || []) {
      const r = render(c, Object.assign({}, ctx, { axis: 'z' }));
      r.el.style.justifySelf = r.h ? 'stretch' : 'center';
      r.el.style.alignSelf = r.v ? 'stretch' : 'center';
      if (r.v && n.t === 'vgrid') r.el.style.minHeight = r.el.style.minHeight || '0';
      el.appendChild(r.el);
    }
    return n.t === 'vgrid' ? { el, h: true, v: false } : { el, h: false, v: true };
  }

  function rGridX(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n gridx');
    let cols = 1;
    for (const r of n.c || []) if (r.t === 'gridrow') cols = Math.max(cols, (r.c || []).length);
    el.style.gridTemplateColumns = `repeat(${cols}, auto)`;
    el.style.columnGap = (isNum(p.hs) ? p.hs : 8) + 'px';
    el.style.rowGap = (isNum(p.vs) ? p.vs : 8) + 'px';
    for (const r of n.c || []) {
      if (r.t === 'gridrow') {
        for (const c of r.c || []) { const cr = render(c, Object.assign({}, ctx, { axis: 'z' })); cr.el.style.justifySelf = cr.h ? 'stretch' : (p.al === 'leading' ? 'start' : p.al === 'trailing' ? 'end' : 'center'); el.appendChild(cr.el); }
        for (let i = (r.c || []).length; i < cols; i++) el.appendChild(mk('div'));
      } else {
        const cr = render(r, Object.assign({}, ctx, { axis: 'z' }));
        cr.el.style.gridColumn = `1 / span ${cols}`;
        cr.el.style.justifySelf = cr.h ? 'stretch' : 'center';
        el.appendChild(cr.el);
      }
    }
    return { el, h: false, v: false };
  }

  // ----- navigation -----
  function rNav(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n nav');
    const depth = p.depth || 0;
    (n.c || []).forEach((pg, i) => {
      const pe = rPage(pg, ctx, i);
      if (i < depth) pe.classList.add('under');
      if (i > 0) { pe.dataset.enter = 'page'; pe.dataset.exit = 'page'; }
      el.appendChild(pe);
    });
    return { el, h: true, v: true };
  }

  // iPad: the sidebar column shows the root page; pages pushed from it show in
  // the detail column, where the first one has no back button, like iPadOS.
  function rSplit(n, ctx) {
    const p = n.p || {};
    const [sidebarNav, ...detailNodes] = n.c || [];
    const el = mk('div', 'n split' + (p.portrait ? ' portrait' : '') + (st.sidebarOpen ? ' open' : ''));
    el.dataset.k = n.id;
    const pages = sidebarNav.c || [];
    const side = mk('div', 'splitside');
    const sideNav = render({ t: 'nav', id: sidebarNav.id, p: { depth: 0 }, c: pages.slice(0, 1) }, Object.assign({}, ctx, { bleed: true, inList: false }));
    side.appendChild(sideNav.el);
    const detail = mk('div', 'splitdetail');
    let detailNode;
    if (pages.length > 1) {
      const pushed = pages.slice(1).map((pg, i) => (i === 0 ? Object.assign({}, pg, { p: Object.assign({}, pg.p, { back: '' }) }) : pg));
      detailNode = render({ t: 'nav', id: sidebarNav.id + '/detail', p: { depth: pushed.length - 1 }, c: pushed }, Object.assign({}, ctx, { bleed: true }));
    } else {
      detailNode = renderGroup(detailNodes, Object.assign({}, ctx, { bleed: true }));
      place(detailNode, 'w');
    }
    detailNode.el.style.flex = '1 1 0px'; detailNode.el.style.alignSelf = 'stretch';
    detail.appendChild(detailNode.el);
    if (p.portrait) {
      const toggle = mk('div', 'sidebartoggle');
      toggle.innerHTML = window.SFSymbol('sidebar.left').svg;
      toggle.dataset.sidebartoggle = '1';
      detail.appendChild(toggle);
      const dim = mk('div', 'splitdim');
      dim.dataset.sidebartoggle = '1';
      el.appendChild(dim);
    }
    el.append(side, detail);
    return { el, h: true, v: true };
  }

  function navItems(nodes, ctx) {
    const out = [];
    for (const c of nodes || []) {
      const wrap = mk('div', 'nbitem');
      const r = render(c, Object.assign({}, ctx, { axis: 'h', toolbar: true, inList: false, bleed: false, pageHeader: null }));
      wrap.appendChild(r.el);
      out.push(wrap);
    }
    return out;
  }

  function rPage(pg, ctx, idx) {
    const p = pg.p || {};
    const el = mk('div', 'page' + (p.mode === 'large' && p.title ? ' large' : ''));
    key(el, pg);
    const content = pg.c || [];
    const single = content.length === 1 ? content[0] : null;
    if (single && single.t === 'list' && (single.p.style || 'insetGrouped') !== 'plain' && !(single.m || []).some((m) => m.n === 'scrollbg')) el.classList.add('grouped');
    const sub = Object.assign({}, ctx, { root: false, bleed: false, pageHeader: null });
    if (!p.hideBar) {
      const nb = mk('div', 'navbar');
      const row = mk('div', 'nbrow');
      const left = mk('div', 'nbleft');
      const right = mk('div', 'nbright');
      if (p.back) {
        const b = mk('div', 'back');
        b.dataset.tap = p.back;
        b.innerHTML = window.SFSymbol('chevron.left').svg;
        b.firstChild.classList.add('backchev');
        const t = mk('span', 'btxt'); t.textContent = p.backT || 'Back';
        b.appendChild(t);
        left.appendChild(b);
      }
      const lead = navItems(p.lead, sub), trail = navItems(p.trail, sub);
      if (lead.length) { const g = mk('div', 'nbgroup'); lead.forEach((x) => g.appendChild(x)); left.appendChild(g); }
      if (trail.length) { const g = mk('div', 'nbgroup'); trail.forEach((x) => g.appendChild(x)); right.appendChild(g); }
      row.appendChild(left);
      const title = mk('div', 'nbtitle');
      if (p.prin) { title.classList.add('prin'); title.appendChild(renderGroup(p.prin, sub).el); }
      else title.textContent = p.title || '';
      row.appendChild(title);
      row.appendChild(right);
      nb.appendChild(row);
      if (p.barBg) { nb.classList.add('hasbg'); if (typeof p.barBg === 'string') nb.style.setProperty('--barbg', p.barBg); else applyPaint(nb, p.barBg); }
      el.appendChild(nb);
    }
    const body = mk('div', 'pagebody');
    const header = [];
    if (p.mode === 'large' && p.title && !p.hideBar) { const lt = mk('div', 'largetitle'); lt.textContent = p.title; header.push(lt); }
    if (p.search) {
      const sb = mk('div', 'searchbox');
      sb.innerHTML = window.SFSymbol('magnifyingglass').svg;
      const inp = mk('input'); inp.placeholder = p.search.ph || 'Search'; inp.value = p.search.v || ''; inp.setAttribute('value', inp.value); inp.dataset.input = p.search.id;
      sb.appendChild(inp);
      header.push(sb);
    }
    if (p.hideBar) body.style.setProperty('--sat', 'var(--sat0)');
    if (single && isBleed(single) && single.t !== 'nav') {
      body.classList.add('bleed');
      const r = render(single, Object.assign({}, sub, { bleed: false, pageHeader: header.length ? header : null }));
      if (r.el.classList.contains('list') || r.el.classList.contains('scroll')) {
        const inner = r.el.classList.contains('scroll') ? r.el.firstChild : null;
        if (inner) inner.style.paddingBottom = 'var(--sab)';
      }
      r.el.style.flex = '1 1 0px';
      r.el.style.alignSelf = 'stretch';
      body.appendChild(r.el);
    } else {
      if (header.length) { body.classList.add('withtop'); header.forEach((h) => body.appendChild(h)); }
      const holder = header.length ? mk('div', 'centerwrap') : body;
      const r = renderGroup(content, sub);
      place(r, 'w');
      holder.appendChild(r.el);
      if (holder !== body) body.appendChild(holder);
      if (!body.classList.contains('withtop')) body.style.paddingBottom = 'var(--sab)';
      else holder.style.paddingBottom = 'var(--sab)';
    }
    el.appendChild(body);
    if (p.bottom && p.bottom.length) {
      el.classList.add('hasbottom');
      const bb = mk('div', 'bottombar');
      navItems(p.bottom, sub).forEach((x) => bb.appendChild(x));
      el.appendChild(bb);
    }
    return el;
  }

  function rTabView(n, ctx) {
    const p = n.p || {};
    const el = mk('div', 'n tabview');
    const items = p.items || [];
    if (p.style === 'page') {
      const pager = mk('div', 'pager');
      pager.dataset.sel = p.sel;
      pager.dataset.ids = JSON.stringify(items.map((x) => x.id));
      for (const c of n.c || []) { const r = render(c, Object.assign({}, ctx, { bleed: false })); pager.appendChild(r.el); }
      el.appendChild(pager);
      if (items.length > 1) {
        const dots = mk('div', 'pagedots');
        items.forEach((_, i) => dots.appendChild(mk('i', i === p.sel ? 'sel' : '')));
        el.appendChild(dots);
      }
      return { el, h: true, v: true };
    }
    const content = mk('div', 'tabcontent');
    const kids = n.c || [];
    const sub = Object.assign({}, ctx, { root: false });
    if (kids.length === 1 && isBleed(kids[0])) {
      const r = render(kids[0], Object.assign({}, sub, { bleed: true }));
      r.el.style.flex = '1 1 0px'; r.el.style.alignSelf = 'stretch';
      content.appendChild(r.el);
      content.style.alignItems = 'stretch';
    } else {
      content.style.paddingTop = 'var(--sat)';
      content.style.paddingBottom = 'var(--sab)';
      const holder = mk('div', 'n w');
      holder.style.flex = '1 1 0px'; holder.style.alignSelf = 'stretch';
      holder.style.setProperty('--sat', '0px'); holder.style.setProperty('--sab', '0px');
      const r = renderGroup(kids, sub);
      place(r, 'w');
      holder.appendChild(r.el);
      content.appendChild(holder);
    }
    el.appendChild(content);
    if (!st.hideTabBar) {
      const bar = mk('div', 'tabbar');
      bar.dataset.k = (n.id || '') + '/bar';
      for (const it of items) {
        const ti = mk('div', 'tabitem' + (it.sel ? ' sel' : ''));
        ti.dataset.tap = it.id;
        if (it.sys) {
          const ic = mk('div', 'sym');
          let name = it.sys;
          if (it.sel && !/\.fill$/.test(name)) name += '.fill';
          ic.innerHTML = window.SFSymbol(name).svg;
          ti.appendChild(ic);
        }
        const t = mk('div'); t.textContent = it.t || ''; ti.appendChild(t);
        if (it.badge) { const b = mk('div', 'tbadge'); b.textContent = it.badge; ti.appendChild(b); }
        bar.appendChild(ti);
      }
      el.appendChild(bar);
    } else {
      content.style.setProperty('--sab', 'var(--sab0)');
    }
    return { el, h: true, v: true };
  }

  // ----- camera and photo library -----
  // These widgets keep their own state (a live webcam stream, a captured photo),
  // so the DOM morph leaves them alone and they are only mounted once.
  function mediaHost(n) {
    const p = n.p || {};
    const host = mk('div', 'n media-host');
    host.dataset.k = n.id;
    host.dataset.selfmanaged = '1';
    host._mediaNode = { kind: n.t === 'camera' && p.source === 'camera' ? 'camera' : 'library', p };
    return host;
  }
  function mountMediaHosts(scr) {
    for (const host of scr.querySelectorAll('.media-host')) {
      if (host._mounted || !host._mediaNode) continue;
      host._mounted = true;
      const { kind, p } = host._mediaNode;
      const onPick = (photos) => send(p.pick, photos);
      const onCancel = () => send(p.cancel);
      if (kind === 'camera') {
        const onChooseFile = () => window.SwitaculousMedia.pickPhotos({ multiple: false, accept: 'image/*' }).then((photos) => { if (photos.length) onPick(photos); });
        window.SwitaculousMedia.mountCamera(host, { front: p.front, onPhoto: (photo) => onPick([photo]), onCancel, onChooseFile });
      } else {
        window.SwitaculousMedia.mountLibrary(host, { multiple: p.multi, accept: p.accept, onPick, onCancel });
      }
    }
  }

  // ----- overlays -----
  function renderOverlay(n) {
    const p = n.p || {};
    if (n.t === 'photopicker') {
      const wrap = mk('div', 'sheetwrap');
      wrap.dataset.k = n.id;
      wrap.dataset.enter = 'sheet';
      wrap.dataset.exit = 'sheet';
      const dim = mk('div', 'dim');
      dim.dataset.tap = p.cancel;
      const panel = mk('div', 'sheetpanel');
      const body = mk('div', 'sheetbody');
      const host = mediaHost(n);
      host.style.flex = '1 1 0px';
      body.appendChild(host);
      panel.appendChild(body);
      wrap.append(dim, panel);
      return wrap;
    }
    if (n.t === 'sheet') {
      const wrap = mk('div', 'sheetwrap' + (p.full ? ' full' : ''));
      wrap.dataset.k = n.id;
      wrap.dataset.enter = 'sheet';
      wrap.dataset.exit = 'sheet';
      const dim = mk('div', 'dim');
      if (p.dis) dim.dataset.tap = p.dis;
      wrap.appendChild(dim);
      const panel = mk('div', 'sheetpanel');
      const det = p.det || [];
      if (det.length) {
        const hasLarge = det.includes('large');
        const nums = det.filter((d) => typeof d === 'number');
        const frac = det.find((d) => d && d.frac);
        if (!hasLarge) {
          wrap.classList.add('medium');
          panel.style.top = 'auto';
          if (det.includes('medium')) panel.style.height = '50%';
          else if (nums.length) panel.style.height = Math.max(...nums) + 'px';
          else if (frac) panel.style.height = (frac.frac * 100) + '%';
        }
        if (det.length > 1) panel.appendChild(mk('div', 'grabber'));
      }
      if (p.bg) applyPaint(panel, p.bg);
      if (p.dis) panel.dataset.drag = p.dis;
      const body = mk('div', 'sheetbody');
      const kids = n.c || [];
      if (p.full) { body.style.setProperty('--sat', 'var(--sat0)'); }
      if (kids.length === 1 && isBleed(kids[0])) {
        const r = render(kids[0], { bleed: true });
        r.el.style.flex = '1 1 0px'; r.el.style.alignSelf = 'stretch';
        body.style.alignItems = 'stretch';
        body.appendChild(r.el);
      } else {
        body.style.paddingTop = p.full ? 'var(--sat)' : '0px';
        body.style.paddingBottom = 'var(--sab)';
        const holder = mk('div', 'n w');
        holder.style.flex = '1 1 0px'; holder.style.alignSelf = 'stretch';
        holder.style.setProperty('--sat', '0px'); holder.style.setProperty('--sab', '0px');
        const r = renderGroup(kids, {});
        place(r, 'w');
        holder.appendChild(r.el);
        body.appendChild(holder);
      }
      panel.appendChild(body);
      wrap.appendChild(panel);
      return wrap;
    }
    if (n.t === 'alert') {
      const sheet = p.style === 'sheet';
      const wrap = mk('div', 'alertwrap' + (sheet ? ' sheet' : ''));
      wrap.dataset.k = n.id;
      wrap.dataset.enter = 'alert';
      wrap.dataset.exit = 'alert';
      const dim = mk('div', 'dim');
      if (sheet && p.dis) dim.dataset.tap = p.dis;
      wrap.appendChild(dim);
      const card = mk('div', 'alert');
      let btns = (p.btns || []).filter((b) => !b.tf);
      const tfs = (p.btns || []).filter((b) => b.tf);
      const text = mk('div', 'atext');
      if (p.title) { const t = mk('div', 'atitle'); t.textContent = p.title; text.appendChild(t); }
      if (p.msg) { const m = mk('div', 'amsg'); m.textContent = p.msg; text.appendChild(m); }
      for (const tf of tfs) { const r = render(tf.tf, {}); r.el.classList.add('atf'); text.appendChild(r.el); }
      const mkBtns = (list) => {
        const box = mk('div', 'abtns');
        list.forEach((b, i) => {
          const d = mk('div', (b.role || '') + (!sheet && list.length <= 2 && i === list.length - 1 && b.role !== 'cancel' && b.role !== 'destructive' ? ' primary' : ''));
          d.textContent = b.t; d.dataset.tap = b.id;
          box.appendChild(d);
        });
        return box;
      };
      if (sheet) {
        const cancel = btns.filter((b) => b.role === 'cancel');
        const rest = btns.filter((b) => b.role !== 'cancel');
        const g = mk('div', 'agroup');
        if (p.title || p.msg) g.appendChild(text);
        g.appendChild(mkBtns(rest));
        card.appendChild(g);
        if (cancel.length) { const g2 = mk('div', 'agroup cancelg'); g2.appendChild(mkBtns(cancel)); card.appendChild(g2); }
      } else {
        if (btns.length === 2) { const c = btns.findIndex((b) => b.role === 'cancel'); if (c === 1) btns = [btns[1], btns[0]]; }
        else if (btns.length > 2) { const c = btns.filter((b) => b.role === 'cancel'); btns = btns.filter((b) => b.role !== 'cancel').concat(c); }
        card.appendChild(text);
        card.appendChild(mkBtns(btns));
      }
      wrap.appendChild(card);
      return wrap;
    }
    return mk('div');
  }

  // ---------- modifiers ----------
  const textProps = new Set(['font', 'fw', 'italic', 'fdesign', 'tabnum', 'fg', 'underline', 'strikethrough', 'kern', 'textcase']);
  function textStyle(el, m) {
    const a = m.a || {};
    switch (m.n) {
      case 'font': fontStyle(el, a.f, true); break;
      case 'fw': el.style.fontWeight = weight(a.w); break;
      case 'italic': el.style.fontStyle = 'italic'; break;
      case 'fdesign': el.classList.add('d-' + a.d); break;
      case 'tabnum': el.style.fontVariantNumeric = 'tabular-nums'; break;
      case 'fg': if (typeof a.c === 'string') el.style.color = a.c; break;
      case 'underline': el.style.textDecoration = 'underline'; break;
      case 'strikethrough': el.style.textDecoration = 'line-through'; break;
      case 'kern': el.style.letterSpacing = a.v + 'px'; break;
      case 'textcase': el.style.textTransform = a.v === 'uppercase' ? 'uppercase' : a.v === 'lowercase' ? 'lowercase' : 'none'; break;
    }
  }
  const weights = { ultraLight: 100, thin: 200, light: 300, regular: 400, medium: 500, semibold: 600, bold: 700, heavy: 800, black: 900 };
  function weight(w) { return weights[w] || 400; }
  function fontStyle(el, f, force) {
    if (!f) return;
    if (!force && el.dataset.font) return;
    el.dataset.font = '1';
    for (const c of [...el.classList]) if (c.startsWith('f-') || c.startsWith('d-')) el.classList.remove(c);
    if (f.st) el.classList.add('f-' + f.st);
    else if (f.sz) { el.style.fontSize = f.sz + 'px'; el.style.lineHeight = Math.round(f.sz * 1.2) + 'px'; el.style.letterSpacing = '0'; }
    if (f.w) el.style.fontWeight = weight(f.w);
    else if (f.st && f.st !== 'headline') el.style.fontWeight = '';
    if (f.d && f.d !== 'default') el.classList.add('d-' + f.d);
    if (f.mono) el.classList.add('d-monospaced');
    if (f.i) el.style.fontStyle = 'italic';
    if (f.fam) el.style.fontFamily = `"${f.fam}", system-ui, "Segoe UI", system-ui, sans-serif`;
  }

  function needsWrap(r) { return r.transformed || r.hasLayer || r.clip; }
  function wrapper(r, cls) {
    const w = mk('div', 'n w' + (cls ? ' ' + cls : ''));
    place(r, 'w');
    w.appendChild(r.el);
    return { el: w, h: r.h, v: r.v, noShrink: r.noShrink, kind: 'wrap' };
  }
  function addTransform(r, t, origin) {
    const s = r.el.style;
    s.transform = (t + ' ' + (s.transform || '')).trim();
    if (origin) s.transformOrigin = origin;
    r.transformed = true;
  }
  function addFilter(r, f) { r.el.style.filter = ((r.el.style.filter || '') + ' ' + f).trim(); r.transformed = true; }

  function layerFor(r, nodes, paint, cls, al, shape) {
    const lay = mk('div', 'layer ' + cls);
    const [jx, jy] = alignXY(al);
    lay.style.alignItems = jx;
    lay.style.justifyContent = jy;
    if (paint != null) {
      applyPaint(lay, paint);
      if (cls === 'bglayer' && !shape) lay.dataset.bgx = '1';
    }
    if (shape) clipShape(lay, shape);
    if (nodes && nodes.length) {
      for (const n of nodes) {
        const c = render(n, { axis: 'w' });
        c.el.style.flex = c.v ? '1 1 0px' : '0 0 auto';
        if (c.h) c.el.style.alignSelf = 'stretch';
        lay.appendChild(c.el);
      }
    }
    return lay;
  }
  function clipShape(el, shape) {
    const k = shape.k;
    if (k === 'circle') el.style.clipPath = 'circle(closest-side at 50% 50%)';
    else if (k === 'capsule') { el.style.borderRadius = '9999px'; el.style.overflow = 'hidden'; }
    else if (k === 'rrect') { el.style.borderRadius = (shape.r || 0) + 'px'; el.style.overflow = 'hidden'; }
    else if (k === 'ellipse') { el.style.borderRadius = '50%'; el.style.overflow = 'hidden'; }
    else if (k === 'uneven') { el.style.borderRadius = `${shape.topLeadingRadius || 0}px ${shape.topTrailingRadius || 0}px ${shape.bottomTrailingRadius || 0}px ${shape.bottomLeadingRadius || 0}px`; el.style.overflow = 'hidden'; }
    else el.style.overflow = 'hidden';
  }

  function applyMods(r, n, ctx) {
    for (const m of n.m) r = applyMod(r, m, n, ctx);
    return r;
  }

  function applyMod(r, m, n, ctx) {
    const a = m.a || {};
    const s = () => r.el.style;
    switch (m.n) {
      case 'pad': {
        r = wrapper(r);
        const pv = (x) => Math.max(0, x || 0) + 'px', mv = (x) => Math.min(0, x || 0) + 'px';
        r.el.style.padding = `${pv(a.t)} ${pv(a.r)} ${pv(a.b)} ${pv(a.l)}`;
        if ((a.t || 0) < 0 || (a.r || 0) < 0 || (a.b || 0) < 0 || (a.l || 0) < 0) r.el.style.margin = `${mv(a.t)} ${mv(a.r)} ${mv(a.b)} ${mv(a.l)}`;
        return r;
      }
      case 'frame': {
        const w = mk('div', 'n frame');
        const [jx, jy] = alignXY(a.al);
        w.style.alignItems = jx;
        w.style.justifyContent = jy;
        let h = r.h, v = r.v;
        const num = (x) => typeof x === 'number';
        if (num(a.w)) { w.style.width = a.w + 'px'; w.style.flexShrink = '0'; h = false; }
        if (num(a.h)) { w.style.height = a.h + 'px'; v = false; }
        if (num(a.minW)) { w.style.minWidth = a.minW + 'px'; }
        if (num(a.minH)) { w.style.minHeight = a.minH + 'px'; }
        if (a.maxW === 'inf') h = true;
        else if (num(a.maxW)) { w.style.maxWidth = a.maxW + 'px'; h = true; }
        if (a.maxH === 'inf') v = true;
        else if (num(a.maxH)) { w.style.maxHeight = a.maxH + 'px'; v = true; }
        if (num(a.iw) && !num(a.w)) w.style.width = w.style.width || '';
        if (!num(a.w) && !h && num(a.minW)) h = false;
        r.el.style.flex = r.v ? '1 1 0px' : '0 0 auto';
        if (r.h) r.el.style.alignSelf = 'stretch';
        if (num(a.w) || num(a.maxW)) r.el.style.maxWidth = r.h ? '' : '100%';
        w.appendChild(r.el);
        const out = { el: w, h, v, noShrink: num(a.w) || r.noShrink };
        if (num(a.w) || num(a.h)) out.fixed = true;
        return out;
      }
      case 'bg': case 'ov': case 'mask': {
        if (m.n === 'mask') {
          if (a.node && a.node.length === 1 && a.node[0].t === 'shape') { clipShape(r.el, a.node[0].p || {}); r.clip = true; }
          return r;
        }
        if ((m.n === 'bg' && r.bg) || needsWrap(r) || r.img) r = wrapper(r);
        const lay = layerFor(r, a.node, a.paint, m.n === 'bg' ? 'bglayer' : 'ovlayer', a.al, a.shape);
        if (m.n === 'bg') { r.el.prepend(lay); r.bg = true; } else r.el.appendChild(lay);
        r.el.classList.add('iso');
        r.hasLayer = true;
        return r;
      }
      case 'fg': {
        if (r.kind === 'shape') return r;
        if (!r.el.dataset.fg) {
          r.el.dataset.fg = '1';
          if (a.c && typeof a.c === 'object' && a.c.g) {
            if (r.kind === 'text') { r.el.style.background = gradientCSS(a.c); r.el.style.webkitBackgroundClip = 'text'; r.el.style.backgroundClip = 'text'; r.el.style.color = 'transparent'; }
            else r.el.style.color = paintColor(a.c);
          } else if (a.c != null) { r.el.style.color = paintColor(a.c); r.el.style.setProperty('--btn-fg', paintColor(a.c)); }
        }
        return r;
      }
      case 'tint': {
        if (!r.el.dataset.tint && a.c) {
          r.el.dataset.tint = '1';
          const c = paintColor(a.c);
          r.el.style.setProperty('--tint', c);
          r.el.style.setProperty('--toggle-tint', c);
        }
        return r;
      }
      case 'font': fontStyle(r.el, a.f, false); return r;
      case 'fw': if (!r.el.style.fontWeight) r.el.style.fontWeight = weight(a.w); return r;
      case 'italic': r.el.style.fontStyle = 'italic'; return r;
      case 'fdesign': r.el.classList.add('d-' + a.d); return r;
      case 'tabnum': r.el.style.fontVariantNumeric = 'tabular-nums'; return r;
      case 'underline': case 'strikethrough': {
        const cur = r.el.style.textDecorationLine || '';
        r.el.style.textDecorationLine = (cur + ' ' + (m.n === 'underline' ? 'underline' : 'line-through')).trim();
        if (a.c) r.el.style.textDecorationColor = paintColor(a.c);
        return r;
      }
      case 'kern': r.el.style.letterSpacing = a.v + 'px'; return r;
      case 'textcase': if (!r.el.style.textTransform) r.el.style.textTransform = a.v === 'uppercase' ? 'uppercase' : a.v === 'lowercase' ? 'lowercase' : 'none'; return r;
      case 'linelimit': {
        if (a.n == null) { r.el.style.setProperty('--lines', 'none'); return r; }
        r.el.style.setProperty('--lines', a.n);
        r.el.classList.add('clamp');
        return r;
      }
      case 'linespacing': r.el.style.lineHeight = `calc(1.3em + ${a.v}px)`; return r;
      case 'talign': r.el.style.textAlign = a.al === 'center' ? 'center' : a.al === 'trailing' ? 'right' : 'left'; return r;
      case 'radius': {
        if (r.transformed) r = wrapper(r);
        r.el.style.borderRadius = a.r + 'px'; r.el.style.overflow = 'hidden'; r.clip = true;
        return r;
      }
      case 'clip': {
        if (r.transformed) r = wrapper(r);
        clipShape(r.el, a.shape || {}); r.clip = true;
        return r;
      }
      case 'clipped': r.el.style.overflow = 'hidden'; r.clip = true; return r;
      case 'shadow': addFilter(r, `drop-shadow(${a.x || 0}px ${a.y || 0}px ${a.r || 0}px ${paintColor(a.c) || 'rgba(0,0,0,.33)'})`); return r;
      case 'opacity': {
        const cur = parseFloat(r.el.style.opacity || '1');
        r.el.style.opacity = String(cur * a.v);
        return r;
      }
      case 'scale': addTransform(r, `scale(${a.x}, ${a.y})`, a.anchor ? `${a.anchor[0] * 100}% ${a.anchor[1] * 100}%` : ''); return r;
      case 'rotate': addTransform(r, `rotate(${a.deg}deg)`, a.anchor ? `${a.anchor[0] * 100}% ${a.anchor[1] * 100}%` : ''); return r;
      case 'rotate3d': {
        const ax = a.ax || [0, 1, 0];
        addTransform(r, `perspective(800px) rotate3d(${ax[0]}, ${ax[1]}, ${ax[2]}, ${a.deg}deg)`);
        return r;
      }
      case 'offset': addTransform(r, `translate(${a.x}px, ${a.y}px)`); return r;
      case 'position': {
        const w = mk('div', 'n');
        r.el.style.position = 'absolute';
        r.el.style.left = a.x + 'px';
        r.el.style.top = a.y + 'px';
        r.el.style.transform = ((r.el.style.transform || '') + ' translate(-50%, -50%)').trim();
        if (r.h && !r.fixed) r.el.style.width = '100%';
        w.appendChild(r.el);
        return { el: w, h: true, v: true };
      }
      case 'blur': addFilter(r, `blur(${a.r}px)`); return r;
      case 'brightness': addFilter(r, `brightness(${1 + a.v})`); return r;
      case 'contrast': addFilter(r, `contrast(${a.v})`); return r;
      case 'saturation': addFilter(r, `saturate(${a.v})`); return r;
      case 'grayscale': addFilter(r, `grayscale(${a.v})`); return r;
      case 'hueRotation': addFilter(r, `hue-rotate(${a.v}deg)`); return r;
      case 'colorInvert': addFilter(r, 'invert(1)'); return r;
      case 'colorMultiply': return r;
      case 'blendMode': r.el.style.mixBlendMode = { multiply: 'multiply', screen: 'screen', overlay: 'overlay', darken: 'darken', lighten: 'lighten', difference: 'difference', plusLighter: 'plus-lighter', colorBurn: 'color-burn', colorDodge: 'color-dodge', softLight: 'soft-light', hardLight: 'hard-light', exclusion: 'exclusion', hue: 'hue', saturation: 'saturation', color: 'color', luminosity: 'luminosity' }[a.v] || 'normal'; return r;
      case 'border': {
        if (r.el.style.boxShadow || r.transformed) r = wrapper(r);
        r.el.style.boxShadow = `inset 0 0 0 ${a.w}px ${paintColor(a.c)}`;
        return r;
      }
      case 'zindex': r.el.style.zIndex = String(Math.round(a.v)); return r;
      case 'priority': if (a.v > 0) { r.prio = true; r.noShrink = true; } return r;
      case 'fixed': {
        if (a.h) { r.el.style.whiteSpace = 'nowrap'; r.el.style.flexShrink = '0'; r.noShrink = true; r.h = false; r.el.style.maxWidth = 'none'; }
        if (a.v) r.v = false;
        return r;
      }
      case 'aspect': {
        if (a.r) {
          r.el.style.aspectRatio = String(a.r);
          if (r.h) { r.v = false; r.el.style.width = '100%'; }
          else if (r.v) { r.h = false; r.el.style.height = '100%'; }
        }
        return r;
      }
      case 'isa': r.el.dataset.isa = (a.t ? 't' : '') + (a.b ? 'b' : ''); return r;
      case 'hidden': r.el.style.visibility = 'hidden'; return r;
      case 'hittest': if (!a.v) r.el.style.pointerEvents = 'none'; return r;
      case 'disabled': r.el.style.pointerEvents = 'none'; r.el.dataset.disabled = '1'; return r;
      case 'tap': {
        if (r.el.dataset.tap || r.el.dataset.dbl) r = wrapper(r);
        if (a.count === 2) r.el.dataset.dbl = a.id; else r.el.dataset.tap = a.id;
        r.el.classList.add('tapable');
        return r;
      }
      case 'longpress': r.el.dataset.lp = a.id; return r;
      case 'anim': r.el.style.transition = `all ${a.dur || 0.35}s ${a.ease === 'linear' ? 'linear' : 'cubic-bezier(.3,.7,.3,1)'}`; return r;
      case 'transition': r.el.dataset.tr = a.k; r.el.dataset.exit = 'view'; return r;
      case 'scheme': r.el.classList.add(a.v === 'dark' ? 'dark' : 'light'); return r;
      case 'imgscale': r.el.style.setProperty('--imgscale', a.s === 'small' ? '0.78' : a.s === 'large' ? '1.32' : '1'); r.el.classList.add('imgscaled'); return r;
      case 'sai': {
        const w = mk('div', 'n');
        w.style.display = 'flex'; w.style.flexDirection = (a.edge === 'leading' || a.edge === 'trailing') ? 'row' : 'column';
        r.el.style.flex = '1 1 0px'; r.el.style.alignSelf = 'stretch'; r.el.style.minHeight = '0';
        const inset = renderGroup(a.node || [], ctx);
        inset.el.style.flex = '0 0 auto';
        if (inset.h || a.edge === 'bottom' || a.edge === 'top') inset.el.style.alignSelf = 'stretch';
        if (a.edge === 'top' || a.edge === 'leading') w.append(inset.el, r.el); else w.append(r.el, inset.el);
        return { el: w, h: true, v: true };
      }
      case 'rowbg': case 'rowinsets': case 'rowsep': case 'scrollbg': case 'scrollind': case 'badge':
        return r;
    }
    return r;
  }

  // ---------- morphing ----------
  function morph(a, b) {
    if (a.nodeType !== b.nodeType || a.nodeName !== b.nodeName) { a.replaceWith(b); enter(b); return; }
    if (a.nodeType === 3) { if (a.nodeValue !== b.nodeValue) a.nodeValue = b.nodeValue; return; }
    if (a.nodeType !== 1) return;
    if (a.dataset.k !== b.dataset.k) { a.replaceWith(b); enter(b); return; }
    if (a.dataset.selfmanaged) return;
    syncAttrs(a, b);
    if (a.tagName === 'INPUT' || a.tagName === 'TEXTAREA') { syncValue(a, b); return; }
    if (a.classList.contains('shape')) { a._shapeKeep = true; return; }
    morphChildren(a, b);
  }
  function syncAttrs(a, b) {
    const keepCls = [];
    if (a.classList.contains('page') && a.classList.contains('scrolled')) keepCls.push('scrolled');
    for (const at of [...a.attributes]) if (!b.hasAttribute(at.name)) a.removeAttribute(at.name);
    for (const at of [...b.attributes]) if (a.getAttribute(at.name) !== at.value) a.setAttribute(at.name, at.value);
    for (const c of keepCls) a.classList.add(c);
  }
  function syncValue(a, b) {
    const nv = b.value;
    if (a.type === 'range') { if (document.activeElement !== a && a.value !== nv) a.value = nv; return; }
    if (a.value === nv) return;
    if (document.activeElement === a) {
      if (a._lastSent !== undefined && a._lastSent !== a.value) return;
      if (a._lastSent === nv) return;
    }
    a.value = nv;
  }
  function liveKids(el) { return [...el.childNodes].filter((c) => !(c.nodeType === 1 && c.classList.contains('exiting'))); }
  function morphChildren(a, b) {
    const oldKids = liveKids(a);
    const newKids = [...b.childNodes];
    const keyed = new Map();
    for (const c of oldKids) if (c.nodeType === 1 && c.dataset.k) keyed.set(c.dataset.k, c);
    const used = new Set();
    let prev = null;
    let oi = 0;
    for (const nk of newKids) {
      let match = null;
      const k = nk.nodeType === 1 ? nk.dataset.k : null;
      if (k) {
        const cand = keyed.get(k);
        if (cand && !used.has(cand) && cand.nodeName === nk.nodeName) match = cand;
      } else {
        while (oi < oldKids.length && (used.has(oldKids[oi]) || (oldKids[oi].nodeType === 1 && oldKids[oi].dataset.k))) oi++;
        const cand = oldKids[oi];
        if (cand && cand.nodeName === nk.nodeName) { match = cand; oi++; }
      }
      const next = prev ? prev.nextSibling : a.firstChild;
      if (match) {
        used.add(match);
        if (match !== next) a.insertBefore(match, next);
        morph(match, nk);
        prev = match;
      } else {
        a.insertBefore(nk, next);
        enter(nk);
        prev = nk;
      }
    }
    for (const c of oldKids) if (!used.has(c)) exit(c);
  }
  function enter(el) {
    if (el.nodeType !== 1 || !st.animateEnter) return;
    const kind = el.dataset.enter;
    if (kind) {
      el.classList.add('enter-' + kind);
      el.getBoundingClientRect();
      requestAnimationFrame(() => requestAnimationFrame(() => el.classList.remove('enter-' + kind)));
    }
    const tr = el.dataset.tr;
    if (tr) {
      const cls = 't-enter-' + (tr === 'identity' ? 'opacity' : tr);
      el.classList.add(cls);
      el.style.transition = el.style.transition || 'opacity .3s, transform .35s';
      el.getBoundingClientRect();
      requestAnimationFrame(() => requestAnimationFrame(() => el.classList.remove(cls)));
    }
  }
  function exit(el) {
    if (el.nodeType === 1 && el.dataset.exit && st.animateEnter) {
      el.classList.add('exiting');
      if (el.dataset.exit === 'view') { el.classList.add('exiting-view'); el.style.opacity = '0'; if (el.dataset.tr && el.dataset.tr.startsWith('move')) el.classList.add('t-enter-' + el.dataset.tr); }
      setTimeout(() => el.remove(), 420);
      return;
    }
    el.remove();
  }

  // ---------- post layout ----------
  function postLayout(p) {
    const scr = $('#screen');
    if (!scr) return;
    for (const el of scr.querySelectorAll('.shape')) drawShape(el);
    mountMediaHosts(scr);
    // Like on a real phone, a background that touches the screen edge also fills the status bar and home indicator area.
    const dev = st.device;
    const sr = scr.getBoundingClientRect();
    const sc = sr.width / dev.w || 1;
    const touch = (el) => {
      const r = el.getBoundingClientRect();
      const top = (r.top - sr.top) / sc, bottom = (r.bottom - sr.top) / sc;
      return { t: Math.abs(top - dev.top) < 1.5, b: Math.abs(bottom - (dev.h - dev.bottom)) < 1.5, top, bottom };
    };
    for (const el of scr.querySelectorAll('[data-isa]')) {
      el.style.marginTop = ''; el.style.marginBottom = '';
      const t = touch(el);
      const ed = el.dataset.isa;
      const leaf = el.classList.contains('color') || el.classList.contains('gradient') || el.classList.contains('imgbox') || el.classList.contains('shape');
      if (ed.includes('t') && t.t) { el.style.marginTop = (-dev.top) + 'px'; if (!leaf) el.style.paddingTop = dev.top + 'px'; }
      if (ed.includes('b') && t.b) { el.style.marginBottom = (-dev.bottom) + 'px'; if (!leaf) el.style.paddingBottom = dev.bottom + 'px'; }
    }
    for (const lay of scr.querySelectorAll('[data-bgx]')) {
      const host = lay.parentElement;
      if (!host) continue;
      const t = touch(host);
      lay.style.top = t.t ? (-dev.top) + 'px' : '0';
      lay.style.bottom = t.b ? (-dev.bottom) + 'px' : '0';
    }
    for (const el of scr.querySelectorAll('.shape')) drawShape(el);
    const geos = [];
    for (const el of scr.querySelectorAll('[data-gid]')) {
      const w = el.offsetWidth, h = el.offsetHeight;
      const r = el.getBoundingClientRect();
      const x = (r.left - sr.left) / sc, y = (r.top - sr.top) / sc;
      const id = el.dataset.gid;
      const c = st.geoCache[id];
      if (!c || Math.abs(c.w - w) > 0.5 || Math.abs(c.h - h) > 0.5) { st.geoCache[id] = { w, h }; geos.push({ id, w, h, x, y }); }
    }
    if (geos.length) fetch('/api/geo', { method: 'POST', body: JSON.stringify(geos) });
    const af = scr.querySelector('[data-autofocus]');
    if (af && document.activeElement !== af) af.focus();
    for (const ta of scr.querySelectorAll('textarea')) autoGrow(ta);
    for (const pg of scr.querySelectorAll('.pager')) {
      const sel = parseInt(pg.dataset.sel || '0', 10);
      if (!pg._userScroll && pg.clientWidth) {
        const target = sel * pg.clientWidth;
        if (Math.abs(pg.scrollLeft - target) > 2) pg.scrollTo({ left: target, behavior: st.firstPager ? 'smooth' : 'auto' });
      }
      st.firstPager = true;
    }
    if (p && p.scrollTo) {
      const t = scr.querySelector(`[data-sid="${CSS.escape(p.scrollTo)}"]`);
      if (t) t.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
    if (!st.ro) {
      st.ro = new ResizeObserver(() => { const s = $('#screen'); if (s) for (const el of s.querySelectorAll('.shape')) drawShape(el); reportGeo(); });
    }
    for (const el of scr.querySelectorAll('.shape, [data-gid]')) if (!el._ro) { el._ro = true; st.ro.observe(el); }
  }
  let geoT = null;
  function reportGeo() {
    clearTimeout(geoT);
    geoT = setTimeout(() => postLayout(null), 60);
  }
  function autoGrow(ta) {
    if (ta.closest('.editor')) return;
    ta.style.height = 'auto';
    ta.style.height = ta.scrollHeight + 'px';
  }

  // ---------- interaction ----------
  function bindScreen(scr) {
    scr.addEventListener('click', (e) => {
      const target = e.target;
      if (st.openMenu) {
        const item = target.closest('.menupop .mi');
        if (item) {
          if (item.dataset.sub) { openSubmenu(item.dataset.sub); return; }
          if (item.dataset.tap) send(item.dataset.tap, item.dataset.val === undefined ? null : item.dataset.val);
          closeMenu();
          return;
        }
        if (target.closest('.menupop')) return;
        closeMenu();
        return;
      }
      const sw = target.closest('.row[data-swipe]');
      if (sw && sw._open && !target.closest('.swipe')) { closeSwipe(sw); return; }
      const urlEl = target.closest('[data-url]');
      if (urlEl && scr.contains(urlEl)) { window.open(urlEl.dataset.url, '_blank', 'noopener'); return; }
      const sidebarToggle = target.closest('[data-sidebartoggle]');
      if (sidebarToggle && scr.contains(sidebarToggle)) {
        st.sidebarOpen = !st.sidebarOpen;
        for (const split of scr.querySelectorAll('.split.portrait')) split.classList.toggle('open', st.sidebarOpen);
        return;
      }
      const menuEl = target.closest('[data-menu]');
      if (menuEl && scr.contains(menuEl)) { openMenuAt(menuEl, st.menuDefs[menuEl.dataset.menu]); return; }
      const photosButton = target.closest('[data-photos]');
      if (photosButton && scr.contains(photosButton)) {
        const pickedFor = photosButton.dataset.photos;
        window.SwitaculousMedia.pickPhotos({ multiple: photosButton.dataset.multi === '1', accept: photosButton.dataset.accept })
          .then((photos) => { if (photos.length) send(pickedFor, photos); });
        return;
      }
      const t = target.closest('[data-tap]');
      if (!t || !scr.contains(t)) return;
      if (t.closest('[data-disabled]')) return;
      if (target.closest('input,textarea') && t.contains(target.closest('input,textarea'))) return;
      const sw2 = t.closest('.row[data-swipe]');
      if (sw2 && sw2._open && t.closest('.swipe')) closeSwipe(sw2);
      send(t.dataset.tap, t.dataset.val === undefined ? null : t.dataset.val);
    });
    scr.addEventListener('dblclick', (e) => {
      const t = e.target.closest('[data-dbl]');
      if (t) send(t.dataset.dbl);
    });
    scr.addEventListener('input', (e) => {
      const el = e.target;
      if (el.dataset.input) { el._lastSent = el.value; send(el.dataset.input, el.value); if (el.tagName === 'TEXTAREA') autoGrow(el); }
      if (el.dataset.slide) {
        const v = parseFloat(el.value);
        const pct = el.max > el.min ? ((v - el.min) / (el.max - el.min)) * 100 : 0;
        el.style.setProperty('--pct', pct + '%');
        send(el.dataset.slide, v);
      }
    });
    scr.addEventListener('change', (e) => {
      const el = e.target;
      if (el.dataset.change) send(el.dataset.change, el.value);
    });
    scr.addEventListener('keydown', (e) => {
      const el = e.target;
      if (e.key === 'Enter' && el.dataset.sub && !(el.tagName === 'TEXTAREA' && e.shiftKey)) {
        if (el.tagName === 'TEXTAREA' && !e.metaKey && !e.ctrlKey) return;
        e.preventDefault();
        send(el.dataset.sub);
      }
      if (e.key === 'Escape' && st.openMenu) closeMenu();
    });
    scr.addEventListener('focusin', (e) => { if (e.target.dataset && e.target.dataset.fid) send(e.target.dataset.fid, true); });
    scr.addEventListener('focusout', (e) => { if (e.target.dataset && e.target.dataset.fid) send(e.target.dataset.fid, false); });
    scr.addEventListener('scroll', (e) => {
      const t = e.target;
      if (!(t instanceof Element)) return;
      const page = t.closest('.page');
      if (page && (t.classList.contains('list') || t.classList.contains('scroll'))) page.classList.toggle('scrolled', t.scrollTop > 36);
      if (t.classList.contains('pager')) {
        t._userScroll = true;
        clearTimeout(t._st);
        t._st = setTimeout(() => {
          t._userScroll = false;
          const idx = Math.round(t.scrollLeft / Math.max(1, t.clientWidth));
          const ids = JSON.parse(t.dataset.ids || '[]');
          if (ids[idx] && String(idx) !== t.dataset.sel) send(ids[idx]);
        }, 140);
      }
    }, true);
    scr.addEventListener('contextmenu', (e) => {
      const t = e.target.closest('[data-ctx]');
      if (t) { e.preventDefault(); openMenuAt(t, st.menuDefs[t.dataset.ctx], true); }
    });
    let lpTimer = null, drag = null;
    scr.addEventListener('pointerdown', (e) => {
      const lp = e.target.closest('[data-lp]');
      if (lp) { lpTimer = setTimeout(() => { send(lp.dataset.lp); lpTimer = null; }, 500); }
      const row = e.target.closest('.row[data-swipe]');
      if (row && !e.target.closest('input,textarea,.swipe')) drag = { kind: 'swipe', row, x: e.clientX, y: e.clientY, dx: 0, moved: false };
      const sheet = e.target.closest('.sheetpanel[data-drag]');
      if (sheet && !drag) {
        const r = sheet.getBoundingClientRect();
        if ((e.clientY - r.top) / st.scale < 60 && !e.target.closest('input,textarea,[data-tap]')) drag = { kind: 'sheet', el: sheet, y: e.clientY, dy: 0 };
      }
    });
    window.addEventListener('pointermove', (e) => {
      if (lpTimer && (Math.abs(e.movementX) > 3 || Math.abs(e.movementY) > 3)) { clearTimeout(lpTimer); lpTimer = null; }
      if (!drag) return;
      if (drag.kind === 'swipe') {
        drag.dx = (e.clientX - drag.x) / st.scale;
        const dy = (e.clientY - drag.y) / st.scale;
        if (!drag.moved && Math.abs(drag.dx) > 8 && Math.abs(drag.dx) > Math.abs(dy)) drag.moved = true;
        if (drag.moved) {
          drag.row.classList.add('swiping');
          const sw = drag.row.querySelector('.swipe');
          const maxw = sw ? sw.offsetWidth : 80;
          const left = drag.row.dataset.swipe === 'l';
          let tx = (drag.row._open ? (left ? maxw : -maxw) : 0) + drag.dx;
          tx = left ? Math.max(0, Math.min(maxw * 1.2, tx)) : Math.min(0, Math.max(-maxw * 1.2, tx));
          const c = drag.row.querySelector('.rowc');
          c.style.transition = 'none';
          c.style.transform = `translateX(${tx}px)`;
        }
      } else if (drag.kind === 'sheet') {
        drag.dy = Math.max(0, (e.clientY - drag.y) / st.scale);
        drag.el.style.transition = 'none';
        drag.el.style.transform = `translateY(${drag.dy}px)`;
      }
    });
    window.addEventListener('pointerup', () => {
      if (lpTimer) { clearTimeout(lpTimer); lpTimer = null; }
      if (!drag) return;
      if (drag.kind === 'swipe' && drag.moved) {
        const c = drag.row.querySelector('.rowc');
        const sw = drag.row.querySelector('.swipe');
        const maxw = sw ? sw.offsetWidth : 80;
        const left = drag.row.dataset.swipe === 'l';
        c.style.transition = '';
        const base = drag.row._open ? (left ? maxw : -maxw) : 0;
        const tx = base + drag.dx;
        drag.row.classList.remove('swiping');
        if ((!left && tx < -maxw / 2) || (left && tx > maxw / 2)) { c.style.transform = `translateX(${left ? maxw : -maxw}px)`; drag.row._open = true; drag.row.classList.add('open'); }
        else closeSwipe(drag.row);
        const row = drag.row;
        setTimeout(() => { row._justDragged = false; }, 50);
        row._justDragged = true;
      } else if (drag.kind === 'sheet') {
        drag.el.style.transition = '';
        if (drag.dy > 110) send(drag.el.dataset.drag);
        else drag.el.style.transform = '';
      }
      drag = null;
    });
  }
  function closeSwipe(row) { const c = row.querySelector('.rowc'); if (c) { c.style.transition = ''; c.style.transform = ''; } row._open = false; setTimeout(() => row.classList.remove('open', 'swiping'), 260); }

  // ---------- menus ----------
  function menuRowsFor(def) {
    const rows = [];
    if (!def) return rows;
    if (def.kind === 'picker') {
      for (const o of def.opts || []) rows.push({ t: o.t, sys: o.sys, check: o.k === def.sel, tap: def.id, val: o.k, showCheck: true });
      return rows;
    }
    const walk = (nodes, depth) => {
      for (const n of nodes || []) {
        const p = n.p || {};
        switch (n.t) {
          case 'button': case 'navlink': case 'tap':
            rows.push({ t: plainText(n.c), sys: firstSys(n.c), tap: p.id, role: p.role, dis: p.dis });
            break;
          case 'divider': rows.push({ sep: true }); break;
          case 'section':
            if (rows.length && !rows[rows.length - 1].sep) rows.push({ bigsep: true });
            if (p.h && p.h.length) rows.push({ head: plainText(p.h) });
            walk(n.c, depth);
            rows.push({ bigsep: true });
            break;
          case 'menu': {
            const sid = (n.id || 'sub') + '#sub';
            st.menuDefs[sid] = { kind: 'items', items: p.items || [] };
            rows.push({ t: plainText(n.c), sys: firstSys(n.c), sub: sid });
            break;
          }
          case 'picker':
            if (p.label && p.label.length && (p.style === 'menu')) {
              const sid = (n.id || 'pk') + '#sub';
              st.menuDefs[sid] = { kind: 'picker', opts: p.opts, sel: p.sel, id: p.id };
              const sel = (p.opts || []).find((o) => o.k === p.sel);
              rows.push({ t: plainText(p.label), detail: sel ? sel.t : '', sub: sid });
            } else {
              for (const o of p.opts || []) rows.push({ t: o.t, sys: o.sys, check: o.k === p.sel, tap: p.id, val: o.k, showCheck: true });
            }
            break;
          case 'toggle':
            rows.push({ t: plainText(n.c), check: !!p.on, tap: p.id, showCheck: true });
            break;
          case 'text': case 'label':
            rows.push({ t: plainText([n]), dis: true });
            break;
          default:
            walk(n.c, depth);
        }
      }
    };
    walk(def.items, 0);
    while (rows.length && (rows[rows.length - 1].bigsep || rows[rows.length - 1].sep)) rows.pop();
    while (rows.length && (rows[0].bigsep || rows[0].sep)) rows.shift();
    return rows;
  }
  function buildMenu(def) {
    const pop = mk('div', 'menupop');
    const rows = menuRowsFor(def);
    const anyCheck = rows.some((r) => r.showCheck);
    for (const r of rows) {
      if (r.sep) { pop.appendChild(mk('div', 'msep')); continue; }
      if (r.bigsep) { pop.appendChild(mk('div', 'mbigsep')); continue; }
      if (r.head) { const h = mk('div', 'mhead'); h.textContent = r.head; pop.appendChild(h); continue; }
      const it = mk('div', 'mi' + (r.role === 'destructive' ? ' destructive' : '') + (r.dis ? ' dis' : ''));
      if (anyCheck) { const ck = mk('div', 'chk'); if (r.check) ck.innerHTML = window.SFSymbol('checkmark').svg; it.appendChild(ck); }
      const t = mk('div', 'mt'); t.textContent = r.t; it.appendChild(t);
      if (r.detail) { const d = mk('div'); d.style.color = 'var(--c-secondary)'; d.textContent = r.detail; it.appendChild(d); }
      if (r.sub) { const ic = mk('div', 'mic'); ic.innerHTML = window.SFSymbol('chevron.right').svg; it.appendChild(ic); it.dataset.sub = r.sub; }
      else if (r.sys) { const ic = mk('div', 'mic'); ic.innerHTML = window.SFSymbol(r.sys).svg; it.appendChild(ic); }
      if (r.tap) { it.dataset.tap = r.tap; if (r.val !== undefined) it.dataset.val = r.val; }
      pop.appendChild(it);
    }
    if (!rows.length) { const it = mk('div', 'mi dis'); it.textContent = 'No items'; pop.appendChild(it); }
    return pop;
  }
  function openMenuAt(anchor, def, atPointer) {
    closeMenu();
    if (!def) return;
    const scr = $('#screen');
    const pop = buildMenu(def);
    scr.appendChild(pop);
    const sr = scr.getBoundingClientRect();
    const ar = anchor.getBoundingClientRect();
    const sc = st.scale || 1;
    const ax = (ar.left - sr.left) / sc, ay = (ar.top - sr.top) / sc, aw = ar.width / sc, ah = ar.height / sc;
    const pw = pop.offsetWidth, ph = pop.offsetHeight;
    const W = st.device.w, H = st.device.h;
    let left = Math.min(Math.max(8, ax + aw - pw), W - pw - 8);
    if (ax + pw < W - 8 && ax < W / 2) left = Math.max(8, ax);
    let top = ay + ah + 6;
    if (top + ph > H - 20) { top = Math.max(st.device.top, ay - ph - 6); pop.style.transformOrigin = 'bottom center'; }
    pop.style.left = left + 'px';
    pop.style.top = top + 'px';
    st.openMenu = { pop, anchor };
  }
  function openSubmenu(id) {
    if (!st.openMenu) return;
    const old = st.openMenu.pop;
    const pop = buildMenu(st.menuDefs[id]);
    pop.style.left = old.style.left;
    pop.style.top = old.style.top;
    old.replaceWith(pop);
    st.openMenu.pop = pop;
  }
  function closeMenu() {
    if (st.openMenu) { st.openMenu.pop.remove(); st.openMenu = null; }
  }

  initChrome();
  connect();
})();
