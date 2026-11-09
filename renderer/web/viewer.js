(function (global) {
  "use strict";

  const PALETTE = ["#b5b583", "#8daf91", "#b29abc", "#d0af76", "#85a9b3", "#bd8e79"];
  const ATTRIBUTE_LABELS = {disabled: "Disabled", backtracking: "Saved return · inverted arrows", sequencecontinuation: "Scripted sequence · next step", crossage: "Between Ages", selfreference: "Self-reference", notimplemented: "Absent target", restrictivetransitivitytail: "Paired return · tail", restrictivetransitivityhead: "Paired return · head", conditional: "Conditional", zip: "Zip navigation"};
  const NODE_LABELS = {virtual: "Absent target", stack: "Stack", source: "Source flag", sink: "Sink flag", isolated: "Isolate", containsbluepage: "Blue page", containsredpage: "Red page", containswhitepage: "White page"};
  const HIGHLIGHTS = [
    {id: "source", label: "Sources", flag: "source", description: "Uses the exported source flag. Source, sink and isolate flags may overlap."},
    {id: "sink", label: "Sinks", flag: "sink", description: "Uses the exported sink flag. Source, sink and isolate flags may overlap."},
    {id: "isolated", label: "Isolated", flag: "isolated", description: "Uses the exported isolate flag; no categories are inferred again."},
    {id: "selfReference", label: "Self-references", description: "At least one non-disabled raw reference record connects this node to itself."},
    {id: "bluePage", label: "Blue page cards", flag: "containsbluepage", description: "Cards with the exported blue-page flag."},
    {id: "redPage", label: "Red page cards", flag: "containsredpage", description: "Cards with the exported red-page flag."},
    {id: "whitePage", label: "White page cards", flag: "containswhitepage", description: "Cards with the exported white-page flag."},
    {id: "virtual", label: "Absent targets", flag: "virtual", description: "Referenced targets absent from the imported input."},
    {id: "stack", label: "Stacks", flag: "stack", description: "Stack abstractions, separate from individual cards."}
  ];
  const finite = (value, fallback = 0) => Number.isFinite(Number(value)) ? Number(value) : fallback;
  const clamp = (value, low, high) => Math.max(low, Math.min(high, value));
  const attribute = value => String(value || "").replace(/[^a-z]/gi, "").toLowerCase().replace(/^is(?=card|stack|virtual|source|sink|isolated)/, "");
  const has = (object, name) => object.flags.has(name);
  const formatNumber = value => finite(value).toLocaleString("en-US");
  const safeImagePath = value => typeof value === "string" && value && !/^(?:[a-z][a-z0-9+.-]*:|\/\/|#)/i.test(value) && !value.includes("\0") ? value : null;
  // Text entry owns printable keys, arrows and Escape. A focused checkbox or button does not.
  const typingTarget = element => !!element && (element.isContentEditable === true || element.tagName === "TEXTAREA" || element.tagName === "SELECT" || element.tagName === "INPUT" && !/^(?:checkbox|radio|button|submit|reset)$/i.test(element.type || ""));

  function normalizeGraph(raw = {}) {
    const nodes = [], byID = new Map();
    for (const input of Array.isArray(raw.nodes) ? raw.nodes : []) {
      if (!Number.isFinite(Number(input.id)) || byID.has(Number(input.id))) continue;
      const flags = new Set((input.attributes || []).map(attribute));
      const node = {...input, id: Number(input.id), flags, name: String(input.name || input.id), x: finite(input.x), y: finite(input.y), width: Math.max(24, finite(input.width, 180)), height: Math.max(24, finite(input.height, input.image ? 132 : 56))};
      node.stack = String(input.stack || (flags.has("stack") ? node.name : "Unassigned"));
      node.image = safeImagePath(input.image);
      node.thumbnail = safeImagePath(input.thumbnail) || node.image;
      node.originalName = String(input.originalName || "");
      node.assetName = String(input.assetName || "");
      node.searchText = [node.id, node.name, node.stack, node.originalName, node.assetName].join(" ").toLowerCase();
      node.zones = (Array.isArray(input.zones) ? input.zones : []).map((zone, index) => ({...zone, key: index, id: zone.id == null ? index + 1 : zone.id, type: String(zone.type || "part"), name: String(zone.name || ""), layer: zone.layer === "background" ? "background" : "card", x: finite(zone.x), y: finite(zone.y), width: Math.max(0, finite(zone.width)), height: Math.max(0, finite(zone.height)), hidden: zone.hidden === true, enabled: zone.enabled !== false, script: typeof zone.script === "string" ? zone.script : ""}));
      node.atlas = {x: node.x, y: node.y, width: node.width, height: node.height};
      nodes.push(node); byID.set(node.id, node);
    }
    const stacks = [...new Set(nodes.map(node => node.stack))].sort((a, b) => a.localeCompare(b));
    const colors = new Map(stacks.map((stack, i) => [stack, PALETTE[i % PALETTE.length]]));
    for (const node of nodes) node.color = colors.get(node.stack);
    const edges = [], incoming = new Map(), outgoing = new Map(), parallel = new Map();
    for (const node of nodes) { incoming.set(node.id, []); outgoing.set(node.id, []); }
    for (const input of Array.isArray(raw.edges) ? raw.edges : []) {
      const source = byID.get(Number(input.source)), target = byID.get(Number(input.target));
      if (!source || !target) continue;
      const edge = {...input, source: source.id, target: target.id, from: source, to: target, index: edges.length, flags: new Set((input.attributes || []).map(attribute))};
      const key = source.id <= target.id ? source.id + ":" + target.id : target.id + ":" + source.id;
      if (!parallel.has(key)) parallel.set(key, []);
      parallel.get(key).push(edge); edges.push(edge); incoming.get(target.id).push(edge); outgoing.get(source.id).push(edge);
    }
    for (const group of parallel.values()) group.forEach((edge, index) => { edge.parallel = index - (group.length - 1) / 2; });
    for (const node of nodes) { node.highlightKinds = new Set(HIGHLIGHTS.filter(kind => kind.flag && has(node, kind.flag)).map(kind => kind.id)); node.highlightMatch = true; }
    for (const edge of edges) if (edge.source === edge.target && !has(edge, "disabled")) edge.from.highlightKinds.add("selfReference");
    const graph = {nodes, byID, edges, stacks, colors, atlasColors: colors, incoming, outgoing, meta: raw.meta || {}};
    graph.pdfLayout = normalizePDFLayout(raw.pdfLayout, byID);
    graph.pdfColors = new Map();
    if (graph.pdfLayout) for (const stack of stacks) {
      const all = nodes.filter(node => node.stack === stack), ordinary = all.filter(node => has(node, "card") && !["source", "sink", "isolated", "virtual", "stack"].some(flag => has(node, flag)));
      const counts = new Map();
      for (const node of ordinary.length ? ordinary : all) {
        const drawing = graph.pdfLayout.nodes.get(node.id), color = drawing.fill && !/^(none|transparent)$/i.test(drawing.fill) ? drawing.fill : drawing.stroke;
        if (color) { const key = color.toLowerCase(); counts.set(key, (counts.get(key) || 0) + 1); }
      }
      graph.pdfColors.set(stack, [...counts].sort((a, b) => b[1] - a[1])[0]?.[0] || colors.get(stack));
    }
    return graph;
  }

  function matchesHighlights(node, selected) {
    return selected.size === 0 || [...selected].some(kind => node.highlightKinds.has(kind));
  }

  function highlightCounts(nodes, ages, selected) {
    const visible = nodes.filter(node => ages.has(node.stack));
    return {visible: visible.length, matched: visible.filter(node => matchesHighlights(node, selected)).length};
  }

  // Graphviz's `inv` marker has its wide end at the card boundary and points
  // back along the connection. Both ends mark saved-return entries in the PDF.
  function arrowPolygon(x, y, angle, size, inverted = false) {
    const along = {x: Math.cos(angle) * size, y: Math.sin(angle) * size}, across = {x: -Math.sin(angle) * size * .44, y: Math.cos(angle) * size * .44};
    const base = inverted ? {x, y} : {x: x - along.x, y: y - along.y};
    return [inverted ? [x - along.x, y - along.y] : [x, y], [base.x + across.x, base.y + across.y], [base.x - across.x, base.y - across.y]];
  }

  function grayColor(value, role = "stroke", mode = "pdf") {
    const text = String(value || "").toLowerCase();
    if (text === "transparent" || text === "none") return "#00000000";
    const names = {black: "#000000", white: "#ffffff", gray: "#808080", grey: "#808080", red: "#ff0000", blue: "#0000ff", green: "#008000"};
    const hex = (names[text] || text).match(/^#([a-f0-9]{3}|[a-f0-9]{4}|[a-f0-9]{6}|[a-f0-9]{8})$/i);
    let rgb = [128, 128, 128], alpha = "";
    if (hex) {
      const raw = hex[1].length <= 4 ? [...hex[1]].map(char => char + char).join("") : hex[1];
      rgb = [0, 2, 4].map(offset => parseInt(raw.slice(offset, offset + 2), 16)); alpha = raw.slice(6);
    } else {
      const match = text.match(/^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*([\d.]+))?\s*\)$/);
      if (match) { rgb = match.slice(1, 4).map(Number); if (match[4] != null) alpha = Math.round(clamp(Number(match[4]), 0, 1) * 255).toString(16).padStart(2, "0"); }
    }
    const luminance = .2126 * rgb[0] + .7152 * rgb[1] + .0722 * rgb[2];
    const level = mode === "atlas" ? role === "edge" ? 94 : Math.round(95 + luminance * .25) : role === "edge" ? Math.round(185 + luminance * .16) : role === "fill" ? Math.round(180 + luminance * .25) : Math.round(145 + luminance * .22);
    const channel = clamp(level, 0, 255).toString(16).padStart(2, "0");
    return "#" + channel.repeat(3) + alpha;
  }

  function operationBounds(operations) {
    let result = null, fontSize = 14;
    const add = (x, y) => {
      if (!Number.isFinite(x) || !Number.isFinite(y)) return;
      if (!result) result = {minX: x, minY: y, maxX: x, maxY: y};
      else { result.minX = Math.min(result.minX, x); result.minY = Math.min(result.minY, y); result.maxX = Math.max(result.maxX, x); result.maxY = Math.max(result.maxY, y); }
    };
    for (const operation of operations || []) {
      for (const point of operation.points || []) add(point[0], point[1]);
      if (operation.op === "F") fontSize = Math.max(1, finite(operation.size, 14));
      if (operation.pt && operation.op === "T") {
        const width = Math.max(0, finite(operation.width)), align = operation.align === "r" ? 1 : operation.align === "c" ? .5 : 0;
        add(operation.pt[0] - width * align, operation.pt[1] - fontSize * 1.3); add(operation.pt[0] + width * (1 - align), operation.pt[1] + fontSize * .4);
      }
      if (operation.rect && operation.rect.length >= 4) {
        const [x, y, width, height] = operation.rect;
        if (operation.op === "e" || operation.op === "E") { add(x - width, y - height); add(x + width, y + height); }
        else { add(x, y); add(x + width, y + height); }
      }
    }
    return result;
  }

  function normalizePDFLayout(input, byID) {
    if (!input || !Array.isArray(input.nodes) || input.edges != null && !Array.isArray(input.edges) || input.nodes.length !== byID.size || !byID.size) return null;
    const nodes = new Map();
    for (const node of input.nodes) {
      const id = Number(node.id);
      if (!byID.has(id) || nodes.has(id) || ![node.x, node.y, node.width, node.height].every(value => Number.isFinite(value)) || node.width <= 0 || node.height <= 0) return null;
      nodes.set(id, {...node, id});
    }
    const edges = [];
    for (const edge of input.edges || []) {
      if (!byID.has(Number(edge.source)) || !byID.has(Number(edge.target))) return null;
      const operations = [...(edge.draw || []), ...(edge.head || []), ...(edge.tail || []), ...(edge.labels || [])];
      edges.push({...edge, source: Number(edge.source), target: Number(edge.target), bounds: operationBounds(operations)});
    }
    return {...input, nodes, edges};
  }

  function pdfFont(face) {
    if (/times/i.test(face || "")) return '"Times New Roman", Georgia, serif';
    if (/courier/i.test(face || "")) return '"Courier New", monospace';
    if (/arial|helvetica/i.test(face || "")) return 'Arial, Helvetica, sans-serif';
    return String(face || "Arial").replace(/[^a-z0-9 ,_-]/gi, "") + ", sans-serif";
  }

  // Graphviz JSON draw opcodes (not raw xdot's B/b case convention):
  // https://graphviz.org/docs/outputs/json/ uses b for an unfilled edge spline.
  function replayOperations(ctx, operations, defaults = {}) {
    ctx.save();
    let size = 14, face = "Times-Roman", fontchar = 0;
    const mapped = (value, role) => defaults.colorMap ? defaults.colorMap(value, role) : value;
    ctx.strokeStyle = mapped(defaults.stroke || "#000000", "stroke"); ctx.fillStyle = mapped(defaults.fill || "#ffffff", "fill");
    ctx.lineWidth = Math.max(.01, finite(defaults.penWidth, 1)); ctx.lineCap = "butt"; ctx.lineJoin = "round"; ctx.setLineDash([]);
    function style(value) {
      const width = /setlinewidth\(\s*([0-9.]+)\s*\)/.exec(value || "");
      if (width) ctx.lineWidth = Math.max(.01, finite(width[1], 1));
      if (/\bdashed\b/.test(value)) ctx.setLineDash([6, 6]);
      else if (/\bdotted\b/.test(value)) { ctx.setLineDash([1, 5]); ctx.lineCap = "round"; }
      else if (/\bsolid\b/.test(value)) { ctx.setLineDash([]); ctx.lineCap = "butt"; }
      if (/\bbold\b/.test(value) && !width) ctx.lineWidth = 2;
    }
    function color(operation, role) {
      let result = mapped(operation.color === "none" ? "transparent" : operation.color, role);
      if (operation.grad === "linear" && operation.p0 && operation.p1) result = ctx.createLinearGradient(...operation.p0.slice(0, 2), ...operation.p1.slice(0, 2));
      else if (operation.grad === "radial" && operation.p0 && operation.p1) result = ctx.createRadialGradient(...operation.p0.slice(0, 3), ...operation.p1.slice(0, 3));
      if (result && typeof result.addColorStop === "function") for (const stop of operation.stops || []) result.addColorStop(clamp(finite(stop.frac), 0, 1), mapped(stop.color === "none" ? "transparent" : stop.color, role));
      return result || "#000000";
    }
    style(defaults.style || "");
    for (const operation of operations || []) {
      const points = operation.points || [], rect = operation.rect || [];
      switch (operation.op) {
        case "c": ctx.strokeStyle = color(operation, "stroke"); break;
        case "C": ctx.fillStyle = color(operation, "fill"); break;
        case "S": style(operation.style || ""); break;
        case "F": size = Math.max(.1, finite(operation.size, 14)); face = operation.face || "Times-Roman"; break;
        case "t": fontchar = finite(operation.fontchar); break;
        case "b": case "B":
          if (points.length < 2) break;
          ctx.beginPath(); ctx.moveTo(points[0][0], points[0][1]);
          for (let i = 1; i + 2 < points.length; i += 3) ctx.bezierCurveTo(...points[i], ...points[i + 1], ...points[i + 2]);
          if (operation.op === "B") { ctx.closePath(); ctx.fill(); }
          ctx.stroke(); break;
        case "p": case "P": case "L":
          if (points.length < 2) break;
          ctx.beginPath(); ctx.moveTo(...points[0]); for (let i = 1; i < points.length; i++) ctx.lineTo(...points[i]);
          if (operation.op !== "L") ctx.closePath();
          if (operation.op === "P") ctx.fill(); ctx.stroke(); break;
        case "e": case "E":
          if (rect.length < 4) break;
          ctx.beginPath(); ctx.ellipse(rect[0], rect[1], Math.abs(rect[2]), Math.abs(rect[3]), 0, 0, Math.PI * 2);
          if (operation.op === "E") ctx.fill(); ctx.stroke(); break;
        case "T": {
          if (!operation.pt) break;
          const text = String(operation.text || ""), width = Math.max(0, finite(operation.width));
          ctx.save(); ctx.fillStyle = ctx.strokeStyle; ctx.font = (fontchar & 2 ? "italic " : "") + (fontchar & 1 ? "bold " : "") + size + "px " + pdfFont(face);
          ctx.textAlign = operation.align === "r" ? "right" : operation.align === "l" ? "left" : "center"; ctx.textBaseline = "alphabetic";
          const cacheKey = ctx.font + "\n" + text;
          let measured = defaults.metrics && defaults.metrics.get(cacheKey);
          if (measured == null) { measured = ctx.measureText(text).width; if (defaults.metrics) defaults.metrics.set(cacheKey, measured); }
          ctx.translate(operation.pt[0], operation.pt[1]);
          if (width > 0 && measured > 0) ctx.scale(width / measured, 1);
          ctx.fillText(text, 0, 0);
          if (fontchar & (4 | 32 | 64)) {
            const actual = measured || width, x = operation.align === "r" ? -actual : operation.align === "l" ? 0 : -actual / 2;
            ctx.strokeStyle = ctx.fillStyle; ctx.lineWidth = Math.max(.5, size / 14); ctx.setLineDash([]);
            for (const [flag, y] of [[4, size * .12], [32, -size * .3], [64, -size * .85]]) if (fontchar & flag) { ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + actual, y); ctx.stroke(); }
          }
          ctx.restore(); break;
        }
      }
    }
    ctx.restore();
  }

  const worldPoint = (camera, point) => ({x: (point.x - camera.x) / camera.scale, y: (point.y - camera.y) / camera.scale});
  const screenPoint = (camera, point) => ({x: point.x * camera.scale + camera.x, y: point.y * camera.scale + camera.y});
  function zoomAt(camera, factor, point, minScale = .0001, maxScale = 5) {
    const world = worldPoint(camera, point), scale = clamp(camera.scale * factor, minScale, maxScale);
    return {scale, x: point.x - world.x * scale, y: point.y - world.y * scale};
  }
  function boundsFor(nodes) {
    if (!nodes.length) return {minX: -100, minY: -100, maxX: 100, maxY: 100};
    return nodes.reduce((bounds, node) => ({minX: Math.min(bounds.minX, node.x - node.width / 2), minY: Math.min(bounds.minY, node.y - node.height / 2), maxX: Math.max(bounds.maxX, node.x + node.width / 2), maxY: Math.max(bounds.maxY, node.y + node.height / 2)}), {minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity});
  }
  function fitCamera(bounds, width, height, leftInset = 0) {
    const scale = Math.min((Math.max(100, width - leftInset) - 90) / Math.max(100, bounds.maxX - bounds.minX), (Math.max(120, height) - 100) / Math.max(100, bounds.maxY - bounds.minY), 1.25);
    return {scale: Math.max(.0001, scale), x: leftInset + (width - leftInset) / 2 - (bounds.minX + bounds.maxX) * scale / 2, y: height / 2 - (bounds.minY + bounds.maxY) * scale / 2};
  }
  function intersects(node, box) {
    return node.x + node.width / 2 >= box.minX && node.x - node.width / 2 <= box.maxX && node.y + node.height / 2 >= box.minY && node.y - node.height / 2 <= box.maxY;
  }
  function searchNodes(nodes, query, limit = 20) {
    const terms = String(query).trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    return nodes.filter(node => terms.every(term => node.searchText.includes(term))).map(node => {
      const cardID = node.name.match(/:(\d+)$/);
      const exact = node.name.toLowerCase() === terms.join(" ") || cardID && cardID[1] === terms.join(" ");
      return {node, score: exact ? 0 : node.originalName.toLowerCase() === terms.join(" ") ? 1 : 2};
    }).sort((a, b) => a.score - b.score || a.node.name.localeCompare(b.node.name, undefined, {numeric: true})).slice(0, limit).map(result => result.node);
  }
  function edgeLabel(edge) {
    const labels = [...edge.flags].filter(flag => ATTRIBUTE_LABELS[flag]).map(flag => ATTRIBUTE_LABELS[flag]);
    if (has(edge, "restrictivetransitivitytail") || has(edge, "restrictivetransitivityhead")) {
      if (edge.transitivityId != null) labels.push("ID " + edge.transitivityId);
    }
    return labels.join(" · ") || "Ordinary reference";
  }
  function clippedZone(zone, width, height) {
    const x = clamp(zone.x, 0, width), y = clamp(zone.y, 0, height);
    return {x, y, width: Math.max(0, Math.min(width, zone.x + zone.width) - x), height: Math.max(0, Math.min(height, zone.y + zone.height) - y)};
  }
  function viewGeometry(node) {
    const width = finite(node.viewWidth) > 0 ? Number(node.viewWidth) : finite(node.imageWidth) > 0 ? Number(node.imageWidth) : 544;
    const height = finite(node.viewHeight) > 0 ? Number(node.viewHeight) : finite(node.imageHeight) > 0 ? Number(node.imageHeight) : 332;
    const imageRatio = finite(node.imageWidth) > 0 && finite(node.imageHeight) > 0 ? node.imageWidth / node.imageHeight : width / height;
    return {width, height, aligned: Math.abs(imageRatio / (width / height) - 1) < .04};
  }

  function createApp(document, window, raw) {
    const graph = normalizeGraph(raw), $ = id => document.getElementById(id), canvas = $("graph-canvas"), ctx = canvas.getContext("2d", {alpha: true});
    const state = {camera: {x: 0, y: 0, scale: 1}, width: 800, height: 600, minScale: .001, mode: graph.pdfLayout ? "pdf" : "atlas", pdfImages: true, selected: null, hovered: null, ages: new Set(graph.stacks), highlights: new Set(), highlightCount: {visible: graph.nodes.length, matched: graph.nodes.length}, zones: true, hiddenZones: false, zone: null, dialog: false, search: [], searchIndex: -1};
    const pointers = new Map(), ageInputs = new Map(), highlightInputs = new Map(), highlightLabels = new Map(), cache = new Map(), grayCache = new Map(), imageQueue = [], zoneRectangles = new Map(), pdfTextMetrics = new Map();
    const MAX_GRAY_BUFFERS = 128, MAX_GRAY_PIXELS = 2000000, MAX_GRAY_EDGE = 192;
    let animation = null, activeImages = 0, gesture = null, drag = null, multiTouch = false, frameID = 0, grayPixels = 0, neededGrayImages = new Set();
    const requestFrame = window.requestAnimationFrame ? window.requestAnimationFrame.bind(window) : callback => window.setTimeout(callback, 16);
    const visible = node => state.ages.has(node.stack);
    const muted = node => state.highlights.size > 0 && !node.highlightMatch;
    const edgeMuted = edge => state.highlights.size > 0 && !graph.byID.get(edge.source).highlightMatch && !graph.byID.get(edge.target).highlightMatch;
    const element = (tag, className, text) => { const node = document.createElement(tag); if (className) node.className = className; if (text != null) node.textContent = String(text); return node; };
    const announce = text => { $("sr-announcement").textContent = text; };
    const point = event => { const rect = canvas.getBoundingClientRect(); return {x: event.clientX - rect.left, y: event.clientY - rect.top}; };
    const schedule = () => { if (animation == null) animation = requestFrame(() => { animation = null; draw(); }); };

    function resize(initial = false) {
      const rect = canvas.getBoundingClientRect();
      const oldWidth = state.width, oldHeight = state.height;
      state.width = Math.max(1, rect.width); state.height = Math.max(1, rect.height);
      const dpr = Math.min(2.5, window.devicePixelRatio || 1);
      canvas.width = Math.round(state.width * dpr); canvas.height = Math.round(state.height * dpr);
      if (initial) fit(); else { state.camera.x += (state.width - oldWidth) / 2; state.camera.y += (state.height - oldHeight) / 2; schedule(); }
    }
    function fit() {
      const nodes = graph.nodes.filter(visible);
      let bounds = boundsFor(nodes);
      if (state.mode === "pdf" && state.ages.size === graph.stacks.length) {
        const source = graph.pdfLayout.bounds;
        if (source && [source.minX, source.minY, source.maxX, source.maxY].every(Number.isFinite) && source.maxX > source.minX && source.maxY > source.minY) bounds = source;
      }
      state.camera = fitCamera(bounds, state.width, state.height, state.width > 520 ? 190 : 110);
      state.minScale = Math.min(.02, state.camera.scale * .25); schedule();
    }
    function zoom(factor, anchor = {x: state.width / 2, y: state.height / 2}) {
      state.camera = zoomAt(state.camera, factor, anchor, state.minScale, state.mode === "pdf" ? 12 : 5); schedule();
    }
    function focusNode(node) {
      state.camera.scale = state.mode === "pdf" ? clamp(Math.max(state.camera.scale, 220 / node.width), state.minScale, 8) : clamp(Math.max(state.camera.scale, 1), state.minScale, 2.5);
      state.camera.x = (state.width + (state.width > 520 ? 170 : 0)) / 2 - node.x * state.camera.scale;
      state.camera.y = state.height / 2 - node.y * state.camera.scale; schedule();
    }
    function setLayout(mode, refit = true) {
      if (mode !== "pdf" || !graph.pdfLayout) mode = "atlas";
      state.mode = mode; state.hovered = null; pointers.clear(); drag = gesture = null; canvas.classList.remove("is-dragging");
      for (const node of graph.nodes) {
        const geometry = mode === "pdf" ? graph.pdfLayout.nodes.get(node.id) : node.atlas;
        node.x = geometry.x; node.y = geometry.y; node.width = geometry.width; node.height = geometry.height;
      }
      graph.colors = mode === "pdf" ? graph.pdfColors : graph.atlasColors;
      for (const node of graph.nodes) node.color = graph.colors.get(node.stack);
      $("map-surface").classList.toggle("is-pdf", mode === "pdf"); $("map-surface").classList.toggle("has-layouts", !!graph.pdfLayout);
      $("layout-tools").hidden = !graph.pdfLayout; $("pdf-images-control").hidden = mode !== "pdf";
      $("pdf-layout").setAttribute("aria-pressed", String(mode === "pdf")); $("atlas-layout").setAttribute("aria-pressed", String(mode === "atlas"));
      $("atlas-legend").hidden = mode === "pdf"; $("pdf-legend").hidden = mode !== "pdf";
      canvas.setAttribute("aria-label", (mode === "pdf" ? "Shared PDF graph layout." : "Image atlas layout.") + " Drag to pan, scroll to zoom. Search or use the incoming and outgoing references to select a card.");
      renderFilters();
      updateHighlightCounts();
      if (state.selected) { renderReferences(state.selected, "incoming"); renderReferences(state.selected, "outgoing"); }
      if (!$("search-results").hidden && $("graph-search").value.trim()) renderSearch();
      if (refit) fit();
    }
    function imageFor(url) {
      if (!url) return null;
      if (cache.has(url)) { const entry = cache.get(url); entry.lastSeen = frameID; cache.delete(url); cache.set(url, entry); return entry; }
      const entry = {url, image: null, loaded: false, failed: false, lastSeen: frameID};
      cache.set(url, entry); imageQueue.push(entry); pumpImages();
      return entry;
    }
    function releaseGray(entry) {
      if (!entry.gray) return;
      grayPixels -= entry.gray.width * entry.gray.height; entry.gray.width = entry.gray.height = 0;
      entry.gray = null; grayCache.delete(entry.url);
    }
    function grayImage(entry) {
      if (entry.gray) return entry.gray;
      if (entry.grayFailed || !entry.loaded || !entry.image.naturalWidth) return null;
      const ratio = Math.min(1, MAX_GRAY_EDGE / Math.max(entry.image.naturalWidth, entry.image.naturalHeight));
      const width = Math.max(1, Math.round(entry.image.naturalWidth * ratio)), height = Math.max(1, Math.round(entry.image.naturalHeight * ratio));
      for (const old of grayCache.values()) {
        if (grayCache.size < MAX_GRAY_BUFFERS && grayPixels + width * height <= MAX_GRAY_PIXELS) break;
        if (!neededGrayImages.has(old.url)) releaseGray(old);
      }
      if (grayCache.size >= MAX_GRAY_BUFFERS || grayPixels + width * height > MAX_GRAY_PIXELS) return null;
      const buffer = document.createElement("canvas"); buffer.width = width; buffer.height = height;
      try {
        const context = buffer.getContext("2d");
        if (!context) throw new Error("Offscreen canvas unavailable");
        context.drawImage(entry.image, 0, 0, width, height);
        // Compositing works with file:// images even when pixel reads are
        // prohibited. No getImageData, putImageData, or ctx.filter is used.
        context.globalCompositeOperation = "saturation";
        if (context.globalCompositeOperation !== "saturation") throw new Error("Saturation compositing unavailable");
        context.fillStyle = "#808080"; context.fillRect(0, 0, width, height);
        context.globalCompositeOperation = "destination-in"; context.drawImage(entry.image, 0, 0, width, height);
        context.globalCompositeOperation = "source-over";
      } catch (_) {
        entry.grayFailed = true; buffer.width = buffer.height = 0; return null;
      }
      entry.gray = buffer; grayPixels += width * height; grayCache.set(entry.url, entry); return buffer;
    }
    function drawableImage(entry, isMuted) { return isMuted ? grayImage(entry) : entry.image; }
    function wantsThumbnail(node) {
      return node.thumbnail && (state.mode === "pdf" ? state.pdfImages && node.width * state.camera.scale >= 95 && node.height * state.camera.scale >= 30 && node.width > 38 && node.height > 12 : node.height > 80 && node.width * state.camera.scale >= 86);
    }
    function pumpImages() {
      while (activeImages < 4 && imageQueue.length) {
        const entry = imageQueue.shift();
        if (entry.lastSeen !== frameID) { if (cache.get(entry.url) === entry) cache.delete(entry.url); continue; }
        activeImages++;
        const image = new window.Image(); entry.image = image;
        const finish = () => { activeImages--; schedule(); pumpImages(); };
        image.onload = () => { entry.loaded = true; finish(); };
        image.onerror = () => { entry.failed = true; finish(); };
        image.src = entry.url;
      }
    }
    function roundedRect(x, y, width, height, radius) {
      ctx.beginPath(); ctx.moveTo(x + radius, y); ctx.lineTo(x + width - radius, y); ctx.quadraticCurveTo(x + width, y, x + width, y + radius); ctx.lineTo(x + width, y + height - radius); ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height); ctx.lineTo(x + radius, y + height); ctx.quadraticCurveTo(x, y + height, x, y + height - radius); ctx.lineTo(x, y + radius); ctx.quadraticCurveTo(x, y, x + radius, y); ctx.closePath();
    }
    function fitText(text, width) {
      text = String(text);
      if (ctx.measureText(text).width <= width) return text;
      while (text.length > 1 && ctx.measureText(text + "…").width > width) text = text.slice(0, -1);
      return text + "…";
    }
    function edgeAppearance(edge, selected) {
      const related = selected && (edge.source === selected.id || edge.target === selected.id);
      let color = "#728268", dash = [], alpha = selected ? (related ? .95 : .09) : .35;
      if (has(edge, "crossage")) color = edge.from.color;
      if (has(edge, "restrictivetransitivityhead") || has(edge, "restrictivetransitivitytail")) color = "#c5a269";
      if (has(edge, "backtracking")) color = "#ac94bf";
      if (has(edge, "conditional") || has(edge, "zip")) { color = "#7aada3"; dash = [7, 5]; }
      if (has(edge, "disabled")) { color = "#8d8875"; dash = [5, 5]; alpha *= .7; }
      if (state.highlights.size) {
        if (edgeMuted(edge)) { color = grayColor(color, "edge", "atlas"); alpha = 1; }
        else alpha = has(edge, "disabled") ? .65 : .9;
      }
      return {color, dash, alpha, related};
    }
    function arrow(x, y, angle, size, edge, tail = false) {
      if (!tail && has(edge, "restrictivetransitivitytail")) return;
      ctx.beginPath();
      if (!tail && has(edge, "disabled")) {
        ctx.moveTo(x + Math.sin(angle) * size * .6, y - Math.cos(angle) * size * .6); ctx.lineTo(x - Math.sin(angle) * size * .6, y + Math.cos(angle) * size * .6); ctx.stroke();
      } else {
        const points = arrowPolygon(x, y, angle, size, has(edge, "backtracking"));
        ctx.moveTo(...points[0]); ctx.lineTo(...points[1]); ctx.lineTo(...points[2]); ctx.closePath(); ctx.fill();
      }
    }
    function drawEdge(edge, viewport) {
      const a = edge.from, b = edge.to;
      if (!visible(a) || !visible(b)) return;
      const margin = 150 + Math.abs(edge.parallel) * 15;
      if (Math.max(a.x, b.x) + margin < viewport.minX || Math.min(a.x, b.x) - margin > viewport.maxX || Math.max(a.y, b.y) + margin < viewport.minY || Math.min(a.y, b.y) - margin > viewport.maxY) return;
      const appearance = edgeAppearance(edge, state.selected), scale = state.camera.scale;
      ctx.strokeStyle = ctx.fillStyle = appearance.color; ctx.globalAlpha = appearance.alpha; ctx.lineWidth = (appearance.related ? 1.5 : .7) / Math.max(.4, scale); ctx.setLineDash(appearance.dash.map(value => value / Math.max(.6, scale)));
      if (a.id === b.id) {
        const radius = 24 + Math.abs(edge.parallel) * 9, x = a.x + a.width * .28, y = a.y - a.height / 2;
        ctx.beginPath(); ctx.moveTo(x - 12, y); ctx.bezierCurveTo(x - radius, y - radius * 2, x + radius * 2, y - radius * 2, x + 16, y); ctx.stroke();
        if (scale > .24 || appearance.related) {
          const size = 6 / Math.max(.6, scale);
          arrow(x + 16, y, Math.atan2(radius * 2, 16 - radius * 2), size, edge);
          if (has(edge, "backtracking")) arrow(x - 12, y, Math.atan2(radius * 2, radius - 12), size, edge, true);
        }
        return;
      }
      const dx = b.x - a.x, dy = b.y - a.y, length = Math.hypot(dx, dy) || 1;
      // One offset per original record, including reverse and duplicate records.
      const direction = a.id <= b.id ? 1 : -1, offset = edge.parallel * 13 * direction;
      const control = {x: (a.x + b.x) / 2 - dy / length * offset, y: (a.y + b.y) / 2 + dx / length * offset};
      const boundary = (node, toward) => { const dx = toward.x - node.x, dy = toward.y - node.y; const ratio = Math.min(node.width / 2 / (Math.abs(dx) || .001), node.height / 2 / (Math.abs(dy) || .001)); return {x: node.x + dx * ratio, y: node.y + dy * ratio}; };
      const start = boundary(a, control), end = boundary(b, control);
      ctx.beginPath(); ctx.moveTo(start.x, start.y); ctx.quadraticCurveTo(control.x, control.y, end.x, end.y); ctx.stroke();
      if (scale > .24 || appearance.related) {
        const size = 6 / Math.max(.6, scale);
        arrow(end.x, end.y, Math.atan2(end.y - control.y, end.x - control.x), size, edge);
        if (has(edge, "backtracking")) arrow(start.x, start.y, Math.atan2(start.y - control.y, start.x - control.x), size, edge, true);
      }
    }
    function drawNode(node) {
      const scale = state.camera.scale, selected = state.selected && node.id === state.selected.id, hover = node.id === state.hovered, isMuted = muted(node);
      const x = node.x - node.width / 2, y = node.y - node.height / 2;
      ctx.globalAlpha = state.highlights.size ? 1 : state.selected && !selected && !isNeighbor(node.id) ? .38 : 1;
      ctx.setLineDash([]);
      if (node.width * scale < 18) {
        ctx.fillStyle = isMuted ? "#777777" : selected ? "#f4d790" : node.color; ctx.beginPath(); ctx.arc(node.x, node.y, Math.max(1.6 / scale, 3), 0, Math.PI * 2); ctx.fill();
        if (isMuted && selected) { ctx.strokeStyle = "#d6b879"; ctx.lineWidth = 1.5 / scale; ctx.stroke(); } return;
      }
      roundedRect(x, y, node.width, node.height, 5); ctx.fillStyle = isMuted ? "#2e2e2e" : selected ? "#3a3c29" : "#253026"; ctx.fill();
      ctx.strokeStyle = selected ? "#f0d28a" : isMuted ? hover ? "#aaaaaa" : "#747474" : hover ? "#d4d6b6" : node.color; ctx.lineWidth = (selected ? 2.2 : .8) / Math.max(.6, scale);
      if (has(node, "virtual")) ctx.setLineDash([4, 4]); ctx.stroke(); ctx.setLineDash([]);
      ctx.fillStyle = isMuted ? "#888888" : node.color; ctx.fillRect(x + 5, y + 5, 2, 12);
      if (node.width * scale < 60) return;
      ctx.font = "11px -apple-system, BlinkMacSystemFont, sans-serif"; ctx.textAlign = "left"; ctx.textBaseline = "top"; ctx.fillStyle = isMuted ? "#a0a0a0" : "#d4ddc6";
      ctx.fillText(fitText(node.name, node.width - 21), x + 12, y + 5);
      if (node.height > 80 && node.width * scale >= 86) {
        const top = y + 24, width = node.width - 12, height = node.height - 46, entry = imageFor(node.thumbnail);
        ctx.fillStyle = isMuted ? "#222222" : "#131c15"; ctx.fillRect(x + 6, top, width, height);
        if (entry && entry.loaded && entry.image.naturalWidth) {
          const ratio = Math.min(width / entry.image.naturalWidth, height / entry.image.naturalHeight), iw = entry.image.naturalWidth * ratio, ih = entry.image.naturalHeight * ratio;
          const image = drawableImage(entry, isMuted);
          if (image) ctx.drawImage(image, x + 6 + (width - iw) / 2, top + (height - ih) / 2, iw, ih);
          else { ctx.font = "9px -apple-system, sans-serif"; ctx.fillStyle = "#777777"; ctx.textAlign = "center"; ctx.fillText("Picture muted", node.x, top + height / 2 - 4); ctx.textAlign = "left"; }
        } else {
          ctx.font = "9px -apple-system, sans-serif"; ctx.fillStyle = isMuted ? "#777777" : "#77866c"; ctx.textAlign = "center";
          ctx.fillText(node.thumbnail ? entry && entry.failed ? "Picture unavailable" : "Loading picture…" : "No local picture", node.x, top + height / 2 - 4); ctx.textAlign = "left";
        }
      }
      ctx.font = "9px -apple-system, sans-serif"; ctx.fillStyle = isMuted ? "#777777" : "#97a78a";
      ctx.fillText(fitText(node.assetName || node.originalName || node.stack, node.width - 15), x + 7, y + node.height - 15);
    }
    function isNeighbor(id) {
      if (!state.selected) return false;
      return graph.incoming.get(state.selected.id).some(edge => edge.source === id) || graph.outgoing.get(state.selected.id).some(edge => edge.target === id);
    }
    function drawPDFEdge(edge, viewport) {
      if (!visible(graph.byID.get(edge.source)) || !visible(graph.byID.get(edge.target))) return;
      const bounds = edge.bounds;
      if (bounds && (bounds.maxX < viewport.minX || bounds.minX > viewport.maxX || bounds.maxY < viewport.minY || bounds.minY > viewport.maxY)) return;
      const defaults = {stroke: edge.color, fill: edge.color, penWidth: edge.penWidth, style: edge.style, metrics: pdfTextMetrics};
      if (edgeMuted(edge)) defaults.colorMap = color => grayColor(color, "edge", "pdf");
      for (const operations of [edge.draw, edge.head, edge.tail, edge.labels]) replayOperations(ctx, operations, defaults);
    }
    function drawPDFNode(node) {
      const drawing = graph.pdfLayout.nodes.get(node.id), x = node.x - node.width / 2, y = node.y - node.height / 2, isMuted = muted(node);
      if (/\binvis\b/.test(drawing.style || "")) return;
      const defaults = {stroke: drawing.stroke, fill: drawing.fill, penWidth: drawing.penWidth, style: drawing.style, metrics: pdfTextMetrics};
      if (isMuted) defaults.colorMap = (color, role) => grayColor(color, role, "pdf");
      ctx.globalAlpha = 1;
      if (drawing.draw && drawing.draw.length) replayOperations(ctx, drawing.draw, defaults);
      else { roundedRect(x, y, node.width, node.height, Math.min(4, node.height / 3)); ctx.fillStyle = isMuted ? grayColor(drawing.fill, "fill") : drawing.fill || "#eeeeee"; ctx.fill(); ctx.strokeStyle = isMuted ? grayColor(drawing.stroke) : drawing.stroke || "#000000"; ctx.lineWidth = drawing.penWidth || 1; ctx.stroke(); }
      let compact = false;
      if (state.pdfImages && node.thumbnail && node.width * state.camera.scale >= 95 && node.height * state.camera.scale >= 30 && node.width > 38 && node.height > 12) {
        const entry = imageFor(node.thumbnail);
        const image = entry && entry.loaded && entry.image.naturalWidth && drawableImage(entry, isMuted);
        if (image) {
          const thumbWidth = Math.min(42, node.width * .29, (node.height - 6) * 1.5), thumbHeight = node.height - 6;
          const ratio = Math.min(thumbWidth / entry.image.naturalWidth, thumbHeight / entry.image.naturalHeight), width = entry.image.naturalWidth * ratio, height = entry.image.naturalHeight * ratio;
          ctx.save(); ctx.beginPath(); ctx.rect(x + 2, y + 2, node.width - 4, node.height - 4); ctx.clip();
          ctx.drawImage(image, x + 4 + (thumbWidth - width) / 2, y + 3 + (thumbHeight - height) / 2, width, height);
          const textOperations = (drawing.labels || []).filter(operation => operation.op === "T");
          const lines = textOperations.length ? textOperations.map(operation => String(operation.text || "")) : [node.name, node.assetName || node.originalName].filter(Boolean);
          const font = (drawing.labels || []).find(operation => operation.op === "F"), base = Math.min(finite(font && font.size, 10), (node.height - 5) / Math.max(1, lines.length) / 1.15), family = pdfFont(font && font.face);
          const textX = x + thumbWidth + 9, textWidth = node.width - thumbWidth - 13;
          // The box was sized for its label alone. Beside a thumbnail, shrink the label
          // before cutting it, so that card IDs stay readable; truncate only below 55%.
          ctx.font = base + "px " + family;
          const shrink = clamp(textWidth / Math.max(1, ...lines.map(line => ctx.measureText(line).width)), .55, 1), fitted = lines.map(line => fitText(line, textWidth / shrink));
          const size = base * shrink, lineHeight = size * 1.15;
          ctx.font = size + "px " + family; ctx.textAlign = "left"; ctx.textBaseline = "middle"; ctx.fillStyle = isMuted ? grayColor(drawing.fontColor || "#000000") : drawing.fontColor || "#000000";
          fitted.forEach((line, index) => ctx.fillText(line, textX, node.y + (index - (lines.length - 1) / 2) * lineHeight));
          ctx.restore(); compact = true;
        }
      }
      if (!compact) {
        if (drawing.labels && drawing.labels.length) replayOperations(ctx, drawing.labels, {...defaults, stroke: drawing.fontColor || "#000000"});
        else { ctx.font = "10px Arial, sans-serif"; ctx.fillStyle = isMuted ? grayColor(drawing.fontColor || "#000000") : drawing.fontColor || "#000000"; ctx.textAlign = "center"; ctx.textBaseline = "middle"; ctx.fillText(fitText(node.name, node.width - 6), node.x, node.y); }
      }
      const selected = state.selected && node.id === state.selected.id;
      if (selected || state.hovered === node.id) {
        ctx.save(); ctx.setLineDash([]); ctx.lineWidth = (selected ? 2 : 1) / state.camera.scale; ctx.strokeStyle = selected ? "#9c6f1a" : isMuted ? "#888888" : "#477a69";
        roundedRect(x - 3 / state.camera.scale, y - 3 / state.camera.scale, node.width + 6 / state.camera.scale, node.height + 6 / state.camera.scale, 3 / state.camera.scale); ctx.stroke(); ctx.restore();
      }
    }
    function draw() {
      if (!ctx) return;
      frameID++;
      const dpr = canvas.width / state.width, camera = state.camera;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, state.width, state.height);
      if (state.mode === "pdf") { ctx.globalAlpha = 1; ctx.fillStyle = "#ffffff"; ctx.fillRect(0, 0, state.width, state.height); }
      else {
        ctx.globalAlpha = .12; ctx.strokeStyle = "#7c8b67"; ctx.lineWidth = .5;
        const grid = 38, ox = ((camera.x % grid) + grid) % grid, oy = ((camera.y % grid) + grid) % grid;
        ctx.beginPath();
        for (let x = ox; x < state.width; x += grid) { ctx.moveTo(x, 0); ctx.lineTo(x, state.height); }
        for (let y = oy; y < state.height; y += grid) { ctx.moveTo(0, y); ctx.lineTo(state.width, y); }
        ctx.stroke();
      }
      const p1 = worldPoint(camera, {x: -30, y: -30}), p2 = worldPoint(camera, {x: state.width + 30, y: state.height + 30}), viewport = {minX: p1.x, minY: p1.y, maxX: p2.x, maxY: p2.y};
      ctx.setTransform(dpr * camera.scale, 0, 0, dpr * camera.scale, dpr * camera.x, dpr * camera.y);
      if (state.mode === "pdf") {
        replayOperations(ctx, graph.pdfLayout.draw, {metrics: pdfTextMetrics});
        for (const edge of graph.pdfLayout.edges) drawPDFEdge(edge, viewport);
      } else for (const edge of graph.edges) drawEdge(edge, viewport);
      const nodes = graph.nodes.filter(node => visible(node) && intersects(node, viewport));
      neededGrayImages = new Set(nodes.filter(node => muted(node) && wantsThumbnail(node)).map(node => node.thumbnail));
      const renderer = state.mode === "pdf" ? drawPDFNode : drawNode;
      for (const node of nodes) if (!state.selected || node.id !== state.selected.id) renderer(node);
      if (state.selected && visible(state.selected) && intersects(state.selected, viewport)) renderer(state.selected);
      if (state.mode === "pdf") replayOperations(ctx, graph.pdfLayout.labels, {metrics: pdfTextMetrics});
      // Keep every visible thumbnail. A hard cap smaller than the viewport's
      // node count would evict/reload those same images on every repaint.
      for (const [url, entry] of cache) {
        if (cache.size <= 96) break;
        if (entry.lastSeen !== frameID && (entry.loaded || entry.failed)) { releaseGray(entry); cache.delete(url); }
      }
      for (const entry of grayCache.values()) if (!neededGrayImages.has(entry.url)) releaseGray(entry);
      ctx.globalAlpha = 1; ctx.setLineDash([]);
      $("zoom-level").textContent = Math.round(camera.scale * 100) + "%";
      $("view-status").textContent = (state.highlights.size ? formatNumber(state.highlightCount.matched) + " / " + formatNumber(state.highlightCount.visible) + " highlighted" : formatNumber(state.highlightCount.visible) + " nodes") + " · " + (state.mode === "pdf" ? state.pdfImages ? "PDF layout · zoom for pictures" : "PDF layout · original labels" : "zoom in to reveal pictures");
      $("map-empty").hidden = state.ages.size !== 0 || !graph.nodes.length;
    }

    function renderFilters() {
      const host = $("age-filters"); host.replaceChildren(); ageInputs.clear();
      for (const stack of graph.stacks) {
        const label = element("label", "age-filter"), input = element("input"), name = element("span", "age-name", stack.replace(/ Age$/, "")), count = element("span", "age-count", graph.nodes.filter(node => node.stack === stack).length);
        label.style.setProperty("--age-color", graph.colors.get(stack)); input.type = "checkbox"; input.checked = state.ages.has(stack); input.setAttribute("aria-label", "Show " + stack);
        input.addEventListener("change", () => { if (input.checked) state.ages.add(stack); else state.ages.delete(stack); filtersChanged(); });
        label.append(input, name, count); host.append(label); ageInputs.set(stack, input);
      }
    }
    function filtersChanged() {
      for (const [stack, input] of ageInputs) input.checked = state.ages.has(stack);
      if (state.selected && !visible(state.selected)) clearSelection();
      updateHighlightCounts();
      fit();
    }
    function setAges(ages) { state.ages = new Set(ages.filter(age => graph.stacks.includes(age))); filtersChanged(); }
    function renderHighlightFilters() {
      const host = $("node-highlights"); host.replaceChildren();
      for (const kind of HIGHLIGHTS) {
        const label = element("label", "highlight-option"), input = element("input"), count = element("span", "highlight-category-count");
        input.type = "checkbox"; input.setAttribute("aria-label", "Highlight " + kind.label.toLowerCase()); input.setAttribute("aria-description", kind.description); label.title = kind.description;
        input.addEventListener("change", () => setHighlights([...highlightInputs].filter(([, control]) => control.checked).map(([id]) => id)));
        label.append(input, element("span", "highlight-name", kind.label), count); host.append(label); highlightInputs.set(kind.id, input); highlightLabels.set(kind.id, count);
      }
    }
    function updateHighlightCounts() {
      state.highlightCount = highlightCounts(graph.nodes, state.ages, state.highlights);
      const {visible: total, matched} = state.highlightCount;
      $("highlight-summary").textContent = state.highlights.size ? formatNumber(matched) + " / " + formatNumber(total) + " visible · match any" : "All " + formatNumber(total) + " visible nodes";
      $("highlight-count").textContent = formatNumber(matched) + " / " + formatNumber(total) + " visible";
      $("clear-highlights").disabled = !state.highlights.size;
      for (const kind of HIGHLIGHTS) {
        const input = highlightInputs.get(kind.id), label = highlightLabels.get(kind.id);
        if (input) input.checked = state.highlights.has(kind.id);
        if (label) label.textContent = graph.nodes.filter(node => visible(node) && node.highlightKinds.has(kind.id)).length;
      }
    }
    function setHighlights(kinds) {
      state.highlights = new Set(kinds.filter(id => HIGHLIGHTS.some(kind => kind.id === id)));
      for (const node of graph.nodes) node.highlightMatch = matchesHighlights(node, state.highlights);
      if (!state.highlights.size) for (const entry of grayCache.values()) releaseGray(entry);
      updateHighlightCounts();
      if (state.selected) { renderSelectedTags(state.selected); renderReferences(state.selected, "incoming"); renderReferences(state.selected, "outgoing"); }
      if (!$("search-results").hidden && $("graph-search").value.trim()) renderSearch();
      announce(state.highlights.size ? state.highlightCount.matched + " of " + state.highlightCount.visible + " visible nodes match any selected category. Other nodes remain gray and selectable." : "Node highlights cleared. All visible nodes retain their original colors.");
      schedule();
    }
    function renderSearch() {
      state.search = searchNodes(graph.nodes, $("graph-search").value); state.searchIndex = -1;
      const host = $("search-results"); host.replaceChildren();
      const query = $("graph-search").value.trim(); host.hidden = !query; $("graph-search").setAttribute("aria-expanded", query ? "true" : "false");
      if (!query) return;
      if (!state.search.length) { host.append(element("p", "search-empty", "No cards match. Try a card ID, original name, or picture name.")); return; }
      state.search.forEach((node, index) => {
        const button = element("button", "search-result"), dot = element("span", "result-dot"), copy = element("span", "result-copy");
        button.id = "search-result-" + index; button.setAttribute("role", "option"); button.setAttribute("aria-selected", "false"); dot.style.background = muted(node) ? "#888888" : node.color;
        copy.append(element("strong", "", node.name), element("small", "", node.assetName || node.originalName || "Card view")); button.append(dot, copy, element("span", "result-age", node.stack.replace(/ Age$/, "")));
        button.addEventListener("click", () => { selectNode(node.id, true); closeSearch(); }); host.append(button);
      });
    }
    function closeSearch() { $("search-results").hidden = true; $("graph-search").setAttribute("aria-expanded", "false"); $("graph-search").removeAttribute("aria-activedescendant"); }
    function searchKey(event) {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeSearch(); canvas.focus(); return; }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault(); if (!state.search.length) return;
        state.searchIndex = (state.searchIndex + (event.key === "ArrowDown" ? 1 : -1) + state.search.length) % state.search.length;
        [...$("search-results").children].forEach((child, index) => { child.classList.toggle("is-active", index === state.searchIndex); child.setAttribute("aria-selected", String(index === state.searchIndex)); });
        $("graph-search").setAttribute("aria-activedescendant", "search-result-" + state.searchIndex);
      } else if (event.key === "Enter" && state.search.length) {
        event.preventDefault(); selectNode(state.search[Math.max(0, state.searchIndex)].id, true); closeSearch(); canvas.focus();
      }
    }
    function fact(name, value) { if (!value) return; $("selected-facts").append(element("dt", "", name), element("dd", "", value)); }
    function renderReferences(node, direction) {
      const records = graph[direction].get(node.id), host = $(direction + "-list"); host.replaceChildren(); $(direction + "-count").textContent = records.length;
      if (!records.length) { host.append(element("p", "references-empty", "No extracted " + direction + " references.")); return; }
      for (const edge of records) {
        const other = direction === "incoming" ? edge.from : edge.to, button = element("button", "reference-row"), copy = element("span", "ref-copy"), dot = element("span", "ref-dot");
        copy.append(element("strong", "", other.name), element("small", "", edgeLabel(edge))); dot.style.background = muted(other) ? "#888888" : other.color;
        const inverseHead = has(edge, "restrictivetransitivitytail") ? "" : has(edge, "disabled") ? "⊣" : "◀";
        const marker = element("span", "ref-arrow" + (has(edge, "backtracking") ? " inverted" : ""), has(edge, "backtracking") ? "▶—" + inverseHead : direction === "incoming" ? "↙" : "↗");
        marker.setAttribute("aria-hidden", "true"); button.append(marker, copy, dot);
        button.setAttribute("aria-label", other.name + ", " + edgeLabel(edge)); button.addEventListener("click", () => selectNode(other.id, true)); host.append(button);
      }
    }
    function renderPicture(host, node) {
      host.replaceChildren(); zoneRectangles.set(host, []);
      const stage = element("div", "picture-stage"), {width, height, aligned} = viewGeometry(node);
      stage.style.setProperty("--view-ratio", width + " / " + height);
      const missing = element("div", "picture-placeholder"); missing.append(element("span", "picture-symbol", "▧"), element("span", "", node.image ? "Loading local picture…" : "No local picture for this card.")); stage.append(missing);
      if (node.image) {
        const image = element("img"); image.alt = node.assetName || node.originalName || node.name; image.decoding = "async";
        image.addEventListener("load", () => { missing.hidden = true; });
        image.addEventListener("error", () => { image.hidden = true; missing.replaceChildren(element("span", "picture-symbol", "▧"), element("span", "", "This local picture could not be loaded.")); });
        image.src = node.image; stage.append(image);
      }
      if (state.zones && node.zones.length && !aligned) stage.append(element("span", "alignment-note", "Zone alignment unavailable for this picture."));
      if (state.zones && node.zones.length && aligned) {
        const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg"); svg.setAttribute("class", "zone-overlay"); svg.setAttribute("viewBox", "0 0 " + width + " " + height); svg.setAttribute("preserveAspectRatio", "xMidYMid meet"); svg.setAttribute("aria-label", "Stored card interaction zones");
        for (const zone of node.zones) {
          if (zone.hidden && !state.hiddenZones) continue;
          const box = clippedZone(zone, width, height); if (!box.width || !box.height) continue;
          const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
          rect.setAttribute("class", "zone-rect" + (zone.hidden ? " is-hidden" : "") + (!zone.enabled ? " is-disabled" : "") + (state.zone && state.zone.key === zone.key ? " is-selected" : ""));
          for (const [key, value] of Object.entries(box)) rect.setAttribute(key, value);
          const label = (zone.name || "Part " + zone.id) + " · " + zone.layer + " " + zone.type + (zone.hidden ? " · stored hidden" : "") + (!zone.enabled ? " · stored disabled" : "");
          rect.setAttribute("role", "button"); rect.setAttribute("tabindex", "0"); rect.setAttribute("aria-label", label);
          zoneRectangles.get(host).push({rect, key: zone.key});
          const title = document.createElementNS("http://www.w3.org/2000/svg", "title"); title.textContent = label; rect.append(title);
          rect.addEventListener("pointerenter", () => showZone(zone, false)); rect.addEventListener("focus", () => showZone(zone, false));
          rect.addEventListener("pointerleave", () => { if (state.zone) showZone(state.zone, false); });
          rect.addEventListener("click", event => { event.stopPropagation(); showZone(zone, true); });
          rect.addEventListener("keydown", event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); showZone(zone, true); } }); svg.append(rect);
        }
        stage.append(svg);
      }
      host.append(stage);
    }
    function showZone(zone, pinned) {
      if (pinned) state.zone = zone;
      $("zone-detail").hidden = false; $("zone-name").textContent = zone.name || "Part " + zone.id;
      const meta = zone.layer + " " + zone.type + " · ID " + zone.id + (zone.hidden ? " · stored hidden" : "") + (!zone.enabled ? " · stored disabled" : "");
      $("zone-meta").textContent = meta; $("zone-script").textContent = zone.script || "No script stored on this part.";
      $("dialog-zone-name").textContent = zone.name || "Part " + zone.id; $("dialog-zone-meta").textContent = meta; $("dialog-zone-script").textContent = zone.script || "No script stored on this part.";
      if (pinned) for (const rectangles of zoneRectangles.values()) for (const item of rectangles) item.rect.classList.toggle("is-selected", item.key === zone.key);
    }
    function clearZone() {
      state.zone = null; $("zone-detail").hidden = true; $("dialog-zone-name").textContent = "Select a zone"; $("dialog-zone-meta").textContent = "Hover over a rectangle or select it to inspect its stored script."; $("dialog-zone-script").textContent = "";
      for (const rectangles of zoneRectangles.values()) for (const item of rectangles) item.rect.classList.remove("is-selected");
    }
    function updateZones() {
      state.zones = $("show-zones").checked; state.hiddenZones = $("show-hidden-zones").checked;
      $("hidden-zones-label").hidden = !state.zones; $("zones-note").hidden = !state.zones;
      $("dialog-show-zones").checked = state.zones; $("dialog-show-hidden-zones").checked = state.hiddenZones; $("dialog-hidden-zones-label").hidden = !state.zones;
      if (!state.zones) { state.zone = null; $("zone-detail").hidden = true; }
      if (state.selected) { renderPicture($("selected-picture"), state.selected); if (state.dialog) renderPicture($("dialog-picture"), state.selected); }
    }
    function renderSelectedTags(node) {
      $("selected-tags").replaceChildren();
      for (const flag of node.flags) if (NODE_LABELS[flag]) $("selected-tags").append(element("span", "tag" + (flag === "virtual" ? " warning" : ""), NODE_LABELS[flag]));
      if (muted(node)) $("selected-tags").append(element("span", "tag subdued", "Outside highlight"));
    }
    function selectNode(id, focus = false) {
      const node = graph.byID.get(Number(id)); if (!node) return;
      state.selected = node; state.zone = null;
      if (!visible(node)) { state.ages.add(node.stack); const input = ageInputs.get(node.stack); if (input) input.checked = true; }
      $("welcome-panel").hidden = true; $("selection-panel").hidden = false; $("zone-detail").hidden = true;
      $("selected-age").textContent = node.stack; $("selected-name").textContent = node.name;
      updateHighlightCounts(); renderSelectedTags(node);
      $("show-zones").disabled = !node.zones.length;
      $("dialog-show-zones").disabled = !node.zones.length;
      $("show-zones").checked = $("dialog-show-zones").checked = state.zones;
      $("show-hidden-zones").checked = $("dialog-show-hidden-zones").checked = state.hiddenZones;
      $("hidden-zones-label").hidden = $("dialog-hidden-zones-label").hidden = !state.zones;
      $("zones-note").hidden = !state.zones;
      $("zones-note").textContent = node.zones.length ? "Stored geometry only. Runtime visibility and behavior can change." : "No stored part rectangles were exported for this card.";
      renderPicture($("selected-picture"), node); $("selected-facts").replaceChildren();
      fact("Original", node.originalName); fact("Picture", node.assetName); fact("Age", node.stack);
      const cardID = node.name.match(/:(\d+)$/); fact("Card ID", cardID ? cardID[1] : has(node, "stack") ? "Stack abstraction" : "Not recorded");
      fact("View", finite(node.viewWidth) && finite(node.viewHeight) ? node.viewWidth + " × " + node.viewHeight : "");
      fact("Zones", node.zones.length ? String(node.zones.length) + " stored parts" : "None exported");
      if (has(node, "virtual")) fact("Status", "Referenced target absent from the imported data.");
      renderReferences(node, "incoming"); renderReferences(node, "outgoing");
      announce("Selected " + node.name + ", " + graph.incoming.get(node.id).length + " incoming and " + graph.outgoing.get(node.id).length + " outgoing reference records.");
      if (focus) focusNode(node); else schedule();
      if (state.dialog) openPicture();
    }
    function clearSelection() { state.selected = null; state.zone = null; $("selection-panel").hidden = true; $("welcome-panel").hidden = false; schedule(); }
    function openDialog(dialog) { if (typeof dialog.showModal === "function") { if (!dialog.open) dialog.showModal(); } else dialog.setAttribute("open", ""); }
    function closeDialog(dialog) { if (typeof dialog.close === "function") dialog.close(); else dialog.removeAttribute("open"); }
    function openPicture() {
      if (!state.selected) return; state.dialog = true;
      $("dialog-title").textContent = state.selected.name; $("dialog-age").textContent = state.selected.stack;
      renderPicture($("dialog-picture"), state.selected); if (state.zone) showZone(state.zone, false); else { $("dialog-zone-name").textContent = "Select a zone"; $("dialog-zone-meta").textContent = "Hover over a rectangle or select it to inspect its stored script."; $("dialog-zone-script").textContent = ""; }
      openDialog($("picture-dialog"));
    }
    function hitNode(position) {
      const world = worldPoint(state.camera, position);
      for (let i = graph.nodes.length - 1; i >= 0; i--) {
        const node = graph.nodes[i]; if (!visible(node)) continue;
        const padding = Math.min(30, 3 / state.camera.scale);
        if (Math.abs(world.x - node.x) <= node.width / 2 + padding && Math.abs(world.y - node.y) <= node.height / 2 + padding) return node;
      }
      return null;
    }
    function rebaseGesture() {
      if (pointers.size >= 2) {
        const [a, b] = [...pointers.values()]; const mid = {x: (a.x + b.x) / 2, y: (a.y + b.y) / 2};
        gesture = {distance: Math.max(1, Math.hypot(a.x - b.x, a.y - b.y)), world: worldPoint(state.camera, mid), scale: state.camera.scale}; drag = null; multiTouch = true;
      } else if (pointers.size === 1) { const p = [...pointers.values()][0]; drag = {start: p, camera: {...state.camera}, moved: true}; gesture = null; }
    }
    canvas.addEventListener("pointerdown", event => {
      if (event.button != null && event.button !== 0) return;
      event.preventDefault(); canvas.focus(); const position = point(event); pointers.set(event.pointerId, position);
      if (canvas.setPointerCapture) canvas.setPointerCapture(event.pointerId);
      if (pointers.size === 1) { drag = {start: position, camera: {...state.camera}, moved: false}; multiTouch = false; } else rebaseGesture();
      canvas.classList.add("is-dragging"); closeSearch();
    });
    canvas.addEventListener("pointermove", event => {
      const position = point(event);
      if (!pointers.has(event.pointerId)) {
        const node = hitNode(position), id = node ? node.id : null;
        if (id !== state.hovered) { state.hovered = id; canvas.style.cursor = node ? "pointer" : "grab"; schedule(); }
        return;
      }
      pointers.set(event.pointerId, position);
      if (pointers.size >= 2 && gesture) {
        const [a, b] = [...pointers.values()], mid = {x: (a.x + b.x) / 2, y: (a.y + b.y) / 2};
        const scale = clamp(gesture.scale * Math.hypot(a.x - b.x, a.y - b.y) / gesture.distance, state.minScale, state.mode === "pdf" ? 12 : 5);
        state.camera = {scale, x: mid.x - gesture.world.x * scale, y: mid.y - gesture.world.y * scale};
      } else if (drag) {
        const dx = position.x - drag.start.x, dy = position.y - drag.start.y;
        if (Math.hypot(dx, dy) > 4) drag.moved = true;
        state.camera.x = drag.camera.x + dx; state.camera.y = drag.camera.y + dy;
      }
      schedule();
    });
    function pointerEnd(event, cancelled = false) {
      const click = !cancelled && !multiTouch && pointers.size === 1 && drag && !drag.moved;
      pointers.delete(event.pointerId);
      if (click) { const node = hitNode(point(event)); if (node) selectNode(node.id); else clearSelection(); }
      if (pointers.size) rebaseGesture(); else { drag = gesture = null; canvas.classList.remove("is-dragging"); }
    }
    canvas.addEventListener("pointerup", event => pointerEnd(event)); canvas.addEventListener("pointercancel", event => pointerEnd(event, true));
    canvas.addEventListener("wheel", event => { event.preventDefault(); zoom(Math.exp(-clamp(event.deltaY, -350, 350) * .0018), point(event)); }, {passive: false});
    canvas.addEventListener("dblclick", event => { const p = point(event), node = hitNode(p); if (node) selectNode(node.id, true); else zoom(1.8, p); });
    $("graph-search").addEventListener("input", renderSearch); $("graph-search").addEventListener("keydown", searchKey);
    $("graph-search").addEventListener("focus", () => { if ($("graph-search").value.trim()) renderSearch(); });
    document.addEventListener("pointerdown", event => { if (!$("graph-search").parentNode.contains(event.target)) closeSearch(); });
    document.addEventListener("keydown", event => {
      if (typingTarget(document.activeElement)) return;
      if (event.key === "/") { event.preventDefault(); $("graph-search").focus(); return; }
      if (event.key === "Escape") { if (state.dialog) { closeDialog($("picture-dialog")); state.dialog = false; } else if ($("help-dialog").open) closeDialog($("help-dialog")); else clearSelection(); return; }
      if ($("picture-dialog").open || $("help-dialog").open) return;
      if (event.key === "+" || event.key === "=") { event.preventDefault(); zoom(1.3); }
      else if (event.key === "-" || event.key === "_") { event.preventDefault(); zoom(1 / 1.3); }
      else if (event.key.toLowerCase() === "f") { event.preventDefault(); fit(); }
      else if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) {
        event.preventDefault(); const step = event.shiftKey ? 110 : 45;
        state.camera.x += event.key === "ArrowLeft" ? step : event.key === "ArrowRight" ? -step : 0;
        state.camera.y += event.key === "ArrowUp" ? step : event.key === "ArrowDown" ? -step : 0; schedule();
      }
    });
    $("zoom-in").addEventListener("click", () => zoom(1.35)); $("zoom-out").addEventListener("click", () => zoom(1 / 1.35)); $("fit-graph").addEventListener("click", fit);
    $("pdf-layout").addEventListener("click", () => setLayout("pdf")); $("atlas-layout").addEventListener("click", () => setLayout("atlas"));
    $("pdf-images").addEventListener("change", () => { state.pdfImages = $("pdf-images").checked; schedule(); });
    $("home-link").addEventListener("click", event => { event.preventDefault(); fit(); });
    $("all-ages").addEventListener("click", () => setAges(graph.stacks)); $("no-ages").addEventListener("click", () => setAges([]));
    $("clear-highlights").addEventListener("click", () => setHighlights([]));
    $("clear-selection").addEventListener("click", clearSelection); $("show-zones").addEventListener("change", updateZones); $("show-hidden-zones").addEventListener("change", updateZones); $("clear-zone").addEventListener("click", clearZone);
    $("dialog-show-zones").addEventListener("change", () => { $("show-zones").checked = $("dialog-show-zones").checked; updateZones(); });
    $("dialog-show-hidden-zones").addEventListener("change", () => { $("show-hidden-zones").checked = $("dialog-show-hidden-zones").checked; updateZones(); });
    $("expand-picture").addEventListener("click", openPicture); $("close-picture").addEventListener("click", () => { closeDialog($("picture-dialog")); state.dialog = false; });
    $("picture-dialog").addEventListener("close", () => { state.dialog = false; });
    $("help-button").addEventListener("click", () => openDialog($("help-dialog"))); $("close-help").addEventListener("click", () => closeDialog($("help-dialog")));
    const dock = graph.nodes.find(node => /^Myst:8336$/i.test(node.name)); $("start-dock").disabled = !dock;
    $("start-dock").addEventListener("click", () => { if (dock) selectNode(dock.id, true); });
    $("card-count").textContent = formatNumber(graph.meta.cards == null ? graph.nodes.filter(node => has(node, "card") && !has(node, "virtual")).length : graph.meta.cards);
    $("graph-summary").textContent = formatNumber(graph.nodes.length) + " nodes · " + graph.stacks.length + " Ages";
    $("conditional-legend").hidden = !graph.edges.some(edge => has(edge, "conditional") || has(edge, "zip"));
    if (!graph.nodes.length) { $("welcome-panel").querySelector("h1").textContent = "No graph data."; $("view-status").textContent = "No nodes were exported"; }
    renderHighlightFilters(); setLayout(state.mode, false); resize(true);
    const initial = dock || graph.nodes.find(node => node.image);
    if (initial) selectNode(initial.id);
    if (typeof window.ResizeObserver === "function") new window.ResizeObserver(() => resize()).observe(canvas); else window.addEventListener("resize", () => resize());
    return {graph, state, selectNode, clearSelection, fit, zoom, setAges, setHighlights, setLayout, renderSearch, updateZones, showZone, draw, worldPoint: point => worldPoint(state.camera, point), screenPoint: point => screenPoint(state.camera, point), imageCache: cache, grayCache, grayBudget: {buffers: MAX_GRAY_BUFFERS, pixels: MAX_GRAY_PIXELS, edge: MAX_GRAY_EDGE}};
  }

  const api = {normalizeGraph, normalizePDFLayout, operationBounds, replayOperations, worldPoint, screenPoint, zoomAt, boundsFor, fitCamera, intersects, searchNodes, edgeLabel, clippedZone, viewGeometry, safeImagePath, typingTarget, matchesHighlights, highlightCounts, grayColor, arrowPolygon, HIGHLIGHTS, createApp};
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  if (global.document && global.document.getElementById("graph-canvas")) {
    global.DeMystifyAtlas = api;
    global.DeMystifyAtlas.app = createApp(global.document, global, global.DEMYSTIFY_GRAPH || {});
  }
})(typeof window !== "undefined" ? window : globalThis);
