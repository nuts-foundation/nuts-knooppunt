// Auth Configuration for the demo-dezi-client backend (Login with Dezi).
// The backend handles the Dezi OAuth flow and exposes /login, /userinfo,
// /logout endpoints that the EHR consumes via session cookies.
export const authConfig = {
  baseUrl: process.env.REACT_APP_AUTH_BASE_URL || 'http://localhost:8090',
};
