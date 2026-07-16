// GF viewer · SPA implementation (Preact + htm, vendored, buildless). Zero npm dependencies.
//
//   node server.js   →  http://localhost:3012
//
// The server is an API: it sends the run as JSON over SSE (default `message`
// events) and serves static files. All rendering and state live in the client
// (spa/app.js): a Preact component tree holds the step and the received items,
// and re-renders the card list from them.
//
//   data: {"type":"step","n":4}
//   data: {"type":"item","item":{"kind":"card","title":"…","ms":184}}
//   data: {"type":"finished"}     → the client closes the EventSource

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const { RUN, RUN_END_MS, PANEL_CSS, FONTS } = require('../shared/scenario.js');

const PREACT = fs.readFileSync(path.join(__dirname, 'preact-standalone.js'));
const APP = fs.readFileSync(path.join(__dirname, 'app.js'));
const MAP_SVG = fs.readFileSync(path.join(__dirname, '../shared/map.svg.html'), 'utf8');
const MAP_CSS = fs.readFileSync(path.join(__dirname, '../shared/map.css'), 'utf8');
const MAP_JS = fs.readFileSync(path.join(__dirname, '../shared/map.js'), 'utf8');

const PAGE = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>GF viewer · SPA (Preact)</title>
<style>
${FONTS}
${PANEL_CSS}
${MAP_CSS}
</style>
</head>
<body>
  <div class="panel">
    <div class="head"><h1>GF viewer</h1><span class="live">● live · preact spa over sse</span><a href="/">replay run</a></div>
    <div id="viewer" class="mapwrap" data-s="0">
${MAP_SVG}
    </div>
    <div id="cards"></div>
  </div>
  <script>
${MAP_JS}
  </script>
  <script type="module" src="/app.js"></script>
</body>
</html>`;

function jsonStream(res) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-store',
    'Connection': 'keep-alive',
  });
  return {
    send(obj) { res.write(`data: ${JSON.stringify(obj)}\n\n`); },
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
  } else if (url.pathname === '/preact-standalone.js' || url.pathname === '/app.js') {
    res.writeHead(200, { 'Content-Type': 'text/javascript' });
    res.end(url.pathname === '/app.js' ? APP : PREACT);
  } else if (url.pathname === '/run') {
    const stream = jsonStream(res);
    const timers = RUN.map(([at, step, items]) => setTimeout(() => {
      stream.send({ type: 'step', n: step });
      for (const item of items) stream.send({ type: 'item', item });
    }, at));
    timers.push(setTimeout(() => {
      stream.send({ type: 'finished' });
      stream.close();
    }, RUN_END_MS));
    res.on('close', () => timers.forEach(clearTimeout));
  } else {
    res.writeHead(404);
    res.end('not found');
  }
}).listen(3012, () => console.log('GF viewer · SPA → http://localhost:3012'));
