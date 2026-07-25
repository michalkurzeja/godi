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
import { readFileSync } from 'node:fs';

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
		const box = document.querySelector('[data-show=${JSON.stringify(name)}]')
			|| document.querySelector('[data-flag=${JSON.stringify(name)}]');
		box.checked = ${on};
		box.dispatchEvent(new Event('change'));
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

await test('the box shrinks by exactly the two rows removed', async () => {
	const now = await height(SERVER);
	const shrunk = withMethods - now;
	return Math.abs(shrunk - 2 * 14.3) < 1 || `shrank by ${shrunk}px, expected ${2 * 14.3}`;
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

await test('the qualified forms sit in labelled rows, not loose under it', async () =>
	eq(await ev(`(() => {
		const dl = document.querySelector('#panel dl');
		return [...dl.children].slice(0, 4).map(el => el.textContent);
	})()`), [
		'Type', 'github.com/acme/app.(*Server)',
		'Factory', 'github.com/acme/app.NewServer',
	], 'the first rows'));

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

await test('the signature is the one the model carries', async () =>
	eq(await ev(`document.querySelector('#panel .sig').textContent`),
		'func(*app.Router, app.Logger) github.com/acme/app.(*Server)', 'the signature'));

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
		for (const el of document.querySelectorAll('#bar button, #bar option, #filters .chk, #filters .glabel, #status button')) {
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
	eq(await ev(`document.querySelector('[data-show="compiler-pass"]').parentElement.textContent.trim()`),
		'Compiler pass', 'the filter label'));

await test('every top bar control carries an icon', async () =>
	eq(await ev(`(() => {
		const missing = [];
		for (const el of document.querySelectorAll('#bar button, #bar label')) {
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

await test('the shortcuts link fills the panel', async () => {
	await ev(`(() => { document.getElementById('shortcuts').click(); return true; })()`);
	const headings = await ev(`[...document.querySelectorAll('#panel h2, #panel h3')].map(h => h.textContent)`);
	return eq(headings, ['Shortcuts', 'Keyboard', 'Mouse'], 'the shortcuts panel');
});

await test('keys wear keycaps and gestures do not', async () =>
	eq(await ev(`[
		[...document.querySelectorAll('#panel kbd')].map(k => k.textContent),
		[...document.querySelectorAll('#panel .gesture')].length,
	]`), [['/', 'Enter', 'Esc', 'f', 'r', 't', '?'], 5], 'keys and gestures'));

await test('the ? key opens it too', async () => {
	await selectNode(SERVER);
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: '?', bubbles: true})); return true; })()`);
	return (await panelText()).includes('Focus the search box') || 'the ? key did not open the shortcuts';
});

// --- colour scheme ---------------------------------------------------------

const scheme = () => ev(`(() => ({
	cls: document.documentElement.className,
	label: document.getElementById('theme-label').textContent,
	icon: document.getElementById('theme-icon').getAttribute('href'),
	node: godi.cy.getElementById(${JSON.stringify(SERVER)}).style('background-color'),
}))()`);

await test('the page starts on the scheme it was built with', async () => {
	const s = await scheme();
	return s.cls === 'theme-auto' || `started on ${s.cls}`;
});

await test('the toggle cycles auto to light', async () => {
	await ev(`(() => { document.getElementById('theme').click(); return true; })()`);
	const s = await scheme();
	return (s.cls === 'theme-light' && s.label === 'Light') || JSON.stringify(s);
});

await test('and the canvas is restyled, not just the chrome', async () => {
	const light = await scheme();
	await ev(`(() => { document.getElementById('theme').click(); return true; })()`);
	const dark = await scheme();
	return (dark.cls === 'theme-dark' && dark.node !== light.node)
		|| `node colour stayed ${light.node} going from light to dark`;
});

await test('dark cycles back round to auto', async () => {
	await ev(`(() => { document.getElementById('theme').click(); return true; })()`);
	const s = await scheme();
	return (s.cls === 'theme-auto' && s.label === 'Auto') || JSON.stringify(s);
});

await test('the t key cycles it too', async () => {
	await ev(`(() => { document.dispatchEvent(new KeyboardEvent('keydown', {key: 't', bubbles: true})); return true; })()`);
	const s = await scheme();
	return s.cls === 'theme-light' || `t left it on ${s.cls}`;
});

await ev(`(() => { document.getElementById('theme').click(); document.getElementById('theme').click(); return true; })()`);

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
