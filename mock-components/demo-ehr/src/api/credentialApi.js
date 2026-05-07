// Wraps the Nuts node internal API used to:
//   - ensure a subject exists for the care organization (keyed by URA)
//   - list the subject's DIDs (favoring did:web)
//   - list credentials currently held by that subject
//   - initiate an OpenID4VCI request-credential flow against an issuer
//
// Routed via the demo-ehr Express layer (`/api/nuts/*`), which proxies to the
// Nuts node and enforces a narrow allowlist (see proxy-allowlist.js).

import { apiBase } from '../config';

// Subject IDs in the Nuts node must match [a-zA-Z0-9._-]+ — no colons. We
// prefix URAs with `ura_` to keep them recognizable as Dutch healthcare
// provider identifiers while staying within the allowed character set.
export const URA_SUBJECT_PREFIX = 'ura_';

export const subjectIdForUra = (ura) => `${URA_SUBJECT_PREFIX}${ura}`;

const jsonHeaders = { 'Content-Type': 'application/json', Accept: 'application/json' };

const safeJson = async (res) => {
  const text = await res.text();
  if (!text) return null;
  try { return JSON.parse(text); } catch { return text; }
};

const errorFrom = async (res, label) => {
  const body = await safeJson(res);
  const detail = typeof body === 'string' ? body : (body && (body.detail || body.title || body.error)) || res.statusText;
  return new Error(`${label} (${res.status}): ${detail}`);
};

export const credentialApi = {
  // GET /internal/vdr/v2/subject/{id}. Returns array of DID strings, or null
  // if the subject does not yet exist.
  async listDids(subjectId) {
    const res = await fetch(`${apiBase.nuts}/internal/vdr/v2/subject/${encodeURIComponent(subjectId)}`);
    if (res.status === 404) return null;
    if (!res.ok) throw await errorFrom(res, 'List DIDs failed');
    const body = await safeJson(res);
    return Array.isArray(body) ? body : [];
  },

  // POST /internal/vdr/v2/subject. Creates a subject with the given id; the
  // node also generates DIDs for it.
  async createSubject(subjectId) {
    const res = await fetch(`${apiBase.nuts}/internal/vdr/v2/subject`, {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ subject: subjectId }),
    });
    if (!res.ok) throw await errorFrom(res, 'Create subject failed');
    return await safeJson(res);
  },

  // Idempotent: returns the DID list for the subject, creating the subject
  // first if it doesn't exist yet.
  async ensureSubject(subjectId) {
    const existing = await this.listDids(subjectId);
    if (existing && existing.length) return existing;
    const created = await this.createSubject(subjectId);
    if (created && Array.isArray(created.documents)) {
      return created.documents.map((d) => d.id).filter(Boolean);
    }
    return await this.listDids(subjectId) || [];
  },

  // Pick the wallet DID to use in a VCI request. Favor did:web; fall back to
  // the first DID of any method.
  pickWalletDid(dids) {
    if (!dids || !dids.length) return null;
    const web = dids.find((d) => typeof d === 'string' && d.startsWith('did:web:'));
    return web || dids[0];
  },

  // GET /internal/vcr/v2/holder/{subjectID}/vc. Returns array of VCs (each a
  // JWT or JSON-LD VC). Returns [] when the subject has no credentials.
  async listCredentials(subjectId) {
    const res = await fetch(`${apiBase.nuts}/internal/vcr/v2/holder/${encodeURIComponent(subjectId)}/vc`);
    if (res.status === 404) return [];
    if (!res.ok) throw await errorFrom(res, 'List credentials failed');
    const body = await safeJson(res);
    return Array.isArray(body) ? body : [];
  },

  // Finds the first credential whose `type` array contains the given type
  // identifier. Handles both JSON-LD (object with .type) and JWT (decoded vc
  // claim) shapes commonly returned by the Nuts node.
  findByType(credentials, type) {
    return (credentials || []).find((vc) => {
      const types = extractTypes(vc);
      return types.includes(type);
    }) || null;
  },

  // POST /internal/auth/v2/{subjectID}/request-credential. Returns
  // { redirect_uri, session_id }. Caller must redirect the browser to
  // redirect_uri to complete authorization at the issuer.
  async requestCredential({ subjectId, credentialType, issuer, walletDid, redirectUri, credentialDetails }) {
    const body = {
      wallet_did: walletDid,
      issuer,
      authorization_details: [
        { type: 'openid_credential', credential_configuration_id: credentialType },
      ],
      redirect_uri: redirectUri,
    };
    if (credentialDetails && Object.keys(credentialDetails).length) {
      body.credential_details = credentialDetails;
    }
    const res = await fetch(`${apiBase.nuts}/internal/auth/v2/${encodeURIComponent(subjectId)}/request-credential`, {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify(body),
    });
    if (!res.ok) throw await errorFrom(res, 'Request credential failed');
    return await safeJson(res);
  },
};

const extractTypes = (vc) => {
  if (!vc) return [];
  if (Array.isArray(vc.type)) return vc.type;
  if (typeof vc.type === 'string') return [vc.type];
  if (vc.vc && Array.isArray(vc.vc.type)) return vc.vc.type;
  return [];
};

// Shorthand summary for display: returns issuer, issuance date, and any
// credentialSubject claims that are simple scalars. JWT-encoded VCs are not
// decoded here — the caller can pass already-parsed VC objects.
export const summarizeCredential = (vc) => {
  if (!vc || typeof vc !== 'object') return null;
  const issuer = vc.issuer || (vc.vc && vc.vc.issuer) || null;
  const issued = vc.issuanceDate || vc.validFrom || (vc.vc && (vc.vc.issuanceDate || vc.vc.validFrom)) || null;
  return {
    issuer: typeof issuer === 'object' ? issuer.id || issuer.name : issuer,
    issued,
  };
};
