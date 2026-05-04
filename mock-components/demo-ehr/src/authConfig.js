// Auth Configuration for the demo-dezi-client backend (Login with Dezi).
// The backend handles the Dezi OAuth flow and exposes /login, /userinfo,
// /logout endpoints that the EHR consumes via session cookies.
//
// Reads through runtimeConfig so the value can be set at deploy time via
// the container env (server.js injects it as window.__APP_CONFIG__) — no
// rebuild needed per deployment.
import { runtimeConfig } from './runtimeConfig';

export const authConfig = {
  baseUrl: runtimeConfig.authBaseUrl || 'http://localhost:8090',
};
