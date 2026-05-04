const fs = require('fs');
const path = require('path');
const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');
const allowlist = require('./proxy-allowlist');

const PORT = parseInt(process.env.PORT || '3000', 10);
const STATIC_DIR = path.resolve(__dirname, 'build');

// Optional path prefix for serving the SPA + proxies under a sub-path (e.g.
// "/ehr"). All runtime — no rebuild required. Trailing slash stripped so
// concatenations stay clean.
const BASE_URL = (process.env.BASE_URL || '').replace(/\/+$/, '');
const at = (p) => `${BASE_URL}${p}`;

const KNOOPPUNT_BASE_URL = process.env.KNOOPPUNT_BASE_URL || 'http://knooppunt:8081';
const FHIR_BASE_URL = process.env.FHIR_BASE_URL || process.env.REACT_APP_FHIR_BASE_URL;
const FHIR_STU3_BASE_URL = process.env.FHIR_STU3_BASE_URL || process.env.REACT_APP_FHIR_STU3_BASE_URL;
const FHIR_MCSD_QUERY_BASE_URL = process.env.FHIR_MCSD_QUERY_BASE_URL || process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL;

// Runtime config exposed to the SPA via window.__APP_CONFIG__. Anything the
// browser needs to know that depends on deployment (paths, OIDC issuer,
// reference URLs, feature flags) goes here.
const APP_CONFIG = {
  baseUrl: BASE_URL,
  authority: process.env.REACT_APP_AUTHORITY || '',
  authBaseUrl: process.env.REACT_APP_AUTH_BASE_URL || '',
  fhirBaseURL: process.env.REACT_APP_FHIR_BASE_URL || '',
  fhirStu3BaseURL: process.env.REACT_APP_FHIR_STU3_BASE_URL || '',
  mcsdQueryBaseURL: process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL || '',
  organizationURA: process.env.REACT_APP_ORGANIZATION_URA || '',
  devLoginEnabled:
    process.env.DEV_LOGIN === '1' ||
    process.env.DEV_LOGIN === 'true' ||
    process.env.REACT_APP_DEV_LOGIN === '1' ||
    process.env.REACT_APP_DEV_LOGIN === 'true',
};

// Read index.html once, inject <base href> + window.__APP_CONFIG__. The base
// href is what makes CRA's relative asset paths (./static/...) resolve
// correctly even when the user lands on a deep SPA route.
const indexHtmlSrc = fs.readFileSync(path.join(STATIC_DIR, 'index.html'), 'utf8');
const baseHref = `${BASE_URL || ''}/`;
const safeConfig = JSON.stringify(APP_CONFIG).replace(/</g, '\\u003c');
const injection =
  `<base href="${baseHref}">` +
  `<script>window.__APP_CONFIG__ = ${safeConfig};</script>`;
const indexHtml = indexHtmlSrc.replace(/<head(\s[^>]*)?>/i, (m) => `${m}\n    ${injection}`);

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

app.get(at('/healthz'), (_req, res) => res.json({ ok: true }));

// Dynamic proxy (target supplied via X-Target-URL header).
app.use(
  at('/api/dynamic-proxy'),
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
      pathRewrite: { [`^${at('/api/dynamic-proxy')}`]: '' },
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

mountProxy(app, at('/api/knooppunt'), KNOOPPUNT_BASE_URL, allowlist.KNOOPPUNT, 'knooppunt');
mountProxy(app, at('/api/fhir'), FHIR_BASE_URL, allowlist.FHIR_R4, 'fhir-r4');
mountProxy(app, at('/api/fhir-stu3'), FHIR_STU3_BASE_URL, allowlist.FHIR_STU3, 'fhir-stu3');
mountProxy(app, at('/api/mcsd'), FHIR_MCSD_QUERY_BASE_URL, allowlist.MCSD, 'mcsd');

const sendIndex = (_req, res) => res.type('html').send(indexHtml);
if (BASE_URL) {
  app.use(BASE_URL, express.static(STATIC_DIR, { index: false }));
  app.get(`${BASE_URL}/*`, sendIndex);
} else {
  app.use(express.static(STATIC_DIR, { index: false }));
  app.get('*', sendIndex);
}

app.listen(PORT, () => {
  console.log(`demo-ehr server listening on :${PORT}${BASE_URL ? ` (base=${BASE_URL})` : ''}`);
});
