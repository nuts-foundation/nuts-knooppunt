// Dev-only login that bypasses OIDC. Gated behind REACT_APP_DEV_LOGIN; the
// login button only renders when the flag is set, and AuthProvider only
// restores a stored dev session under the same flag.

import { runtimeConfig } from './runtimeConfig';

const DEV_USER_KEY = 'demo-ehr-dev-user';

export const isDevLoginEnabled = () => !!runtimeConfig.devLoginEnabled;

// `sub` doubles as the requesting organization URA in
// bgzVerweijzingApi.createBgZNotificatonTask, so use a URA-shaped value.
export const buildDevUser = () => ({
  profile: {
    sub: '00000666',
    name: 'Dev User',
    email: 'dev@example.local',
    client_id: 'demo-ehr-dev',
  },
  scopes: ['openid', 'profile'],
  expires_at: Math.floor(Date.now() / 1000) + 24 * 60 * 60,
  __devUser: true,
});

export const loadDevUser = () => {
  try {
    const raw = window.localStorage.getItem(DEV_USER_KEY);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
};

export const saveDevUser = (u) => {
  window.localStorage.setItem(DEV_USER_KEY, JSON.stringify(u));
};

export const clearDevUser = () => {
  window.localStorage.removeItem(DEV_USER_KEY);
};

export const isDevUser = (u) => !!(u && u.__devUser);
