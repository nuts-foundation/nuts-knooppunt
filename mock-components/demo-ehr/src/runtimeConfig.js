// Runtime configuration. In production, server.js injects
// `window.__APP_CONFIG__` into index.html before the bundle runs, so the
// values reflect the container's env at boot time — no rebuild per
// deployment. In dev (`npm start`), there's no injection; we fall back to
// CRA's build-time REACT_APP_* env vars so local development keeps working.

const fromEnv = () => ({
  baseUrl: '',
  authority: process.env.REACT_APP_AUTHORITY || 'http://localhost:8081',
  authBaseUrl: process.env.REACT_APP_AUTH_BASE_URL || 'http://localhost:8090',
  fhirBaseURL: process.env.REACT_APP_FHIR_BASE_URL || '',
  fhirStu3BaseURL: process.env.REACT_APP_FHIR_STU3_BASE_URL || '',
  mcsdQueryBaseURL: process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL || '',
  organizationURA: process.env.REACT_APP_ORGANIZATION_URA || '',
  devLoginEnabled:
    process.env.REACT_APP_DEV_LOGIN === '1' ||
    process.env.REACT_APP_DEV_LOGIN === 'true',
});

const fromWindow =
  typeof window !== 'undefined' && window.__APP_CONFIG__
    ? window.__APP_CONFIG__
    : null;

export const runtimeConfig = fromWindow || fromEnv();

// Strip a trailing slash so concatenations stay clean: `${baseUrl}/foo`.
export const baseUrl = (runtimeConfig.baseUrl || '').replace(/\/+$/, '');
