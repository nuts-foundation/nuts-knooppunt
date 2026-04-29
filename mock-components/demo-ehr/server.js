const path = require('path');
const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');
const allowlist = require('./proxy-allowlist');

const PORT = parseInt(process.env.PORT || '3000', 10);
const STATIC_DIR = path.resolve(__dirname, 'build');

const KNOOPPUNT_BASE_URL = process.env.KNOOPPUNT_BASE_URL || 'http://knooppunt:8081';
const FHIR_BASE_URL = process.env.FHIR_BASE_URL || process.env.REACT_APP_FHIR_BASE_URL;
const FHIR_STU3_BASE_URL = process.env.FHIR_STU3_BASE_URL || process.env.REACT_APP_FHIR_STU3_BASE_URL;
const FHIR_MCSD_QUERY_BASE_URL = process.env.FHIR_MCSD_QUERY_BASE_URL || process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL;

function mountProxy(app, prefix, target, rules, label) {
  if (!target) {
    console.warn(`[${label}] disabled: no upstream configured`);
    app.use(prefix, (_req, res) => res.status(503).json({ error: `${label} upstream not configured` }));
    return;
  }
  console.log(`[${label}] ${prefix} -> ${target}`);
  app.use(
    prefix,
    allowlist.makeGate(label, rules),
    createProxyMiddleware({
      target,
      changeOrigin: true,
      pathRewrite: { [`^${prefix}`]: '' },
      logLevel: 'warn',
      onProxyReq: (proxyReq, req) => {
        console.log(`[${label}] ${req.method} ${req.originalUrl} -> ${proxyReq.protocol}//${proxyReq.host}${proxyReq.path}`);
      },
      onError: (err, _req, res) => {
        console.error(`[${label}] proxy error:`, err.message);
        if (!res.headersSent) res.status(502).json({ error: 'Bad Gateway', detail: err.message });
      },
    })
  );
}

const app = express();
app.disable('x-powered-by');

app.get('/healthz', (_req, res) => res.json({ ok: true }));

// Dynamic proxy (target supplied via X-Target-URL header).
app.use(
  '/api/dynamic-proxy',
  allowlist.makeGate('dynamic-proxy', allowlist.DYNAMIC),
  (req, res, next) => {
    const targetUrl = req.headers['x-target-url'];
    if (!targetUrl) {
      return res.status(400).json({ error: 'Missing X-Target-URL header' });
    }
    let parsed;
    try {
      parsed = new URL(targetUrl);
    } catch {
      return res.status(400).json({ error: 'Invalid X-Target-URL' });
    }
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return res.status(400).json({ error: 'X-Target-URL must be http(s)' });
    }
    const proxy = createProxyMiddleware({
      target: targetUrl,
      changeOrigin: true,
      pathRewrite: { '^/api/dynamic-proxy': '' },
      logLevel: 'warn',
      onProxyReq: (proxyReq) => {
        proxyReq.removeHeader('x-target-url');
      },
      onError: (err, _r, response) => {
        console.error('[dynamic-proxy] error:', err.message);
        if (!response.headersSent) response.status(502).json({ error: 'Bad Gateway', detail: err.message });
      },
    });
    proxy(req, res, next);
  }
);

mountProxy(app, '/api/knooppunt', KNOOPPUNT_BASE_URL, allowlist.KNOOPPUNT, 'knooppunt');
mountProxy(app, '/api/fhir', FHIR_BASE_URL, allowlist.FHIR_R4, 'fhir-r4');
mountProxy(app, '/api/fhir-stu3', FHIR_STU3_BASE_URL, allowlist.FHIR_STU3, 'fhir-stu3');
mountProxy(app, '/api/mcsd', FHIR_MCSD_QUERY_BASE_URL, allowlist.MCSD, 'mcsd');

app.use(express.static(STATIC_DIR, { index: false }));
app.get('*', (_req, res) => res.sendFile(path.join(STATIC_DIR, 'index.html')));

app.listen(PORT, () => {
  console.log(`demo-ehr server listening on :${PORT}`);
});
