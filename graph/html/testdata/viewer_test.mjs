// Regression suite for the viewer, driven from browser_test.go.
//
// Everything asserted here was reported broken at some point. The fixture it
// runs against is regressionModel() in browser_test.go; the node ids and the
// counts below come from there, so the two move together.
//
//   node viewer_test.mjs <page.html> <chrome> <profile-dir>
//
// It speaks the DevTools protocol directly - node's own WebSocket and fetch are
// enough - so the repo needs no JavaScript toolchain. Results go to stdout as
// one JSON object per line; anything else is a diagnostic.

import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';

const [, , pagePath, chromePath, profileDir] = process.argv;
const PAGE = 'file://' + pagePath;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const note = (...args) => console.log('#', ...args);

let failures = 0;
function report(name, ok, detail) {
	if (!ok) failures++;
	console.log(JSON.stringify({ name, ok, detail: detail ?? '' }));
}

async function test(name, fn) {
	try {
		const outcome = await fn();
		if (outcome === true || outcome === undefined) report(name, true);
		else report(name, false, typeof outcome === 'string' ? outcome : JSON.stringify(outcome));
	} catch (err) {
		report(name, false, String(err && err.stack ? err.stack : err));
	}
}

const eq = (got, want, what) =>
	JSON.stringify(got) === JSON.stringify(want) || `${what}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`;

// ---------------------------------------------------------------- browser ---

const chrome = spawn(chromePath, [
	'--headless', '--disable-gpu', '--no-sandbox', '--window-size=1600,1000',
	'--remote-debugging-port=0', '--user-data-dir=' + profileDir, PAGE,
], { stdio: 'ignore' });

// Port zero means Chrome picks one and writes it into the profile, which is
// what keeps concurrent runs from colliding.
async function devtoolsPort() {
	for (let i = 0; i < 80; i++) {
		try {
			return readFileSync(profileDir + '/DevToolsActivePort', 'utf8').split('\n')[0].trim();
		} catch {
			await sleep(250);
		}
	}
	throw new Error('chrome never reported a debugging port');
}

async function target(port) {
	for (let i = 0; i < 80; i++) {
		try {
			const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
			const page = list.find((t) => t.type === 'page' && t.url === PAGE);
			if (page) return page.webSocketDebuggerUrl;
		} catch { /* not up yet */ }
		await sleep(250);
	}
	throw new Error('chrome never opened the page');
}

const ws = new WebSocket(await target(await devtoolsPort()));
await new Promise((resolve, reject) => {
	ws.addEventListener('open', resolve);
	ws.addEventListener('error', reject);
});

let nextID = 0;
const pending = new Map();
const consoleErrors = [];
ws.addEventListener('message', (event) => {
	const msg = JSON.parse(event.data);
	if (msg.id) pending.get(msg.id)?.(msg);
	if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') {
		consoleErrors.push(msg.params.entry.text);
	}
	if (msg.method === 'Runtime.exceptionThrown') {
		const d = msg.params.exceptionDetails;
		consoleErrors.push(d.exception?.description || d.text);
	}
});

function send(method, params = {}) {
	const id = ++nextID;
	ws.send(JSON.stringify({ id, method, params }));
	return new Promise((resolve) => pending.set(id, resolve));
}

async function ev(expression) {
	const res = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
	const thrown = res.result?.exceptionDetails;
	if (thrown) throw new Error(thrown.exception?.description || thrown.text);
	return res.result.result.value;
}

await send('Log.enable');
await send('Runtime.enable');

// ------------------------------------------------------------------ helpers ---

const READY = `typeof godi !== 'undefined' && !!godi.cy
	&& !document.getElementById('canvas').classList.contains('busy')`;

async function settle() {
	for (let i = 0; i < 60; i++) {
		if (await ev(READY) === true) { await sleep(150); return; }
		await sleep(250);
	}
	throw new Error('the viewer never settled');
}

const SERVER = 'root/svc:app.(*Server)';

const label = (id) => ev(`godi.cy.getElementById(${JSON.stringify(id)}).data('label')`);
const rows = async (id) => (await label(id)).split('\n');
const height = (id) => ev(`godi.cy.getElementById(${JSON.stringify(id)}).outerHeight()`);
const visibleNodes = () => ev(`godi.cy.nodes(':childless:visible').map(n => n.id()).sort()`);
const visibleEdges = () => ev(`godi.cy.edges(':visible').map(e => e.id()).sort()`);
const dimmed = () => ev(`godi.cy.nodes(':childless').filter(n => n.hasClass('dim')).map(n => n.id()).sort()`);
const panelText = () => ev(`document.getElementById('panel').textContent`);
const headings = () => ev(`[...document.querySelectorAll('#panel h3')].map(h => h.textContent)`);

async function toggle(name, on) {
	await ev(`(() => {
		const button = document.querySelector('[data-show=${JSON.stringify(name)}], [data-flag=${JSON.stringify(name)}]');
		if ((button.getAttribute('aria-pressed') === 'true') !== ${on}) button.click();
		return true;
	})()`);
	await settle();
}

// The result has to be discarded: a Cytoscape collection is circular, and
// asking the protocol to serialise one fails after the tap has already landed,
// which reads as the assertion failing rather than the harness.
const tapNode = async (id) => {
	await ev(`(() => { godi.cy.getElementById(${JSON.stringify(id)}).emit('tap'); return true; })()`);
	await sleep(120);
};

const tapScope = async () => {
	await ev(`(() => { godi.cy.nodes(':parent')[0].emit('tap'); return true; })()`);
	await sleep(120);
};

// A tap toggles, and the panel may be showing something else entirely - the
// shortcuts, the credits - while the selection is unchanged. So selecting means
// clearing first and then tapping, which leaves both the selection and the
// panel where the test expects them however the previous test left things.
const selectNode = async (id) => {
	await tapScope();
	await tapNode(id);
};

async function setHops(value) {
	await ev(`(() => {
		const h = document.getElementById('hops');
		h.value = ${JSON.stringify(String(value))};
		h.dispatchEvent(new Event('input'));
	})()`);
	await settle();
}

// -------------------------------------------------------------------- tests ---

await settle();

await test('page boots with the whole fixture', async () =>
	eq(await ev(`[godi.cy.nodes(':childless').length, godi.cy.edges().length]`), [8, 5], 'nodes and edges'));

// --- Graphviz laid it out, rather than everything landing on one spot -------

await test('graphviz placed the nodes', async () => {
	const spread = await ev(`(() => {
		const xs = godi.cy.nodes(':childless').map(n => n.position('x'));
		return Math.max(...xs) - Math.min(...xs);
	})()`);
	return spread > 200 || `nodes span only ${spread}px, so the layout did not run`;
});

// --- reported: the method calls filter did not hide anything ---------------

// --- the node box reads as sections -----------------------------------------

await test('the label leaves a blank row between sections', async () => {
	const lines = await rows(SERVER);
	const blanks = lines.map((l, i) => l.trim() === '' ? i : -1).filter((i) => i >= 0);
	return eq(blanks, [1, 4], `blank rows in ${JSON.stringify(lines)}`);
});

// A service is named by the type it provides. The factory used to sit under it
// in every box in the graph, saying little the type did not, and it is in the
// panel with a link to its source.
await test('a service box does not name its factory', async () => {
	const lines = await rows(SERVER);
	return (lines[0] === '▲ app.(*Server)' && !lines.some((l) => l.includes('NewServer')))
		|| `the box reads ${JSON.stringify(lines.slice(0, 2))}`;
});

// A function is named by what it is called. The fixture has no function in it,
// so one is borrowed.
const borrowAsFunction = (props) => ev(`(() => {
	const n = godi.data.nodes.find((x) => x.id === ${JSON.stringify(SERVER)});
	Object.assign(n, ${JSON.stringify(props)});
	return true;
})()`);

const rebuildBoxes = async () => { await toggle('args', false); await toggle('args', true); };

await test('a function box is its name and nothing else', async () => {
	await borrowAsFunction({ kind: 'function' });
	await rebuildBoxes();
	const lines = await rows(SERVER);

	await borrowAsFunction({ kind: 'service' });
	await rebuildBoxes();

	return (lines[0] === '▲ ƒ app.NewServer' && lines[1].trim() === '')
		|| `the box reads ${JSON.stringify(lines.slice(0, 2))}`;
});

// The runtime's name for a literal - the enclosing function and a counter -
// identifies it without describing it, so the signature is the one thing that
// says what it is.
await test('unless it is a literal, which has only a signature to go on', async () => {
	await borrowAsFunction({ kind: 'function', anonymous: true });
	await rebuildBoxes();
	const lines = await rows(SERVER);

	await borrowAsFunction({ kind: 'service', anonymous: false });
	await rebuildBoxes();

	return eq(lines.slice(0, 2), ['▲ ƒ app.NewServer', 'app.(*Server)'], 'the first two rows');
});

// A label is one colour throughout, so the rules are drawn as an image instead
// of typed into it.
await test('and a rule is drawn across each one', async () => {
	const uri = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).data('rules')`);
	const svg = decodeURIComponent(uri.replace(/^data:image\/svg\+xml;charset=utf-8,/, ''));
	return (uri.startsWith('data:image/svg+xml;charset=utf-8,') && (svg.match(/<line /g) || []).length === 2)
		|| `the rules image is ${uri.slice(0, 60)}`;
});

// The image is stretched over the box, so a guess at the box's size slides the
// lines - and slides the lower ones furthest.
await test('the rules image is sized to the box it is stretched over', async () => {
	const [uri, w, h] = await ev(`(() => {
		const n = godi.cy.getElementById(${JSON.stringify(SERVER)});
		return [n.data('rules'), n.outerWidth(), n.outerHeight()];
	})()`);
	const svg = decodeURIComponent(uri.replace(/^data:image\/svg\+xml;charset=utf-8,/, ''));
	const box = svg.match(/viewBox="0 0 ([\d.]+) ([\d.]+)"/);
	return (Math.abs(Number(box[1]) - w) < 0.01 && Math.abs(Number(box[2]) - h) < 0.01)
		|| `viewBox ${box[1]}x${box[2]} against a box of ${w}x${h}`;
});

// A filtered graph stops where the filter did, not where the wiring does. The
// box has to say so, or it reads as a service with nothing else around it.
await test('a node says when a filter cut its neighbours off', async () => {
	const elide = (n) => ev(`(() => {
		const node = godi.data.nodes.find((x) => x.id === ${JSON.stringify(SERVER)});
		${n ? `node.elided = ${n};` : 'delete node.elided;'}
		return true;
	})()`);

	// Toggling a row filter is what rebuilds the labels from the model.
	const rebuild = async () => { await toggle('args', false); await toggle('args', true); };

	await elide(3);
	await rebuild();
	const cut = await rows(SERVER);

	await elide(0);
	await rebuild();
	const whole = await rows(SERVER);

	return (cut.at(-1) === '⋯ +3 more' && cut.at(-2) === '' && !whole.some((l) => l.includes('more')))
		|| `cut: ${JSON.stringify(cut.slice(-3))}, whole: ${JSON.stringify(whole.slice(-2))}`;
});

await test('and the rules stop short of the border', async () => {
	const uri = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).data('rules')`);
	const svg = decodeURIComponent(uri.replace(/^data:image\/svg\+xml;charset=utf-8,/, ''));
	const x1 = Number(svg.match(/x1="([\d.]+)"/)[1]);
	// The same margin the text keeps.
	return Math.abs(x1 - 7) < 0.01 || `a rule starts ${x1}px from the edge`;
});

// The ports are read off the line each argument lands on, so a blank row for a
// rule has to push them down with it.
await test('the ports follow the rules down the box', async () => {
	const lines = await rows(SERVER);
	const edges = await ev(`(() => {
		const node = godi.data.nodes.find((x) => x.id === ${JSON.stringify(SERVER)});
		return godi.data.edges.filter((e) => e.from === node.id).map((e) => {
			const p = node.params.find((q) => q.id === e.param);
			return {
				head: (p.method ? p.method + ' ' : '') + p.index + ' ',
				offset: godi.cy.getElementById(e.id).data('offset'),
			};
		});
	})()`);

	const LINE = 11 * 1.3;
	const adrift = edges.filter(({ head, offset }) => {
		const row = lines.findIndex((l) => l.startsWith(head));
		return row < 0 || Math.abs(offset - (row + 0.5 - lines.length / 2) * LINE) > 0.01;
	});
	return adrift.length === 0 || `edges no longer leaving their own row: ${JSON.stringify(adrift)}`;
});

await test('method rows are in the node box', async () => {
	const lines = await rows(SERVER);
	return (lines.some((l) => l.startsWith('SetLogger')) && lines.some((l) => l.startsWith('SetTimeout')))
		|| `no method rows in ${JSON.stringify(lines)}`;
});

const withMethods = await height(SERVER);

await test('unticking method calls drops their rows', async () => {
	await toggle('method', false);
	const lines = await rows(SERVER);
	return !lines.some((l) => l.startsWith('Set')) || `method rows survived: ${JSON.stringify(lines)}`;
});

await test('unticking method calls hides the edge one carries', async () =>
	(await visibleEdges()).every((id) => !id.includes('SetLogger'))
	|| 'the SetLogger edge is still drawn');

await test('the box shrinks by the rows removed, and their rule', async () => {
	const now = await height(SERVER);
	const shrunk = withMethods - now;
	return Math.abs(shrunk - 3 * 14.3) < 1 || `shrank by ${shrunk}px, expected ${3 * 14.3}`;
});

await test('the node the method call fed goes with it', async () =>
	!(await visibleNodes()).includes('root/svc:app.ConsoleLogger')
	|| 'ConsoleLogger is still shown with nothing wiring it');

await test('re-ticking method calls restores the box', async () => {
	await toggle('method', true);
	return await height(SERVER) === withMethods || 'the box did not return to its old height';
});

// --- reported: the only way out of a selection was a distant empty click ---

const selection = () => ev(`godi.cy.nodes('.sel').map(n => n.id())`);

await test('tapping a node selects it', async () => {
	await tapScope(); // Start from nothing selected.
	await tapNode(SERVER);
	return eq(await selection(), [SERVER], 'the selection');
});

await test('tapping the selected node clears it', async () => {
	await tapNode(SERVER);
	return eq(await selection(), [], 'the selection');
});

await test('clearing the selection un-dims the graph', async () =>
	eq(await dimmed(), [], 'dimmed nodes'));

await test('and empties the panel', async () =>
	(await panelText()).includes('Pick a node') || 'the panel still shows the old selection');

await test('tapping a scope area clears the selection too', async () => {
	await tapNode(SERVER);
	await tapScope();
	return eq(await selection(), [], 'the selection after tapping the scope');
});

await test('tapping a different node moves the selection', async () => {
	await tapNode(SERVER);
	await tapNode('root/svc:app.(*Router)');
	return eq(await selection(), ['root/svc:app.(*Router)'], 'the selection');
});

await tapScope();

// --- reported: literals rendered as "string = string = <value>" ------------

await test('a literal row does not repeat its type', async () => {
	const lines = await rows(SERVER);
	const literal = lines.find((l) => l.includes('127.0.0.1'));
	return literal === '1 ◂ string = 127.0.0.1:9090' || `row reads ${JSON.stringify(literal)}`;
});

await test('no label anywhere doubles a type', async () => {
	const doubled = await ev(`godi.cy.nodes().map(n => n.data('label')).join('\\n').includes('string = string')`);
	return doubled === false || 'a label still reads "string = string"';
});

// --- the header names the node once ----------------------------------------

await test('a service is headed by the type it provides', async () => {
	await selectNode(SERVER);
	return eq(await ev(`document.querySelector('#panel h2').textContent`),
		'app.(*Server)', 'the heading');
});

// The qualified type and factory used to sit here. A generic names its type
// arguments in full, so those rows ran to several lines to say what the heading
// already said plus where it lives - which is all the package says, on one line.
await test('the package sits in a labelled row under the heading', async () =>
	eq(await ev(`(() => {
		const dl = document.querySelector('#panel dl');
		return [...dl.children].slice(0, 4).map(el => el.textContent);
	})()`), [
		'Package', 'github.com/acme/app',
		'Scope', 'root',
	], 'the first rows'));

await test('and the qualified forms are gone from it', async () => {
	const text = await panelText();
	return (!text.includes('github.com/acme/app.(*Server)') && !text.includes('github.com/acme/app.NewServer'))
		|| 'a fully qualified name is still in the panel';
});

// Every path in it, not just the last: a generic names its type arguments, and
// leaving those qualified is what made the old rows unreadable.
await test('the signature is shortened throughout', async () => {
	const shown = await ev(`document.querySelector('#panel .sig').textContent`);
	return shown === 'func(app.Handler[app.Request], app.Logger) app.(*Server)'
		|| `the signature reads ${JSON.stringify(shown)}`;
});

// Shortening is for reading; the qualified form is still the answer to "which
// app package", so it stays a hover away.
await test('and the whole of it is still there on hover', async () => {
	const full = await ev(`document.querySelector('#panel .sig').title`);
	return full === 'func(github.com/acme/app.Handler[github.com/acme/app.Request], app.Logger) github.com/acme/app.(*Server)'
		|| `the tooltip reads ${JSON.stringify(full)}`;
});

await test('nothing under the heading repeats it', async () => {
	await selectNode(SERVER);
	const heading = await ev(`document.querySelector('#panel h2').textContent`);
	const repeats = await ev(`(() => {
		const h = document.querySelector('#panel h2').textContent;
		return [...document.querySelectorAll('#panel dd, #panel .sig')]
			.map(el => el.textContent).filter(t => t === h);
	})()`);
	return repeats.length === 0 || `${JSON.stringify(heading)} appears again in ${JSON.stringify(repeats)}`;
});

// --- the signature, above the arguments ------------------------------------

await test('the panel leads with the signature', async () => {
	await selectNode(SERVER);
	const headings = await ev(`[...document.querySelectorAll('#panel h3')].map(h => h.textContent)`);
	return eq(headings.slice(0, 3), ['Factory signature', 'Factory arguments', 'Method calls'],
		'the panel sections, in order');
});

// --- reported: the panel ran method calls in with constructor arguments ----

await test('the panel separates method calls from arguments', async () => {
	await selectNode(SERVER);
	const found = await headings();
	const args = found.indexOf('Factory arguments');
	const calls = found.indexOf('Method calls');
	return (args >= 0 && calls > args) || `panel headings were ${JSON.stringify(found)}`;
});

await test('each method call is headed by its name', async () =>
	eq(await ev(`[...document.querySelectorAll('#panel .method')].map(m => m.textContent)`),
		['SetLogger()', 'SetTimeout()'], 'method headings'));

await test('the argument section holds only factory arguments', async () => {
	const upto = await ev(`(() => {
		const panel = document.getElementById('panel');
		const kids = [...panel.children];
		const from = kids.findIndex(k => k.textContent === 'Factory arguments');
		const to = kids.findIndex(k => k.textContent === 'Method calls');
		return kids.slice(from + 1, to).filter(k => k.classList.contains('param')).length;
	})()`);
	return upto === 2 || `found ${upto} argument blocks, expected 2`;
});

await test('a literal reads cleanly in the panel too', async () =>
	(await panelText()).includes('= 127.0.0.1:9090')
	|| 'the panel does not show the literal value');

await test('the chrome starts at the top of the window', async () => {
	const top = await ev(`document.getElementById('bar').getBoundingClientRect().top`);
	return top === 0 || `the bar sits ${top}px down, so something above it is taking up space`;
});

await test('the icon sprite takes up no room', async () =>
	eq(await ev(`(() => {
		const box = document.querySelector('body > svg[hidden]').getBoundingClientRect();
		return [box.width, box.height];
	})()`), [0, 0], 'the sprite box'));

// --- chrome: labels, icons, shortcuts --------------------------------------

await test('every control label starts with a capital', async () => {
	const labels = await ev(`(() => {
		const text = [];
		for (const el of document.querySelectorAll('#bar button, #bar option, #filters button, #filters .glabel, #status button, #status option')) {
			const t = el.textContent.trim();
			if (t) text.push(t);
		}
		return text;
	})()`);
	// "godi" is the library's own name and is lowercase everywhere else too.
	const lower = labels.filter((t) => t !== 'godi' && t[0] !== t[0].toUpperCase());
	return lower.length === 0 || `these do not start with a capital: ${JSON.stringify(lower)}`;
});

await test('the compiler pass filter is not called "a compiler pass"', async () =>
	eq(await ev(`document.querySelector('[data-show="compiler-pass"]').textContent.trim()`),
		'Compiler pass', 'the filter label'));

// Filters are toggles rather than a one-of-many choice, so they say "on" by
// standing out of the bar rather than by filling with the accent.
await test('the filters are grouped toggle buttons', async () =>
	eq(await ev(`[...document.querySelectorAll('#filters .seg')].map(g =>
		[g.classList.contains('toggles'), [...g.querySelectorAll('button')].map(b => b.dataset.show || b.dataset.flag)])`),
		[
			[true, ['manual', 'autowiring', 'compiler-pass']],
			[true, ['method', 'args', 'isolate', 'rootsOnly']],
		], 'the filter groups'));

await test('a filter button carries its state', async () => {
	const before = await ev(`document.querySelector('[data-show="method"]').getAttribute('aria-pressed')`);
	await toggle('method', false);
	const after = await ev(`document.querySelector('[data-show="method"]').getAttribute('aria-pressed')`);
	await toggle('method', true);
	return eq([before, after], ['true', 'false'], 'the pressed state');
});

await test('every top bar control carries an icon', async () =>
	eq(await ev(`(() => {
		const missing = [];
		for (const el of document.querySelectorAll('#bar > button, #bar > label, #bar > .seg button')) {
			if (!el.querySelector('svg use')) missing.push(el.id || el.textContent.trim());
		}
		return missing;
	})()`), [], 'controls with no icon'));

await test('the icons resolve to symbols that exist', async () =>
	eq(await ev(`(() => {
		const broken = [];
		for (const use of document.querySelectorAll('#bar svg use')) {
			const id = use.getAttribute('href').slice(1);
			if (!document.getElementById(id)) broken.push(id);
		}
		return broken;
	})()`), [], 'dangling icon references'));

// --- edge routing ----------------------------------------------------------

const routeAs = (style) => ev(`(() => {
	const s = document.getElementById('routing'); s.value = ${JSON.stringify(style)};
	s.dispatchEvent(new Event('change'));
	return godi.cy.edges()[0].style('curve-style');
})()`);

await test('the routing selector starts curved', async () =>
	eq(await ev(`[document.getElementById('routing').value, godi.cy.edges()[0].style('curve-style')]`),
		['unbundled-bezier', 'unbundled-bezier'], 'the default routing'));

await test('every routing the selector offers reaches the edges', async () => {
	const offered = await ev(`[...document.querySelectorAll('#routing option')].map(o => o.value)`);
	for (const style of offered) {
		const got = await routeAs(style);
		if (got !== style) return `${style} did not take: edges are ${got}`;
	}
	await routeAs('unbundled-bezier');
	return eq(offered, ['unbundled-bezier', 'straight', 'segments', 'taxi'], 'the routings offered');
});

// A curve has to actually bend: a plain bezier is drawn straight for an edge
// that has no siblings between the same pair of nodes.
await test('curved edges are not straight ones', async () => {
	const bowed = await ev(`(() => {
		const e = godi.cy.edges()[0];
		const mid = e.midpoint();
		const s = e.sourceEndpoint(), t = e.targetEndpoint();
		return Math.hypot(mid.x - (s.x + t.x) / 2, mid.y - (s.y + t.y) / 2);
	})()`);
	return bowed > 2 || `the midpoint sits ${bowed.toFixed(2)}px off the straight line`;
});

// --- what search looks at ---------------------------------------------------

const scopeState = () => ev(`[...document.querySelectorAll('[data-scope]')].map(b =>
	[b.dataset.scope, b.getAttribute('aria-pressed') === 'true'])`);

const setScope = async (name, on) => {
	await ev(`(() => {
		const b = document.querySelector('[data-scope=${JSON.stringify(name)}]');
		if ((b.getAttribute('aria-pressed') === 'true') !== ${on}) b.click();
		return true;
	})()`);
	await sleep(60);
};

const searchFor = async (text) => {
	await ev(`(() => {
		const s = document.getElementById('search');
		s.value = ${JSON.stringify(text)}; s.dispatchEvent(new Event('input'));
		return true;
	})()`);
	await sleep(220);
	return ev(`godi.cy.nodes('.match').map(n => n.id()).sort()`);
};

await test('the scope panel is hidden until the box is focused', async () => {
	const before = await ev(`getComputedStyle(document.getElementById('search-scopes')).display`);
	await ev(`(() => { document.getElementById('search').focus(); return true; })()`);
	const after = await ev(`getComputedStyle(document.getElementById('search-scopes')).display`);
	return (before === 'none' && after !== 'none') || `display went ${before} -> ${after}`;
});

await test('the type and the factory are searched, the rest are not', async () =>
	eq(await scopeState(), [
		['type', true], ['factory', true], ['args', false], ['literals', false],
		['methods', false], ['scope', false], ['labels', false],
	], 'the scopes out of the box'));

await test('a literal is not searched until literals are', async () => {
	const before = await searchFor('127.0.0.1');
	await setScope('literals', true);
	const after = await searchFor('127.0.0.1');
	await setScope('literals', false);
	return (before.length === 0 && after.includes('root/svc:app.(*Server)'))
		|| `found ${JSON.stringify(before)} then ${JSON.stringify(after)}`;
});

// Method arguments are only in reach once method calls are.
await test('argument types reach method arguments only with method calls on', async () => {
	await setScope('args', true);
	const withoutMethods = await searchFor('app.logger');
	await setScope('methods', true);
	const withMethods = await searchFor('app.logger');
	await setScope('args', false);
	await setScope('methods', false);
	await searchFor('');

	// Only the Server takes a Logger, and only through a method call.
	return (!withoutMethods.includes('root/svc:app.(*Server)') && withMethods.includes('root/svc:app.(*Server)'))
		|| `without ${JSON.stringify(withoutMethods)}, with ${JSON.stringify(withMethods)}`;
});

// --- the legend ------------------------------------------------------------

const legendShown = () => ev(`document.getElementById('app').classList.contains('legend-open')`);

await test('the legend starts closed', async () =>
	await legendShown() === false || 'it started open');

await test('its tab reads Legend', async () =>
	eq(await ev(`document.getElementById('legend-tab').textContent.trim()`), 'Legend', 'the tab'));

await test('the tab opens it and it stays', async () => {
	await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
	await sleep(80);
	return (await legendShown() === true && await ev(`document.getElementById('legend').offsetWidth > 0`))
		|| 'the tab did not open the legend';
});

await test('it covers all three channels an edge is drawn with', async () =>
	eq(await ev(`[...document.querySelectorAll('#legend .legend-head')].map(h => h.firstChild.textContent)`),
		['Head', 'Cycle', 'Colour', 'Line'], 'the legend sections'));

await test('and every variation within them', async () =>
	eq(await ev(`[...document.querySelectorAll('#legend .legend-row')].map(r => r.textContent)`), [
		'Exact type', 'Interface binding', 'Loops back',
		'You', 'godi', 'A compiler pass',
		'You', 'godi', 'A compiler pass',
	], 'the legend rows'));

// The samples read the same custom properties the canvas does, so the legend
// cannot come to disagree with the graph it explains.
await test('the samples are the colours the graph actually uses', async () => {
	const pairs = await ev(`(() => {
		const flat = (c) => c.replace(/\\s/g, '');
		const of = (decided) => {
			const e = godi.cy.edges().filter(x => x.data('decidedBy') === decided)[0];
			return e ? flat(e.style('line-color')) : null;
		};
		return {
			manual: [flat(getComputedStyle(document.querySelector('#legend .sample.manual')).color), of('manual')],
			auto: [flat(getComputedStyle(document.querySelector('#legend .sample.auto')).color), of('autowiring')],
			pass: [flat(getComputedStyle(document.querySelector('#legend .sample.pass')).color), of('compiler-pass')],
		};
	})()`);
	const wrong = Object.entries(pairs).filter(([, [a, b]]) => b !== null && a !== b);
	return wrong.length === 0 || `legend and graph disagree: ${JSON.stringify(wrong)}`;
});

// The tab rides on the panel's edge rather than the window's, so it has to move
// when the panel opens.
// Only the Head section is about heads; elsewhere an arrow would draw the eye
// to the wrong channel.
await test('only the head section draws arrowheads', async () =>
	eq(await ev(`[...document.querySelectorAll('#legend .legend-group')].map(g =>
		[g.querySelector('.legend-head').firstChild.textContent, g.querySelectorAll('.sample .head').length])`),
		[['Head', 2], ['Cycle', 0], ['Colour', 0], ['Line', 0]], 'arrowheads per section'));

await test('the tab travels with the panel', async () => {
	const openAt = await ev(`document.getElementById('legend-tab').getBoundingClientRect().left`);
	await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
	await sleep(80);
	const shutAt = await ev(`document.getElementById('legend-tab').getBoundingClientRect().left`);
	await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
	await sleep(80);
	return (openAt > shutAt + 50) || `tab sat at ${shutAt} closed and ${openAt} open`;
});

await test('and sits against the window edge when closed', async () => {
	await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
	await sleep(80);
	return eq(await ev(`Math.round(document.getElementById('legend-tab').getBoundingClientRect().left)`), 0,
		'the closed tab');
});

await test('the tab closes it again', async () => {
	if (await legendShown() === false) {
		await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
		await sleep(80);
	}
	await ev(`(() => { document.getElementById('legend-tab').click(); return true; })()`);
	return await legendShown() === false || 'the tab did not close it';
});

// --- the panel gets out of the way -----------------------------------------

const panelShown = () => ev(`!document.getElementById('app').classList.contains('panel-hidden')`);

await test('the panel starts open', async () => await panelShown() === true || 'it started hidden');

await test('the tab hides it', async () => {
	await ev(`(() => { document.getElementById('panel-tab').click(); return true; })()`);
	return await panelShown() === false || 'the tab did not hide it';
});

await test('the canvas takes the room', async () =>
	await ev(`document.getElementById('panel').offsetWidth === 0`)
	|| 'the panel is hidden but still occupying its column');

// The whole point: a stray click must not cost the reader the space they asked
// for, but it must not silently swallow the change either.
await test('selecting a node while hidden does not take it back', async () => {
	await selectNode('root/svc:app.(*Router)');
	return await panelShown() === false || 'a click reopened the panel';
});

await test('it flashes the tab instead', async () =>
	await ev(`document.getElementById('panel-tab').classList.contains('flash')`)
	|| 'nothing signalled that the panel had changed');

await test('the panel filled itself anyway, ready for its return', async () =>
	eq(await ev(`document.querySelector('#panel h2').textContent`), 'app.(*Router)', 'the hidden panel'));

await test('the bottom row brings it back', async () => {
	await ev(`(() => { document.getElementById('help').click(); return true; })()`);
	return (await panelShown() === true && (await panelText()).includes('Two-finger swipe'))
		|| 'Controls did not restore the panel';
});

await test('and so does the ? key', async () => {
	await ev(`(() => { document.getElementById('panel-tab').click(); return true; })()`);
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: '?', bubbles: true})); return true; })()`);
	return await panelShown() === true || 'the ? key left the panel hidden';
});

await selectNode(SERVER);

// --- navigation: panning, zooming, and who is holding the wheel -------------

const MOD_BIT = 4; // Meta, in the protocol's modifier mask. Macs only here.
// cy.pan() returns a live object rather than a snapshot, so it has to be
// copied before anything is compared against it.
const view = () => ev(`({ pan: {...godi.cy.pan()}, zoom: godi.cy.zoom() })`);
const nodeAt = () => ev(`(() => {
	const bb = godi.cy.getElementById(${JSON.stringify(SERVER)}).renderedBoundingBox();
	const r = document.getElementById('cy').getBoundingClientRect();
	return { x: Math.round(r.left + (bb.x1 + bb.x2) / 2), y: Math.round(r.top + (bb.y1 + bb.y2) / 2) };
})()`);

async function dragFrom(at, { button = 'left', modifiers = 0 } = {}) {
	const buttons = button === 'middle' ? 4 : 1;
	await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: at.x, y: at.y, button, buttons, modifiers, clickCount: 1 });
	for (let i = 1; i <= 4; i++) {
		await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: at.x + i * 16, y: at.y + i * 9, button, buttons, modifiers });
		await sleep(25);
	}
	await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: at.x + 64, y: at.y + 36, button, buttons: 0, modifiers, clickCount: 1 });
	await sleep(150);
}

const moved = (a, b) => Math.round(a.x - b.x) !== 0 || Math.round(a.y - b.y) !== 0;

await test('a plain drag on a node still moves the node', async () => {
	const at = await nodeAt();
	const before = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const pan = (await view()).pan;
	await dragFrom(at);
	const after = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const panAfter = (await view()).pan;
	return (moved(after, before) && !moved(panAfter, pan)) || 'a plain drag no longer moves the node';
});

await test('the modifier turns a drag on a node into a pan', async () => {
	const at = await nodeAt();
	const before = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const pan = (await view()).pan;

	// The mode is armed by the key, so the key has to be down first.
	await send('Input.dispatchKeyEvent', { type: 'rawKeyDown', modifiers: MOD_BIT, key: 'Meta', code: 'MetaLeft', windowsVirtualKeyCode: 91 });
	await dragFrom(at, { modifiers: MOD_BIT });
	await send('Input.dispatchKeyEvent', { type: 'keyUp', modifiers: 0, key: 'Meta', code: 'MetaLeft', windowsVirtualKeyCode: 91 });

	const after = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const panAfter = (await view()).pan;
	return (moved(panAfter, pan) && !moved(after, before))
		|| `pan moved: ${moved(panAfter, pan)}, node moved: ${moved(after, before)}`;
});

await test('and releasing it hands the node back', async () =>
	eq(await ev(`godi.cy.autoungrabify()`), false, 'grabbing after the modifier is released'));

await test('losing the window releases it too', async () => {
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Meta', metaKey: true})); return true; })()`);
	const armed = await ev(`godi.cy.autoungrabify()`);
	await ev(`(() => { window.dispatchEvent(new Event('blur')); return true; })()`);
	return (armed === true && await ev(`godi.cy.autoungrabify()`) === false)
		|| 'the pan mode survives losing focus, so the graph would stay stuck';
});

await test('the middle button pans from on top of a node', async () => {
	const at = await nodeAt();
	const before = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const pan = (await view()).pan;
	await dragFrom(at, { button: 'middle' });
	const after = await ev(`godi.cy.getElementById(${JSON.stringify(SERVER)}).position()`);
	const panAfter = (await view()).pan;
	return (moved(panAfter, pan) && !moved(after, before))
		|| `pan moved: ${moved(panAfter, pan)}, node moved: ${moved(after, before)}`;
});

const wheel = (opts) => ev(`(() => {
	const before = { pan: {...godi.cy.pan()}, zoom: godi.cy.zoom() };
	document.getElementById('cy').dispatchEvent(new WheelEvent('wheel', Object.assign(
		{ clientX: 600, clientY: 400, bubbles: true, cancelable: true }, ${JSON.stringify(opts)})));
	const after = { pan: {...godi.cy.pan()}, zoom: godi.cy.zoom() };
	return {
		zoomed: Math.abs(after.zoom - before.zoom) > 1e-6,
		panned: Math.round(after.pan.x - before.pan.x) !== 0 || Math.round(after.pan.y - before.pan.y) !== 0,
	};
})()`);

const setWheel = (mode) => ev(`(() => {
	const s = document.getElementById('wheel'); s.value = ${JSON.stringify(mode)};
	s.dispatchEvent(new Event('change')); return true; })()`);

await test('a trackpad swipe pans and does not zoom', async () =>
	eq(await wheel({ deltaX: 14, deltaY: 23 }), { zoomed: false, panned: true }, 'a fractional two-finger swipe'));

await test('a pinch zooms, because the browser flags it', async () =>
	(await wheel({ deltaY: 18, ctrlKey: true })).zoomed || 'a pinch did not zoom');

await test('a notched mouse wheel zooms', async () =>
	(await wheel({ deltaY: 120 })).zoomed || 'a 120-step wheel did not zoom');

await test('the modifier zooms whatever the wheel is', async () =>
	(await wheel({ deltaX: 3, deltaY: 7, metaKey: true })).zoomed || 'the modifier did not zoom');

await test('shift pans sideways', async () =>
	eq(await wheel({ deltaY: 40, shiftKey: true }), { zoomed: false, panned: true }, 'shift and wheel'));

// A trackpad swipe already carries both axes, so shift must not flatten it.
await test('shift leaves a two-axis swipe alone', async () => {
	const got = await ev(`(() => {
		const before = {...godi.cy.pan()};
		document.getElementById('cy').dispatchEvent(new WheelEvent('wheel',
			{ deltaX: 12, deltaY: 30, shiftKey: true, clientX: 600, clientY: 400, bubbles: true, cancelable: true }));
		const after = {...godi.cy.pan()};
		return { dx: Math.round(after.x - before.x), dy: Math.round(after.y - before.y) };
	})()`);
	return eq(got, { dx: -12, dy: -30 }, 'a shifted swipe with both axes');
});

await test('shift still pans sideways with the switch forced to mouse', async () => {
	await setWheel('mouse');
	const got = await wheel({ deltaY: 40, shiftKey: true });
	await setWheel('auto');
	return eq(got, { zoomed: false, panned: true }, 'shift and wheel in mouse mode');
});

await test('the modifier still zooms with the switch forced to trackpad', async () => {
	await setWheel('trackpad');
	const got = await wheel({ deltaX: 5, deltaY: 9, metaKey: true });
	await setWheel('auto');
	return got.zoomed || 'the modifier did not zoom in trackpad mode';
});

await test('forcing trackpad makes a notched wheel pan', async () => {
	await setWheel('trackpad');
	const got = await wheel({ deltaY: 120 });
	await setWheel('auto');
	return eq(got, { zoomed: false, panned: true }, 'a 120-step wheel with the switch on trackpad');
});

await test('forcing mouse makes a fractional swipe zoom', async () => {
	await setWheel('mouse');
	const got = await wheel({ deltaX: 14, deltaY: 23 });
	await setWheel('auto');
	return got.zoomed || 'a swipe did not zoom with the switch forced to mouse';
});

// --- the help panel documents all of it -------------------------------------

await test('the help link opens the panel', async () => {
	await ev(`(() => { document.getElementById('help').click(); return true; })()`);
	const headings = await ev(`[...document.querySelectorAll('#panel h2, #panel h3')].map(h => h.textContent)`);
	return eq(headings, ['Help', 'Search', 'Keyboard', 'Mouse', 'Trackpad'], 'the help panel');
});

// Search is the one thing here that cannot be worked out by trying it, so it
// leads, and the wheel preamble goes: the rows below already say it.
await test('it explains what search matches, first', async () => {
	const text = await ev(`document.getElementById('panel').textContent`);
	const missing = ['ignores case', 'Every word has to match', 'Type', 'Factory', 'Scope', 'Labels']
		.filter((w) => !text.includes(w));
	return missing.length === 0 || `search is not explained: ${JSON.stringify(missing)}`;
});

await test('and drops the wheel preamble', async () =>
	!(await ev(`document.getElementById('panel').textContent`)).includes('guesses which you are using')
	|| 'the preamble is still there');

await test('it names every way to pan and zoom', async () => {
	const text = await ev(`document.getElementById('panel').textContent`);
	const want = ['Middle-drag', 'Two-finger swipe', 'Pinch', 'Shift + wheel'];
	const missing = want.filter((w) => !text.includes(w));
	return missing.length === 0 || `undocumented: ${JSON.stringify(missing)}`;
});

// The list is for what you would not guess by trying. Clicking a node to select
// it does not need saying, and saying it buries what does.
await test('it leaves out what needs no telling', async () => {
	const text = await ev(`document.getElementById('panel').textContent`);
	const gone = ['Click a node', 'Click a scope', 'Drag a node', 'Drag the background', 'Legend tab'];
	const kept = gone.filter((g) => text.includes(g));
	return kept.length === 0 || `still explained: ${JSON.stringify(kept)}`;
});

await test('the modifier is spelled for this platform', async () => {
	const text = await ev(`document.getElementById('panel').textContent`);
	const mac = await ev(`/Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent)`);
	return text.includes(mac ? '⌘ + drag' : 'Ctrl + drag')
		|| `no modifier row for ${mac ? 'macOS' : 'this platform'}`;
});

await test('the search box has a clear button, once there is something to clear', async () => {
	const before = await ev(`document.getElementById('search-clear').hidden`);
	await ev(`(() => {
		const s = document.getElementById('search');
		s.value = 'handler'; s.dispatchEvent(new Event('input'));
		return true;
	})()`);
	await sleep(200);
	return (before === true && await ev(`document.getElementById('search-clear').hidden`) === false)
		|| 'the clear button did not appear';
});

await test('and it empties the box and the matches', async () => {
	await ev(`(() => { document.getElementById('search-clear').click(); return true; })()`);
	await sleep(200);
	return eq(await ev(`[document.getElementById('search').value, document.getElementById('found').textContent,
		godi.cy.nodes('.match').length, document.getElementById('search-clear').hidden]`),
		['', '', 0, true], 'after clearing');
});

await test('c clears the selection, like Esc', async () => {
	await selectNode(SERVER);
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'c', bubbles: true})); return true; })()`);
	return eq(await ev(`godi.cy.nodes('.sel').length`), 0, 'the selection after pressing c');
});

await test('typing c in the search box does not clear it', async () => {
	await selectNode(SERVER);
	await ev(`(() => {
		const s = document.getElementById('search');
		s.dispatchEvent(new KeyboardEvent('keydown', {key: 'c', bubbles: true}));
		return true;
	})()`);
	return eq(await ev(`godi.cy.nodes('.sel').length`), 1, 'the selection while typing');
});

await test('the controls list names both keys', async () => {
	await ev(`(() => { document.getElementById('help').click(); return true; })()`);
	return eq(await ev(`(() => {
		const rows = [...document.querySelectorAll('#panel dl.keys dt')];
		const row = rows.find(r => [...r.querySelectorAll('kbd')].some(k => k.textContent === 'Esc'));
		return [...row.querySelectorAll('kbd')].map(k => k.textContent);
	})()`), ['Esc', 'c'], 'the keys on the clear row');
});

const pressQuestion = () => ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: '?', bubbles: true})); return true; })()`);

await test('the ? key opens it too', async () => {
	await selectNode(SERVER);
	await pressQuestion();
	return (await panelText()).includes('Focus the search box') || 'the ? key did not open the controls';
});

await test('and pressing it again puts them away', async () => {
	await pressQuestion();
	return await panelShown() === false || 'the panel stayed open';
});

await test('while ? on a node view opens the controls rather than closing', async () => {
	await selectNode(SERVER);
	await pressQuestion();
	return ((await panelText()).includes('Focus the search box') && await panelShown() === true)
		|| 'the controls did not come back';
});

// --- the chrome, where the eye notices before a test does -------------------

const box = (sel) => ev(`(() => { const r = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect();
	return { left: r.left, right: r.right, top: r.top, bottom: r.bottom }; })()`);

await test('the legend keeps the edge its tab does not cover', async () => {
	await ev(`(() => { if (!document.getElementById('app').classList.contains('legend-open'))
		document.getElementById('legend-tab').click(); return true; })()`);
	await sleep(80);
	// The tab is shorter than the panel, so without this the edge above it is
	// simply missing.
	return eq(await ev(`getComputedStyle(document.getElementById('legend')).borderRightStyle`),
		'solid', 'the legend right edge');
});

await test('the legend rests on the bottom bar', async () => {
	const [legend, status] = [await box('#legend'), await box('#status')];
	return Math.abs(legend.bottom - status.top) < 1
		|| `legend bottom ${legend.bottom}, bar top ${status.top}`;
});

await test('and hides its bottom border behind that bar', async () =>
	eq(await ev(`getComputedStyle(document.getElementById('legend')).borderBottomWidth`), '0px',
		'the legend bottom border'));

await test('the detail panel tab covers the edge behind it', async () => {
	const [tab, panel] = [await box('#panel-tab'), await box('#panel')];
	// Overlapping, not merely touching: a seam of exactly zero still shows the
	// panel's own border running behind the tab.
	return tab.right > panel.left
		|| `tab ends at ${tab.right}, panel starts at ${panel.left}`;
});

// The segmented buttons overlap by a pixel to share a border, so a highlighted
// one has to come forward or its neighbour paints over the edge.
await test('a hovered segmented button comes forward', async () => {
	const at = await box('[data-dir="down"]');
	await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: Math.round((at.left + at.right) / 2), y: Math.round((at.top + at.bottom) / 2) });
	await sleep(80);
	const z = await ev(`getComputedStyle(document.querySelector('[data-dir="down"]')).zIndex`);
	await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: 600, y: 500 });
	return z === '2' || `the hovered button sits at z-index ${z}`;
});

await test('holding the canvas draws no disc under the pointer', async () =>
	await ev(`godi.cy.style().json().some(r => r.selector === 'core'
		&& Number(r.style['active-bg-opacity']) === 0)`)
	|| "cytoscape's active background is still on");

// Two groups, one meaning of "active".
await test('a pressed toggle looks like a pressed choice', async () => {
	const [toggle, choice] = await ev(`[
		getComputedStyle(document.querySelector('[data-show="method"][aria-pressed=true]')).backgroundColor,
		getComputedStyle(document.querySelector('[data-dir="both"][aria-pressed=true]')).backgroundColor,
	]`);
	return toggle === choice || `toggle ${toggle}, choice ${choice}`;
});

await test('the bottom bar menus point their arrows the way they open', async () => {
	const arrow = await ev(`(() => {
		const style = getComputedStyle(document.querySelector('#status .ctl'), '::after');
		return { up: style.borderBottomWidth, down: style.borderTopWidth,
			native: getComputedStyle(document.getElementById('routing')).appearance };
	})()`);
	return (arrow.native === 'none' && arrow.up !== '0px' && arrow.down === '0px')
		|| `arrow was ${JSON.stringify(arrow)}`;
});

// --- colour scheme ---------------------------------------------------------

const scheme = () => ev(`(() => ({
	cls: document.documentElement.className,
	label: document.getElementById('theme').value,
	icon: document.getElementById('theme-icon').getAttribute('href'),
	node: godi.cy.getElementById(${JSON.stringify(SERVER)}).style('background-color'),
}))()`);

await test('the page starts on the scheme it was built with', async () => {
	const s = await scheme();
	return s.cls === 'theme-auto' || `started on ${s.cls}`;
});

await test('the menu switches to light', async () => {
	await ev(`(() => { const t = document.getElementById('theme'); t.value = 'light'; t.dispatchEvent(new Event('change')); return true; })()`);
	const s = await scheme();
	return (s.cls === 'theme-light' && s.label === 'light') || JSON.stringify(s);
});

await test('and the canvas is restyled, not just the chrome', async () => {
	const light = await scheme();
	await ev(`(() => { const t = document.getElementById('theme'); t.value = 'dark'; t.dispatchEvent(new Event('change')); return true; })()`);
	const dark = await scheme();
	return (dark.cls === 'theme-dark' && dark.node !== light.node)
		|| `node colour stayed ${light.node} going from light to dark`;
});

await test('and back to auto', async () => {
	await ev(`(() => { const t = document.getElementById('theme'); t.value = 'auto'; t.dispatchEvent(new Event('change')); return true; })()`);
	const s = await scheme();
	return (s.cls === 'theme-auto' && s.label === 'auto') || JSON.stringify(s);
});

await test('d hides and shows the detail panel', async () => {
	const before = await panelShown();
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'd', bubbles: true})); return true; })()`);
	const after = await panelShown();
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'd', bubbles: true})); return true; })()`);
	return before !== after || 'd did nothing';
});

await test('l shows and hides the legend', async () => {
	const before = await legendShown();
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'l', bubbles: true})); return true; })()`);
	const after = await legendShown();
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 'l', bubbles: true})); return true; })()`);
	return before !== after || 'l did nothing';
});

// The key still cycles: there is no menu to open from the keyboard.
await test('the t key cycles it too', async () => {
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 't', bubbles: true})); return true; })()`);
	const s = await scheme();
	return s.cls === 'theme-light' || `t left it on ${s.cls}`;
});

await ev(`(() => { const t = document.getElementById('theme'); t.value = 'auto'; t.dispatchEvent(new Event('change')); return true; })()`);

// --- the panel can be resized ----------------------------------------------

const gripAt = () => ev(`(() => {
	const r = document.getElementById('panel-grip').getBoundingClientRect();
	return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + 120) };
})()`);

const panelWidth = () => ev(`document.getElementById('panel').getBoundingClientRect().width`);

// Dragged with real pointer events, because that is the whole of the feature:
// the grip captures the pointer so the edge keeps up with it even once it is
// out over the canvas.
async function dragGrip(by) {
	const at = await gripAt();
	await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: at.x, y: at.y, button: 'left', buttons: 1, clickCount: 1 });
	for (let i = 1; i <= 4; i++) {
		await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: at.x + (by * i) / 4, y: at.y, button: 'left', buttons: 1 });
		await sleep(20);
	}
	await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: at.x + by, y: at.y, button: 'left', buttons: 0, clickCount: 1 });
	await sleep(120);
}

await test('dragging the grip widens the panel', async () => {
	const before = await panelWidth();
	await dragGrip(-150);
	const after = await panelWidth();
	return Math.abs(after - (before + 150)) < 3
		|| `the panel went from ${before} to ${after}, wanted ${before + 150}`;
});

// Cytoscape measures its container once and then watches only the window, so
// without being told, the canvas keeps the width it had and everything on it
// lands in the wrong place.
// Cytoscape watches its own container, so nothing here has to tell it - but if
// that ever stopped being true the graph would be drawn at the wrong size, and
// this is the only place it would show.
await test('and the drawing follows the space it has', async () => {
	// The drawing surface, not the element around it: cy.width() reads the
	// container live and so agrees either way, while the <canvas> layers keep
	// whatever pixel size they were last given.
	const gap = await ev(`(() => {
		const box = document.getElementById('cy').getBoundingClientRect();
		const layer = document.querySelector('#cy canvas');
		return Math.round(Math.abs(layer.width / (window.devicePixelRatio || 1) - box.width));
	})()`);
	return gap <= 1 || `the drawing surface is ${gap}px out from the space it fills`;
});

await test('dragging it back narrows the panel again', async () => {
	const before = await panelWidth();
	await dragGrip(150);
	const after = await panelWidth();
	return Math.abs(after - (before - 150)) < 3 || `the panel went from ${before} to ${after}`;
});

// A panel dragged past the window edge would leave no canvas at all, and one
// dragged shut is what the tab is for.
await test('it cannot be dragged away entirely', async () => {
	await dragGrip(2000);
	const narrow = await panelWidth();
	await dragGrip(-4000);
	const wide = await panelWidth();
	const room = await ev(`window.innerWidth`);

	return (narrow >= 220 && narrow < 260 && wide <= room * 0.8 + 1)
		|| `it clamped to ${narrow} and ${wide} in a window of ${room}`;
});

await test('double-clicking the grip puts the default back', async () => {
	const at = await gripAt();
	for (let i = 1; i <= 2; i++) {
		await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: at.x, y: at.y, button: 'left', buttons: 1, clickCount: i });
		await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: at.x, y: at.y, button: 'left', buttons: 0, clickCount: i });
	}
	await sleep(120);

	const [width, rem] = await ev(`[
		document.getElementById('panel').getBoundingClientRect().width,
		parseFloat(getComputedStyle(document.documentElement).fontSize) * 23,
	]`);
	return Math.abs(width - rem) < 1 || `the panel came back at ${width}, not the ${rem} the stylesheet declares`;
});

// Reachable without a pointer at all.
await test('the arrow keys resize it too', async () => {
	const before = await panelWidth();
	await ev(`(() => {
		const g = document.getElementById('panel-grip');
		g.focus();
		g.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true }));
		return true;
	})()`);
	const after = await panelWidth();
	return after === before + 16 || `left arrow took it from ${before} to ${after}`;
});

await test('the grip goes away with the panel', async () => {
	await ev(`(() => { document.getElementById('panel-tab').click(); return true; })()`);
	await sleep(80);
	const hidden = await ev(`getComputedStyle(document.getElementById('panel-grip')).display`);
	await ev(`(() => { document.getElementById('panel-tab').click(); return true; })()`);
	await sleep(80);
	return hidden === 'none' || `the grip was ${hidden} with nothing to resize`;
});

// --- a service registered as a value ----------------------------------------

// A value registered as a service is described by that value. Its name is the
// one thing the type above does not say, so it goes beside the signature -
// unless the runtime made the name up, and then the signature is all there is.
const asValue = (props) => ev(`(() => {
	const n = godi.data.nodes.find((x) => x.id === ${JSON.stringify(SERVER)});
	Object.assign(n, ${JSON.stringify(props)});
	return true;
})()`);

const sigLines = async () => {
	await selectNode(SERVER);
	return ev(`[...document.querySelectorAll('#panel .sig div')].map(d => d.textContent)`);
};

await test('a named value is its name and its signature, under a heading of its own', async () => {
	await asValue({ fromValue: true, anonymous: false, name: 'github.com/acme/app.validateEmail' });
	const lines = await sigLines();
	const heading = await ev(`[...document.querySelectorAll('#panel h3')].map(h => h.textContent)[0]`);

	await asValue({ fromValue: false, anonymous: false, name: 'github.com/acme/app.NewServer' });
	await selectNode(SERVER);

	return (heading === 'Value' && eq(lines, ['app.validateEmail', 'func(app.Handler[app.Request], app.Logger) app.(*Server)'], 'x') === true)
		|| `heading ${JSON.stringify(heading)}, lines ${JSON.stringify(lines)}`;
});

await test('and an anonymous one is the signature alone', async () => {
	await asValue({ fromValue: true, anonymous: true, name: 'github.com/acme/app.build.func1' });
	const lines = await sigLines();

	await asValue({ fromValue: false, anonymous: false, name: 'github.com/acme/app.NewServer' });
	await selectNode(SERVER);

	return (lines.length === 1 && !lines[0].includes('func1'))
		|| `the block reads ${JSON.stringify(lines)}`;
});

await test('a factory-built service keeps its own heading and no name', async () => {
	await selectNode(SERVER);
	const heading = await ev(`[...document.querySelectorAll('#panel h3')].map(h => h.textContent)[0]`);
	const lines = await sigLines();
	return (heading === 'Factory signature' && lines.length === 1)
		|| `heading ${JSON.stringify(heading)}, lines ${JSON.stringify(lines)}`;
});

// --- source locations ------------------------------------------------------

await test('the panel shows where a service was registered and defined', async () => {
	await selectNode(SERVER);
	const rows = await ev(`(() => {
		const dl = document.querySelector('#panel dl');
		return [...dl.children].map(el => el.textContent);
	})()`);
	const want = ['Registered', 'wiring.go:42', 'Defined', 'http/server.go:118'];
	return want.every((w) => rows.includes(w)) || `panel rows were ${JSON.stringify(rows)}`;
});

await test('a location becomes a link when a template was given', async () =>
	eq(await ev(`(() => {
		const a = [...document.querySelectorAll('#panel dd a')].map(el => el.getAttribute('href'));
		return a;
	})()`), [
		'test://open?file=/home/me/app/wiring.go&rel=wiring.go&line=42',
		'test://open?file=/home/me/app/http/server.go&rel=http/server.go&line=118',
	], 'the editor links'));

await test('a node with no location grows no rows for it', async () => {
	await selectNode('root/svc:app.(*Router)');
	const text = await panelText();
	await selectNode(SERVER);
	return (!text.includes('Registered') && !text.includes('Defined'))
		|| 'the panel invented a location for a node that has none';
});

// --- reported: the compiler pass filter missed pass-created bindings -------

await test('a binding a pass created is credited to the pass', async () =>
	eq(await ev(`godi.data.edges.filter(e => e.to.includes('JSONReporter')).map(e => [e.origin, e.bindOrigin, e.decidedBy])`),
		[['autowiring', 'compiler-pass', 'compiler-pass']], 'the Auditor edge'));

await test('the pass is named on the edge, however it was responsible', async () => {
	const labels = await ev(`godi.cy.edges().map(e => [e.target().id(), e.data('label')])
		.filter(([, l]) => l).sort()`);
	return eq(labels, [['root/svc:app.JSONReporter', 'bind reporter']],
		'edge labels naming a pass');
});

await test('unticking compiler pass hides the binding it created', async () => {
	await toggle('compiler-pass', false);
	return (await visibleEdges()).every((id) => !id.includes('Auditor'))
		|| 'the Auditor edge survived the compiler pass filter';
});

await test('and leaves the autowired edges alone', async () =>
	(await visibleEdges()).some((id) => id.includes('Router'))
	|| 'unticking compiler pass took the autowired edges with it');

await test('re-ticking compiler pass brings it back', async () => {
	await toggle('compiler-pass', true);
	return eq((await visibleEdges()).length, 5, 'edge count');
});

// --- reported: filters left nodes behind with no edges ---------------------

await test('unticking godi hides the nodes it stranded', async () => {
	await toggle('autowiring', false);
	return eq(await visibleNodes(), [
		'root/svc:app.(*Auditor)',
		'root/svc:app.(*Config)',
		'root/svc:app.(*Metrics)',
		'root/svc:app.(*Repo)',
		'root/svc:app.JSONReporter',
	], 'what survives with autowiring hidden');
});

await test('but keeps the service nothing ever wired', async () =>
	(await visibleNodes()).includes('root/svc:app.(*Metrics)')
	|| 'the never-wired service was hidden as though a filter had stranded it');

await test('every shown node still has an edge, or never had one', async () =>
	await ev(`godi.cy.nodes(':childless:visible').every(n =>
		n.connectedEdges(':visible').length > 0
		|| godi.data.edges.every(e => e.from !== n.id() && e.to !== n.id()))`)
	|| 'a node is shown with no edges despite having some in the full graph');

await test('re-ticking godi restores every node and edge', async () => {
	await toggle('autowiring', true);
	return eq(await ev(`[godi.cy.nodes(':childless:visible').length, godi.cy.edges(':visible').length]`),
		[8, 5], 'counts after restoring');
});

// --- roots: the tops of the dependency trees --------------------------------

await test('a node nothing injects is a root', async () =>
	eq(await ev(`godi.data.nodes.filter(n => n.root).map(n => n.id).sort()`), [
		'root/svc:app.(*Auditor)',
		'root/svc:app.(*Metrics)',
		'root/svc:app.(*Repo)',
		'root/svc:app.(*Server)',
	], 'the roots'));

await test('a root is marked on the node itself', async () => {
	const [root, plain] = await ev(`[
		godi.cy.getElementById('root/svc:app.(*Server)').data('label'),
		godi.cy.getElementById('root/svc:app.(*Router)').data('label'),
	]`);
	return (root.startsWith('▲ ') && !plain.includes('▲'))
		|| `root label ${JSON.stringify(root.split('\n')[0])}, plain ${JSON.stringify(plain.split('\n')[0])}`;
});

await test('a root is tinted rather than warned about', async () => {
	const [root, plain] = await ev(`[
		godi.cy.getElementById('root/svc:app.(*Server)').style('background-color'),
		godi.cy.getElementById('root/svc:app.(*Router)').style('background-color'),
	]`);
	return root !== plain || `both nodes are ${root}, so a root is not distinguishable`;
});

await test('the panel badges it', async () => {
	await selectNode(SERVER);
	const badges = await ev(`[...document.querySelectorAll('#panel .badge')].map(b => b.textContent)`);
	return badges.includes('Root') || `badges were ${JSON.stringify(badges)}`;
});

await test('roots only hides everything else', async () => {
	await toggle('rootsOnly', true);
	const shown = await visibleNodes();
	await toggle('rootsOnly', false);
	return eq(shown, [
		'root/svc:app.(*Auditor)',
		'root/svc:app.(*Metrics)',
		'root/svc:app.(*Repo)',
		'root/svc:app.(*Server)',
	], 'what survives the roots-only view');
});

await test('the count is in the status bar', async () =>
	(await ev(`document.getElementById('counts').textContent`)).includes('4 roots')
	|| 'the status bar does not count the roots');

// --- reported: the hop slider had no way to say "all" ----------------------

// Selecting a node and seeing only its immediate neighbours hides the thing you
// selected it to look at. Narrowing from the whole tree is the easier direction
// to work in, and the slider is right there.
await test('the hop slider starts at "all"', async () => {
	return eq(await ev(`[
		document.getElementById('hops').value === document.getElementById('hops').max,
		document.getElementById('hops-out').textContent,
		godi.state.hops === Infinity,
	]`), [true, 'all', true], 'the slider, its label and the state on a fresh page');
});

await test('the hop slider tops out at "all"', async () => {
	await selectNode(SERVER);
	await setHops(await ev(`document.getElementById('hops').max`));
	return eq(await ev(`[document.getElementById('hops-out').textContent, godi.state.hops === Infinity]`),
		['all', true], 'the slider at its last stop');
});

await test('"all" lights the whole subgraph the selection reaches', async () => {
	// Config sits two hops from Server, through Router.
	const dim = await dimmed();
	return !dim.includes('root/svc:app.(*Config)') || 'Config is dim even at unlimited hops';
});

// The direction control is pressed constantly, so it is buttons rather than a
// menu, and exactly one of them is on at a time.
// Grouped because they all act on the view as a whole, and grouping them says
// so without a word of explanation.
await test('the whole-view actions are grouped', async () =>
	eq(await ev(`[...document.querySelectorAll('#bar > .seg')].map(g =>
		[...g.querySelectorAll('button')].map(b => b.id || b.dataset.dir))`),
		[['both', 'down', 'up'], ['relayout', 'fit', 'clear']], 'the button groups'));

await test('the direction group starts on Both', async () =>
	eq(await ev(`[...document.querySelectorAll('[data-dir]')].map(b => [b.dataset.dir, b.getAttribute('aria-pressed')])`),
		[['both', 'true'], ['down', 'false'], ['up', 'false']], 'the direction buttons'));

await test('pressing one switches the direction and the pressed state', async () => {
	await ev(`(() => { document.querySelector('[data-dir="down"]').click(); return true; })()`);
	await sleep(120);
	return eq(await ev(`[godi.state.dir, [...document.querySelectorAll('[data-dir][aria-pressed=true]')].map(b => b.dataset.dir)]`),
		['down', ['down']], 'after pressing Dependencies');
});

await test('following dependencies only lights what the selection needs', async () => {
	await selectNode('root/svc:app.(*Config)');
	await setHops(1);
	const dim = await dimmed();
	await ev(`(() => { document.querySelector('[data-dir="both"]').click(); return true; })()`);
	await sleep(120);
	// Config depends on nothing, so following downstream lights only itself.
	return dim.includes('root/svc:app.(*Repo)') || 'a consumer stayed lit while following dependencies';
});

await selectNode(SERVER);

await test('one hop lights only the immediate neighbours', async () => {
	await setHops(1);
	const dim = await dimmed();
	return dim.includes('root/svc:app.(*Config)') || 'Config is lit at one hop, so the limit does nothing';
});

await test('the hop limit follows the wiring, not the drawing', async () => {
	await setHops(2);
	const dim = await dimmed();
	return !dim.includes('root/svc:app.(*Config)') || 'Config is still dim at two hops';
});

// Repo and Router both take the Config, and neither knows about the other. A
// walk that follows both directions at once gets from one to the other by going
// down to the Config and back up, and calls that two hops - but a sibling is not
// on any path through the selection.
await test('a hop never turns around: siblings are not neighbours', async () => {
	await selectNode('root/svc:app.(*Repo)');
	await setHops(2);
	const dim = await dimmed();
	await selectNode(SERVER);
	return dim.includes('root/svc:app.(*Router)')
		|| 'the Router lit up two hops from the Repo, which only shares its Config';
});

// --- remembered preferences -------------------------------------------------
//
// These reload the page, so they come last: everything above runs against the
// first load.

async function reload() {
	await ev(`(() => { location.reload(); return true; })()`);
	await sleep(400);
	await settle();
}

// A restored choice has to reach the derived state, not just the button that
// shows it. The text a search looks at is built once and kept, so a scope
// restored after it was built is a button that says one thing and a search that
// does another.
await test('a remembered search scope is searched after a reload', async () => {
	await ev(`(() => {
		localStorage.setItem('godi.searchScopes', JSON.stringify({
			type: true, factory: true, args: false, literals: true,
			methods: false, scope: false, labels: false,
		}));
		return true;
	})()`);
	await reload();

	const pressed = await ev(`document.querySelector('[data-scope="literals"]').getAttribute('aria-pressed')`);
	const hits = await searchFor('127.0.0.1');
	return (pressed === 'true' && hits.includes(SERVER))
		|| `the button says ${pressed} and the search found ${JSON.stringify(hits)}`;
});

// Same shape of mistake, in the other direction: the canvas is styled before
// the stored choice is read, so the edges have to be restyled once it is.
await test('and a remembered line routing is drawn after one', async () => {
	await ev(`(() => { localStorage.setItem('godi.routing', 'taxi'); return true; })()`);
	await reload();

	const drawn = await ev(`godi.cy.edges()[0].style('curve-style')`);
	const shown = await ev(`document.getElementById('routing').value`);
	return (drawn === 'taxi' && shown === 'taxi') || `the menu says ${shown} and the edges are drawn ${drawn}`;
});

// A browser puts back what was typed and dragged when a page is reloaded,
// whether or not the page asked it to, and then the controls hold values the
// page never derived anything from. Headless Chrome does not do it, so the same
// situation is set up by hand: the slider is moved off its default and the
// search box filled in the markup, while the label beside the slider still reads
// what it always did - which is exactly what restoration leaves behind, since an
// <output> is not a form control and does not come back with the rest.
await test('the controls are read at load rather than assumed', async () => {
	const restored = pagePath.replace(/\.html$/, '-restored.html');
	const before = readFileSync(pagePath, 'utf8');
	const after = before
		.replace('id="hops" type="range" min="1" max="7" step="1" value="7"',
			'id="hops" type="range" min="1" max="7" step="1" value="3"')
		.replace('<input id="search" type="search"', '<input id="search" value="router" type="search"');
	if (after === before) return 'the markup this test rewrites has moved';
	writeFileSync(restored, after);

	await ev(`location.href = ${JSON.stringify('file://' + restored)}`);
	await sleep(500);
	await settle();

	return eq(await ev(`[
		document.getElementById('hops-out').textContent,
		godi.state.hops === Infinity ? 'all' : String(godi.state.hops),
		godi.state.query,
		document.getElementById('search-clear').hidden,
		godi.cy.nodes('.match').length > 0,
	]`), ['3', '3', 'router', false, true],
		'the hop label, the hop state, the query, the clear button and the matches');
});

await ev(`(() => { localStorage.clear(); return true; })()`);

// --------------------------------------------------------------------- done ---

await test('nothing was logged to the console', () =>
	consoleErrors.length === 0 || consoleErrors.join('\n'));

note(failures === 0 ? 'all viewer checks passed' : `${failures} viewer checks failed`);

ws.close();
chrome.kill();

// Wait for it to actually go: the caller deletes the profile directory as soon
// as this process exits, and a browser still writing to it fails that cleanup.
await new Promise((resolve) => {
	chrome.once('exit', resolve);
	setTimeout(resolve, 5000);
});

process.exit(0);
