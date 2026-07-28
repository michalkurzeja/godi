'use strict';
(() => {

const $ = (id) => document.getElementById(id);
const data = JSON.parse($('godi-data').textContent);
const engine = document.documentElement.dataset.layout;

// Node labels are laid out by hand rather than wrapped, so the line a given
// argument sits on is known. That is what lets an edge leave the node at the row
// it feeds instead of from the middle of the box.
const FONT = 11;
const LINE_HEIGHT = 1.3;
const LINE = FONT * LINE_HEIGHT;
const PAD = 7;
const MAX_CHARS = 40;

// ------------------------------------------------------------------ model ---

const nodes = new Map();
const params = new Map();
for (const n of data.nodes) {
	nodes.set(n.id, n);
	for (const p of n.params || []) params.set(p.id, p);
}

const edges = new Map();
const out = new Map();
const into = new Map();
const push = (m, k, v) => { const a = m.get(k); if (a) a.push(v); else m.set(k, [v]); };
for (const e of data.edges) {
	edges.set(e.id, e);
	push(out, e.from, e);
	push(into, e.to, e);
}

const scopes = new Map(data.scopes.map((s) => [s.id, s]));
const roots = data.nodes.filter((n) => n.root).length;

// How many edges a node has before any filtering. It tells a node a filter
// stripped bare from one that never had a connection.
const wired = new Map(data.nodes.map((n) =>
	[n.id, (out.get(n.id) || []).length + (into.get(n.id) || []).length]));

// The path to a scope, a step at a time. Each step is the definition that declared
// it rather than its full label. "children of" is worth saying once beside a box,
// but a path of them buries the names it is there to show.
function scopePath(id) {
	const parts = [];
	for (let s = scopes.get(id); s; s = s.parent ? scopes.get(s.parent) : null) {
		parts.unshift(s.owner || s.label);
	}
	return parts.join(' › ') || id;
}

// Only an extension is worth naming. godi's own automation runs under a pass too,
// and "autowiring (autowiring)" says nothing. "none" needs its own wording: it
// means nobody has wired the argument yet.
const named = (origin, pass) => {
	if (origin === 'none') return 'not wired';
	return origin === 'compiler-pass' && pass ? origin + ': ' + pass : origin;
};
const boundBy = (e) => named(e.bindOrigin, e.bindPass);

// The model works out what a node is headed by and what goes under it, so every
// format gives the same answer. Both arrive ready to print.
//
// Which nodes get the second line is this page's own business. A box is smaller
// than a drawing, so only a function literal earns one. Everything else is named
// by the line above.
const displaySub = (n) => n.anonymous ? n.subtitle : '';

// short drops the package path from every qualified name in a signature, keeping
// the last segment of each. It is the same rule graph/internal/render applies in
// Go.
//
// Every path goes, not just one. A signature can carry several, since a generic
// names its type arguments, and shortening around only the last slash would leave
// the rest qualified.
const PATH_BYTE = /[A-Za-z0-9./\-_~+]/;

function short(sig) {
	let out = '';
	for (let i = 0; i < sig.length;) {
		if (!PATH_BYTE.test(sig[i])) {
			out += sig[i++];
			continue;
		}

		const start = i;
		while (i < sig.length && PATH_BYTE.test(sig[i])) i++;

		const run = sig.slice(start, i);
		const slash = run.lastIndexOf('/');
		out += slash < 0 ? run : run.slice(slash + 1);
	}
	return out;
}

const clip = (s, max = MAX_CHARS) => s.length <= max ? s : s.slice(0, max - 1) + '…';

// ------------------------------------------------------------------ state ---

const state = {
	query: '',
	focus: null,
	// The whole subtree by default. Not seeing the far end of a selection is the
	// more common disappointment, and the slider is right there.
	hops: Infinity,
	dir: 'both',
	isolate: false,
	rootsOnly: false,
	args: true,
	layout: 'layered',
	routing: 'unbundled-bezier',
	wheel: 'auto',
	show: { manual: true, autowiring: true, 'compiler-pass': true, method: true },
	// What a search looks at. A type and a factory name are what people reach
	// for. The rest widen the net when they need to, and would add noise by
	// default.
	scopes: {
		type: true, factory: true, args: false, literals: false,
		methods: false, scope: false, labels: false,
	},
};

const ROUTINGS = ['unbundled-bezier', 'straight', 'segments', 'taxi'];
const WHEELS = ['auto', 'mouse', 'trackpad'];

// What the reader chose last time, read before anything is derived from it.
//
// The text a search looks at and the style the canvas is drawn with are both built
// from state, and neither is rebuilt on its own. A preference restored after the
// fact would leave a control saying one thing while the page does another.
//
// The colour scheme is not here. It is classes on the page as well as state, so
// installTheme applies it whole.
function restorePreferences() {
	const oneOf = (key, allowed, fallback) => {
		const stored = recall(key);
		return allowed.includes(stored) ? stored : fallback;
	};
	state.routing = oneOf('godi.routing', ROUTINGS, state.routing);
	state.wheel = oneOf('godi.wheel', WHEELS, state.wheel);

	try {
		// null parses to null, and assigning that changes nothing.
		Object.assign(state.scopes, JSON.parse(recall('godi.searchScopes')));
	} catch { /* keep the defaults */ }
}

restorePreferences();

// The text of a node, in whichever parts are being searched. It is read on every
// keystroke, so it is built once per node and kept until the scopes change.
let haystacks = new Map();

function rebuildHaystacks() {
	const looksAt = state.scopes;

	haystacks = new Map(data.nodes.map((n) => {
		const parts = [];
		if (looksAt.type) parts.push(n.type);
		if (looksAt.factory) parts.push(n.name);
		if (looksAt.scope) parts.push(String(n.scope));
		if (looksAt.labels) parts.push(...(n.labels || []));

		// Method arguments are only in reach once method calls are.
		const reachable = (n.params || []).filter((p) => looksAt.methods || !isMethod(p));
		if (looksAt.methods) parts.push(...reachable.filter(isMethod).map((p) => p.method));
		if (looksAt.args) parts.push(...reachable.map((p) => p.short));
		if (looksAt.literals) parts.push(...reachable.flatMap((p) => p.literals || []));

		return [n.id, parts.join(' ').toLowerCase()];
	}));
}

// ------------------------------------------------------------------ labels ---

// rows returns the lines of a node's box, and where each argument landed, so
// that an edge can be anchored to its own row.
const isMethod = (p) => p.kind === 'method-arg' || p.kind === 'method-receiver';

// shown reports whether an injection point belongs in the picture at all.
//
// Hiding method calls drops their rows as well as their edges. A row nothing can
// arrive at explains nothing.
const shownParam = (p) => state.args && (state.show.method || !isMethod(p));

function rows(n) {
	const lines = [];
	// A root is the top of a tree, and a function is a function. A root function
	// wears both marks. A node missing something it needs wears none: the red
	// border already says so.
	const head = (n.root ? '▲ ' : '') + (n.kind === 'function' ? 'ƒ ' : '');
	const heading = n.title;
	const sub = displaySub(n);
	lines.push(clip(head + heading));
	if (sub && sub !== heading) lines.push(clip(sub));

	const paramLine = new Map();
	const rules = [];

	const paramText = (p) => {
		let text = (p.method ? p.method + ' ' : '') + p.index + ' ◂ ' + p.short;
		if (p.literals && p.literals.length) text += ' = ' + p.literals.join(', ');
		return clip(text, MAX_CHARS + 6);
	};

	// A rule between the header, the constructor arguments and the method calls.
	// It goes in the label because a Cytoscape node has nowhere else to draw.
	//
	// The ports follow on their own, since each one is read off the line its
	// argument ends up on.
	const section = (params) => {
		const shown = params.filter(shownParam);
		if (!shown.length) return;

		rules.push(lines.length);
		lines.push('');
		for (const p of shown) {
			paramLine.set(p.id, lines.length);
			lines.push(paramText(p));
		}
	};

	section((n.params || []).filter((p) => !isMethod(p)));
	section((n.params || []).filter(isMethod));

	// A filter stopped here rather than the wiring. A box that does not say so
	// reads as a service with nothing around it.
	if (n.elided) {
		rules.push(lines.length);
		lines.push('');
		lines.push('⋯ +' + n.elided + ' more');
	}

	return { text: lines.join('\n'), count: lines.length, paramLine, rules };
}

// rowOffset is how far the argument's row sits from the middle of the box.
// Cytoscape centres the label block, so the middle line is at zero.
function rowOffset(layout, paramID) {
	const line = layout.paramLine.get(paramID);
	if (line === undefined) return 0;
	return (line + 0.5 - layout.count / 2) * LINE;
}

const layouts = new Map();

// The rules are drawn rather than typed, so they can take the border's colour
// instead of the text's. One image per node, sized to that node's own box.
//
// The box is measured rather than assumed. The image is stretched over whatever
// Cytoscape laid out, so a guess a few pixels out does not slide the lines, which
// it would do most to the lower ones.
function ruleImage(layout, colour, width, height) {
	if (!layout.rules.length) return 'none';

	// The label block is centred in the box, so the first line starts here.
	const top = (height - layout.count * LINE) / 2;
	const inset = PAD; // The same margin the text keeps from the border.

	const drawn = layout.rules
		.map((i) => {
			const y = (top + (i + 0.5) * LINE).toFixed(2);
			return `<line x1="${inset}" x2="${(width - inset).toFixed(2)}" y1="${y}" y2="${y}"/>`;
		})
		.join('');

	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${width.toFixed(2)}"` +
		` height="${height.toFixed(2)}" viewBox="0 0 ${width.toFixed(2)} ${height.toFixed(2)}">` +
		`<g stroke="${colour}" stroke-width="1">${drawn}</g></svg>`;

	// charset, not ";utf8". The latter is not a valid data URI parameter, and the
	// browser refuses the image outright.
	return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
}

// Drawn after the boxes exist, because it takes their measurements. Runs again
// whenever the rows or the colours change.
function paintRules() {
	const colour = palette().nodeBorder;

	cy.batch(() => {
		for (const n of data.nodes) {
			const el = cy.getElementById(n.id);
			const layout = layouts.get(n.id);
			if (layout) el.data('rules', ruleImage(layout, colour, el.outerWidth(), el.outerHeight()));
		}
	});
}

// Everything about a node's box that depends on its rows, in one place. The three
// callers are the first build, a change of what is shown, and a change of colour
// scheme.
function boxOf(n) {
	const layout = rows(n);
	layouts.set(n.id, layout);
	return {
		label: layout.text,
		height: layout.count * LINE + 2 * PAD,
		rules: 'none', // paintRules fills this in, once there is a box to measure.
	};
}

function buildElements() {
	const els = [];

	for (const s of data.scopes) {
		els.push({ data: { id: 'scope:' + s.id, label: s.label, parent: s.parent ? 'scope:' + s.parent : undefined } });
	}

	for (const n of data.nodes) {
		els.push({
			data: {
				id: n.id, parent: 'scope:' + n.scope,
				kind: n.kind, shared: n.shared, lazy: n.lazy, root: n.root,
				incomplete: !!n.incomplete,
				...boxOf(n),
			},
		});
	}

	for (const e of data.edges) {
		els.push({
			data: {
				id: e.id, source: e.from, target: e.to,
				origin: e.origin, decidedBy: e.decidedBy, kind: e.kind,
				bound: !!e.bindInterface, cycle: !!e.cycle,
				// A pass is named however it was responsible. It may have wired
				// the argument, or created the binding the argument resolved
				// through. The colour says a pass decided, and this says which.
				label: e.pass || '',
				offset: rowOffset(layouts.get(e.from), e.param),
			},
		});
	}

	return els;
}

// ------------------------------------------------------------------ colours ---

// Cytoscape draws to a canvas, so it cannot read CSS variables. They are resolved
// here instead, and re-read whenever the colour scheme changes. That is what makes
// switching themes instant.
function palette() {
	const css = getComputedStyle(document.documentElement);
	const v = (name) => css.getPropertyValue(name).trim();
	return {
		text: v('--text'), muted: v('--muted'), accent: v('--accent'), warn: v('--warn'),
		node: v('--node'), nodeBorder: v('--node-border'), rootNode: v('--node-root'),
		scope: v('--scope'), scopeBorder: v('--scope-border'),
		manual: v('--manual'), auto: v('--auto'), pass: v('--pass'),
	};
}

// The two channels are independent, as in the DOT output. The arrowhead says how
// the dependency was matched, and the colour says who decided on it.
//
// The filters key off the same field, so what is drawn purple is what the compiler
// pass box hides.
function edgeColour(p) {
	const of = { manual: p.manual, autowiring: p.auto, 'compiler-pass': p.pass };
	return (e) => e.data('cycle') ? p.warn : (of[e.data('decidedBy')] || p.muted);
}

function stylesheet() {
	const p = palette();
	const colour = edgeColour(p);

	return [
		{
			selector: 'node', style: {
				shape: 'round-rectangle',
				// Nothing injects a root, so it is the top of a tree. The fill
				// says so, because width and style already mean lazy and shared.
				// A root is not a problem, so it gets no warning colour.
				'background-color': (n) => n.data('root') ? p.rootNode : p.node,
				// The warning colour outranks everything else. A node missing
				// what it needs is the one thing worth finding at a glance, and
				// the border is the only channel loud enough to say it.
				'border-color': (n) => n.data('incomplete') ? p.warn : p.nodeBorder,
				'border-width': (n) => n.data('incomplete') ? 2.6 : (n.data('lazy') ? 1 : 2.2),
				'border-style': (n) => n.data('kind') === 'service' && !n.data('shared') ? 'dashed' : 'solid',
				label: 'data(label)',
				color: p.text,
				'font-family': 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
				'font-size': FONT,
				'line-height': LINE_HEIGHT,
				// "wrap" is what honours the newlines in the label. With "none"
				// the whole box collapses onto one line. Lines are already
				// clipped to a known length, so the max width never bites.
				'text-wrap': 'wrap',
				'text-max-width': 1000,
				'text-justification': 'left',
				'text-valign': 'center',
				'text-halign': 'center',
				width: 'label',
				height: 'data(height)',
				padding: PAD,
				'background-image': 'data(rules)',
				'background-fit': 'none',
				'background-width': '100%',
				'background-height': '100%',
				'background-position-x': '0%',
				'background-position-y': '0%',
			},
		},
		{
			selector: '$node > node', style: {
				'background-color': p.scope, 'border-color': p.scopeBorder, 'border-width': 1,
				'border-style': 'solid', label: 'data(label)', color: p.muted,
				'font-family': 'system-ui, sans-serif', 'font-size': 10,
				'text-valign': 'top', 'text-halign': 'center', 'text-margin-y': -3,
				padding: 18, shape: 'round-rectangle',
			},
		},
		{
			selector: 'edge', style: {
				// Curved by default. Boxy routes from the node's centre and then
				// joins the argument row with a stub, which draws a spike back up
				// the box. The others leave the row directly.
				'curve-style': state.routing,
				// A plain bezier is drawn straight unless the edge is one of
				// several between the same pair, so a curve has to be asked for
				// with control points of its own.
				'control-point-distances': 34,
				'control-point-weights': 0.5,
				'taxi-direction': 'rightward',
				'taxi-turn': 24,
				'taxi-turn-min-distance': 8,
				width: (e) => e.data('origin') === 'compiler-pass' ? 2.2 : 1.4,
				'line-color': colour,
				'line-style': (e) => e.data('origin') === 'manual' ? 'solid' : 'dashed',
				'target-arrow-color': colour,
				'target-arrow-shape': (e) => e.data('bound') ? 'diamond' : 'triangle',
				'target-arrow-fill': (e) => e.data('bound') ? 'hollow' : 'filled',
				'arrow-scale': 0.9,
				'source-endpoint': (e) => '50% ' + e.data('offset') + 'px',
				'target-endpoint': 'outside-to-node',
				label: 'data(label)',
				'font-family': 'system-ui, sans-serif',
				'font-size': 9,
				color: p.muted,
				'text-background-color': p.node,
				'text-background-opacity': 0.85,
				'text-background-padding': 1,
			},
		},
		// Cytoscape marks a held background with a grey disc under the pointer,
		// which reads as something having gone wrong.
		{ selector: 'core', style: { 'active-bg-opacity': 0, 'active-bg-size': 0 } },
		// Dimmed is still readable and still there to be clicked. What is not in
		// the selection is context rather than clutter, and a reader can pick
		// their next node out of it. Isolate selection is the switch for wanting
		// it gone.
		{ selector: 'node.dim', style: { opacity: 0.3 } },
		// Edges read as noise when they are not about the selection, so they fade
		// much further and their labels go entirely.
		{ selector: 'edge.dim', style: { opacity: 0.13, 'text-opacity': 0 } },
		{ selector: 'node.match', style: { 'border-color': p.accent, 'border-width': 2.4 } },
		{ selector: 'node.sel', style: { 'border-color': p.accent, 'border-width': 3.4 } },
	];
}

// ------------------------------------------------------------------ canvas ---

const cy = cytoscape({
	container: $('cy'),
	elements: buildElements(),
	style: stylesheet(),
	layout: { name: 'preset' },
	boxSelectionEnabled: false,
	// The wheel is handled below. What it should do depends on whether the hand
	// on it is holding a mouse or resting on a trackpad.
	userZoomingEnabled: false,
});

// ------------------------------------------------------------------ layout ---

let viz = null;
const vizInstance = () => (viz ??= Viz.instance());

const busy = (on) => $('canvas').classList.toggle('busy', on);

const BUILTIN = { breadthfirst: 'breadthfirst', concentric: 'concentric', cose: 'cose', grid: 'grid' };

async function relayout() {
	busy(true);
	try {
		if (state.layout === 'layered') {
			if (engine === 'graphviz') await graphvizLayout();
			else await runLayout({ name: 'dagre', rankDir: 'LR', nodeSep: 22, rankSep: 90 });
		} else {
			await runLayout({ name: BUILTIN[state.layout], animate: false, fit: true, padding: 30 });
		}
	} catch (err) {
		console.error('layout failed', err);
	} finally {
		busy(false);
	}
}

function runLayout(options) {
	return new Promise((resolve) => {
		const l = cy.elements(':visible').layout({ padding: 30, fit: true, ...options });
		l.one('layoutstop', () => resolve());
		l.run();
	});
}

// graphvizLayout asks Graphviz where the nodes go, then puts them there.
//
// The DOT it builds is a request for geometry rather than a drawing: sizes,
// nesting and edges, with no colour or label in it. The sizes have to come from
// Cytoscape, because Cytoscape draws the boxes.
async function graphvizLayout() {
	const shown = cy.nodes(':childless:visible');
	if (shown.empty()) return;

	const instance = await vizInstance();
	const json = JSON.parse(instance.renderString(layoutDot(shown), { format: 'json', engine: 'dot' }));

	const bb = String(json.bb).split(',').map(Number);
	const height = bb[3] || 0;

	cy.batch(() => {
		for (const o of json.objects || []) {
			if (!o.pos) continue;
			const node = cy.getElementById(String(o.name));
			if (node.empty()) continue;
			const [x, y] = String(o.pos).split(',').map(Number);
			// Graphviz measures from the bottom left, Cytoscape from the top left.
			node.position({ x, y: height - y });
		}
	});

	cy.animate({ fit: { eles: cy.elements(':visible'), padding: 30 }, duration: 250 });
}

function layoutDot(shown) {
	const q = (s) => '"' + String(s).replace(/(["\\])/g, '\\$1') + '"';
	const lines = [
		'digraph {',
		'graph [rankdir=LR, nodesep=0.3, ranksep=0.9, compound=true, pad=0.1, newrank=true];',
		'node [shape=box, fixedsize=true];',
	];

	// Graphviz reports positions in points and sizes in inches.
	const emitNode = (node) => lines.push('\t' + q(node.id()) +
		` [width=${(node.outerWidth() / 72).toFixed(4)}, height=${(node.outerHeight() / 72).toFixed(4)}];`);

	const inScope = new Map();
	shown.forEach((node) => push(inScope, node.parent().id(), node));

	const children = (scope) => data.scopes.filter((s) => s.parent === scope.id);

	// A scope with nothing on show is left out rather than emitted empty.
	// Graphviz reserves room for an empty cluster, and that room came out as a
	// hole in the middle of a filtered graph.
	const occupied = (scope) =>
		(inScope.get('scope:' + scope.id) || []).length > 0 || children(scope).some(occupied);

	const emitScope = (scope) => {
		if (!occupied(scope)) return;
		lines.push('\tsubgraph ' + q('cluster_' + scope.id) + ' {');
		for (const node of inScope.get('scope:' + scope.id) || []) emitNode(node);
		for (const child of children(scope)) emitScope(child);
		lines.push('\t}');
	};

	for (const scope of data.scopes) {
		if (!scope.parent) emitScope(scope);
	}

	cy.edges(':visible').forEach((e) => {
		if (shown.contains(e.source()) && shown.contains(e.target())) {
			lines.push('\t' + q(e.source().id()) + ' -> ' + q(e.target().id()) + ';');
		}
	});

	lines.push('}');
	return lines.join('\n');
}

// -------------------------------------------------------------- visibility ---

function found() {
	const terms = state.query.toLowerCase().split(/\s+/).filter(Boolean);
	if (!terms.length) return null;
	const hit = new Set();
	for (const n of data.nodes) {
		const text = haystacks.get(n.id);
		if (text && terms.every((t) => text.includes(t))) hit.add(n.id);
	}
	return hit;
}

// neighbourhood walks outwards from the selection, following only the edges still
// on show. Hiding a kind of wiring hides what it reached.
//
// Each direction is walked on its own, and the two results are put together.
// Following both at once would let a path turn around partway: down to a
// dependency, then back up to something else that uses it. That reaches the
// selection's siblings and calls them two hops away.
//
// They are not on any path through the selection. The reader asked what this
// service is built from and what uses it, not what its dependencies are shared
// with.
function neighbourhood(id, live) {
	const walk = (adjacent, far) => {
		const seen = new Set([id]);
		let frontier = [id];
		for (let hop = 0; hop < state.hops && frontier.length; hop++) {
			const next = [];
			for (const cur of frontier) {
				for (const e of adjacent.get(cur) || []) {
					const other = far(e);
					if (live.has(e.id) && !seen.has(other)) { seen.add(other); next.push(other); }
				}
			}
			frontier = next;
		}
		return seen;
	};

	const seen = new Set([id]);
	if (state.dir !== 'up') for (const n of walk(out, (e) => e.to)) seen.add(n);
	if (state.dir !== 'down') for (const n of walk(into, (e) => e.from)) seen.add(n);
	return seen;
}

// apply is the single place anything becomes hidden, dimmed or marked. It returns
// whether the set of visible elements changed, which is when the graph is worth
// laying out again.
function apply() {
	// What the wiring filters keep. Which nodes are on show is not consulted
	// here. This set says whether a node has any wiring left, and hiding its
	// neighbours is not the same as taking its wiring away.
	const passing = new Set();
	for (const e of data.edges) {
		if (e.decidedBy !== 'none' && !state.show[e.decidedBy]) continue;
		if (!state.show.method && isMethod(e)) continue;
		passing.add(e.id);
	}

	// A node a wiring filter has stripped of every edge is an artefact of the
	// filter, so it goes with them. A node that never had an edge is a finding of
	// its own, an unwired service, so it stays.
	const connected = new Set();
	for (const id of passing) {
		const e = edges.get(id);
		connected.add(e.from);
		connected.add(e.to);
	}
	const stranded = (id) => wired.get(id) > 0 && !connected.has(id);

	// Hiding nodes outright is a different thing. The reader asked to see just
	// these, so having no visible neighbours is the point, and the rule above
	// must not take them away again.
	const hidden = new Set();
	if (state.rootsOnly) {
		for (const n of data.nodes) if (!n.root) hidden.add(n.id);
	}

	const dropped = (id) => hidden.has(id) || stranded(id);

	const live = new Set();
	for (const id of passing) {
		const e = edges.get(id);
		if (!dropped(e.from) && !dropped(e.to)) live.add(id);
	}

	const hits = found();
	const near = nodes.has(state.focus) ? neighbourhood(state.focus, live) : null;
	const outside = (id) => near !== null && !near.has(id);

	// Which scopes still hold something, counted here rather than asked of the
	// canvas. A node inside a hidden scope reads as hidden whatever its own state,
	// so a scope taken away on one pass could never come back on the next.
	const populated = new Set();
	const populate = (id) => {
		for (let s = scopes.get(id); s && !populated.has(s.id); s = s.parent ? scopes.get(s.parent) : null) {
			populated.add(s.id);
		}
	};

	let changed = false;
	cy.batch(() => {
		for (const n of data.nodes) {
			const el = cy.getElementById(n.id);
			const gone = dropped(n.id) || (state.isolate && outside(n.id));
			if (!gone) populate(n.scope);
			if (el.visible() === gone) changed = true;
			gone ? el.hide() : el.show();
			el.toggleClass('dim', !gone && (outside(n.id) || (hits !== null && !hits.has(n.id))));
			el.toggleClass('match', hits !== null && hits.has(n.id));
			el.toggleClass('sel', n.id === state.focus);
		}
		for (const e of data.edges) {
			const el = cy.getElementById(e.id);
			const spans = !outside(e.from) && !outside(e.to);
			const gone = !live.has(e.id) || (state.isolate && !spans);
			if (el.visible() === gone) changed = true;
			gone ? el.hide() : el.show();
			el.toggleClass('dim', !gone && !spans);
		}

		// A scope left with nothing on show has to be taken off the canvas by
		// name. Cytoscape stops drawing it once its children are hidden, so it
		// looks gone, but it still counts towards the box of the scope holding it,
		// from wherever the last layout left it. That is what stretched an
		// isolated selection's root box down over empty space.
		for (const scope of data.scopes) {
			const el = cy.getElementById('scope:' + scope.id);
			if (el.nonempty()) populated.has(scope.id) ? el.show() : el.hide();
		}
	});

	const shownNodes = cy.nodes(':childless:visible').length;
	const shownEdges = cy.edges(':visible').length;
	$('found').textContent = hits === null ? '' : hits.size + (hits.size === 1 ? ' match' : ' matches');

	const counts = [
		shownNodes + ' of ' + data.nodes.length + ' nodes',
		shownEdges + ' of ' + data.edges.length + ' edges',
		roots + (roots === 1 ? ' root' : ' roots'),
	];
	// Every fault the extractor found is on the node it is about: a red border on
	// the box, the reason on the argument row. A tally in the corner would say how
	// many without saying where.
	const incomplete = cy.nodes(':childless:visible').filter((n) => n.data('incomplete')).length;
	if (incomplete) counts.push(incomplete + ' incomplete');
	$('counts').textContent = counts.join(' · ');

	return changed;
}

// ---------------------------------------------------------------- snapshot ---

// A graph taken while the container was still being built is missing whatever the
// passes after it would have added. It looks like a finished container with
// dependencies lost, and nothing else on the page says otherwise.
//
// So it is said once, across the top, where it applies to everything below.
function showSnapshot() {
	const snap = data.snapshot;
	if (!snap) return;

	const strip = $('snapshot');
	$('snapshot-label').textContent = 'Snapshot: ' + snap.label + '.';
	strip.hidden = false;
	// Read once and dismissed. It says the same thing all session, and the canvas
	// is what the reader came for.
	$('snapshot-close').addEventListener('click', () => {
		strip.hidden = true;
		cy.resize();
	});
}

// ------------------------------------------------------------- diagnostics ---

// Every fault in the wiring is marked on the node it is about.
//
// Not every notice is a fault. A scope belonging to no definition, or a file
// written against a schema this build does not know, is about the graph itself and
// has nowhere else to go.
//
// So the strip says how many there are of each, and the panel lists them. With
// nothing selected the panel has nothing else to show.
function showDiagnostics() {
	const diagnostics = data.diagnostics || [];
	if (!diagnostics.length) return;

	const strip = $('diagnostics');
	$('diagnostics-label').textContent = severityTally(diagnostics) + ' — listed in the details panel.';
	strip.hidden = false;
	$('diagnostics-close').addEventListener('click', () => {
		strip.hidden = true;
		cy.resize();
	});
}

// Counted by severity rather than totalled, because the two kinds are not the same
// news. A fault in the wiring is something to fix. A note about the graph, such as
// a scope a compiler pass made, is not. "3 notices" would put both under one
// number.
function severityTally(diagnostics) {
	const counts = new Map();
	for (const d of diagnostics) counts.set(d.severity, (counts.get(d.severity) || 0) + 1);

	return [...counts]
		.map(([severity, n]) => n + ' ' + severity + (n === 1 ? '' : 's'))
		.join(' · ');
}

// ------------------------------------------------------------------ legend ---

// Every way an edge can be drawn, in the three channels it is drawn with. The
// samples take their colour from the same variables the canvas does, so the legend
// cannot drift from the graph it explains.
//
// The three channels are columns. A cycle is not an answer to who chose an edge: it
// overrides the colour outright, so it gets a section of its own.
const LEGEND = [
	[
		{
			title: 'Head', hint: 'how it matched', rows: [
				{ head: 'filled', text: 'Exact type' },
				{ head: 'hollow', text: 'Interface binding' },
			],
		},
		{
			title: 'Cycle', hint: 'overrides the colour', rows: [
				{ tint: 'warn', text: 'Loops back' },
			],
		},
	],
	[
		{
			title: 'Colour', hint: 'who chose it', rows: [
				{ tint: 'manual', text: 'You' },
				{ tint: 'auto', text: 'godi' },
				{ tint: 'pass', text: 'A compiler pass' },
			],
		},
	],
	[
		{
			title: 'Line', hint: 'who wired the argument', rows: [
				{ text: 'You' },
				{ dash: '4 3', text: 'godi' },
				{ dash: '4 3', width: 2.6, text: 'A compiler pass' },
			],
		},
	],
];

const SVG_NS = 'http://www.w3.org/2000/svg';

function edgeSample({ tint = 'muted', head = 'none', dash = '', width = 1.4 }) {
	const svg = document.createElementNS(SVG_NS, 'svg');
	svg.setAttribute('viewBox', '0 0 46 12');
	svg.setAttribute('class', 'sample ' + tint);

	const line = document.createElementNS(SVG_NS, 'path');
	line.setAttribute('class', 'line');
	line.setAttribute('d', head === 'none' ? 'M1 6h44' : 'M1 6h32');
	line.setAttribute('stroke-width', String(width));
	if (dash) line.setAttribute('stroke-dasharray', dash);
	svg.append(line);

	if (head !== 'none') {
		const arrow = document.createElementNS(SVG_NS, 'path');
		arrow.setAttribute('class', 'head ' + head);
		arrow.setAttribute('d', head === 'hollow' ? 'M34 6l5-4.2 5 4.2-5 4.2z' : 'M34 1.8l10 4.2-10 4.2z');
		svg.append(arrow);
	}

	return svg;
}

function buildLegend() {
	const parts = [];
	for (const sections of LEGEND) {
		const column = make('div', 'legend-col');

		for (const section of sections) {
			const group = make('div', 'legend-group');
			const head = make('div', 'legend-head', section.title);
			head.append(make('span', 'hint', section.hint));
			group.append(head);

			for (const row of section.rows) {
				const line = make('div', 'legend-row');
				line.append(edgeSample(row), make('span', null, row.text));
				group.append(line);
			}
			column.append(group);
		}
		parts.push(column);
	}
	$('legend').replaceChildren(...parts);
}

function setLegend(open) {
	$('app').classList.toggle('legend-open', open);
	$('legend-tab').title = open ? 'Hide the legend' : 'What the arrows mean';
	remember('godi.legend', open ? 'open' : 'closed');
}

const legendOpen = () => $('app').classList.contains('legend-open');

// -------------------------------------------------------------- side panel ---

const make = (tag, cls, text) => {
	const el = document.createElement(tag);
	if (cls) el.className = cls;
	if (text !== undefined) el.textContent = text;
	return el;
};

function nodeLink(id) {
	const n = nodes.get(id);
	const btn = make('button', 'link', n ? n.title : id);
	btn.type = 'button';
	btn.addEventListener('click', () => select(id, true));
	return btn;
}

// Badges read as labels, not as the model's own slugs, so they are capitalised
// and the hyphens the vocabulary uses are spaced out.
const sentence = (text) => text.charAt(0).toUpperCase() + text.slice(1).replace(/^([a-z]+)-([a-z]+)/, '$1 $2');

const badge = (text, cls) => make('span', 'badge' + (cls ? ' ' + cls : ''), sentence(text));

// locationCell shows a place in the source, as a link when the graph was built
// with a template to make one from. Paths are stored relative to the source
// root, so {file} has to be put back together.
function locationCell(loc) {
	const cell = make('dd', 'mono');
	if (!data.sourceLink) {
		cell.textContent = loc.text;
		return cell;
	}

	const absolute = data.sourceRoot ? data.sourceRoot + '/' + loc.file : loc.file;
	const link = make('a', 'link', loc.text);
	link.href = data.sourceLink
		.replaceAll('{file}', absolute)
		.replaceAll('{rel}', loc.file)
		.replaceAll('{line}', String(loc.line));
	cell.append(link);
	return cell;
}

// copyIDButton takes the node's graph id away with you. The id itself is not
// shown. It wraps over several lines and there is nothing to read in it: it is for
// pasting somewhere else.
function copyIDButton(id) {
	const button = make('button', 'copy');
	button.type = 'button';
	button.title = 'Copy this node\'s graph id, to paste into the go-to window or a message';
	button.append(icon('i-copy'), make('span', null, 'Copy ID'));

	button.addEventListener('click', async () => {
		const ok = await copy(id);
		button.classList.toggle('done', ok);
		button.lastChild.textContent = ok ? 'Copied' : 'Could not copy';
		setTimeout(() => {
			button.classList.remove('done');
			button.lastChild.textContent = 'Copy ID';
		}, 1200);
	});

	return button;
}

function icon(name) {
	const svg = document.createElementNS(SVG_NS, 'svg');
	svg.setAttribute('class', 'i');
	const use = document.createElementNS(SVG_NS, 'use');
	use.setAttribute('href', '#' + name);
	svg.append(use);
	return svg;
}

// A page opened from disk is a secure context, so the clipboard is there. It is
// still a permission and can be refused, and then the old way works.
async function copy(text) {
	try {
		await navigator.clipboard.writeText(text);
		return true;
	} catch { /* fall through */ }

	const field = make('textarea');
	field.value = text;
	field.setAttribute('aria-hidden', 'true');
	field.style.cssText = 'position:fixed;top:-1000px';
	document.body.append(field);
	field.select();
	try {
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		field.remove();
	}
}

// The panel can be sent away for more canvas. It still fills itself while hidden,
// which costs nothing, so bringing it back shows the current selection rather than
// whatever was there when it went.
function setPanel(open) {
	$('app').classList.toggle('panel-hidden', !open);
	$('panel-tab').title = open ? 'Hide the detail panel' : 'Show the detail panel';
	remember('godi.panel', open ? 'open' : 'hidden');
}

const panelOpen = () => !$('app').classList.contains('panel-hidden');

// The default width is the one the stylesheet declares, so the two cannot drift.
// Double-clicking the grip puts back whatever the page was built with.
const DEFAULT_PANEL_WIDTH = getComputedStyle($('app')).getPropertyValue('--panel-w').trim();

// Narrow enough that a signature still wraps somewhere sensible, and never so
// wide that the canvas is squeezed out of existence.
const MIN_PANEL_WIDTH = 220;

const panelWidth = () => $('panel').getBoundingClientRect().width;
const maxPanelWidth = () => Math.max(MIN_PANEL_WIDTH, window.innerWidth * 0.8);

// setPanelWidth takes a CSS length, or a number of pixels to be clamped.
//
// Nothing has to be notified. The panel, the tab and the grip all follow the one
// property, and Cytoscape watches its own container for size changes.
function setPanelWidth(width) {
	if (typeof width === 'number') {
		width = Math.round(Math.min(Math.max(width, MIN_PANEL_WIDTH), maxPanelWidth())) + 'px';
	}
	$('app').style.setProperty('--panel-w', width);
}

function installPanelResize() {
	const grip = $('panel-grip');
	const app = $('app');

	const stored = recall('godi.panelWidth');
	if (stored) setPanelWidth(Number(stored) || stored);

	// The width is measured from the right edge of the window rather than from
	// where the drag started, so the panel edge stays under the pointer however
	// far it has travelled.
	const widthAt = (ev) => window.innerWidth - ev.clientX;

	grip.addEventListener('pointerdown', (ev) => {
		ev.preventDefault();
		grip.setPointerCapture(ev.pointerId);
		grip.classList.add('dragging');
		app.classList.add('resizing');
	});

	grip.addEventListener('pointermove', (ev) => {
		if (!grip.hasPointerCapture(ev.pointerId)) return;
		setPanelWidth(widthAt(ev));
	});

	const stop = (ev) => {
		if (!grip.hasPointerCapture(ev.pointerId)) return;
		grip.releasePointerCapture(ev.pointerId);
		grip.classList.remove('dragging');
		app.classList.remove('resizing');
		remember('godi.panelWidth', String(Math.round(panelWidth())));
	};
	grip.addEventListener('pointerup', stop);
	grip.addEventListener('pointercancel', stop);

	grip.addEventListener('dblclick', () => {
		setPanelWidth(DEFAULT_PANEL_WIDTH);
		remember('godi.panelWidth', DEFAULT_PANEL_WIDTH);
	});

	// Reachable without a pointer. The arrows read the way the edge moves, so left
	// widens the panel.
	grip.addEventListener('keydown', (ev) => {
		const step = { ArrowLeft: 16, ArrowRight: -16 }[ev.key];
		if (step === undefined) return;
		ev.preventDefault();
		setPanelWidth(panelWidth() + step);
		remember('godi.panelWidth', String(Math.round(panelWidth())));
	});
}


// A selection made while the panel is away says so on the tab rather than taking
// the space back. An accidental click must not cost the reader the room they asked
// for.
function flashTab() {
	const tab = $('panel-tab');
	tab.classList.remove('flash');
	void tab.offsetWidth; // Restart the animation rather than let it be ignored.
	tab.classList.add('flash');
}

// With nothing picked, the panel has only ever had an instruction to show. The
// notices go here instead. The reader is looking at the panel already, and the
// strip above sent them.
function showNotices(panel) {
	const diagnostics = data.diagnostics || [];
	const hint = make('p', 'empty', 'Pick a node to see how it is wired.');
	if (!diagnostics.length) {
		panel.replaceChildren(hint);
		panel.dataset.view = 'empty';
		return;
	}

	// The same word text and DOT print them under. A reader moving between the
	// formats should not have to learn a third name for one thing.
	const parts = [make('h2', null, 'Notices')];
	for (const d of diagnostics) {
		const notice = make('div', 'notice');
		const head = make('div', 'phead');
		head.append(badge(d.severity, 'severity'), make('span', null, d.message));
		notice.append(head);

		const where = noticeWhere(d);
		if (where) notice.append(where);
		parts.push(notice);
	}
	parts.push(hint);

	panel.replaceChildren(...parts);
	panel.dataset.view = 'notices';
	panel.scrollTop = 0;
}

// Where the notice is about, for the ones that are about anywhere. A schema the
// reader cannot make sense of is about the file and names nothing.
//
// A filtered graph keeps the notices of the nodes it dropped, so the id is only a
// link when there is still something to select.
function noticeWhere(d) {
	const where = make('div', 'where');

	if (d.node) where.append(nodes.has(d.node) ? nodeLink(d.node) : make('span', 'via mono', d.node));
	else if (d.scope) where.append(make('span', 'via', scopePath(d.scope)));

	const p = params.get(d.param);
	if (p) where.append(make('span', 'via', paramLabel(p)));

	return where.children.length ? where : null;
}

function showPanel(id) {
	const panel = $('panel');
	const n = nodes.get(id);
	if (n && !panelOpen()) flashTab();
	if (!n) {
		showNotices(panel);
		return;
	}

	const service = n.kind === 'service';
	const heading = n.title;
	const parts = [make('h2', null, heading)];

	const badges = make('div', 'badges');
	badges.append(badge(n.kind));
	if (service) badges.append(badge(n.shared ? 'shared' : 'not shared'));
	badges.append(badge(n.lazy ? 'lazy' : 'eager'));
	if (!n.autowired) badges.append(badge('not autowired'));
	if (n.instantiated) badges.append(badge('instantiated'));
	if (n.root) badges.append(badge('root', 'root'));
	// Last, so it reads as the answer to the border rather than as another flag.
	if (n.incomplete) badges.append(badge('incomplete', 'incomplete'));
	parts.push(badges);
	parts.push(copyIDButton(n.id));

	// The package, rather than the qualified type and factory. A generic names its
	// type arguments in full, so the qualified forms run to several lines and
	// repeat the heading. This is the part the heading leaves out: where the
	// service lives.
	const dl = make('dl');
	if (n.package) dl.append(make('dt', null, 'Package'), make('dd', 'mono', n.package));
	dl.append(make('dt', null, 'Scope'), make('dd', null, scopePath(n.scope)));
	// A label is the reader's own word for a service. The badges above are godi's
	// vocabulary, and running the two together made a label read as something the
	// container had decided.
	if (n.labels && n.labels.length) {
		const cell = make('dd', 'badges');
		for (const label of n.labels) cell.append(badge(label));
		dl.append(make('dt', null, 'Labels'), cell);
	}
	if (n.registered) dl.append(make('dt', null, 'Registered'), locationCell(n.registered));
	if (n.declared) dl.append(make('dt', null, 'Declared'), locationCell(n.declared));
	parts.push(dl);

	// The signature says in one line what the argument rows say one at a time, so
	// it goes above them. For a service it belongs to the factory rather than to
	// the service, and the headings say so.
	//
	// For a service registered as a value, what implements it is the value itself.
	// A named one is already the heading, so only a literal has anything to add
	// here.
	if (n.signature) {
		parts.push(make('h3', null, n.fromValue ? 'Value' : (service ? 'Factory signature' : 'Signature')));

		const sig = make('div', 'mono sig');
		if (n.fromValue && n.anonymous && n.name) sig.append(make('div', 'sig-name', short(n.name)));
		// Shortened for the same reason the qualified rows went. A generic names
		// its type arguments in full, and a signature carrying several of those is
		// unreadable. The whole thing is a hover away.
		sig.append(make('div', null, short(n.signature)));
		if (short(n.signature) !== n.signature) sig.title = n.signature;
		parts.push(sig);
	}

	const outgoing = out.get(n.id) || [];
	const args = (n.params || []).filter((p) => !isMethod(p));
	const calls = (n.params || []).filter(isMethod);

	// Arguments and method calls are different acts of wiring. One builds the
	// service, the other reaches into it afterwards, so they are not run together
	// in one list.
	if (args.length) {
		parts.push(make('h3', null, service ? 'Factory arguments' : 'Arguments'));
		for (const p of args) parts.push(paramBlock(p, outgoing));
	}

	if (calls.length) {
		parts.push(make('h3', null, 'Method calls'));
		let method = null;
		for (const p of calls) {
			if (p.method !== method) {
				method = p.method;
				parts.push(make('div', 'method', method + '()'));
			}
			parts.push(paramBlock(p, outgoing));
		}
	}

	if (n.elided) {
		parts.push(make('p', 'via',
			n.elided + ' of this node’s neighbours were filtered out of the graph.'));
	}

	const incoming = into.get(n.id) || [];
	parts.push(make('h3', null, 'Used by (' + incoming.length + ')'));
	if (!incoming.length) parts.push(make('p', 'empty', 'Nothing in the container asks for this.'));
	for (const e of incoming) {
		const row = make('div', 'rel');
		row.append(nodeLink(e.from), make('span', 'via', paramLabel(params.get(e.param), e)));
		parts.push(row);
	}

	panel.replaceChildren(...parts);
	panel.dataset.view = 'node';
	panel.scrollTop = 0;
}

function paramBlock(p, outgoing) {
	const block = make('div', 'param');

	// The method name is on the heading above, so the row only needs the index.
	const head = make('div', 'phead');
	head.append(make('span', 'pidx', '#' + p.index));
	head.append(make('span', 'mono', p.short));
	head.append(badge(named(p.origin, p.originPass), 'origin-' + p.origin));
	block.append(head);

	for (const lit of p.literals || []) block.append(make('div', 'plit', '= ' + lit));
	if (p.label) block.append(make('div', 'plit', 'label: ' + p.label));
	if (p.note) block.append(make('div', 'pnote', p.note));

	for (const e of outgoing) {
		if (e.param !== p.id) continue;
		const row = make('div', 'rel');
		row.append(make('span', 'via', '→'), nodeLink(e.to), make('span', 'via', resolutionText(e)));
		block.append(row);
	}

	return block;
}

function resolutionText(e) {
	const bits = [e.resolution];
	if (e.bindInterface) bits.push('binding on ' + e.bindInterface + ' (' + boundBy(e) + ')');
	if (e.cycle) bits.push('cycle');
	return bits.join(' · ');
}

const paramLabel = (p, e) => p ? (p.method ? p.method + ' ' : '') + '#' + p.index + ' ' + p.short : e.type;

// Keys wear keycaps and gestures do not. "Drag the canvas" in a keycap reads as
// something you could press. The modifier is spelled for the platform the reader is
// on, not for both.
const CONTROLS = () => [
	{
		title: 'Keyboard', keys: true, rows: [
			['/', 'Focus the search box'],
			['Enter', 'Jump to the first match, from the search box'],
			[['Esc', 'c'], 'Drop the selection and the search'],
			['f', 'Fit the whole graph'],
			['r', 'Lay the graph out again'],
			['t', 'Cycle the colour scheme'],
			['d', 'Show or hide the detail panel'],
			['l', 'Show or hide the legend'],
			['g', 'Go to a node by its graph id'],
			['?', 'Show or hide this panel'],
		],
	},
	{
		title: 'Mouse', keys: false, rows: [
			[MOD_LABEL + ' + drag', 'Pan from anywhere, over nodes and scopes alike'],
			['Middle-drag', 'Pan from anywhere, the same way'],
			['Wheel', 'Zoom'],
			[MOD_LABEL + ' + wheel', 'Zoom, whatever the wheel is set to do'],
			['Shift + wheel', 'Pan sideways'],
		],
	},
	{
		title: 'Trackpad', keys: false, rows: [
			['Two-finger swipe', 'Pan'],
			['Pinch', 'Zoom'],
			[MOD_LABEL + ' + swipe', 'Zoom'],
		],
	},
];

// What search does is the one thing here that trying it does not reveal, so it
// leads.
const SEARCHES = [
	['Type', 'The full path, github.com/acme/app/http.(*Server)'],
	['Factory', 'The constructor, or the function itself'],
	['Argument types', 'What a factory or function asks for'],
	['Literals', 'The constants passed in, where values were included'],
	['Method calls', 'Their names, and it lets the two above reach their arguments'],
	['Scope', 'root, or the node a child scope belongs to'],
	['Labels', 'Anything given to Labels()'],
];

function showHelp() {
	const parts = [
		make('h2', null, 'Help'),
		make('h3', null, 'Search'),
		make('p', 'via',
			'Matches anywhere in the text and ignores case. Every word has to match, in any order, ' +
			'so "http server" finds a server in the http package. Enter jumps to the first match, ' +
			'and everything that does not match dims. Focus the box to choose what it looks at: ' +
			'the type and the factory to begin with.'),
	];

	const where = make('dl', 'keys');
	for (const [field, what] of SEARCHES) {
		const dt = make('dt');
		dt.append(make('span', 'gesture', field));
		where.append(dt, make('dd', null, what));
	}
	parts.push(where);

	for (const section of CONTROLS()) {
		parts.push(make('h3', null, section.title));

		const dl = make('dl', 'keys');
		for (const [action, what] of section.rows) {
			const dt = make('dt');
			for (const key of [action].flat()) {
				if (dt.children.length) dt.append(make('span', 'or', 'or'));
				dt.append(make(section.keys ? 'kbd' : 'span', section.keys ? null : 'gesture', key));
			}
			dl.append(dt, make('dd', null, what));
		}
		parts.push(dl);
	}

	$('panel').replaceChildren(...parts);
	$('panel').dataset.view = 'help';
}

// ? put the controls there, so pressing it again takes them away. Otherwise the
// reader has to find the tab.
function toggleHelp() {
	if (panelOpen() && $('panel').dataset.view === 'help') {
		setPanel(false);
		return;
	}
	setPanel(true);
	showHelp();
}

function showAbout() {
	const parts = [make('h2', null, 'About'), make('p', null,
		'A godi dependency graph. Everything this page needs is inside it: no network, no server.')];

	parts.push(make('h3', null, 'Built on'));
	for (const c of data.credits || []) {
		const row = make('div', 'rel');
		const link = make('a', 'link', c.name + ' ' + c.version);
		link.href = c.url;
		link.target = '_blank';
		link.rel = 'noreferrer';
		row.append(link, make('span', 'via', c.licence));
		parts.push(row);
	}

	parts.push(make('h3', null, 'Roots'));
	parts.push(make('p', 'via',
		'A root is a node nothing injects: the top of a dependency tree. That is either an entry ' +
		'point or wiring nothing uses, and the container cannot tell the two apart.'));

	$('panel').replaceChildren(...parts);
	$('panel').dataset.view = 'about';
}

// -------------------------------------------------------------- navigation ---

// Command on a Mac, Control everywhere else. They are not interchangeable:
// Control-click is the secondary click on macOS, so Control-dragging would open a
// menu instead of panning.
const APPLE = /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent);
const MOD = APPLE ? 'metaKey' : 'ctrlKey';
const MOD_LABEL = APPLE ? '⌘' : 'Ctrl';

const canvasEl = () => $('cy');

// While the modifier is down, dragging pans instead of moving what is under the
// pointer.
//
// Cytoscape reads grabbability when the button goes down, so the mode is armed on
// the key rather than on the press. That also lets the cursor say so before
// anything is dragged.
let panArmed = false;

function armPan(on) {
	if (panArmed === on) return;
	panArmed = on;
	cy.autoungrabify(on);
	canvasEl().classList.toggle('pan-armed', on);
}

function installPanning() {
	document.addEventListener('keydown', (ev) => { if (ev[MOD]) armPan(true); });
	document.addEventListener('keyup', (ev) => { if (!ev[MOD]) armPan(false); });

	// Tabbing away eats the keyup, and a graph stuck in pan mode looks broken.
	window.addEventListener('blur', () => armPan(false));
	document.addEventListener('visibilitychange', () => { if (document.hidden) armPan(false); });

	// The middle button and the modifier start the same gesture. Cytoscape hands a
	// drag on an ungrabbable scope back as a pan, but not one on a node, so the
	// viewport is driven from here.
	//
	// The press is captured on the way down and stopped there, so Cytoscape never
	// sees it. If it panned as well, the graph would move twice as far.
	let from = null;
	canvasEl().addEventListener('mousedown', (ev) => {
		if (ev.button !== 1 && !(ev.button === 0 && ev[MOD])) return;
		ev.preventDefault();
		ev.stopPropagation();
		from = { x: ev.clientX, y: ev.clientY };
		canvasEl().classList.add('panning');
	}, { capture: true });

	window.addEventListener('mousemove', (ev) => {
		if (!from) return;
		cy.panBy({ x: ev.clientX - from.x, y: ev.clientY - from.y });
		from = { x: ev.clientX, y: ev.clientY };
	});
	window.addEventListener('mouseup', () => {
		if (!from) return;
		from = null;
		canvasEl().classList.remove('panning');
	});

	// Otherwise the middle button scrolls, or pastes.
	canvasEl().addEventListener('auxclick', (ev) => { if (ev.button === 1) ev.preventDefault(); });
}

// A notched mouse wheel reports whole multiples of 120, and Firefox reports lines
// rather than pixels for one. A trackpad sends small or fractional steps, usually
// with a sideways component.
//
// It is a good guess rather than a fact, which is why the reader can override it.
function fromAMouse(ev) {
	if (state.wheel !== 'auto') return state.wheel === 'mouse';
	if (ev.deltaMode !== 0) return true;
	if (ev.wheelDeltaY !== undefined) {
		return ev.wheelDeltaY !== 0 && Math.abs(ev.wheelDeltaY) % 120 === 0;
	}
	return ev.deltaX === 0 && Math.abs(ev.deltaY) >= 100 && Number.isInteger(ev.deltaY);
}

function zoomAt(ev, by) {
	const box = canvasEl().getBoundingClientRect();
	cy.zoom({
		level: cy.zoom() * Math.pow(2, -by / 300),
		renderedPosition: { x: ev.clientX - box.left, y: ev.clientY - box.top },
	});
}

function installWheel() {
	canvasEl().addEventListener('wheel', (ev) => {
		ev.preventDefault();

		// What the reader asked for outranks what the hardware looks like, so the
		// modifiers are read first. A trackpad pinch arrives as a wheel with
		// ctrlKey set, and the modifier means the same thing by hand.
		if (ev.ctrlKey || ev.metaKey) {
			zoomAt(ev, ev.deltaY);
			return;
		}
		// A mouse wheel has only a vertical axis, so shift is how you scroll
		// sideways with one. A trackpad already has both axes, so shift only
		// redirects a purely vertical gesture.
		if (ev.shiftKey) {
			cy.panBy(ev.deltaX === 0
				? { x: -ev.deltaY, y: 0 }
				: { x: -ev.deltaX, y: -ev.deltaY });
			return;
		}
		if (fromAMouse(ev)) {
			zoomAt(ev, ev.deltaY);
			return;
		}
		cy.panBy({ x: -ev.deltaX, y: -ev.deltaY });
	}, { passive: false });
}

// -------------------------------------------------------------- go to node ---

// A node id names a node exactly, which is what makes it worth pasting into a
// message. This is the other end of that exchange.
function installGoto() {
	const dialog = $('goto');
	const field = $('goto-id');
	const note = $('goto-note');
	const advice = note.textContent;

	const say = (text, bad) => {
		note.textContent = text;
		note.classList.toggle('bad', !!bad);
	};

	$('goto-form').addEventListener('submit', (ev) => {
		// The form would close the dialog on its own. A miss should leave it open
		// with the text still in it, to be corrected rather than retyped.
		ev.preventDefault();

		const id = tidyID(field.value);
		if (!id) return;

		const node = nodes.get(id);
		if (!node) {
			say('No node has that id in this graph.', true);
			return;
		}

		// A node a filter has taken away cannot be shown. Saying nothing would
		// look like the id was wrong.
		if (!cy.getElementById(id).visible()) {
			say('That node is filtered out of the picture at the moment.', true);
			return;
		}

		dialog.close();
		select(id, true);
	});

	// Clicking the backdrop is outside every child, which is how a click on it
	// is told from a click on the dialog.
	dialog.addEventListener('click', (ev) => {
		if (ev.target === dialog) dialog.close();
	});

	dialog.addEventListener('close', () => say(advice, false));

	return () => {
		say(advice, false);
		field.value = '';
		if (!dialog.open) dialog.showModal();
		field.focus();
		field.select();
	};
}

// tidyID takes what was pasted rather than what was meant. An id travels through
// chat, which wraps it in backticks, and through mail, which wraps it in quotes.
function tidyID(text) {
	const trimmed = text.trim();
	const first = trimmed[0];
	if ((first === '`' || first === '"' || first === "'") && trimmed.at(-1) === first) {
		return trimmed.slice(1, -1).trim();
	}
	return trimmed;
}

// ----------------------------------------------------------------- actions ---

// Selecting only dims, except while the selection is isolated. Then it changes
// which nodes are on the canvas, and the ones that just appeared are still where
// the last layout put them: scattered across a picture of the whole container, with
// their scope box stretched to reach them.
//
// So the filters' rule applies here too. The shape changed, so lay it out again.
async function select(id, centre) {
	state.focus = id;
	showPanel(id);
	// Laying out again frames what is left, so there is nothing to centre on
	// afterwards. Two animations arguing over the viewport reads as a jump.
	if (apply()) {
		await relayout();
		return;
	}
	if (id && centre) {
		const el = cy.getElementById(id);
		if (el.nonempty() && el.visible()) cy.animate({ center: { eles: el }, duration: 200 });
	}
}

async function reset() {
	state.focus = null;
	state.query = '';
	$('search').value = '';
	$('search-clear').hidden = true;
	showPanel(null);
	// Dropping an isolated selection brings the rest of the container back, and
	// it comes back to wherever the layout of the isolated few left it.
	if (apply()) {
		await relayout();
		return;
	}
	cy.animate({ fit: { eles: cy.elements(':visible'), padding: 30 }, duration: 200 });
}

// refresh reacts to a filter change. It lays out again only when the picture
// changed shape, so ticking a box that hides nothing does not shuffle the graph
// under the reader.
//
// A resize means the boxes themselves changed size. That always needs both a
// rebuild and a fresh layout.
async function refresh({ resize = false } = {}) {
	if (resize) rebuildLabels();
	const changed = apply();
	if (resize || changed) await relayout();
}

// ------------------------------------------------------------------ themes ---

const THEMES = ['auto', 'light', 'dark'];

const currentTheme = () =>
	THEMES.find((t) => document.documentElement.classList.contains('theme-' + t)) || 'auto';

function setTheme(theme) {
	const root = document.documentElement;
	root.classList.remove(...THEMES.map((t) => 'theme-' + t));
	root.classList.add('theme-' + theme);
	$('theme').value = theme;
	$('theme-icon').setAttribute('href', '#i-' + theme);
	remember('godi.theme', theme);

	// Cytoscape draws to a canvas, which cannot read CSS variables, so the
	// stylesheet is rebuilt from the ones now in force. The rules are images with
	// a colour baked in, so they are redrawn too.
	cy.style(stylesheet());
	paintRules();
}

// A page opened from disk may have no storage to speak of, depending on the
// browser, so remembering the choice is best effort and never fatal.
function remember(key, value) {
	try { localStorage.setItem(key, value); } catch { /* nowhere to put it */ }
}

function recall(key) {
	try { return localStorage.getItem(key); } catch { return null; }
}

function installTheme() {
	const stored = recall('godi.theme');
	setTheme(THEMES.includes(stored) ? stored : currentTheme());

	// Following the system setting means restyling when it changes, but only
	// while that is what the reader asked for.
	matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
		if (currentTheme() === 'auto') cy.style(stylesheet());
	});

	$('theme').addEventListener('change', () => setTheme($('theme').value));
}

// The keyboard still cycles, because there is no menu to open from a key.
function cycleTheme() {
	setTheme(THEMES[(THEMES.indexOf(currentTheme()) + 1) % THEMES.length]);
}

// ------------------------------------------------------------------ wiring ---

// Every native control below is read once as it is wired up, not just listened to.
//
// A browser puts back what was typed and dragged when a page is reloaded, whether
// the page asked it to or not. The markup's value is therefore a starting point
// rather than a fact. Without the read, the slider comes back where it was left
// while its label and the graph still hold the default.
//
// It is the same rule as for a stored preference, applied to the one store that is
// not ours.
function installControls() {
	const search = $('search');
	const searchClear = $('search-clear');
	let pending = 0;
	const readSearch = () => {
		state.query = search.value;
		searchClear.hidden = search.value === '';
	};
	search.addEventListener('input', () => {
		readSearch();
		clearTimeout(pending);
		pending = setTimeout(apply, 90);
	});
	readSearch();
	searchClear.addEventListener('click', () => {
		search.value = '';
		state.query = '';
		searchClear.hidden = true;
		search.focus();
		apply();
	});
	search.addEventListener('keydown', (ev) => {
		if (ev.key !== 'Enter') return;
		const hits = found();
		if (hits && hits.size) select([...hits][0], true);
	});

	// The last stop on the slider is "everything the selection reaches". That is
	// what you want when chasing a chain rather than a neighbour.
	const hops = $('hops');
	const unlimited = Number(hops.max);
	const readHops = () => {
		const value = Number(hops.value);
		state.hops = value === unlimited ? Infinity : value;
		$('hops-out').textContent = value === unlimited ? 'all' : hops.value;
	};
	readHops();
	hops.addEventListener('input', () => {
		readHops();
		refresh();
	});

	const dirs = [...document.querySelectorAll('[data-dir]')];
	for (const button of dirs) {
		button.addEventListener('click', () => {
			state.dir = button.dataset.dir;
			for (const other of dirs) other.setAttribute('aria-pressed', String(other === button));
			refresh();
		});
	}

	const routing = $('routing');
	routing.value = state.routing;
	routing.addEventListener('change', () => {
		state.routing = routing.value;
		remember('godi.routing', state.routing);
		cy.style(stylesheet());
	});

	for (const button of document.querySelectorAll('[data-scope]')) {
		const key = button.dataset.scope;
		button.setAttribute('aria-pressed', String(!!state.scopes[key]));
		button.addEventListener('click', () => {
			state.scopes[key] = !state.scopes[key];
			button.setAttribute('aria-pressed', String(state.scopes[key]));
			remember('godi.searchScopes', JSON.stringify(state.scopes));
			rebuildHaystacks();
			apply();
		});
	}

	const wheel = $('wheel');
	wheel.value = state.wheel;
	wheel.addEventListener('change', () => {
		state.wheel = wheel.value;
		remember('godi.wheel', state.wheel);
	});

	const layout = $('layout');
	state.layout = layout.value;
	layout.addEventListener('change', () => {
		state.layout = layout.value;
		cy.style(stylesheet()); // The edge style follows the layout.
		relayout();
	});

	// Dropping method calls or arguments takes rows out of the boxes, so those
	// two change the geometry rather than just what is on show.
	const resizes = new Set(['method', 'args']);

	for (const button of document.querySelectorAll('[data-show], [data-flag]')) {
		button.addEventListener('click', () => {
			const on = button.getAttribute('aria-pressed') !== 'true';
			button.setAttribute('aria-pressed', String(on));

			const key = button.dataset.show || button.dataset.flag;
			if (button.dataset.show) state.show[key] = on;
			else state[key] = on;

			refresh({ resize: resizes.has(key) });
		});
	}

	$('relayout').addEventListener('click', relayout);
	$('fit').addEventListener('click', () => cy.fit(cy.elements(':visible'), 30));
	$('clear').addEventListener('click', reset);
	$('about').addEventListener('click', () => { setPanel(true); showAbout(); });
	$('help').addEventListener('click', () => { setPanel(true); showHelp(); });
	$('panel-tab').addEventListener('click', () => setPanel(!panelOpen()));
	$('legend-tab').addEventListener('click', () => setLegend(!legendOpen()));

	// Tapping the selection again drops it. On a large graph the empty canvas can
	// be a long way off, or off screen, so the node has to be its own way out. A
	// scope's own area counts as empty space here.
	cy.on('tap', 'node', (ev) => {
		if (panArmed || (ev.originalEvent && ev.originalEvent[MOD])) return;
		const node = ev.target;
		if (node.isParent()) { select(null, false); return; }
		select(node.id() === state.focus ? null : node.id(), false);
	});
	cy.on('tap', (ev) => {
		if (panArmed || (ev.originalEvent && ev.originalEvent[MOD])) return;
		if (ev.target === cy) select(null, false);
	});

	document.addEventListener('keydown', (ev) => {
		if (ev.target.tagName === 'INPUT' || ev.target.tagName === 'SELECT') {
			if (ev.key === 'Escape') ev.target.blur();
			return;
		}
		if (ev.key === '/') { ev.preventDefault(); $('search').focus(); }
		else if (ev.key === 'Escape' || ev.key === 'c') reset();
		else if (ev.key === 'f') cy.fit(cy.elements(':visible'), 30);
		else if (ev.key === 'r') relayout();
		else if (ev.key === 't') cycleTheme();
		else if (ev.key === 'd') setPanel(!panelOpen());
		else if (ev.key === 'l') setLegend(!legendOpen());
		else if (ev.key === 'g') { ev.preventDefault(); openGoto(); }
		else if (ev.key === '?') toggleHelp();
	});
}

// Dropping the argument rows changes every box's size, so the labels and the
// edge anchors have to be rebuilt before the graph is laid out again.
function rebuildLabels() {
	cy.batch(() => {
		for (const n of data.nodes) cy.getElementById(n.id).data(boxOf(n));
		for (const e of data.edges) {
			cy.getElementById(e.id).data('offset', rowOffset(layouts.get(e.from), e.param));
		}
	});
	paintRules();
}

// A handle on the graph, for anyone who wants to poke at their own wiring from the
// console. godi.cy is the Cytoscape instance and godi.data is the model.
//
// godi.mod is the modifier this platform pans with, so a driver of the page does
// not have to work that rule out again.
window.godi = { cy, data, state, apply, relayout, mod: MOD };

showSnapshot();
showDiagnostics();
showPanel(null);
setPanel(recall('godi.panel') !== 'hidden');
rebuildHaystacks();
paintRules();
buildLegend();
setLegend(recall('godi.legend') === 'open');

const openGoto = installGoto();

installControls();
installPanelResize();
installPanning();
installWheel();
installTheme();
apply();
relayout();

})();
