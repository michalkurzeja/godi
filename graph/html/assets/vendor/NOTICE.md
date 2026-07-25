# Third-party code bundled in the viewer

These files are vendored verbatim from npm and embedded, unmodified, in every
page `graph/html` writes. They are not godi's work. Their notices travel with
the page, which is what redistributing them requires.

| file | package | version | licence |
|---|---|---|---|
| `cytoscape.min.js` | [cytoscape](https://github.com/cytoscape/cytoscape.js) | 3.34.0 | MIT |
| `viz-global.js` | [@viz-js/viz](https://github.com/mdaines/viz-js) | 3.28.0 | MIT, wrapping Graphviz (EPL-2.0) and Expat (MIT) |
| `dagre.min.js` | [@dagrejs/dagre](https://github.com/dagrejs/dagre) | 3.0.0 | MIT |
| `cytoscape-dagre.min.js` | [cytoscape-dagre](https://github.com/cytoscape/cytoscape.js-dagre) | 4.0.0 | MIT |

## Graphviz is EPL-2.0

`viz-global.js` is Graphviz compiled to WebAssembly. Viz.js itself is MIT, but
the object code it carries is Graphviz under the Eclipse Public License 2.0.

EPL-2.0 is weak, file-level copyleft. It does not reach godi's own MIT code, nor
an application that uses godi. It asks that the notice is kept and that the
covered source stays available, which it is: <https://gitlab.com/graphviz/graphviz>.

If you would rather ship nothing but MIT, build the page with the dagre layout
engine, which leaves Graphviz out of the file entirely:

	html.New(html.Layout(html.Dagre))

## The notice dagre does not carry

Every bundle but `dagre.min.js` begins with its own header, which survives into
the page. Dagre's does not, so it is reproduced here and prepended to the bundle
when the page is written.

### dagre

Copyright (c) 2012-2014 Chris Pettitt

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

## Updating

	curl -sL -o cytoscape.min.js       https://cdn.jsdelivr.net/npm/cytoscape@3.34.0/dist/cytoscape.min.js
	curl -sL -o viz-global.js          https://cdn.jsdelivr.net/npm/@viz-js/viz@3.28.0/dist/viz-global.js
	curl -sL -o dagre.min.js           https://cdn.jsdelivr.net/npm/@dagrejs/dagre@3.0.0/dist/dagre.min.js
	curl -sL -o cytoscape-dagre.min.js https://cdn.jsdelivr.net/npm/cytoscape-dagre@4.0.0/dist/cytoscape-dagre.min.js

Bump the versions in this table when you do, and re-run the tests: the page
names the SHA-256 of every script in its content security policy, so a changed
byte that is not re-hashed stops the page dead.
