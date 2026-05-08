// Renders the status of a fixed list of credential types for one Nuts subject:
//   - Present: shows issuer + issuance date.
//   - Missing: shows a Request button that kicks off OpenID4VCI against the
//     configured issuer for that type. The browser is redirected to the
//     issuer; on return, /credential-callback bounces back here with flash
//     query params (vci=success|error, vci_type=<type>, vci_msg=<text>).
//
// Used both on HomePage (org-level credentials) and PatientPage (patient
// enrollment), parameterized by `types` and `buildCredentialDetails`.

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { credentialApi, subjectIdForUra, summarizeCredential } from '../api/credentialApi';
import { config } from '../config';

const VCI_FLASH_KEYS = ['vci', 'vci_type', 'vci_msg'];
const SESSION_RETURN_PREFIX = 'vci_return:';

const stripFlashParams = () => {
  const url = new URL(window.location.href);
  let changed = false;
  for (const k of VCI_FLASH_KEYS) {
    if (url.searchParams.has(k)) {
      url.searchParams.delete(k);
      changed = true;
    }
  }
  if (changed) {
    window.history.replaceState({}, '', url.toString());
  }
};

const readFlash = () => {
  const url = new URL(window.location.href);
  const status = url.searchParams.get('vci');
  if (!status) return null;
  return {
    status,
    type: url.searchParams.get('vci_type') || '',
    msg: url.searchParams.get('vci_msg') || '',
  };
};

export default function CredentialStatusCard({
  title,
  description,
  ura,
  types,
  buildCredentialDetails,
  emptyHint,
}) {
  const [credentials, setCredentials] = useState(null);
  const [dids, setDids] = useState(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(null);
  const [pendingType, setPendingType] = useState(null);
  const [rowMessages, setRowMessages] = useState({});
  const [cardMessage, setCardMessage] = useState(null);

  const subjectId = ura ? subjectIdForUra(ura) : null;

  useEffect(() => {
    const flash = readFlash();
    if (!flash) return;
    const text = flash.status === 'success'
      ? 'Credential requested successfully.'
      : (flash.msg || 'Credential request failed.');
    if (flash.type) {
      setRowMessages((prev) => ({ ...prev, [flash.type]: { status: flash.status, text } }));
    } else {
      // No credential type came back — show a card-level banner so the user
      // still sees the outcome even when the issuer didn't echo correlation.
      setCardMessage({ status: flash.status, text });
    }
    stripFlashParams();
  }, []);

  const refresh = useCallback(async () => {
    if (!subjectId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setLoadError(null);
    try {
      const [didList, creds] = await Promise.all([
        credentialApi.listDids(subjectId),
        credentialApi.listCredentials(subjectId).catch(() => []),
      ]);
      setDids(didList);
      setCredentials(creds);
    } catch (err) {
      setLoadError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, [subjectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const onRequest = async (type) => {
    setPendingType(type);
    setRowMessages((prev) => ({ ...prev, [type]: null }));
    try {
      let didList = dids;
      if (!didList || !didList.length) {
        didList = await credentialApi.ensureSubject(subjectId);
        setDids(didList);
      }
      const walletDid = credentialApi.pickWalletDid(didList);
      if (!walletDid) throw new Error('No DIDs available for subject');

      const issuer = (config.credentialIssuers || {})[type];
      if (!issuer || issuer === 'x509') {
        throw new Error(`No OpenID4VCI issuer configured for ${type}`);
      }

      // Stash where to return so /credential-callback can route back.
      const redirectBase = `${window.location.origin}${(window.__APP_CONFIG__ && window.__APP_CONFIG__.baseUrl) || ''}`;
      const redirectUri = `${redirectBase}/credential-callback`;

      const credentialDetails = buildCredentialDetails
        ? buildCredentialDetails({ type, walletDid, ura })
        : undefined;

      const result = await credentialApi.requestCredential({
        subjectId,
        credentialType: type,
        issuer,
        walletDid,
        redirectUri,
        credentialDetails,
      });

      if (!result || !result.redirect_uri) {
        throw new Error('Nuts node did not return a redirect_uri');
      }

      const sessionId = result.session_id || '';
      const stash = JSON.stringify({ origin: window.location.href, type });
      // Stable fallback key in case the Nuts node doesn't echo session_id on
      // the return redirect — last-started request wins, which is fine for
      // the demo since concurrent requests are not expected.
      window.sessionStorage.setItem(`${SESSION_RETURN_PREFIX}current`, stash);
      if (sessionId) {
        window.sessionStorage.setItem(`${SESSION_RETURN_PREFIX}${sessionId}`, stash);
      }
      window.location.href = result.redirect_uri;
    } catch (err) {
      setRowMessages((prev) => ({
        ...prev,
        [type]: { status: 'error', text: err.message || String(err) },
      }));
      setPendingType(null);
    }
  };

  const rows = useMemo(() => types.map((t) => {
    const present = credentials ? credentialApi.findByType(credentials, t.type) : null;
    return { ...t, vc: present };
  }), [types, credentials]);

  return (
    <div className="card">
      <h3>{title}</h3>
      {description && <p>{description}</p>}

      {loadError && (
        <div className="error-message" style={{ marginBottom: '12px' }}>
          {loadError}
        </div>
      )}

      {cardMessage && (
        <div
          style={{
            fontSize: '13px',
            marginBottom: '12px',
            color: cardMessage.status === 'success' ? '#065f46' : '#b91c1c',
            background: cardMessage.status === 'success' ? '#ecfdf5' : '#fef2f2',
            border: '1px solid',
            borderColor: cardMessage.status === 'success' ? '#a7f3d0' : '#fecaca',
            borderRadius: '4px',
            padding: '8px 10px',
          }}
        >
          {cardMessage.text}
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', marginTop: '10px' }}>
        {rows.map((row) => {
          const summary = row.vc ? summarizeCredential(row.vc) : null;
          const msg = rowMessages[row.type];
          const issuer = (config.credentialIssuers || {})[row.type];
          const requestable = row.requestable !== false && issuer && issuer !== 'x509';
          return (
            <div
              key={row.type}
              style={{
                border: '1px solid #e5e7eb',
                borderRadius: '4px',
                padding: '10px 12px',
                background: '#fafafa',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '10px' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '13px', color: '#1f2937' }}>
                    {row.label || row.type}
                  </div>
                  <div style={{ fontSize: '12px', color: '#6b7280', marginTop: '2px' }}>
                    {loading
                      ? 'Loading…'
                      : row.vc
                        ? <span style={{ color: '#059669' }}>✓ Present</span>
                        : <span style={{ color: '#b45309' }}>⚠ Missing</span>}
                    {summary && summary.issued && (
                      <span style={{ color: '#6b7280' }}> · {summary.issued}</span>
                    )}
                  </div>
                </div>
                {!loading && !row.vc && requestable && (
                  <button
                    onClick={() => onRequest(row.type)}
                    className="button"
                    disabled={pendingType === row.type}
                  >
                    {pendingType === row.type ? `${row.actionLabel || 'Enroll'}…` : (row.actionLabel || 'Enroll')}
                  </button>
                )}
              </div>
              {summary && summary.subject && Object.keys(summary.subject).length > 0 && (
                <div style={{ fontSize: '11px', color: '#6b7280', marginTop: '4px', display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                  {Object.entries(summary.subject).map(([k, v]) => (
                    <span key={k}>
                      <span style={{ color: '#9ca3af' }}>{k}:</span> {String(v)}
                    </span>
                  ))}
                </div>
              )}
              {msg && (
                <div
                  style={{
                    fontSize: '12px',
                    marginTop: '6px',
                    color: msg.status === 'success' ? '#065f46' : '#b91c1c',
                    background: msg.status === 'success' ? '#ecfdf5' : '#fef2f2',
                    border: '1px solid',
                    borderColor: msg.status === 'success' ? '#a7f3d0' : '#fecaca',
                    borderRadius: '4px',
                    padding: '6px 8px',
                  }}
                >
                  {msg.text}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {emptyHint && !loading && credentials && credentials.length === 0 && (
        <p style={{ marginTop: '12px', fontSize: '12px' }}>{emptyHint}</p>
      )}
    </div>
  );
}
