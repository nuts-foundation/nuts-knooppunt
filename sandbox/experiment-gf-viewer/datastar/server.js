// GF viewer · Datastar implementation. Zero npm dependencies.
//
//   node server.js   →  http://localhost:3010
//
// The page is server-rendered once. Everything after that is Datastar's two
// SSE event types (https://data-star.dev/reference/sse_events):
//
//   datastar-patch-signals   data: signals {step: n}
//     → the `step` signal; data-attr:data-s keeps #viewer's data-s attribute
//       in sync, which is all the vanilla journey strip watches.
//
//   datastar-patch-elements  data: mode append / data: selector #cards
//     → server-rendered step cards appended to the card list.

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const { RUN, RUN_END_MS, PANEL_CSS, FONTS, renderItem } = require('../shared/scenario.js');

const DATASTAR = fs.readFileSync(path.join(__dirname, 'datastar.js'));
const MAP_SVG = fs.readFileSync(path.join(__dirname, '../shared/map.svg.html'), 'utf8');
const MAP_CSS = fs.readFileSync(path.join(__dirname, '../shared/map.css'), 'utf8');
const MAP_JS = fs.readFileSync(path.join(__dirname, '../shared/map.js'), 'utf8');

const PAGE = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>GF viewer · Datastar</title>
<script type="module" src="/datastar.js"></script>
<style>
${FONTS}
${PANEL_CSS}
${MAP_CSS}
</style>
</head>
<body>
  <div class="panel">
    <div class="head"><h1>GF viewer</h1><span class="live">● live · datastar over sse</span><a href="/">replay run</a></div>
    <div id="viewer" class="mapwrap" data-signals:step="0" data-attr:data-s="$step" data-init="@get('/run')">
${MAP_SVG}
    </div>
    <div id="cards"></div>
  </div>
  <script>
${MAP_JS}
  </script>
</body>
</html>`;

/* Datastar SSE wire format; see the reference for the event and data-line grammar. */
function datastarStream(res) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-store',
    'Connection': 'keep-alive',
  });
  return {
    patchSignals(signals) {
      res.write(`event: datastar-patch-signals\ndata: signals ${JSON.stringify(signals)}\n\n`);
    },
    appendElements(selector, html) {
      res.write(`event: datastar-patch-elements\ndata: selector ${selector}\ndata: mode append\ndata: elements ${html}\n\n`);
    },
    close() { res.end(); },
  };
}

http.createServer((req, res) => {
  if (req.method !== 'GET') {
    res.writeHead(405, { Allow: 'GET' });
    return res.end();
  }
  const url = new URL(req.url, 'http://localhost');
  if (url.pathname === '/') {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(PAGE);
  } else if (url.pathname === '/datastar.js') {
    res.writeHead(200, { 'Content-Type': 'text/javascript' });
    res.end(DATASTAR);
  } else if (url.pathname === '/run') {
    const stream = datastarStream(res);
    const timers = RUN.map(([at, step, items]) => setTimeout(() => {
      stream.patchSignals({ step });
      for (const item of items) stream.appendElements('#cards', renderItem(item));
    }, at));
    timers.push(setTimeout(() => stream.close(), RUN_END_MS));
    res.on('close', () => timers.forEach(clearTimeout));
  } else {
    res.writeHead(404);
    res.end('not found');
  }
}).listen(3010, () => console.log('GF viewer · Datastar → http://localhost:3010'));
