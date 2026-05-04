// Dev-only login that bypasses Dezi auth. Gated behind REACT_APP_DEV_LOGIN; the
// login button only renders when the flag is set, and AuthProvider only
// restores a stored dev session under the same flag.

import { runtimeConfig } from './runtimeConfig';

const DEV_USER_KEY = 'demo-ehr-dev-user';

export const isDevLoginEnabled = () => !!runtimeConfig.devLoginEnabled;

// Flat user shape mirrors what the demo-dezi-client backend returns from
// /userinfo, so the rest of the app sees a consistent user object.
// `sub` doubles as the requesting organization URA in
// bgzVerweijzingApi.createBgZNotificatonTask, so use a URA-shaped value.
export const buildDevUser = () => ({
  sub: '00000666',
  name: 'Dev User',
  email: 'dev@example.local',
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
