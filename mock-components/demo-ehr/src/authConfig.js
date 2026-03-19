// Auth Configuration for demo-dezi-client
const env = window._env_ || {};

export const authConfig = {
  baseUrl: env.AUTH_BASE_URL || process.env.REACT_APP_AUTH_BASE_URL || 'http://localhost:8090',
};
