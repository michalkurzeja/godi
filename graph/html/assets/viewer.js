'use strict';
(() => {

const $ = (id) => document.getElementById(id);
const data = JSON.parse($('godi-data').textContent);
const engine = document.documentElement.dataset.layout;

// Node labels are laid out by hand rather than wrapped, so that the line a
// given argument sits on is known: that is what lets an edge leave the node at
// the row it feeds instead of from the middle of the box.
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
const unreachable = data.nodes.filter((n) => !n.reachable).length;

// How many edges a node has before any filtering, which is what tells a node a
// filter stripped bare from one that never had a connection at all.
const wired = new Map(data.nodes.map((n) =>
	[n.id, (out.get(n.id) || []).length + (into.get(n.id) || []).length]));

function scopePath(id) {
	const parts = [];
	for (let s = scopes.get(id); s; s = s.parent ? scopes.get(s.parent) : null) {
		parts.unshift(s.label);
		if (!s.parent) break;
	}
	return parts.join(' › ') || id;
}

// Only an extension is worth naming. godi's own automation runs under a pass
// too, but "autowiring (autowiring)" tells the reader nothing.
const named = (origin, pass) => origin === 'compiler-pass' && pass ? origin + ': ' + pass : origin;
const wiredBy = (e) => named(e.origin, e.originPass);
const boundBy = (e) => named(e.bindOrigin, e.bindPass);

// A service is known by the type it provides; a function by its name, since a
// function's type is just its signature - and a wrapped two-line signature makes
// a poor heading. This is the same swap graph/dot makes in its node labels.
const displayName = (n) => n.kind === 'function' ? short(n.name) : n.short;

// The line under the heading: the other half of the same pair.
const displaySub = (n) => n.kind === 'function' ? n.short : short(n.name);

function short(sig) {
	const i = sig.lastIndexOf('/');
	return i < 0 ? sig : sig.slice(i + 1);
}

const clip = (s, max = MAX_CHARS) => s.length <= max ? s : s.slice(0, max - 1) + '…';

// ------------------------------------------------------------------ state ---

const state = {
	query: '',
	focus: null,
	hops: 1,
	dir: 'both',
	isolate: false,
	args: true,
	layout: 'layered',
	show: { manual: true, autowiring: true, 'compiler-pass': true, method: true, unreachable: true },
};

// ------------------------------------------------------------------ labels ---

// rows returns the lines of a node's box, and where each argument landed, so
// that an edge can be anchored to its own row.
const isMethod = (p) => p.kind === 'method-arg' || p.kind === 'method-receiver';

// shown reports whether an injection point belongs in the picture at all.
// Hiding method calls has to drop their rows too, not just their edges: a row
// nothing can arrive at is worse than no row.
const shownParam = (p) => state.args && (state.show.method || !isMethod(p));

function rows(n) {
	const lines = [];
	const head = (n.reachable ? '' : '⚠ ') + (n.kind === 'function' ? 'ƒ ' : '');
	const heading = displayName(n);
	const sub = displaySub(n);
	lines.push(clip(head + heading));
	if (sub && sub !== heading) lines.push(clip(sub));

	const headerLines = lines.length;
	const paramLine = new Map();

	for (const p of n.params || []) {
		if (!shownParam(p)) continue;
		paramLine.set(p.id, lines.length);
		let text = (p.method ? p.method + ' ' : '') + p.index + ' ◂ ' + p.short;
		if (p.literals && p.literals.length) text += ' = ' + p.literals.join(', ');
		lines.push(clip(text, MAX_CHARS + 6));
	}

	return { text: lines.join('\n'), count: lines.length, headerLines, paramLine };
}

// rowOffset is how far the argument's row sits from the middle of the box.
// Cytoscape centres the label block, so the middle line is at zero.
function rowOffset(layout, paramID) {
	const line = layout.paramLine.get(paramID);
	if (line === undefined) return 0;
	return (line + 0.5 - layout.count / 2) * LINE;
}

const layouts = new Map();

function buildElements() {
	const els = [];

	for (const s of data.scopes) {
		els.push({ data: { id: 'scope:' + s.id, label: s.label, parent: s.parent ? 'scope:' + s.parent : undefined } });
	}

	for (const n of data.nodes) {
		const layout = rows(n);
		layouts.set(n.id, layout);
		els.push({
			data: {
				id: n.id, parent: 'scope:' + n.scope, label: layout.text,
				kind: n.kind, shared: n.shared, lazy: n.lazy, reachable: n.reachable,
				height: layout.count * LINE + 2 * PAD,
			},
		});
	}

	for (const e of data.edges) {
		els.push({
			data: {
				id: e.id, source: e.from, target: e.to,
				origin: e.origin, decidedBy: e.decidedBy, kind: e.kind,
				bound: !!e.bindInterface, cycle: !!e.cycle,
				// A pass is named however it was responsible: it may have wired
				// the argument, or created the binding the argument resolved
				// through. The colour says a pass decided; this says which.
				label: e.pass || '',
				offset: rowOffset(layouts.get(e.from), e.param),
			},
		});
	}

	return els;
}

// ------------------------------------------------------------------ colours ---

// Cytoscape draws to a canvas, so it cannot read CSS variables. They are
// resolved here instead and re-read whenever the colour scheme changes, which
// is what makes switching themes instant.
function palette() {
	const css = getComputedStyle(document.documentElement);
	const v = (name) => css.getPropertyValue(name).trim();
	return {
		text: v('--text'), muted: v('--muted'), accent: v('--accent'), warn: v('--warn'),
		node: v('--node'), nodeBorder: v('--node-border'),
		scope: v('--scope'), scopeBorder: v('--scope-border'),
		manual: v('--manual'), auto: v('--auto'), pass: v('--pass'),
	};
}

// The two channels are independent, as in the DOT output: the arrowhead says
// how the dependency was matched, the colour says who decided on it. The
// filters key off the same field, so what is drawn purple is what the compiler
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
				'background-color': p.node,
				'border-color': p.nodeBorder,
				'border-width': (n) => n.data('lazy') ? 1 : 2.2,
				'border-style': (n) => n.data('kind') === 'service' && !n.data('shared') ? 'dashed' : 'solid',
				label: 'data(label)',
				color: p.text,
				'font-family': 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
				'font-size': FONT,
				'line-height': LINE_HEIGHT,
				// "wrap" is what honours the newlines in the label. With "none"
				// the whole box collapses onto one line. Lines are clipped to a
				// known length already, so the max width never actually bites.
				'text-wrap': 'wrap',
				'text-max-width': 1000,
				'text-justification': 'left',
				'text-valign': 'center',
				'text-halign': 'center',
				width: 'label',
				height: 'data(height)',
				padding: PAD,
			},
		},
		{ selector: 'node[!reachable]', style: { 'border-color': p.warn } },
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
				'curve-style': state.layout === 'layered' ? 'taxi' : 'bezier',
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
		{ selector: '.dim', style: { opacity: 0.12, 'text-opacity': 0 } },
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
	wheelSensitivity: 0.25,
	boxSelectionEnabled: false,
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
// The DOT it builds is a request for geometry, not a drawing: sizes, nesting
// and edges, with no colour or label in it. The sizes have to come from
// Cytoscape, because Cytoscape is what draws the boxes.
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

	const emitScope = (scope) => {
		lines.push('\tsubgraph ' + q('cluster_' + scope.id) + ' {');
		for (const node of inScope.get('scope:' + scope.id) || []) emitNode(node);
		for (const child of data.scopes.filter((s) => s.parent === scope.id)) emitScope(child);
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
		if (terms.every((t) => n.search.includes(t))) hit.add(n.id);
	}
	return hit;
}

// neighbourhood walks outwards from the selection, following only the edges
// still on show: hiding a kind of wiring hides what it reached.
function neighbourhood(id, live) {
	const seen = new Set([id]);
	let frontier = [id];
	for (let hop = 0; hop < state.hops && frontier.length; hop++) {
		const next = [];
		for (const cur of frontier) {
			if (state.dir !== 'up') {
				for (const e of out.get(cur) || []) {
					if (live.has(e.id) && !seen.has(e.to)) { seen.add(e.to); next.push(e.to); }
				}
			}
			if (state.dir !== 'down') {
				for (const e of into.get(cur) || []) {
					if (live.has(e.id) && !seen.has(e.from)) { seen.add(e.from); next.push(e.from); }
				}
			}
		}
		frontier = next;
	}
	return seen;
}

// apply is the single place anything becomes hidden, dimmed or marked. It
// returns whether the set of visible elements changed, because that is when the
// graph is worth laying out again.
function apply() {
	const hidden = new Set();
	if (!state.show.unreachable) {
		for (const n of data.nodes) if (!n.reachable) hidden.add(n.id);
	}

	const live = new Set();
	for (const e of data.edges) {
		if (hidden.has(e.from) || hidden.has(e.to)) continue;
		if (e.decidedBy !== 'none' && !state.show[e.decidedBy]) continue;
		if (!state.show.method && isMethod(e)) continue;
		live.add(e.id);
	}

	// A node a filter has stripped of every edge is an artefact of the filter
	// rather than anything about the wiring, so it goes with them. A node that
	// never had an edge at all is a finding in its own right - that is what
	// unreachable is for - so it stays until that box says otherwise.
	const connected = new Set();
	for (const id of live) {
		const e = edges.get(id);
		connected.add(e.from);
		connected.add(e.to);
	}
	const stranded = (id) => wired.get(id) > 0 && !connected.has(id);

	const hits = found();
	const near = nodes.has(state.focus) ? neighbourhood(state.focus, live) : null;
	const outside = (id) => near !== null && !near.has(id);

	let changed = false;
	cy.batch(() => {
		for (const n of data.nodes) {
			const el = cy.getElementById(n.id);
			const gone = hidden.has(n.id) || stranded(n.id) || (state.isolate && outside(n.id));
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
	});

	const shownNodes = cy.nodes(':childless:visible').length;
	const shownEdges = cy.edges(':visible').length;
	$('found').textContent = hits === null ? '' : hits.size + (hits.size === 1 ? ' match' : ' matches');

	const counts = [
		shownNodes + ' of ' + data.nodes.length + ' nodes',
		shownEdges + ' of ' + data.edges.length + ' edges',
		unreachable + ' not reachable from a root',
	];
	if (data.notices && data.notices.length) counts.push(data.notices.length + ' warnings');
	$('counts').textContent = counts.join(' · ');
	if (data.notices && data.notices.length) $('counts').title = data.notices.join('\n');

	return changed;
}

// -------------------------------------------------------------- side panel ---

const make = (tag, cls, text) => {
	const el = document.createElement(tag);
	if (cls) el.className = cls;
	if (text !== undefined) el.textContent = text;
	return el;
};

function nodeLink(id) {
	const n = nodes.get(id);
	const btn = make('button', 'link', n ? displayName(n) : id);
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

function showPanel(id) {
	const panel = $('panel');
	const n = nodes.get(id);
	if (!n) {
		panel.replaceChildren(make('p', 'empty', 'Pick a node to see how it is wired.'));
		return;
	}

	const service = n.kind === 'service';
	const heading = displayName(n);
	const parts = [make('h2', null, heading)];

	const badges = make('div', 'badges');
	badges.append(badge(n.kind));
	if (n.kind === 'service') badges.append(badge(n.shared ? 'shared' : 'not shared'));
	badges.append(badge(n.lazy ? 'lazy' : 'eager'));
	if (!n.autowired) badges.append(badge('not autowired'));
	if (n.instantiated) badges.append(badge('instantiated'));
	if (!n.reachable) badges.append(badge('not reachable from a root', 'warn'));
	for (const label of n.labels || []) badges.append(badge(label));
	parts.push(badges);

	// Every value gets a label, so nothing floats loose under the heading. The
	// qualified forms are what these rows are for, so a row whose value is the
	// heading again - which happens in package main, where there is no import
	// path to qualify with - has nothing to add and is left out.
	const dl = make('dl');
	if (service && n.type !== heading) {
		dl.append(make('dt', null, 'Type'), make('dd', 'mono', n.type));
	}
	if (n.name !== heading) {
		dl.append(make('dt', null, service ? 'Factory' : 'Func'), make('dd', 'mono', n.name));
	}
	dl.append(make('dt', null, 'Scope'), make('dd', null, scopePath(n.scope)));
	if (n.registered) dl.append(make('dt', null, 'Registered'), locationCell(n.registered));
	if (n.defined) dl.append(make('dt', null, 'Defined'), locationCell(n.defined));
	parts.push(dl);

	// The signature says in one line what the argument rows say one at a time,
	// so it goes above them. For a service it belongs to the factory rather than
	// to the service itself, and the headings say so.
	if (n.signature) {
		parts.push(make('h3', null, service ? 'Factory signature' : 'Signature'));
		parts.push(make('div', 'mono sig', n.signature));
	}

	const outgoing = out.get(n.id) || [];
	const args = (n.params || []).filter((p) => !isMethod(p));
	const calls = (n.params || []).filter(isMethod);

	// Arguments and method calls are different acts of wiring - one builds the
	// service, the other reaches into it afterwards - so they are not run
	// together in one list.
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

	const incoming = into.get(n.id) || [];
	parts.push(make('h3', null, 'Used by (' + incoming.length + ')'));
	if (!incoming.length) parts.push(make('p', 'empty', 'Nothing in the container asks for this.'));
	for (const e of incoming) {
		const row = make('div', 'rel');
		row.append(nodeLink(e.from), make('span', 'via', paramLabel(params.get(e.param), e)));
		parts.push(row);
	}

	panel.replaceChildren(...parts);
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
	if (e.bindInterface) bits.push('binding on ' + short(e.bindInterface) + ' (' + boundBy(e) + ')');
	if (e.cycle) bits.push('cycle');
	return bits.join(' · ');
}

const paramLabel = (p, e) => p ? (p.method ? p.method + ' ' : '') + '#' + p.index + ' ' + p.short : e.type;

// Keys wear keycaps; gestures do not, because "Drag the canvas" in a keycap
// reads as something you could press.
const SHORTCUTS = [
	{
		title: 'Keyboard', keys: true, rows: [
			['/', 'Focus the search box'],
			['Enter', 'Jump to the first match, from the search box'],
			['Esc', 'Drop the selection and the search'],
			['f', 'Fit the whole graph'],
			['r', 'Lay the graph out again'],
			['t', 'Cycle the colour scheme'],
			['?', 'Show this list'],
		],
	},
	{
		title: 'Mouse', keys: false, rows: [
			['Click a node', 'Select it, or clear it if it is already selected'],
			['Click a scope', 'Clear the selection'],
			['Drag a node', 'Move it; it stays where you put it'],
			['Drag the canvas', 'Pan'],
			['Scroll', 'Zoom'],
		],
	},
];

function showShortcuts() {
	const parts = [make('h2', null, 'Shortcuts')];

	for (const section of SHORTCUTS) {
		parts.push(make('h3', null, section.title));

		const dl = make('dl', 'keys');
		for (const [action, what] of section.rows) {
			const dt = make('dt');
			dt.append(make(section.keys ? 'kbd' : 'span', section.keys ? null : 'gesture', action));
			dl.append(dt, make('dd', null, what));
		}
		parts.push(dl);
	}

	$('panel').replaceChildren(...parts);
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

	parts.push(make('h3', null, 'Reachability'));
	parts.push(make('p', 'via',
		'Services fetched at runtime with SvcByType and friends are invisible to the container, ' +
		'so "not reachable" marks a candidate for dead wiring, never a proof of it.'));

	$('panel').replaceChildren(...parts);
}

// ----------------------------------------------------------------- actions ---

function select(id, centre) {
	state.focus = id;
	showPanel(id);
	apply();
	if (id && centre) {
		const el = cy.getElementById(id);
		if (el.nonempty() && el.visible()) cy.animate({ center: { eles: el }, duration: 200 });
	}
}

function reset() {
	state.focus = null;
	state.query = '';
	$('search').value = '';
	showPanel(null);
	apply();
	cy.animate({ fit: { eles: cy.elements(':visible'), padding: 30 }, duration: 200 });
}

// refresh reacts to a filter change. It lays out again only when the picture
// actually changed shape, so ticking a box that hides nothing does not shuffle
// the graph under the reader. A resize means the boxes themselves changed size,
// which always needs both a rebuild and a fresh layout.
async function refresh({ resize = false } = {}) {
	if (resize) rebuildLabels();
	const changed = apply();
	if (resize || changed) await relayout();
}

// ------------------------------------------------------------------ themes ---

const THEMES = ['auto', 'light', 'dark'];
const THEME_LABELS = { auto: 'Auto', light: 'Light', dark: 'Dark' };

const currentTheme = () =>
	THEMES.find((t) => document.documentElement.classList.contains('theme-' + t)) || 'auto';

function setTheme(theme) {
	const root = document.documentElement;
	root.classList.remove(...THEMES.map((t) => 'theme-' + t));
	root.classList.add('theme-' + theme);
	$('theme-label').textContent = THEME_LABELS[theme];
	$('theme-icon').setAttribute('href', '#i-' + theme);
	remember('godi.theme', theme);

	// Cytoscape draws to a canvas, which cannot read CSS variables, so the
	// stylesheet has to be rebuilt from the ones now in force.
	cy.style(stylesheet());
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
	// while that is still what the reader asked for.
	matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
		if (currentTheme() === 'auto') cy.style(stylesheet());
	});

	$('theme').addEventListener('click', cycleTheme);
}

function cycleTheme() {
	setTheme(THEMES[(THEMES.indexOf(currentTheme()) + 1) % THEMES.length]);
}

// ------------------------------------------------------------------ wiring ---

function installControls() {
	const search = $('search');
	let pending = 0;
	search.addEventListener('input', () => {
		state.query = search.value;
		clearTimeout(pending);
		pending = setTimeout(apply, 90);
	});
	search.addEventListener('keydown', (ev) => {
		if (ev.key !== 'Enter') return;
		const hits = found();
		if (hits && hits.size) select([...hits][0], true);
	});

	// The last stop on the slider is "everything the selection reaches", which is
	// what you want as soon as you are chasing a chain rather than a neighbour.
	const hops = $('hops');
	const unlimited = Number(hops.max);
	hops.addEventListener('input', () => {
		const value = Number(hops.value);
		state.hops = value === unlimited ? Infinity : value;
		$('hops-out').textContent = value === unlimited ? 'all' : hops.value;
		refresh();
	});

	$('dir').addEventListener('change', (ev) => { state.dir = ev.target.value; refresh(); });

	$('layout').addEventListener('change', (ev) => {
		state.layout = ev.target.value;
		cy.style(stylesheet()); // The edge style follows the layout.
		relayout();
	});

	// Dropping method calls or arguments takes rows out of the boxes, so those
	// two change the geometry rather than just what is on show.
	const resizes = new Set(['method', 'args']);

	for (const box of document.querySelectorAll('[data-show]')) {
		box.addEventListener('change', () => {
			state.show[box.dataset.show] = box.checked;
			refresh({ resize: resizes.has(box.dataset.show) });
		});
	}
	for (const box of document.querySelectorAll('[data-flag]')) {
		box.addEventListener('change', () => {
			state[box.dataset.flag] = box.checked;
			refresh({ resize: resizes.has(box.dataset.flag) });
		});
	}

	$('relayout').addEventListener('click', relayout);
	$('fit').addEventListener('click', () => cy.fit(cy.elements(':visible'), 30));
	$('clear').addEventListener('click', reset);
	$('about').addEventListener('click', showAbout);
	$('shortcuts').addEventListener('click', showShortcuts);

	// Tapping the selection again drops it. On a large graph the empty canvas
	// can be a long way off, or off screen entirely, so the node has to be its
	// own way out. A scope's own area counts as empty space for this.
	cy.on('tap', 'node', (ev) => {
		const node = ev.target;
		if (node.isParent()) { select(null, false); return; }
		select(node.id() === state.focus ? null : node.id(), false);
	});
	cy.on('tap', (ev) => { if (ev.target === cy) select(null, false); });

	document.addEventListener('keydown', (ev) => {
		if (ev.target.tagName === 'INPUT' || ev.target.tagName === 'SELECT') {
			if (ev.key === 'Escape') ev.target.blur();
			return;
		}
		if (ev.key === '/') { ev.preventDefault(); $('search').focus(); }
		else if (ev.key === 'Escape') reset();
		else if (ev.key === 'f') cy.fit(cy.elements(':visible'), 30);
		else if (ev.key === 'r') relayout();
		else if (ev.key === 't') cycleTheme();
		else if (ev.key === '?') showShortcuts();
	});
}

// Dropping the argument rows changes every box's size, so the labels and the
// edge anchors have to be rebuilt before the graph is laid out again.
function rebuildLabels() {
	cy.batch(() => {
		for (const n of data.nodes) {
			const layout = rows(n);
			layouts.set(n.id, layout);
			cy.getElementById(n.id).data({ label: layout.text, height: layout.count * LINE + 2 * PAD });
		}
		for (const e of data.edges) {
			cy.getElementById(e.id).data('offset', rowOffset(layouts.get(e.from), e.param));
		}
	});
}

// A handle on the graph, for anyone who wants to poke at their own wiring from
// the console: godi.cy is the Cytoscape instance, godi.data the model.
window.godi = { cy, data, state, apply, relayout };

installControls();
installTheme();
apply();
relayout();

})();
