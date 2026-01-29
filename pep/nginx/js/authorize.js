/**
 * Knooppunt PEP Authorization Module
 *
 * This module handles authorization requests by:
 * 1. Extracting OAuth bearer/DPoP token from Authorization header
 * 2. Introspecting the token via Nuts node (RFC 7662)
 * 3. Validating DPoP token binding if present (RFC 9449)
 * 4. Building PDPInput request for Knooppunt PDP
 * 5. Enforcing the PDP's authorization decision
 *
 * The PDP will translate this to XACML format for Mitz "gesloten vraag".
 */

/**
 * Extract bearer or DPoP token from Authorization header
 * Supports both "Bearer <token>" and "DPoP <token>" formats
 * @param {Object} request - NGINX request object
 * @returns {string|null} - Token or null
 */
function extractBearerToken(request) {
    const authHeader = request.headersIn['Authorization'];
    if (!authHeader) {
        return null;
    }

    const match = authHeader.match(/^(Bearer|DPoP)\s+(.+)$/i);
    return match ? match[2] : null;
}

/**
 * Get the token type from Authorization header
 * @param {Object} request - NGINX request object
 * @returns {string|null} - "Bearer" or "DPoP" or null
 */
function getTokenType(request) {
    const authHeader = request.headersIn['Authorization'];
    if (!authHeader) {
        return null;
    }

    const match = authHeader.match(/^(Bearer|DPoP)\s+/i);
    return match ? match[1].toLowerCase() : null;
}

/**
 * Parse OAuth scopes from space-separated string
 * @param {string} scopeString - Space-separated scopes
 * @returns {Array<string>} - Array of scopes
 */
function parseScopes(scopeString) {
    if (!scopeString) return [];
    return scopeString.split(' ').filter(s => s.length > 0);
}

/**
 * Parse query parameters from query string
 * @param {string} queryString - Query string without leading ?
 * @returns {Object} - Map of param name to array of values
 */
function parseQueryParams(queryString) {
    if (!queryString) return {};
    const params = {};
    queryString.split('&').forEach(pair => {
        const idx = pair.indexOf('=');
        if (idx > 0) {
            const key = decodeURIComponent(pair.substring(0, idx));
            const value = decodeURIComponent(pair.substring(idx + 1));
            if (!params[key]) params[key] = [];
            params[key].push(value);
        }
    });
    return params;
}

// Standard OAuth/JWT/OIDC claims that should not be forwarded to PDP
// These are either handled specially (client_id, scope) or are token metadata
// Using object instead of Set for njs compatibility
// See: RFC 7662 (Introspection), RFC 9068 (JWT Access Token), OpenID Connect Core
const STANDARD_CLAIMS = {
    // RFC 7662 Introspection Response
    'active': true, 'client_id': true, 'scope': true, 'token_type': true,
    // RFC 7519 JWT / RFC 9068 JWT Access Token
    'iss': true, 'sub': true, 'aud': true, 'exp': true, 'nbf': true, 'iat': true, 'jti': true,
    // RFC 9449 DPoP
    'cnf': true,
    // OpenID Connect Core
    'azp': true, 'nonce': true, 'auth_time': true, 'sid': true, 'at_hash': true, 'c_hash': true
};

/**
 * Normalize a claim value for PDP
 * - Arrays are preserved as arrays (PDP may need to iterate)
 * - Plain objects are converted to JSON strings (structure unknown)
 * - Primitives are converted to strings
 * - null/undefined become empty strings
 * @param {*} value - Claim value from introspection
 * @returns {string|Array} - Normalized value
 */
function normalizeClaimValue(value) {
    if (value === null || value === undefined) {
        return '';
    }
    // Preserve arrays - PDP may need to check membership
    if (Array.isArray(value)) {
        return value;
    }
    // Convert plain objects to JSON string
    if (typeof value === 'object') {
        return JSON.stringify(value);
    }
    return String(value);
}

/**
 * Extract non-standard claims from introspection response
 * Filters out standard OAuth/JWT/OIDC claims and returns all custom claims
 * (typically populated by the Presentation Definition on the authorization server)
 * @param {Object} introspection - Introspection response from Nuts node
 * @returns {Object} - Custom claims to forward to PDP
 */
function extractPDClaims(introspection) {
    if (!introspection || typeof introspection !== 'object') {
        return {};
    }
    const claims = {};
    const keys = Object.keys(introspection);
    for (let i = 0; i < keys.length; i++) {
        const key = keys[i];
        if (!STANDARD_CLAIMS[key]) {
            claims[key] = normalizeClaimValue(introspection[key]);
        }
    }
    return claims;
}

/**
 * Build PDPInput request matching the Go PDPInput struct
 *
 * All non-standard claims from the introspection response are forwarded to the PDP.
 * The Presentation Definition on the authorization server defines which claims are
 * extracted from VCs - these are passed through automatically.
 *
 * @param {Object} introspection - Introspection response
 * @param {Object} request - NGINX request object
 * @returns {Object} - PDPInput request
 */
function buildPDPRequest(introspection, request) {
    const uri = request.variables.request_uri || request.uri || '';
    const uriParts = uri.split('?');
    let requestPath = uriParts[0];
    const queryString = uriParts[1];

    // Strip /fhir prefix to get the FHIR resource path
    // e.g., /fhir/Condition -> /Condition
    // The PEP always exposes /fhir/ externally; the PDP works with FHIR resource paths
    // Note: FHIR_BASE_PATH env var is for the backend path (e.g., /fhir/DEFAULT), not for stripping
    if (requestPath.startsWith('/fhir/')) {
        requestPath = requestPath.substring('/fhir'.length);
    }

    // Extract all PD-defined claims (non-standard OAuth/JWT claims)
    const pdClaims = extractPDClaims(introspection);

    // Build properties object - use Object.assign since njs doesn't support spread operator
    const properties = {
        client_id: introspection.client_id || '',
        client_qualifications: parseScopes(introspection.scope)
    };
    // Merge all PD-defined claims into properties
    Object.assign(properties, pdClaims);

    return {
        input: {
            subject: {
                type: 'organization',
                id: introspection.client_id || '',
                properties: properties
            },
            request: {
                method: request.variables.request_method || request.method || 'GET',
                protocol: 'HTTP/1.1',
                path: requestPath || '/',
                query_params: parseQueryParams(queryString),
                header: {},
                body: ''
            },
            context: {
                data_holder_organization_id: process.env.DATA_HOLDER_ORGANIZATION_URA || '',
                data_holder_facility_type: process.env.DATA_HOLDER_FACILITY_TYPE || '',
                patient_bsn: ''
            }
        }
    };
}

/**
 * Validate DPoP token binding (RFC 9449)
 * @param {Object} request - NGINX request object
 * @param {Object} introspection - Introspection response
 * @returns {Promise<Object>} - { valid: boolean, reason?: string }
 */
async function validateDPoP(request, introspection) {
    // If token doesn't have DPoP binding (no cnf.jkt), validation passes
    if (!introspection.cnf || !introspection.cnf.jkt) {
        return { valid: true };
    }

    const dpopHeader = request.headersIn['DPoP'];
    if (!dpopHeader) {
        return { valid: false, reason: 'DPoP header required but missing' };
    }

    const token = extractBearerToken(request);
    const host = request.headersIn['Host'] || request.headersIn['host'] || '';

    const payload = {
        dpop_proof: dpopHeader,
        method: request.variables.request_method || request.method || 'GET',
        thumbprint: introspection.cnf.jkt,
        token: token,
        url: `https://${host}${request.variables.request_uri || request.uri || ''}`
    };

    // Use ngx.fetch for DPoP validation (same pattern as introspection)
    const nutsHost = process.env.NUTS_NODE_HOST || 'knooppunt';
    const nutsPort = process.env.NUTS_NODE_INTERNAL_PORT || '8081';
    const dpopValidateUrl = `http://${nutsHost}:${nutsPort}/internal/auth/v2/dpop/validate`;

    request.warn(`DPoP validate URL: ${dpopValidateUrl}`);
    request.warn(`DPoP validate payload: ${JSON.stringify(payload)}`);

    try {
        const response = await ngx.fetch(dpopValidateUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(payload)
        });
        request.warn(`DPoP validate response status: ${response.status}`);

        if (!response.ok) {
            const errorText = await response.text();
            return { valid: false, reason: `DPoP validation returned ${response.status}: ${errorText}` };
        }

        const result = await response.json();
        return { valid: result.valid === true, reason: result.reason || '' };
    } catch (e) {
        return { valid: false, reason: `DPoP validation error: ${e}` };
    }
}

/**
 * Main authorization function called by NGINX auth_request
 * @param {Object} request - NGINX request object
 */
async function checkAuthorization(request) {
    try {
        request.warn('=== Starting authorization check ===');

        // Step 1: Extract token from Authorization header
        const token = extractBearerToken(request);
        if (!token) {
            request.error('Missing or invalid Authorization header');
            request.return(401);
            return;
        }
        request.warn(`Token extracted (first 20 chars): ${token.substring(0, 20)}...`);

        // RFC 9449: If using DPoP authorization scheme, DPoP header is required
        const tokenType = getTokenType(request);
        if (tokenType === 'dpop' && !request.headersIn['DPoP']) {
            request.error('DPoP authorization scheme requires DPoP header');
            request.return(401);
            return;
        }

        request.log('Token found, introspecting via Nuts node...');

        // Step 2: Introspect token via Nuts node (RFC 7662)
        // Use ngx.fetch for POST body support (njs subrequest doesn't work well with proxy_pass)
        const nutsHost = process.env.NUTS_NODE_HOST || 'knooppunt';
        const nutsPort = process.env.NUTS_NODE_INTERNAL_PORT || '8081';
        const introspectUrl = `http://${nutsHost}:${nutsPort}/internal/auth/v2/accesstoken/introspect`;

        let introspectionResponse;
        try {
            introspectionResponse = await ngx.fetch(introspectUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded'
                },
                body: `token=${encodeURIComponent(token)}`
            });
        } catch (e) {
            request.error(`Token introspection fetch failed: ${e}`);
            request.return(503);
            return;
        }

        if (!introspectionResponse.ok) {
            request.error(`Token introspection failed: ${introspectionResponse.status}`);
            request.return(introspectionResponse.status === 401 ? 401 : 503);
            return;
        }

        let introspection;
        try {
            introspection = await introspectionResponse.json();
        } catch (e) {
            request.error(`Failed to parse introspection response: ${e}`);
            request.return(401);
            return;
        }

        if (!introspection.active) {
            request.error('Token is not active');
            request.return(401);
            return;
        }

        request.log(`Token active: client_id=${introspection.client_id}, scope=${introspection.scope}`);

        // Step 3: Validate DPoP if token has cnf claim
        request.warn(`DPoP validation starting: cnf=${JSON.stringify(introspection.cnf)}`);
        const dpopResult = await validateDPoP(request, introspection);
        request.warn(`DPoP validation result: ${JSON.stringify(dpopResult)}`);
        if (!dpopResult.valid) {
            request.error(`DPoP validation failed: ${dpopResult.reason}`);
            request.return(401);
            return;
        }

        // Step 4: Build PDPInput request
        const pdpRequest = buildPDPRequest(introspection, request);

        request.log(`Calling PDP: client_id=${pdpRequest.input.subject.id}, ` +
            `path=${pdpRequest.input.request.path}, method=${pdpRequest.input.request.method}`);

        // Step 5: Call Knooppunt PDP using ngx.fetch
        const pdpHost = process.env.KNOOPPUNT_PDP_HOST || 'knooppunt';
        const pdpPort = process.env.KNOOPPUNT_PDP_PORT || '8081';
        const pdpUrl = `http://${pdpHost}:${pdpPort}/pdp`;

        let pdpResponse;
        try {
            pdpResponse = await ngx.fetch(pdpUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(pdpRequest)
            });
        } catch (e) {
            request.error(`PDP fetch failed: ${e}`);
            request.return(503);
            return;
        }

        if (!pdpResponse.ok) {
            request.error(`PDP returned error status: ${pdpResponse.status}`);
            request.return(503);
            return;
        }

        let pdpResult;
        try {
            pdpResult = await pdpResponse.json();
        } catch (e) {
            request.error(`Failed to parse PDP response: ${e}`);
            request.return(500);
            return;
        }

        // Step 6: Enforce decision
        if (pdpResult.result && pdpResult.result.allow === true) {
            request.log('Access ALLOWED by PDP');
            request.return(200);
        } else {
            const reasons = (pdpResult.result && pdpResult.result.reasons) ? pdpResult.result.reasons : [];
            request.warn(`Access DENIED by PDP: ${JSON.stringify(reasons)}`);
            request.return(403);
        }

    } catch (e) {
        request.error(`Authorization error: ${e}`);
        request.return(500);
    }
}

export default {
    checkAuthorization,
    extractBearerToken,
    getTokenType,
    parseScopes,
    parseQueryParams,
    normalizeClaimValue,
    extractPDClaims,
    buildPDPRequest,
    validateDPoP,
    STANDARD_CLAIMS
};
