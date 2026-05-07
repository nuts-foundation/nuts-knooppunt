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
  // identifier. Handles both JSON-LD (object) and JWT-encoded (string) VCs.
  findByType(credentials, type) {
    return (credentials || []).find((vc) => extractTypes(vc).includes(type)) || null;
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

// base64url decode for JWT segments. Browsers don't have a native
// base64url decoder; convert to base64 then atob().
const decodeBase64Url = (s) => {
  const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4));
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/') + pad;
  try {
    return decodeURIComponent(escape(atob(b64)));
  } catch {
    return null;
  }
};

const decodeJwtPayload = (jwt) => {
  if (typeof jwt !== 'string') return null;
  const parts = jwt.split('.');
  if (parts.length < 2) return null;
  const decoded = decodeBase64Url(parts[1]);
  if (!decoded) return null;
  try { return JSON.parse(decoded); } catch { return null; }
};

// Returns the JSON-LD VC view for a credential, regardless of wire form.
// JWT VCs carry the VC under the `vc` claim per W3C VC-JWT.
const asVcObject = (vc) => {
  if (!vc) return null;
  if (typeof vc === 'string') {
    const payload = decodeJwtPayload(vc);
    if (!payload) return null;
    // Top-level claims (iss, sub, nbf) supplement what's in payload.vc.
    return { ...(payload.vc || {}), __jwt: payload };
  }
  if (typeof vc === 'object') return vc;
  return null;
};

const extractTypes = (vc) => {
  const obj = asVcObject(vc);
  if (!obj) return [];
  if (Array.isArray(obj.type)) return obj.type;
  if (typeof obj.type === 'string') return [obj.type];
  return [];
};

// Shorthand summary for display: returns issuer and issuance date for either
// JSON-LD or JWT-encoded VCs.
export const summarizeCredential = (vc) => {
  const obj = asVcObject(vc);
  if (!obj) return null;
  const issuer = obj.issuer || (obj.__jwt && obj.__jwt.iss) || null;
  let issued = obj.issuanceDate || obj.validFrom || null;
  if (!issued && obj.__jwt && obj.__jwt.nbf) {
    issued = new Date(obj.__jwt.nbf * 1000).toISOString().slice(0, 10);
  }
  return {
    issuer: typeof issuer === 'object' ? issuer.id || issuer.name : issuer,
    issued,
  };
};
