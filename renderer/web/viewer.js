(function (global) {
  "use strict";

  const PALETTE = ["#b5b583", "#8daf91", "#b29abc", "#d0af76", "#85a9b3", "#bd8e79"];
  const ATTRIBUTE_LABELS = {disabled: "Disabled", backtracking: "Saved-return entry", crossage: "Between Ages", selfreference: "Self-reference", notimplemented: "Absent target", restrictivetransitivitytail: "Paired return · tail", restrictivetransitivityhead: "Paired return · head", conditional: "Conditional", zip: "Zip navigation"};
  const NODE_LABELS = {virtual: "Absent target", stack: "Stack", source: "Source flag", sink: "Sink flag", isolated: "Isolate", containsbluepage: "Blue page", containsredpage: "Red page", containswhitepage: "White page"};
  const finite = (value, fallback = 0) => Number.isFinite(Number(value)) ? Number(value) : fallback;
  const clamp = (value, low, high) => Math.max(low, Math.min(high, value));
  const attribute = value => String(value || "").replace(/[^a-z]/gi, "").toLowerCase().replace(/^is(?=card|stack|virtual|source|sink|isolated)/, "");
  const has = (object, name) => object.flags.has(name);
  const formatNumber = value => finite(value).toLocaleString("en-US");
  const safeImagePath = value => typeof value === "string" && value && !/^(?:[a-z][a-z0-9+.-]*:|\/\/|#)/i.test(value) && !value.includes("\0") ? value : null;

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
    return {nodes, byID, edges, stacks, colors, incoming, outgoing, meta: raw.meta || {}};
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
    const state = {camera: {x: 0, y: 0, scale: 1}, width: 800, height: 600, minScale: .001, selected: null, hovered: null, ages: new Set(graph.stacks), zones: true, hiddenZones: false, zone: null, dialog: false, search: [], searchIndex: -1};
    const pointers = new Map(), ageInputs = new Map(), cache = new Map(), imageQueue = [], zoneRectangles = new Map();
    let animation = null, activeImages = 0, gesture = null, drag = null, multiTouch = false, frameID = 0;
    const requestFrame = window.requestAnimationFrame ? window.requestAnimationFrame.bind(window) : callback => window.setTimeout(callback, 16);
    const visible = node => state.ages.has(node.stack);
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
      state.camera = fitCamera(boundsFor(nodes), state.width, state.height, state.width > 520 ? 190 : 110);
      state.minScale = Math.min(.02, state.camera.scale * .25); schedule();
    }
    function zoom(factor, anchor = {x: state.width / 2, y: state.height / 2}) {
      state.camera = zoomAt(state.camera, factor, anchor, state.minScale, 5); schedule();
    }
    function focusNode(node) {
      state.camera.scale = clamp(Math.max(state.camera.scale, 1), state.minScale, 2.5);
      state.camera.x = (state.width + (state.width > 520 ? 170 : 0)) / 2 - node.x * state.camera.scale;
      state.camera.y = state.height / 2 - node.y * state.camera.scale; schedule();
    }
    function imageFor(url) {
      if (!url) return null;
      if (cache.has(url)) { const entry = cache.get(url); entry.lastSeen = frameID; cache.delete(url); cache.set(url, entry); return entry; }
      const entry = {url, image: null, loaded: false, failed: false, lastSeen: frameID};
      cache.set(url, entry); imageQueue.push(entry); pumpImages();
      return entry;
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
      return {color, dash, alpha, related};
    }
    function arrow(x, y, angle, size, edge) {
      if (has(edge, "restrictivetransitivitytail")) return;
      ctx.beginPath();
      if (has(edge, "disabled")) {
        ctx.moveTo(x + Math.sin(angle) * size * .6, y - Math.cos(angle) * size * .6); ctx.lineTo(x - Math.sin(angle) * size * .6, y + Math.cos(angle) * size * .6); ctx.stroke();
      } else {
        ctx.moveTo(x, y); ctx.lineTo(x - Math.cos(angle - .45) * size, y - Math.sin(angle - .45) * size); ctx.lineTo(x - Math.cos(angle + .45) * size, y - Math.sin(angle + .45) * size); ctx.closePath(); ctx.fill();
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
        if (scale > .24) arrow(x + 16, y, 2.1, 6 / Math.max(.6, scale), edge);
        return;
      }
      const dx = b.x - a.x, dy = b.y - a.y, length = Math.hypot(dx, dy) || 1;
      // One offset per original record, including reverse and duplicate records.
      const direction = a.id <= b.id ? 1 : -1, offset = edge.parallel * 13 * direction;
      const control = {x: (a.x + b.x) / 2 - dy / length * offset, y: (a.y + b.y) / 2 + dx / length * offset};
      const boundary = (node, toward) => { const dx = toward.x - node.x, dy = toward.y - node.y; const ratio = Math.min(node.width / 2 / (Math.abs(dx) || .001), node.height / 2 / (Math.abs(dy) || .001)); return {x: node.x + dx * ratio, y: node.y + dy * ratio}; };
      const start = boundary(a, control), end = boundary(b, control);
      ctx.beginPath(); ctx.moveTo(start.x, start.y); ctx.quadraticCurveTo(control.x, control.y, end.x, end.y); ctx.stroke();
      if (scale > .24 || appearance.related) arrow(end.x, end.y, Math.atan2(end.y - control.y, end.x - control.x), 6 / Math.max(.6, scale), edge);
    }
    function drawNode(node) {
      const scale = state.camera.scale, selected = state.selected && node.id === state.selected.id, hover = node.id === state.hovered;
      const x = node.x - node.width / 2, y = node.y - node.height / 2;
      ctx.globalAlpha = state.selected && !selected && !isNeighbor(node.id) ? .38 : 1;
      ctx.setLineDash([]);
      if (node.width * scale < 18) {
        ctx.fillStyle = selected ? "#f4d790" : node.color; ctx.beginPath(); ctx.arc(node.x, node.y, Math.max(1.6 / scale, 3), 0, Math.PI * 2); ctx.fill(); return;
      }
      roundedRect(x, y, node.width, node.height, 5); ctx.fillStyle = selected ? "#3a3c29" : "#253026"; ctx.fill();
      ctx.strokeStyle = selected ? "#f0d28a" : hover ? "#d4d6b6" : node.color; ctx.lineWidth = (selected ? 2.2 : .8) / Math.max(.6, scale);
      if (has(node, "virtual")) ctx.setLineDash([4, 4]); ctx.stroke(); ctx.setLineDash([]);
      ctx.fillStyle = node.color; ctx.fillRect(x + 5, y + 5, 2, 12);
      if (node.width * scale < 60) return;
      ctx.font = "11px -apple-system, BlinkMacSystemFont, sans-serif"; ctx.textAlign = "left"; ctx.textBaseline = "top"; ctx.fillStyle = "#d4ddc6";
      ctx.fillText(fitText(node.name, node.width - 21), x + 12, y + 5);
      if (node.height > 80 && node.width * scale >= 86) {
        const top = y + 24, width = node.width - 12, height = node.height - 46, entry = imageFor(node.thumbnail);
        ctx.fillStyle = "#131c15"; ctx.fillRect(x + 6, top, width, height);
        if (entry && entry.loaded && entry.image.naturalWidth) {
          const ratio = Math.min(width / entry.image.naturalWidth, height / entry.image.naturalHeight), iw = entry.image.naturalWidth * ratio, ih = entry.image.naturalHeight * ratio;
          ctx.drawImage(entry.image, x + 6 + (width - iw) / 2, top + (height - ih) / 2, iw, ih);
        } else {
          ctx.font = "9px -apple-system, sans-serif"; ctx.fillStyle = "#77866c"; ctx.textAlign = "center";
          ctx.fillText(node.thumbnail ? entry && entry.failed ? "Picture unavailable" : "Loading picture…" : "No local picture", node.x, top + height / 2 - 4); ctx.textAlign = "left";
        }
      }
      ctx.font = "9px -apple-system, sans-serif"; ctx.fillStyle = "#97a78a";
      ctx.fillText(fitText(node.assetName || node.originalName || node.stack, node.width - 15), x + 7, y + node.height - 15);
    }
    function isNeighbor(id) {
      if (!state.selected) return false;
      return graph.incoming.get(state.selected.id).some(edge => edge.source === id) || graph.outgoing.get(state.selected.id).some(edge => edge.target === id);
    }
    function draw() {
      if (!ctx) return;
      frameID++;
      const dpr = canvas.width / state.width, camera = state.camera;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, state.width, state.height);
      ctx.globalAlpha = .12; ctx.strokeStyle = "#7c8b67"; ctx.lineWidth = .5;
      const grid = 38, ox = ((camera.x % grid) + grid) % grid, oy = ((camera.y % grid) + grid) % grid;
      ctx.beginPath();
      for (let x = ox; x < state.width; x += grid) { ctx.moveTo(x, 0); ctx.lineTo(x, state.height); }
      for (let y = oy; y < state.height; y += grid) { ctx.moveTo(0, y); ctx.lineTo(state.width, y); }
      ctx.stroke();
      const p1 = worldPoint(camera, {x: -30, y: -30}), p2 = worldPoint(camera, {x: state.width + 30, y: state.height + 30}), viewport = {minX: p1.x, minY: p1.y, maxX: p2.x, maxY: p2.y};
      ctx.setTransform(dpr * camera.scale, 0, 0, dpr * camera.scale, dpr * camera.x, dpr * camera.y);
      for (const edge of graph.edges) drawEdge(edge, viewport);
      const nodes = graph.nodes.filter(node => visible(node) && intersects(node, viewport));
      for (const node of nodes) if (!state.selected || node.id !== state.selected.id) drawNode(node);
      if (state.selected && visible(state.selected) && intersects(state.selected, viewport)) drawNode(state.selected);
      // Keep every visible thumbnail. A hard cap smaller than the viewport's
      // node count would evict/reload those same images on every repaint.
      for (const [url, entry] of cache) {
        if (cache.size <= 96) break;
        if (entry.lastSeen !== frameID && (entry.loaded || entry.failed)) cache.delete(url);
      }
      ctx.globalAlpha = 1; ctx.setLineDash([]);
      $("zoom-level").textContent = Math.round(camera.scale * 100) + "%";
      $("view-status").textContent = formatNumber(graph.nodes.filter(visible).length) + " nodes · zoom in to reveal pictures";
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
      fit();
    }
    function setAges(ages) { state.ages = new Set(ages.filter(age => graph.stacks.includes(age))); filtersChanged(); }
    function renderSearch() {
      state.search = searchNodes(graph.nodes, $("graph-search").value); state.searchIndex = -1;
      const host = $("search-results"); host.replaceChildren();
      const query = $("graph-search").value.trim(); host.hidden = !query; $("graph-search").setAttribute("aria-expanded", query ? "true" : "false");
      if (!query) return;
      if (!state.search.length) { host.append(element("p", "search-empty", "No cards match. Try a card ID, original name, or picture name.")); return; }
      state.search.forEach((node, index) => {
        const button = element("button", "search-result"), dot = element("span", "result-dot"), copy = element("span", "result-copy");
        button.id = "search-result-" + index; button.setAttribute("role", "option"); button.setAttribute("aria-selected", "false"); dot.style.background = node.color;
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
        copy.append(element("strong", "", other.name), element("small", "", edgeLabel(edge))); dot.style.background = other.color;
        button.append(element("span", "ref-arrow", direction === "incoming" ? "↙" : "↗"), copy, dot);
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
    function selectNode(id, focus = false) {
      const node = graph.byID.get(Number(id)); if (!node) return;
      state.selected = node; state.zone = null;
      if (!visible(node)) { state.ages.add(node.stack); const input = ageInputs.get(node.stack); if (input) input.checked = true; }
      $("welcome-panel").hidden = true; $("selection-panel").hidden = false; $("zone-detail").hidden = true;
      $("selected-age").textContent = node.stack; $("selected-name").textContent = node.name;
      $("selected-tags").replaceChildren();
      for (const flag of node.flags) if (NODE_LABELS[flag]) $("selected-tags").append(element("span", "tag" + (flag === "virtual" ? " warning" : ""), NODE_LABELS[flag]));
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
        const scale = clamp(gesture.scale * Math.hypot(a.x - b.x, a.y - b.y) / gesture.distance, state.minScale, 5);
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
      const tag = document.activeElement && document.activeElement.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || document.activeElement && document.activeElement.isContentEditable) return;
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
    $("home-link").addEventListener("click", event => { event.preventDefault(); fit(); });
    $("all-ages").addEventListener("click", () => setAges(graph.stacks)); $("no-ages").addEventListener("click", () => setAges([]));
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
    renderFilters(); resize(true);
    const initial = dock || graph.nodes.find(node => node.image);
    if (initial) selectNode(initial.id);
    if (typeof window.ResizeObserver === "function") new window.ResizeObserver(() => resize()).observe(canvas); else window.addEventListener("resize", () => resize());
    return {graph, state, selectNode, clearSelection, fit, zoom, setAges, renderSearch, updateZones, showZone, draw, worldPoint: point => worldPoint(state.camera, point), screenPoint: point => screenPoint(state.camera, point), imageCache: cache};
  }

  const api = {normalizeGraph, worldPoint, screenPoint, zoomAt, boundsFor, fitCamera, intersects, searchNodes, edgeLabel, clippedZone, viewGeometry, safeImagePath, createApp};
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  if (global.document && global.document.getElementById("graph-canvas")) {
    global.DeMystifyAtlas = api;
    global.DeMystifyAtlas.app = createApp(global.document, global, global.DEMYSTIFY_GRAPH || {});
  }
})(typeof window !== "undefined" ? window : globalThis);
