// Lands here after the OpenID4VCI flow returns from the issuer. Reads the
// session_id from the query string, retrieves the originating page URL stashed
// in sessionStorage by CredentialStatusCard, and redirects back with flash
// query params describing the outcome. The originating page picks the params
// up on mount and renders an inline banner.

import React, { useEffect } from 'react';
import { baseUrl } from '../runtimeConfig';

const SESSION_RETURN_PREFIX = 'vci_return:';

export default function CredentialCallbackPage() {
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const sessionId = params.get('session_id') || params.get('state') || '';
    const errorParam = params.get('error');
    const errorDesc = params.get('error_description');

    let returnUrl = `${window.location.origin}${baseUrl || ''}/`;
    let credentialType = '';

    // Try the per-session stash first; fall back to the "current" stash if the
    // Nuts node didn't echo session_id on the return redirect.
    const candidates = [];
    if (sessionId) candidates.push(`${SESSION_RETURN_PREFIX}${sessionId}`);
    candidates.push(`${SESSION_RETURN_PREFIX}current`);
    for (const key of candidates) {
      const raw = window.sessionStorage.getItem(key);
      if (!raw) continue;
      try {
        const stashed = JSON.parse(raw);
        if (stashed.origin) returnUrl = stashed.origin;
        if (stashed.type) credentialType = stashed.type;
      } catch {
        // fall through to default returnUrl
      }
      window.sessionStorage.removeItem(key);
      break;
    }
    // Always clear the fallback so a stale value doesn't bleed into a later
    // unrelated visit to /credential-callback.
    window.sessionStorage.removeItem(`${SESSION_RETURN_PREFIX}current`);

    const target = new URL(returnUrl, window.location.origin);
    if (errorParam) {
      target.searchParams.set('vci', 'error');
      if (credentialType) target.searchParams.set('vci_type', credentialType);
      target.searchParams.set('vci_msg', errorDesc || errorParam);
    } else {
      target.searchParams.set('vci', 'success');
      if (credentialType) target.searchParams.set('vci_type', credentialType);
    }

    window.location.replace(target.toString());
  }, []);

  return (
    <div className="loading-container" style={{ padding: '40px' }}>
      <div className="spinner"></div>
      <p>Finalizing credential request…</p>
    </div>
  );
}
