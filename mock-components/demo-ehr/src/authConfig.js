// OIDC Configuration for Knooppunt.
//
// redirect_uri / post_logout_redirect_uri are derived at runtime from
// window.location.origin + the runtime baseUrl, so the same image works at
// http://localhost:3000/ and at https://example.com/ehr/.
import { baseUrl, runtimeConfig } from './runtimeConfig';

const origin = typeof window !== 'undefined' ? window.location.origin : '';

export const oidcConfig = {
  authority: runtimeConfig.authority || 'http://localhost:8081',
  client_id: 'demo-ehr',
  client_secret: 'demo-ehr-secret',
  redirect_uri: `${origin}${baseUrl}/callback`,
  response_type: 'code',
  scope: 'openid profile',
  post_logout_redirect_uri: `${origin}${baseUrl}/`,
  automaticSilentRenew: false,
  loadUserInfo: false,
};
