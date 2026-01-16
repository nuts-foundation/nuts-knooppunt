// OIDC Configuration for Knooppunt
export const oidcConfig = {
  authority: 'http://localhost:8081',
  client_id: 'demo-ehr',
  client_secret: 'demo-ehr-secret',
  redirect_uri: 'http://localhost:3000/callback',
  response_type: 'code',
  scope: 'openid profile',
  post_logout_redirect_uri: 'http://localhost:3000',
  automaticSilentRenew: false,
  loadUserInfo: false,
};

