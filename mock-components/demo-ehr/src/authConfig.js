// OIDC Configuration for Knooppunt.
//
// redirect_uri / post_logout_redirect_uri are derived from the running origin
// plus PUBLIC_URL (the path the app is served under), so the same build works
// at http://localhost:3000/ and at https://example.com/ehr/. PUBLIC_URL is
// baked at build time by CRA.

const baseUrl = (() => {
  if (typeof window === 'undefined') return '';
  const prefix = process.env.PUBLIC_URL || '';
  return `${window.location.origin}${prefix}`;
})();

export const oidcConfig = {
  authority: process.env.REACT_APP_AUTHORITY || 'http://localhost:8081',
  client_id: 'demo-ehr',
  client_secret: 'demo-ehr-secret',
  redirect_uri: `${baseUrl}/callback`,
  response_type: 'code',
  scope: 'openid profile',
  post_logout_redirect_uri: `${baseUrl}/`,
  automaticSilentRenew: false,
  loadUserInfo: false,
};
