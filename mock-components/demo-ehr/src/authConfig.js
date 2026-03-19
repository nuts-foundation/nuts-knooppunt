// OIDC Configuration for Dezi via dezi-client
export const oidcConfig = {
  authority: 'http://localhost:8090',
  client_id: 'demo-ehr',
  redirect_uri: 'http://localhost:3000/callback',
  response_type: 'code',
  scope: 'openid',
  post_logout_redirect_uri: 'http://localhost:3000',
  automaticSilentRenew: false,
  loadUserInfo: true,
};

