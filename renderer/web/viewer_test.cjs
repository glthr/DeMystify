"use strict";
// These are DOM/canvas contract tests, not a claim of browser or visual QA.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const source = fs.readFileSync(path.join(__dirname, "viewer.js"), "utf8");
const html = fs.readFileSync(path.join(__dirname, "viewer.html"), "utf8");

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
  getContext() { return this.ownerDocument.context; }
}
const all = root => root.children.flatMap(child => [child, ...all(child)]);
const descendants = (root, tag) => all(root).filter(element => element.tagName === tag.toUpperCase());

function harness(data) {
  const frames = [], requestedImages = [];
  const document = new Element("document", null); document.ownerDocument = document; document.ids = new Map(); document.activeElement = null;
  document.getElementById = key => document.ids.get(key); document.createElement = tag => new Element(tag, document); document.createElementNS = (_, tag) => new Element(tag, document);
  const canvasCalls = [], canvasPaints = [];
  document.context = new Proxy({measureText: text => ({width: String(text).length * 5.5})}, {get(target, key) { if (!(key in target)) target[key] = (...args) => { canvasCalls.push([key, ...args]); if (["stroke", "fill", "fillRect", "fillText", "drawImage"].includes(key)) canvasPaints.push({method: key, fill: target.fillStyle, stroke: target.strokeStyle, alpha: target.globalAlpha, font: target.font, args}); }; return target[key]; }});
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
  const window = {document, DEMYSTIFY_GRAPH: data, Image: LocalImage, devicePixelRatio: 2, requestAnimationFrame: callback => { frames.push(callback); return frames.length; }, addEventListener() {}, setTimeout};
  const context = vm.createContext({window, console, setTimeout}); vm.runInContext(source, context, {filename: "viewer.js"});
  const app = window.DeMystifyAtlas.app;
  const flush = () => { for (let n = 0; frames.length && n < 30; n++) frames.shift()(); assert.ok(frames.length < 30, "render loop must settle without image loads"); };
  flush();
  return {window, document, app, api: window.DeMystifyAtlas, get: id => document.getElementById(id), requestedImages, canvasCalls, canvasPaints, flush};
}

function fixture() {
  return {nodes: [
    {id: 1, name: "Myst:8336", stack: "Myst", originalName: "Dock", assetName: "Dock-N", image: "images/full-1.png", thumbnail: "images/thumb-1.png", imageWidth: 544, imageHeight: 333, viewWidth: 544, viewHeight: 332, x: 0, y: 0, width: 180, height: 132, attributes: ["card"], zones: [
      {id: 7, type: "button", layer: "card", name: "Forward <button>", x: 10, y: 15, width: 40, height: 60, script: 'on mouseUp\nput "<img src=x onerror=alert(1)>" into message\nend mouseUp'},
      {id: 8, type: "field", layer: "background", name: "Stored hidden", x: -10, y: -2, width: 30, height: 20, hidden: true, enabled: false, script: "pop card"}]},
    {id: 2, name: "Myst:42", stack: "Myst", assetName: "Library view", image: "images/full-2.png", thumbnail: "images/thumb-2.png", x: 320, y: 0, width: 180, height: 132, attributes: ["card"]},
    {id: 3, name: "Selenitic Age:31832", stack: "Selenitic Age", x: 900, y: 500, width: 180, height: 56, attributes: ["card", "virtual"]}],
    edges: [{source: 1, target: 2, attributes: ["intraAge"]}, {source: 1, target: 2, attributes: ["disabled"]}, {source: 2, target: 1, attributes: ["backtracking"]}, {source: 1, target: 1, attributes: ["selfReference"]}], meta: {cards: 2, stacks: 2}};
}
const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < .00001, `${actual} differs from ${expected}`);

test("initial overview selects genuine dock picture and visible stored zones without changing fit", () => {
  const h = harness(fixture());
  assert.equal(h.app.state.selected.id, 1); assert.equal(h.get("selection-panel").hidden, false);
  assert.equal(descendants(h.get("selected-picture"), "img")[0].src, "images/full-1.png");
  assert.equal(descendants(h.get("selected-picture"), "rect").length, 1); assert.equal(h.get("show-zones").checked, true);
  assert.equal(h.get("conditional-legend").hidden, true); assert.equal(h.get("graph-canvas").width, 2000);
  const expected = h.api.fitCamera(h.api.boundsFor(h.app.graph.nodes), 1000, 700, 190);
  near(h.app.state.camera.scale, expected.scale); near(h.app.state.camera.x, expected.x);
});

test("wheel zoom is anchored at cursor; controls and keyboard update the camera", () => {
  const h = harness(fixture()), canvas = h.get("graph-canvas"), anchor = {x: 280, y: 190}, before = h.app.worldPoint(anchor), oldScale = h.app.state.camera.scale;
  canvas.dispatch("wheel", {clientX: anchor.x, clientY: anchor.y, deltaY: -120}); h.flush();
  near(h.app.worldPoint(anchor).x, before.x); near(h.app.worldPoint(anchor).y, before.y); assert.ok(h.app.state.camera.scale > oldScale);
  const scale = h.app.state.camera.scale; h.get("zoom-in").dispatch("click"); assert.ok(h.app.state.camera.scale > scale);
  canvas.focus(); const x = h.app.state.camera.x; h.document.dispatch("keydown", {key: "ArrowLeft", shiftKey: true}); near(h.app.state.camera.x, x + 110);
  h.document.dispatch("keydown", {key: "f"}); near(h.app.state.camera.scale, oldScale);
  h.document.dispatch("keydown", {key: "/"}); assert.equal(h.document.activeElement, h.get("graph-search"));
});

test("drag pans without selecting; canvas click and neighbor references select nodes", () => {
  const h = harness(fixture()), canvas = h.get("graph-canvas"), original = {...h.app.state.camera};
  canvas.dispatch("pointerdown", {clientX: 850, clientY: 580}); canvas.dispatch("pointermove", {clientX: 885, clientY: 560}); canvas.dispatch("pointerup", {clientX: 885, clientY: 560});
  near(h.app.state.camera.x, original.x + 35); near(h.app.state.camera.y, original.y - 20); assert.equal(h.app.state.selected.id, 1);
  const position = h.app.screenPoint(h.app.graph.byID.get(2));
  canvas.dispatch("pointerdown", {clientX: position.x, clientY: position.y}); canvas.dispatch("pointerup", {clientX: position.x, clientY: position.y});
  assert.equal(h.app.state.selected.id, 2); h.get("outgoing-list").children[0].dispatch("click"); assert.equal(h.app.state.selected.id, 1);
  assert.equal(h.get("outgoing-list").children.length, 3); assert.match(h.get("outgoing-list").textContent, /Disabled/); assert.equal(h.get("incoming-list").children.length, 2);
});

test("pinch preserves its world anchor and does not create a node click", () => {
  const h = harness(fixture()), canvas = h.get("graph-canvas"), scale = h.app.state.camera.scale, anchor = h.app.worldPoint({x: 300, y: 250});
  canvas.dispatch("pointerdown", {pointerId: 1, clientX: 200, clientY: 250}); canvas.dispatch("pointerdown", {pointerId: 2, clientX: 400, clientY: 250});
  canvas.dispatch("pointermove", {pointerId: 2, clientX: 500, clientY: 250});
  near(h.app.state.camera.scale, scale * 1.5); near(h.app.worldPoint({x: 350, y: 250}).x, anchor.x); near(h.app.worldPoint({x: 350, y: 250}).y, anchor.y);
  canvas.dispatch("pointerup", {pointerId: 2, clientX: 500, clientY: 250}); canvas.dispatch("pointerup", {pointerId: 1, clientX: 200, clientY: 250}); assert.equal(h.app.state.selected.id, 1);
});

test("Age filters work and keyboard search can reveal a card in a hidden Age", () => {
  const h = harness(fixture()), labels = h.get("age-filters").children, myst = labels.find(label => label.textContent.startsWith("Myst"));
  myst.children[0].checked = false; myst.children[0].dispatch("change"); assert.equal(h.app.state.ages.has("Myst"), false); assert.equal(h.app.state.selected, null);
  h.get("no-ages").dispatch("click"); h.flush(); assert.equal(h.get("map-empty").hidden, false);
  const search = h.get("graph-search"); search.focus(); search.value = "31832"; search.dispatch("input");
  assert.equal(h.get("search-results").children.length, 1); search.dispatch("keydown", {key: "ArrowDown"}); assert.equal(search.getAttribute("aria-activedescendant"), "search-result-0");
  search.dispatch("keydown", {key: "Enter"}); assert.equal(h.app.state.selected.id, 3); assert.equal(h.app.state.ages.has("Selenitic Age"), true); assert.ok(h.app.state.camera.scale >= 1);
  search.value = "Dock-N"; search.dispatch("input"); search.dispatch("keydown", {key: "Enter"}); assert.equal(h.app.state.selected.id, 1);
  h.get("all-ages").dispatch("click"); assert.equal(h.app.state.ages.size, 2);
});

test("Escape closes search without its bubbled event clearing the selected card", () => {
  const h = harness(fixture()), search = h.get("graph-search"); search.focus(); search.value = "Dock"; search.dispatch("input");
  search.dispatch("keydown", {key: "Escape"}); assert.equal(h.get("search-results").hidden, true); assert.equal(h.app.state.selected.id, 1); assert.equal(h.document.activeElement, h.get("graph-canvas"));
});

test("zones are clipped, hidden separately, keyboard operable, and script text is never HTML", () => {
  const h = harness(fixture()); let rectangles = descendants(h.get("selected-picture"), "rect"); assert.equal(rectangles.length, 1);
  rectangles[0].dispatch("keydown", {key: "Enter"});
  assert.equal(h.get("zone-name").textContent, "Forward <button>"); assert.equal(h.get("zone-script").textContent, fixture().nodes[0].zones[0].script); assert.equal(h.get("zone-script").children.length, 0);
  assert.equal(h.app.state.zone.id, 7); assert.equal(rectangles[0].classList.contains("is-selected"), true);
  h.get("show-hidden-zones").checked = true; h.get("show-hidden-zones").dispatch("change"); rectangles = descendants(h.get("selected-picture"), "rect"); assert.equal(rectangles.length, 2);
  assert.equal(rectangles[1].getAttribute("x"), "0"); assert.equal(rectangles[1].getAttribute("y"), "0"); assert.equal(rectangles[1].getAttribute("width"), "20"); assert.equal(rectangles[1].getAttribute("height"), "18");
  rectangles[1].dispatch("click"); assert.match(h.get("zone-meta").textContent, /background field.*stored hidden.*stored disabled/);
  h.get("show-zones").checked = false; h.get("show-zones").dispatch("change"); assert.equal(descendants(h.get("selected-picture"), "svg").length, 0); assert.equal(h.get("zone-detail").hidden, true);
});

test("expanded picture has synchronized zone controls; missing pictures remain explicit", () => {
  const h = harness(fixture()); h.get("expand-picture").dispatch("click"); assert.equal(h.get("picture-dialog").open, true);
  assert.equal(descendants(h.get("dialog-picture"), "svg")[0].getAttribute("viewBox"), "0 0 544 332");
  h.get("dialog-show-zones").checked = false; h.get("dialog-show-zones").dispatch("change"); assert.equal(h.get("show-zones").checked, false); assert.equal(descendants(h.get("dialog-picture"), "svg").length, 0);
  h.get("close-picture").dispatch("click"); assert.equal(h.app.state.dialog, false);
  h.app.selectNode(3); assert.equal(descendants(h.get("selected-picture"), "img").length, 0); assert.match(h.get("selected-picture").textContent, /No local picture/);
});

test("geometry uses positive native view dimensions and refuses misleading aspect-mismatched overlays", () => {
  const data = fixture(); data.nodes[0].viewWidth = 0; data.nodes[0].viewHeight = 0;
  let h = harness(data); assert.equal(descendants(h.get("selected-picture"), "svg")[0].getAttribute("viewBox"), "0 0 544 333");
  data.nodes[0].viewWidth = 544; data.nodes[0].viewHeight = 332; data.nodes[0].imageWidth = data.nodes[0].imageHeight = 512;
  h = harness(data); assert.equal(descendants(h.get("selected-picture"), "svg").length, 0); assert.match(h.get("selected-picture").textContent, /Zone alignment unavailable/);
});

test("overview does not eagerly load node images; moderate zoom uses at most four concurrent thumbnail requests", () => {
  const nodes = Array.from({length: 300}, (_, index) => ({id: index, name: "Myst:" + (index + 1), stack: "Myst", image: "images/full-" + index + ".png", thumbnail: "images/thumb-" + index + ".png", x: index % 20 * 250, y: Math.floor(index / 20) * 200, width: 180, height: 132, attributes: ["card"]}));
  const h = harness({nodes, edges: []}); assert.equal(h.requestedImages.length, 0);
  h.app.selectNode(0, true); h.flush(); assert.ok(h.requestedImages.length > 0 && h.requestedImages.length <= 4);
  assert.ok(h.requestedImages.every(image => image.url.includes("thumb-"))); assert.ok(h.app.imageCache.size < nodes.length);
});

test("150 visible thumbnails are retained instead of evicted and fetched on every redraw", () => {
  const nodes = Array.from({length: 150}, (_, index) => ({id: index, name: "Myst:" + index, stack: "Myst", image: "images/full-" + index + ".png", thumbnail: "images/thumb-" + index + ".png", x: index % 5, y: index % 7, width: 180, height: 132, attributes: ["card"]}));
  const h = harness({nodes, edges: []});
  for (let round = 0; round < 50; round++) { for (const image of [...h.requestedImages]) image.finish(); h.flush(); }
  assert.equal(h.requestedImages.length, 150); assert.equal(h.app.imageCache.size, 150);
  h.app.draw(); h.app.draw(); h.flush(); assert.equal(h.requestedImages.length, 150);
});

test("all valid reference records survive normalization and local images cannot become remote requests", () => {
  const h = harness(fixture()); assert.equal(h.app.graph.edges.length, 4); assert.notEqual(h.app.graph.edges[0].parallel, h.app.graph.edges[1].parallel);
  for (const path of ["https://example.test/a.png", "//example.test/a.png", "javascript:alert(1)"]) assert.equal(h.api.safeImagePath(path), null);
  assert.equal(h.api.safeImagePath("images-abc/a.png"), "images-abc/a.png");
});

function pdfFixture() {
  const data = fixture();
  const geometries = [{id: 1, x: 120, y: 80, width: 84, height: 36, fill: "#d9fcc5"}, {id: 2, x: 320, y: 260, width: 92, height: 30, fill: "#ff9955"}, {id: 3, x: 90, y: 300, width: 60, height: 24, fill: "#ffffba"}];
  data.pdfLayout = {bounds: {minX: 0, minY: 0, maxX: 900, maxY: 550}, nodes: geometries.map(node => ({...node, stroke: "#000000", fontColor: "#000000", penWidth: 1.2, shape: "box", style: "filled,rounded", draw: [{op: "c", color: "#000000"}, {op: "C", color: node.fill}, {op: "P", points: [[node.x - node.width / 2, node.y - node.height / 2], [node.x + node.width / 2, node.y - node.height / 2], [node.x + node.width / 2, node.y + node.height / 2], [node.x - node.width / 2, node.y + node.height / 2]]}], labels: [{op: "F", size: 10, face: "Arial"}, {op: "c", color: "#000000"}, {op: "T", pt: [node.x, node.y - 3], align: "c", width: 55, text: data.nodes.find(record => record.id === node.id).name}, {op: "T", pt: [node.x, node.y + 9], align: "c", width: 37, text: "Original label"}]})), edges: [
    {source: 1, target: 2, color: "#000000", penWidth: .6, draw: [{op: "c", color: "#000000"}, {op: "b", points: [[162, 80], [700, -120], [850, 510], [320, 245]]}], head: [{op: "C", color: "#000000"}, {op: "P", points: [[317, 240], [320, 245], [323, 239]]}]},
    {source: 1, target: 2, color: "#cccccc", style: "dashed", penWidth: .6, draw: [{op: "S", style: "dashed"}, {op: "b", points: [[150, 98], [170, 150], [250, 200], [300, 245]]}]},
    {source: 1, target: 1, color: "#000000", draw: [{op: "b", points: [[100, 62], [60, 10], [190, 10], [140, 62]]}]}], draw: [{op: "c", color: "#fffffe00"}, {op: "C", color: "#ffffff"}, {op: "P", points: [[0, 0], [900, 0], [900, 550], [0, 550]]}]};
  return data;
}

test("PDF layout is default when present and uses exact boxes without dropping inspector records", () => {
  const data = pdfFixture(), h = harness(data);
  assert.equal(h.app.state.mode, "pdf"); assert.equal(h.get("layout-tools").hidden, false); assert.equal(h.get("map-surface").classList.contains("is-pdf"), true);
  assert.equal(h.get("pdf-layout").getAttribute("aria-pressed"), "true");
  assert.equal(h.app.graph.byID.get(1).width, 84); assert.equal(h.app.graph.byID.get(1).height, 36); assert.equal(h.app.graph.byID.get(1).x, 120);
  assert.equal(h.app.graph.edges.length, 4); assert.equal(h.app.graph.pdfLayout.edges.length, 3); assert.equal(h.get("outgoing-list").children.length, 3);
  assert.ok(h.canvasPaints.some(paint => paint.method === "fill" && paint.fill === "#d9fcc5"));
  assert.ok(h.canvasPaints.some(paint => paint.method === "fill" && paint.fill === "#ff9955"));
  assert.ok(h.canvasPaints.filter(paint => paint.method === "stroke").every(paint => paint.alpha === 1), "initial selected dock must not wash out the PDF graph");
});

test("layout switching preserves selected card, Age filters, zones and metadata while refitting", () => {
  const h = harness(pdfFixture()); h.app.setAges(["Myst"]); h.app.selectNode(2); h.app.showZone(h.app.graph.byID.get(1).zones[0], false);
  h.get("atlas-layout").dispatch("click"); assert.equal(h.app.state.mode, "atlas"); assert.equal(h.app.state.selected.id, 2); assert.equal(h.app.state.ages.size, 1);
  assert.equal(h.app.graph.byID.get(1).x, 0); assert.equal(h.app.graph.byID.get(1).width, 180); assert.equal(h.get("pdf-images-control").hidden, true);
  h.get("pdf-layout").dispatch("click"); h.flush(); assert.equal(h.app.state.mode, "pdf"); assert.equal(h.app.state.selected.id, 2); assert.equal(h.app.state.ages.has("Selenitic Age"), false);
  assert.equal(h.app.graph.byID.get(1).x, 120); assert.equal(h.app.graph.byID.get(1).width, 84); assert.equal(h.app.graph.nodes[0].zones[0].x, 10);
});

test("PDF Age colors and legends follow the drawing and restore the atlas palette on switching", () => {
  const data = pdfFixture(); data.nodes[1].attributes.push("sink");
  const h = harness(data), atlasColor = h.app.graph.atlasColors.get("Myst");
  assert.equal(h.app.graph.colors.get("Myst"), "#d9fcc5", "sink highlight must not replace the Age's ordinary fill");
  let myst = h.get("age-filters").children.find(label => label.textContent.startsWith("Myst"));
  assert.equal(myst.style.getPropertyValue("--age-color"), "#d9fcc5");
  assert.equal(h.get("pdf-legend").hidden, false); assert.equal(h.get("atlas-legend").hidden, true);
  assert.equal(h.get("outgoing-list").children[0].children[2].style.background, "#d9fcc5");
  const search = h.get("graph-search"); search.value = "Myst:42"; search.dispatch("input");
  assert.equal(h.get("search-results").children[0].children[0].style.background, "#d9fcc5");
  h.get("atlas-layout").dispatch("click"); myst = h.get("age-filters").children.find(label => label.textContent.startsWith("Myst"));
  assert.equal(myst.style.getPropertyValue("--age-color"), atlasColor); assert.equal(h.get("search-results").children[0].children[0].style.background, atlasColor);
  assert.equal(h.get("pdf-legend").hidden, true); assert.equal(h.get("atlas-legend").hidden, false);
});

test("PDF images stay inside original boxes; Images off replays original labels without thumbnail drawing", () => {
  const h = harness(pdfFixture()); h.app.selectNode(1, true); h.flush();
  for (const image of [...h.requestedImages]) image.finish(); h.flush();
  const images = h.canvasPaints.filter(paint => paint.method === "drawImage" && paint.args[0].url === "images/thumb-1.png"); assert.ok(images.length);
  for (const image of images) { const [, x, y, width, height] = image.args; assert.ok(x >= 78 && y >= 62 && x + width <= 162 && y + height <= 98); }
  assert.equal(h.app.graph.byID.get(1).width, 84);
  h.canvasPaints.length = 0; h.get("pdf-images").checked = false; h.get("pdf-images").dispatch("change"); h.flush();
  assert.equal(h.canvasPaints.filter(paint => paint.method === "drawImage").length, 0);
  assert.ok(h.canvasPaints.some(paint => paint.method === "fillText" && paint.args[0] === "Original label"));
});

test("PDF routed-curve culling uses full control-point bounds rather than endpoints", () => {
  const h = harness(pdfFixture()); h.app.state.camera = {scale: 1, x: -600, y: 0}; h.canvasCalls.length = 0; h.app.draw();
  assert.ok(h.canvasCalls.some(call => call[0] === "bezierCurveTo" && call[1] === 700 && call[2] === -120 && call[3] === 850 && call[4] === 510));
  const bounds = h.api.operationBounds(pdfFixture().pdfLayout.edges[0].draw); assert.equal(bounds.maxX, 850); assert.equal(bounds.minY, -120);
});

test("PDF mode falls back honestly when unavailable or incomplete; atlas controls stay hidden", () => {
  const data = fixture(); data.pdfLayout = {...pdfFixture().pdfLayout, nodes: pdfFixture().pdfLayout.nodes.slice(1)};
  let h = harness(data); assert.equal(h.app.state.mode, "atlas"); assert.equal(h.get("layout-tools").hidden, true);
  data.pdfLayout = {nodes: pdfFixture().pdfLayout.nodes, edges: null, bounds: {minX: 0, minY: 0, maxX: 400, maxY: 400}};
  h = harness(data); assert.equal(h.app.state.mode, "pdf"); assert.equal(h.app.graph.pdfLayout.edges.length, 0);
});

function operationContext() {
  const calls = [], paints = [], saved = [], data = {strokeStyle: "original-stroke", fillStyle: "original-fill", lineWidth: 9, globalAlpha: 1};
  const target = {measureText: text => ({width: text.length * 6}), save: () => saved.push({...data}), restore: () => { const last = saved.pop(); for (const key of Object.keys(data)) delete data[key]; Object.assign(data, last); }, createLinearGradient: (...args) => ({args, stops: [], addColorStop(frac, color) { this.stops.push({frac, color}); }}), createRadialGradient: (...args) => ({args, stops: [], addColorStop(frac, color) { this.stops.push({frac, color}); }})};
  const context = new Proxy(target, {get(target, key) { if (key in data) return data[key]; if (!(key in target)) target[key] = (...args) => { calls.push([key, ...args]); if (["stroke", "fill", "fillText"].includes(key)) paints.push({op: key, ...data, args}); }; return target[key]; }, set(_, key, value) { data[key] = value; return true; }});
  return {context, calls, paints, data};
}

test("Graphviz JSON operation replay preserves case-sensitive fills, style state, fonts and top-down baselines", () => {
  const h = harness(fixture()), trace = operationContext(), points = [[10, 20], [30, 40], [50, 60], [70, 80]];
  h.api.replayOperations(trace.context, [{op: "C", color: "#d9fcc5"}, {op: "c", color: "#123456"}, {op: "S", style: "setlinewidth(2.5),dashed"}, {op: "b", points}, {op: "B", points}, {op: "p", points}, {op: "P", points}, {op: "L", points}, {op: "e", rect: [30, 40, 10, 20]}, {op: "E", rect: [30, 40, 10, 20]}, {op: "F", face: "Arial", size: 10}, {op: "t", fontchar: 3}, {op: "T", pt: [200, 99], align: "r", width: 60, text: "Label"}]);
  assert.equal(trace.paints.filter(paint => paint.op === "fill").length, 3, "JSON B/P/E fill; b/p/e do not");
  assert.equal(trace.paints.filter(paint => paint.op === "stroke").length, 7);
  assert.ok(trace.paints.filter(paint => paint.op === "stroke").every(paint => paint.strokeStyle === "#123456" && paint.lineWidth === 2.5));
  const text = trace.paints.find(paint => paint.op === "fillText"); assert.match(text.font, /italic bold 10px Arial/); assert.equal(text.textAlign, "right"); assert.equal(text.textBaseline, "alphabetic");
  assert.ok(trace.calls.some(call => call[0] === "translate" && call[1] === 200 && call[2] === 99)); assert.equal(trace.data.lineWidth, 9, "drawing-attribute state must not leak to the next object");
});

test("Graphviz gradients keep normalized endpoint geometry and color-stop fractions", () => {
  const h = harness(fixture()), trace = operationContext();
  h.api.replayOperations(trace.context, [{op: "C", grad: "linear", p0: [2, 3], p1: [20, 30], stops: [{frac: 0, color: "#ffffff"}, {frac: 1, color: "#000000"}]}, {op: "P", points: [[0, 0], [1, 0], [1, 1]]}]);
  const fill = trace.paints.find(paint => paint.op === "fill").fillStyle; assert.deepEqual(fill.args, [2, 3, 20, 30]); assert.deepEqual(fill.stops, [{frac: 0, color: "#ffffff"}, {frac: 1, color: "#000000"}]);
});
