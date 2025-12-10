const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
  // Dynamic proxy for arbitrary endpoint addresses
  // Usage: /api/dynamic-proxy with X-Target-URL header
  app.use(
    '/api/dynamic-proxy',
    (req, res, next) => {
      const targetUrl = req.headers['x-target-url'];

      if (!targetUrl) {
        return res.status(400).json({ error: 'Missing X-Target-URL header' });
      }

      console.log(`[Dynamic Proxy] Proxying to: ${targetUrl}`);

      // Create a proxy middleware on the fly for this request
      const proxy = createProxyMiddleware({
        target: targetUrl,
        changeOrigin: true,
        pathRewrite: {
          '^/api/dynamic-proxy': '', // remove /api/dynamic-proxy prefix
        },
        onProxyReq: (proxyReq, req, res) => {
          // Remove the X-Target-URL header before forwarding
          proxyReq.removeHeader('x-target-url');
          console.log(`[Dynamic Proxy] ${req.method} ${req.path} -> ${proxyReq.protocol}//${proxyReq.host}${proxyReq.path}`);
        },
        onError: (err, req, res) => {
          console.error('[Dynamic Proxy Error]', err);
        },
      });

      proxy(req, res, next);
    }
  );

  // Proxy for Knooppunt API (NVI endpoints)
  app.use(
    '/api/knooppunt',
    createProxyMiddleware({
      target: 'http://knooppunt:8081',
      changeOrigin: true,
      pathRewrite: {
        '^/api/knooppunt': '', // remove /api/knooppunt prefix when forwarding
      },
      onProxyReq: (proxyReq, req, res) => {
        console.log(`[Proxy] ${req.method} ${req.path} -> ${proxyReq.protocol}//${proxyReq.host}${proxyReq.path}`);
      },
      onError: (err, req, res) => {
        console.error('[Proxy Error]', err);
      },
    })
  );

  // Proxy for FHIR API if needed
  if (process.env.REACT_APP_FHIR_BASE_URL) {
    app.use(
      '/api/fhir',
      createProxyMiddleware({
        target: process.env.REACT_APP_FHIR_BASE_URL,
        changeOrigin: true,
        pathRewrite: {
          '^/api/fhir': '', // remove /api/fhir prefix when forwarding
        },
        onProxyReq: (proxyReq, req, res) => {
          console.log(`[Proxy] ${req.method} ${req.path} -> ${proxyReq.protocol}//${proxyReq.host}${proxyReq.path}`);
        },
        onError: (err, req, res) => {
          console.error('[Proxy Error]', err);
        },
      })
    );
  }
};
