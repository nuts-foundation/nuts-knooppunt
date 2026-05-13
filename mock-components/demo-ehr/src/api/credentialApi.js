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
const URA_SUBJECT_PREFIX = 'ura_';

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
  // Case-insensitive: issuers occasionally use slightly different casing
  // (e.g. HealthCareProfessionalDelegationCredential as the request
  // identifier vs HealthcareProfessionalDelegationCredential on the issued VC).
  // `predicate(vc)` is an optional secondary filter — useful e.g. to scope a
  // PatientEnrollmentCredential match to a specific BSN.
  findByType(credentials, type, predicate) {
    const target = String(type).toLowerCase();
    return (credentials || []).find((vc) => {
      const matchesType = extractTypes(vc).some((t) => String(t).toLowerCase() === target);
      if (!matchesType) return false;
      if (predicate && !predicate(vc)) return false;
      return true;
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
    // credential_identifier in credential_request_params must match the requested
    // credential type so the issuer knows which credential to issue.
    body.credential_request_params = { ...(credentialDetails || {}), credential_identifier: credentialType };
    const res = await fetch(`${apiBase.nuts}/internal/auth/v2/${encodeURIComponent(subjectId)}/request-credential`, {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify(body),
    });
    if (!res.ok) throw await errorFrom(res, 'Request credential failed');
    return await safeJson(res);
  },
};

// base64url → UTF-8 string. Convert to standard base64, atob to a binary
// string, then decode the bytes as UTF-8 via TextDecoder.
const decodeBase64Url = (s) => {
  const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4));
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/') + pad;
  try {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    return new TextDecoder('utf-8').decode(bytes);
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
    return payload.vc || null;
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

// Returns the first credentialSubject as a plain object (handles both single
// and array forms). Used by per-row claim extractors that want to dig into
// nested credentialSubject structures (e.g. hasDelegation.scope.*).
export const getCredentialSubject = (vc) => {
  const obj = asVcObject(vc);
  if (!obj) return null;
  const raw = obj.credentialSubject;
  if (Array.isArray(raw)) return raw[0] || null;
  return raw || null;
};

// Walks a dotted path against the value, returning undefined if any segment
// is missing. Numeric segments index into arrays.
const getByPath = (root, path) => {
  if (root == null) return undefined;
  let cur = root;
  for (const seg of String(path).split('.')) {
    if (cur == null) return undefined;
    if (Array.isArray(cur)) {
      const idx = Number(seg);
      cur = Number.isInteger(idx) ? cur[idx] : undefined;
    } else if (typeof cur === 'object') {
      cur = cur[seg];
    } else {
      return undefined;
    }
  }
  return cur;
};

// Resolves a `{ label: dotPath }` map against the VC's first credentialSubject
// and returns a `{ label: value }` map for paths that exist (skipping
// undefined/null/empty results). Values can be scalars or arrays.
export const extractClaimsByPaths = (vc, paths) => {
  if (!paths) return {};
  const cs = getCredentialSubject(vc);
  if (!cs) return {};
  const out = {};
  for (const [label, path] of Object.entries(paths)) {
    const v = getByPath(cs, path);
    if (v == null) continue;
    if (Array.isArray(v) && v.length === 0) continue;
    out[label] = v;
  }
  return out;
};

// Default claim summary: returns the scalar credentialSubject claims
// (skipping `id`, `@type`, and any nested objects). Works for both JSON-LD
// and JWT-encoded VCs. Pages can override this per-credential by passing
// `claims` (a `{ label: dotPath }` map) on the row config.
export const summarizeSubjectClaims = (vc) => {
  const obj = asVcObject(vc);
  if (!obj) return null;

  const subject = {};
  const raw = obj.credentialSubject;
  const subjects = Array.isArray(raw) ? raw : raw ? [raw] : [];
  for (const cs of subjects) {
    if (!cs || typeof cs !== 'object') continue;
    for (const [k, v] of Object.entries(cs)) {
      if (k === 'id' || k === '@type') continue;
      if (v == null) continue;
      if (typeof v === 'object') continue;
      if (subject[k] == null) subject[k] = v;
    }
  }

  return subject;
};
