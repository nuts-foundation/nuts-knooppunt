// Used by `react-scripts start` to provide the same /api/* proxies that
// server.js provides in production. The allowlist is shared so dev and prod
// enforce identical operations against each upstream.
const { createProxyMiddleware } = require('http-proxy-middleware');
const allowlist = require('../proxy-allowlist');

const KNOOPPUNT_TARGET = process.env.REACT_APP_KNOOPPUNT_BASE_URL || 'http://knooppunt:8081';

const upstreams = [
  { prefix: '/api/fhir',      target: process.env.REACT_APP_FHIR_BASE_URL,            rules: allowlist.FHIR_R4,    label: 'fhir-r4' },
  { prefix: '/api/fhir-stu3', target: process.env.REACT_APP_FHIR_STU3_BASE_URL,       rules: allowlist.FHIR_STU3,  label: 'fhir-stu3' },
  { prefix: '/api/mcsd',      target: process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL, rules: allowlist.MCSD,       label: 'mcsd' },
  { prefix: '/api/knooppunt', target: KNOOPPUNT_TARGET,                               rules: allowlist.KNOOPPUNT,  label: 'knooppunt' },
];

module.exports = function (app) {
  // Dynamic proxy: target supplied via X-Target-URL header.
  app.use(
    '/api/dynamic-proxy',
    allowlist.makeGate('dynamic-proxy', allowlist.DYNAMIC),
    (req, res, next) => {
      const targetUrl = req.headers['x-target-url'];
      if (!targetUrl) {
        return res.status(400).json({ error: 'Missing X-Target-URL header' });
      }
      const proxy = createProxyMiddleware({
        target: targetUrl,
        changeOrigin: true,
        pathRewrite: { '^/api/dynamic-proxy': '' },
        onProxyReq: (proxyReq) => proxyReq.removeHeader('x-target-url'),
        onError: (err) => console.error('[dynamic-proxy]', err.message),
      });
      proxy(req, res, next);
    }
  );

  for (const { prefix, target, rules, label } of upstreams) {
    if (!target) {
      console.warn(`[setupProxy ${label}] disabled: no target configured`);
      continue;
    }
    console.log(`[setupProxy ${label}] ${prefix} -> ${target}`);
    app.use(
      prefix,
      allowlist.makeGate(label, rules),
      createProxyMiddleware({
        target,
        changeOrigin: true,
        pathRewrite: { [`^${prefix}`]: '' },
        onProxyReq: (proxyReq, req) => {
          console.log(`[setupProxy ${label}] ${req.method} ${req.path} -> ${proxyReq.protocol}//${proxyReq.host}${proxyReq.path}`);
        },
        onError: (err) => console.error(`[setupProxy ${label}]`, err.message),
      })
    );
  }
};
