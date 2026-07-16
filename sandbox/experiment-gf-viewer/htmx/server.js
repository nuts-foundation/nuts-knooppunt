// GF viewer · htmx implementation. Zero npm dependencies.
//
//   node server.js   →  http://localhost:3011
//
// The page is server-rendered once. Updates arrive via the htmx SSE extension
// (https://htmx.org/extensions/sse/) as named SSE events:
//
//   event: card       → sse-swap="card" with hx-swap="beforeend" appends the
//                       server-rendered fragment to the card list.
//   event: step       → swapped into a hidden sink element (the extension
//                       discards events without an sse-swap target); an
//                       htmx:sseBeforeMessage listener intercepts it and writes
//                       the value to #viewer's data-s, which the strip watches.
//   event: finished   → sse-close closes the EventSource so it does not
//                       auto-reconnect and replay the run.

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const { RUN, RUN_END_MS, PANEL_CSS, FONTS, renderItem } = require('../shared/scenario.js');

const HTMX = fs.readFileSync(path.join(__dirname, 'htmx.min.js'));
const SSE_EXT = fs.readFileSync(path.join(__dirname, 'sse.js'));
const MAP_SVG = fs.readFileSync(path.join(__dirname, '../shared/map.svg.html'), 'utf8');
const MAP_CSS = fs.readFileSync(path.join(__dirname, '../shared/map.css'), 'utf8');
const MAP_JS = fs.readFileSync(path.join(__dirname, '../shared/map.js'), 'utf8');

const PAGE = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>GF viewer · htmx</title>
<script src="/htmx.min.js"></script>
<script src="/sse.js"></script>
<style>
${FONTS}
${PANEL_CSS}
${MAP_CSS}
</style>
</head>
<body>
  <div class="panel" hx-ext="sse" sse-connect="/run" sse-close="finished">
    <div class="head"><h1>GF viewer</h1><span class="live">● live · htmx over sse</span><a href="/">replay run</a></div>
    <div id="viewer" class="mapwrap" data-s="0">
${MAP_SVG}
    </div>
    <span sse-swap="step" hidden></span>
    <div id="cards" sse-swap="card" hx-swap="beforeend"></div>
  </div>
  <script>
  /* The step event has no DOM target: route it to #viewer's data-s attribute. */
  document.body.addEventListener('htmx:sseBeforeMessage', function (e) {
    if (e.detail.type === 'step') {
      document.getElementById('viewer').dataset.s = e.detail.data;
      e.preventDefault();
    }
  });
${MAP_JS}
  </script>
</body>
</html>`;

/* Named SSE events, as the htmx SSE extension expects them. */
function htmxStream(res) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-store',
    'Connection': 'keep-alive',
  });
  return {
    event(name, data) {
      res.write(`event: ${name}\ndata: ${data}\n\n`);
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
  } else if (url.pathname === '/htmx.min.js' || url.pathname === '/sse.js') {
    res.writeHead(200, { 'Content-Type': 'text/javascript' });
    res.end(url.pathname === '/sse.js' ? SSE_EXT : HTMX);
  } else if (url.pathname === '/run') {
    const stream = htmxStream(res);
    const timers = RUN.map(([at, step, items]) => setTimeout(() => {
      stream.event('step', String(step));
      for (const item of items) stream.event('card', renderItem(item));
    }, at));
    timers.push(setTimeout(() => {
      stream.event('finished', 'done');
      stream.close();
    }, RUN_END_MS));
    res.on('close', () => timers.forEach(clearTimeout));
  } else {
    res.writeHead(404);
    res.end('not found');
  }
}).listen(3011, () => console.log('GF viewer · htmx → http://localhost:3011'));
