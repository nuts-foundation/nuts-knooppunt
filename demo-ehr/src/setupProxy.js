const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
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
