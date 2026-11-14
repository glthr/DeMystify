"use strict";
// DOM/canvas harness adapted from DeMystify (c)2025 Guillaume Lethuillier, MIT.
// License: ../riven_graph/web/LICENSE.demystify. This does not replace browser QA.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const source = fs.readFileSync(path.join(__dirname, "../riven_graph/web/viewer.js"), "utf8");
const html = fs.readFileSync(path.join(__dirname, "../riven_graph/web/viewer.html"), "utf8");

class Element {
  constructor(tag, document) {
    this.tagName = tag.toUpperCase(); this.ownerDocument = document; this.children = []; this.parentNode = null; this.attributes = {}; this.listeners = new Map();
    this.value = ""; this.checked = false; this.disabled = false; this.hidden = false; this.className = ""; this.open = false; this._text = "";
    const properties = new Map(); this.style = {setProperty: (key, value) => properties.set(key, value), getPropertyValue: key => properties.get(key)};
    this.classList = {contains: key => this.className.split(/\s+/).includes(key), add: key => this.classList.toggle(key, true), remove: key => this.classList.toggle(key, false), toggle: (key, enabled) => { const set = new Set(this.className.split(/\s+/).filter(Boolean)); const value = enabled == null ? !set.has(key) : enabled; if (value) set.add(key); else set.delete(key); this.className = [...set].join(" "); return value; }};
  }
  setAttribute(key, value) { this.attributes[key] = String(value); if (key === "id") { this.id = value; this.ownerDocument.ids.set(value, this); } if (key === "class") this.className = String(value); if (key === "hidden") this.hidden = true; if (key === "checked") this.checked = true; if (key === "type") this.type = value; }
  getAttribute(key) { return this.attributes[key] == null ? null : this.attributes[key]; }
  removeAttribute(key) { delete this.attributes[key]; if (key === "open") this.open = false; }
  append(...children) { for (const child of children) { if (child.parentNode) child.parentNode.children = child.parentNode.children.filter(value => value !== child); child.parentNode = this; this.children.push(child); } }
  replaceChildren(...children) { for (const child of this.children) child.parentNode = null; this.children = []; this._text = ""; this.append(...children); }
  get textContent() { return this._text + this.children.map(child => child.textContent).join(""); }
  set textContent(value) { this.replaceChildren(); this._text = String(value); }
  set innerHTML(_) { throw new Error("HTML interpolation is forbidden in viewer UI"); }
  addEventListener(name, callback) { if (!this.listeners.has(name)) this.listeners.set(name, []); this.listeners.get(name).push(callback); }
  dispatch(name, values = {}) {
    const event = {type: name, target: this, key: "", pointerId: 1, button: 0, clientX: 0, clientY: 0, deltaY: 0, preventDefault() { this.defaultPrevented = true; }, stopPropagation() { this.stopped = true; }, ...values};
    let current = this;
    while (current) { for (const callback of current.listeners.get(name) || []) callback(event); if (event.stopped) break; current = current.parentNode; }
    return event;
  }
  focus() { this.ownerDocument.activeElement = this; this.dispatch("focus"); }
  contains(other) { return other === this || this.children.some(child => child.contains(other)); }
  querySelector(selector) { return all(this).find(element => selector.startsWith(".") ? element.classList.contains(selector.slice(1)) : element.tagName === selector.toUpperCase()) || null; }
  getBoundingClientRect() { return {left: 0, top: 0, width: 1000, height: 700}; }
  setPointerCapture() {}
  showModal() { this.open = true; }
  close() { this.open = false; this.dispatch("close"); }
  getContext() {
    if (this.id === "graph-canvas") return this.ownerDocument.context;
    if (!this.context) { this.context = this.ownerDocument.newCanvasContext(); this.ownerDocument.offscreenContexts.push({canvas: this, ...this.context}); }
    return this.context.context;
  }
}
const all = root => root.children.flatMap(child => [child, ...all(child)]);
const descendants = (root, tag) => all(root).filter(element => element.tagName === tag.toUpperCase());

function harness(data) {
  const frames = [], requestedImages = [];
  const document = new Element("document", null); document.ownerDocument = document; document.ids = new Map(); document.activeElement = null;
  document.getElementById = key => document.ids.get(key); document.createElement = tag => new Element(tag, document); document.createElementNS = (_, tag) => new Element(tag, document);
  const canvasCalls = [], canvasPaints = [];
  document.offscreenContexts = [];
  document.newCanvasContext = (calls = [], paints = []) => {
    const saved = [], values = {globalCompositeOperation: "source-over", globalAlpha: 1};
    const methods = {measureText: text => ({width: String(text).length * 5.5}), save: () => saved.push({...values}), restore: () => { const previous = saved.pop(); for (const key of Object.keys(values)) delete values[key]; Object.assign(values, previous); }};
    const context = new Proxy(methods, {
      get(target, key) {
        if (key === "getImageData" || key === "putImageData") return () => { throw new Error("Offline images must never be read as pixels"); };
        if (key in values) return values[key];
        if (!(key in target)) target[key] = (...args) => { calls.push([key, ...args]); if (["stroke", "fill", "fillRect", "fillText", "drawImage"].includes(key)) paints.push({method: key, fill: values.fillStyle, stroke: values.strokeStyle, alpha: values.globalAlpha, composite: values.globalCompositeOperation, font: values.font, args}); };
        return target[key];
      },
      set(_, key, value) { if (key === "filter") throw new Error("ctx.filter is not an offline grayscale dependency"); values[key] = value; return true; }
    });
    return {context, calls, paints};
  };
  document.context = document.newCanvasContext(canvasCalls, canvasPaints).context;
  const stack = [document], voidTags = new Set(["meta", "link", "input", "br", "hr", "img"]);
  for (const match of html.matchAll(/<\/?([a-z][a-z0-9-]*)([^>]*)>/gi)) {
    const tag = match[1].toLowerCase();
    if (match[0].startsWith("</")) { while (stack.length > 1 && stack.pop().tagName !== tag.toUpperCase()) {} continue; }
    const element = document.createElement(tag);
    for (const attribute of match[2].matchAll(/([^\s=\/]+)(?:\s*=\s*"([^"]*)")?/g)) element.setAttribute(attribute[1], attribute[2] || "");
    stack.at(-1).append(element); if (!voidTags.has(tag) && !match[0].endsWith("/>")) stack.push(element);
  }
  class LocalImage {
    constructor() { requestedImages.push(this); this.naturalWidth = 160; this.naturalHeight = 98; }
    set src(value) { this.url = value; }
    finish() { if (this.finished) return; this.finished = true; if (this.onload) this.onload(); }
  }
  // The harness has no layout: a test calls an observer to stand for a change of canvas size.
  const resizeObservers = []; class RecordingResizeObserver { constructor(callback) { resizeObservers.push(callback); } observe() {} }
  const window = {document, DEMYSTIFY_GRAPH: data, Image: LocalImage, ResizeObserver: RecordingResizeObserver, resizeObservers, devicePixelRatio: 2, requestAnimationFrame: callback => { frames.push(callback); return frames.length; }, addEventListener() {}, setTimeout};
  const context = vm.createContext({window, console, setTimeout}); vm.runInContext(source, context, {filename: "viewer.js"});
  const app = window.DeMystifyAtlas.app;
  const flush = () => { for (let n = 0; frames.length && n < 30; n++) frames.shift()(); assert.ok(frames.length < 30, "render loop must settle without image loads"); };
  flush();
  return {window, document, app, api: window.DeMystifyAtlas, get: id => document.getElementById(id), requestedImages, canvasCalls, canvasPaints, offscreenContexts: document.offscreenContexts, flush};
}

function fixture(pdf = false) {
  const nodes = [
    {id: 1, sourceId: 'tspit:155', name: 'Temple:155', stack: 'Temple', originalName: 'Entry', rmap: '0x00006e9a', image: 'images/start.png', thumbnail: 'images/start-thumb.png', imageWidth: 608, imageHeight: 392, x: 0, y: 0, width: 180, height: 132, viewWidth: 608, viewHeight: 392, attributes: ['card', 'source'], scriptText: '</script><img src=x onerror=alert(1)>', provenance: {resource: 'CARD:155'}, zones: [{id: 4, type: 'hotspot', name: 'Forward <button>', x: 10, y: 20, width: 80, height: 100, script: 'change_card(156);'}]},
    {id: 2, sourceId: 'tspit:156', name: 'Temple:156', stack: 'Temple', x: 320, y: 0, width: 180, height: 56, attributes: ['card']},
    {id: 3, sourceId: 'ospit:1', name: 'Gehn:1', stack: 'Gehn', x: 650, y: 0, width: 180, height: 56, attributes: ['card']},
    {id: 4, sourceId: 'gspit:1', name: 'Garden:1', stack: 'Garden', x: 950, y: 0, width: 180, height: 56, attributes: ['card']},
    {id: 5, sourceId: 'aspit:1', name: 'Menu:1', stack: 'Menu', x: 0, y: 400, width: 180, height: 56, attributes: ['card']},
    {id: 6, sourceId: 'tspit:361', name: 'Temple:361', stack: 'Temple', x: 320, y: 400, width: 180, height: 56, attributes: ['virtual']},
    {id: 7, sourceId: 'aspit:5', name: 'Menu:5', stack: 'Menu', x: 650, y: 400, width: 180, height: 56, attributes: ['card'], dynamicNavigation: [{kind: 'dynamic-return'}]}
  ];
  const edges = [
    {source: 1, target: 2, kind: 'script', attributes: ['intraAge'], evidence: [{resource: 'HSPT:155', byte_offset: 30}]},
    {source: 1, target: 3, kind: 'native', attributes: ['native', 'crossAge'], evidence: [{engine_source: 'https://example.com/source'}]},
    {source: 1, target: 4, kind: 'zip', attributes: ['zip'], evidence: [{condition: 'previously visited'}]},
    {source: 1, target: 5, kind: 'native-ending', attributes: ['native'], evidence: [{condition: 'ending'}]},
    {source: 2, target: 6, kind: 'script', attributes: ['notImplemented'], evidence: [{missing_target: true}]},
    {source: 3, target: 4, kind: 'script', attributes: [], evidence: []},
  ];
  const data = {nodes, edges, meta: {cards: 6, startNodeID: 1}};
  if (pdf) data.pdfLayout = {
    bounds: {minX: -100, minY: -100, maxX: 1100, maxY: 500},
    nodes: nodes.map(n => ({id: n.id, x: n.x, y: n.y, width: n.width, height: n.height, fill: '#d0af76', stroke: '#223344'})),
    edges: edges.map((e, i) => ({...e, color: `#ee${String(i).padStart(4, '0')}`, penWidth: 1, draw: [{op: 'L', points: [[i*50, 0], [i*50+10, 20]]}]}))
  };
  return data;
}

test('Riven source IDs, RMAP search and start entry survive normalization', () => {
  const h = harness(fixture());
  assert.equal(h.app.state.selected.sourceId, 'tspit:155');
  assert.equal(h.api.searchNodes(h.app.graph.nodes, 'tspit:155')[0].id, 1);
  assert.equal(h.api.searchNodes(h.app.graph.nodes, '0x00006e9a')[0].id, 1);
  assert.deepEqual([...h.api.HIGHLIGHTS.map(x => x.id)], ['source', 'sink', 'isolated', 'selfReference', 'virtual']);
  assert.equal(h.get('selected-script').textContent, '</script><img src=x onerror=alert(1)>');
  assert.equal(descendants(h.get('selected-script'), 'img').length, 0);
  assert.equal(h.get('selected-provenance').textContent, JSON.stringify({resource: 'CARD:155'}, null, 2));
  const rect = descendants(h.get('selected-picture'), 'rect')[0];
  assert.equal(rect.getAttribute('x'), '10'); assert.equal(rect.getAttribute('width'), '80');
  rect.dispatch('click'); assert.equal(h.get('zone-script').textContent, 'change_card(156);');
});

test('navigation layers filter references and preserve every underlying record', () => {
  const h = harness(fixture());
  assert.equal(h.app.graph.edges.length, 6);
  assert.equal(h.get('outgoing-count').textContent, '2');
  assert.equal(h.app.graph.edges.filter(h.app.edgeAllowed).length, 4);
  h.get('zip-navigation').checked = true; h.get('zip-navigation').dispatch('change');
  assert.equal(h.get('outgoing-count').textContent, '3');
  h.get('ending-resets').checked = true; h.get('ending-resets').dispatch('change');
  assert.equal(h.get('outgoing-count').textContent, '4');
  h.get('native-navigation').checked = false; h.get('native-navigation').dispatch('change');
  assert.equal(h.get('outgoing-count').textContent, '2', 'turning native off removes both movement and reset');
  assert.equal(h.app.graph.edges.length, 6, 'filtering never rewrites the extracted model');
  assert.equal(h.app.graph.edges[0].evidence[0].byte_offset, 30);
  assert.match(h.get('outgoing-list').textContent, /Inspect script evidence · 1 callsites/);
});

test('both atlas and PDF drawing obey layer filters', () => {
  const h = harness(fixture(true));
  assert.equal(h.app.state.mode, 'pdf');
  h.app.state.camera = {x: 200, y: 150, scale: 0.5};
  const pdfPaints = () => {
    h.canvasPaints.length = 0; h.app.draw();
    return new Set(h.canvasPaints.filter(p => p.method === 'stroke').map(p => p.stroke));
  };
  let colors = pdfPaints();
  assert.ok(colors.has('#ee0000')); assert.ok(colors.has('#ee0001'));
  assert.ok(!colors.has('#ee0002'), 'Zip hidden in PDF'); assert.ok(!colors.has('#ee0003'), 'ending hidden in PDF');
  h.get('zip-navigation').checked = true; h.get('zip-navigation').dispatch('change');
  colors = pdfPaints(); assert.ok(colors.has('#ee0002'));
  h.get('native-navigation').checked = false; h.get('native-navigation').dispatch('change');
  colors = pdfPaints(); assert.ok(!colors.has('#ee0001'));
  h.app.setLayout('atlas');
  assert.equal(h.app.state.mode, 'atlas');
  h.get('zip-navigation').checked = false; h.get('zip-navigation').dispatch('change');
  h.app.state.camera = {x: 200, y: 150, scale: 0.05};
  h.canvasCalls.length = 0; h.app.draw();
  assert.equal(h.canvasCalls.filter(c => c[0] === 'quadraticCurveTo').length, 3, 'only three script connections remain at a dot-only zoom');
});

test('shortest structural path follows active layers without inventing saved returns', () => {
  const h = harness(fixture());
  h.get('set-path-origin').dispatch('click');
  h.app.selectNode(4);
  assert.deepEqual([...h.app.state.path], [1, 3, 4]);
  assert.match(h.get('path-status').textContent, /2 references/);
  h.get('zip-navigation').checked = true; h.get('zip-navigation').dispatch('change');
  assert.deepEqual([...h.app.state.path], [1, 4]);
  h.get('native-navigation').checked = false; h.get('native-navigation').dispatch('change');
  h.get('zip-navigation').checked = false; h.get('zip-navigation').dispatch('change');
  assert.deepEqual([...h.app.state.path], []);
  h.app.selectNode(7); h.get('set-path-origin').dispatch('click'); h.app.selectNode(1);
  assert.deepEqual([...h.app.state.path], []);
  assert.equal(h.app.graph.outgoing.get(7).length, 0, 'journal metadata cannot become a path shortcut');
});

test('a virtual target stays selectable without a fabricated picture', () => {
  const h = harness(fixture()); h.app.selectNode(6);
  assert.equal(h.get('selected-name').textContent, 'Temple:361');
  assert.match(h.get('selected-tags').textContent, /Absent target/);
  assert.equal(descendants(h.get('selected-picture'), 'img').length, 0);
  assert.equal(h.get('show-zones').disabled, true);
  assert.match(h.get('selected-facts').textContent, /Referenced target absent/);
});
