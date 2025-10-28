/**
 * Knooppunt PEP Authorization Module
 *
 * This module handles authorization requests by:
 * 1. Extracting OAuth bearer token from Authorization header
 * 2. Introspecting the token to get claims
 * 3. Building flat JSON OPA request with FHIR context
 * 4. Forwarding to Knooppunt PDP which makes "gesloten vraag" to Mitz
 * 5. Enforcing the PDP's authorization decision
 *
 * The PDP (Issue #216) will translate this to XACML format for Mitz.
 */

/**
 * Extract bearer token from Authorization header
 * @param {Object} request - NGINX request object
 * @returns {string|null} - Bearer token or null
 */
function extractBearerToken(request) {
    const authHeader = request.headersIn['Authorization'];
    if (!authHeader) {
        return null;
    }

    const match = authHeader.match(/^Bearer\s+(.+)$/i);
    return match ? match[1] : null;
}

/**
 * Mock token introspection for testing (format: bearer-<ura>-<role>-<uzi>)
 * Called by /_introspect endpoint
 * @param {Object} request - NGINX request object
 */
function mockIntrospect(request) {
    try {
        // Parse request body to get token
        const body = request.requestText || '';
        const tokenMatch = body.match(/token=([^&]+)/);

        if (!tokenMatch) {
            request.return(400, JSON.stringify({
                error: 'invalid_request',
                error_description: 'Missing token parameter'
            }));
            return;
        }

        const token = decodeURIComponent(tokenMatch[1]);

        // Mock token format: bearer-<ura>-<role>-<uzi>
        // Example: bearer-00000020-practitioner-123456789
        const parts = token.split('-');

        if (parts.length < 4 || parts[0] !== 'bearer') {
            request.return(200, JSON.stringify({
                active: false
            }));
            return;
        }

        // Return RFC 7662 compliant introspection response
        request.return(200, JSON.stringify({
            active: true,
            sub: 'mock-user',
            ura: parts[1],
            role: parts[2],
            uzi: parts[3] || null,
            organization: 'Mock Organization',
            scope: 'fhir/Patient.read fhir/Observation.read'
        }));

    } catch (e) {
        request.error(`Mock introspection error: ${e}`);
        request.return(500, JSON.stringify({
            error: 'server_error',
            error_description: 'Introspection failed'
        }));
    }
}

/**
 * Extract FHIR resource type and ID from URI
 * Supports: /fhir/Patient/123, /fhir/Observation/456
 * @param {string} uri - Request URI
 * @returns {Object} - {resourceType, resourceId}
 */
function extractFhirContext(uri) {
    const context = {
        resourceType: null,
        resourceId: null
    };

    // Remove /fhir/ prefix
    const fhirPath = uri.replace(/^\/fhir\//, '');

    // Extract resource type and ID from path: Patient/123
    const pathMatch = fhirPath.match(/^([A-Za-z]+)(?:\/([^?]+))?/);
    if (pathMatch) {
        context.resourceType = pathMatch[1];
        context.resourceId = pathMatch[2] || null;
    }

    return context;
}

/**
 * Parse URI path into array
 * @param {string} uri - Request URI
 * @returns {Array} - Path segments
 */
function parsePathArray(uri) {
    // Remove leading slash and query string
    const path = uri.replace(/^\//, '').split('?')[0];
    return path.split('/').filter(segment => segment.length > 0);
}

/**
 * Build OPA request for PDP (snake_case format per OPA style guide)
 * This format will be translated to XACML by the PDP for Mitz "gesloten vraag"
 * @param {Object} tokenClaims - Claims from introspected token
 * @param {Object} fhirContext - FHIR resource context
 * @param {Object} request - NGINX request object
 * @returns {Object} - OPA-compliant request with snake_case fields
 */
function buildOpaRequest(tokenClaims, fhirContext, request) {
    const uri = request.variables.request_uri || '';

    return {
        input: {
            // HTTP Request context
            method: request.variables.request_method || request.method,
            path: parsePathArray(uri),

            // Subject (requester) information from token
            subject_type: tokenClaims.role || 'unknown',
            subject_id: tokenClaims.sub || null,
            subject_role: tokenClaims.role || null,
            subject_uzi: tokenClaims.uzi || null,

            // Organization context
            organization_ura: tokenClaims.ura || null,

            // Resource context (optional fields for FHIR)
            resource_type: fhirContext.resourceType,
            resource_id: fhirContext.resourceId,

            // Purpose
            purpose_of_use: 'treatment' // TODO: extract from token or request
        }
    };
}

/**
 * Main authorization function called by NGINX auth_request
 * @param {Object} request - NGINX request object
 */
async function checkAuthorization(request) {
    try {
        // Step 1: Extract bearer token from Authorization header
        const token = extractBearerToken(request);
        if (!token) {
            request.error('Missing or invalid Authorization header');
            request.return(401);
            return;
        }

        request.log('Bearer token found, introspecting...');

        // Step 2: Introspect token via OAuth endpoint
        // For testing: /_introspect calls mockIntrospect() function
        // For production: Change /_introspect to proxy to real OAuth server
        const introspectionResponse = await request.subrequest('/_introspect', {
            method: 'POST',
            body: `token=${encodeURIComponent(token)}`
        });

        if (introspectionResponse.status !== 200) {
            request.error(`Token introspection failed: ${introspectionResponse.status}`);
            request.return(401);
            return;
        }

        let tokenClaims;
        try {
            tokenClaims = JSON.parse(introspectionResponse.responseText);
        } catch (e) {
            request.error(`Failed to parse introspection response: ${e}`);
            request.return(401);
            return;
        }

        if (!tokenClaims.active) {
            request.error('Token is not active');
            request.return(401);
            return;
        }

        request.log(`Token parsed: sub=${tokenClaims.sub}, ura=${tokenClaims.ura}`);

        // Step 3: Extract FHIR context from request
        const fhirContext = extractFhirContext(request.variables.request_uri || '');

        request.log(`FHIR context: resourceType=${fhirContext.resourceType}, ` +
              `resourceId=${fhirContext.resourceId}`);

        // Step 4: Build flat JSON OPA request for PDP
        const opaRequest = buildOpaRequest(tokenClaims, fhirContext, request);

        // Step 5: Call Knooppunt PDP
        const pdpRequestOpts = {
            method: 'POST',
            body: JSON.stringify(opaRequest)
        };

        const opaRequestInput = opaRequest.input;
        request.log(`Calling PDP: OrganizationURA=${opaRequestInput.organization_ura}, ResourceType=${opaRequestInput.resource_type}, ` +
              `ResourceId=${opaRequestInput.resource_id}, Method=${opaRequestInput.method}`);

        const pdpResponse = await request.subrequest('/_pdp', pdpRequestOpts);

        // Step 6: Process PDP response
        if (pdpResponse.status !== 200) {
            request.error(`PDP returned error status: ${pdpResponse.status}`);
            request.return(500);
            return;
        }

        let opaResponse;
        try {
            opaResponse = JSON.parse(pdpResponse.responseText);
        } catch (e) {
            request.error(`Failed to parse PDP response: ${e}`);
            request.return(500);
            return;
        }

        // Step 7: Extract decision from OPA result
        if (!opaResponse.result) {
            request.error('PDP response missing result field');
            request.return(500);
            return;
        }

        const decision = opaResponse.result;

        // Step 8: Enforce decision
        if (decision.allow === true) {
            request.log('Access ALLOWED by PDP');
            request.return(200);
        } else {
            const reason = decision.reason || 'policy-denied';
            request.warn(`Access DENIED by PDP: reason=${reason}`);
            request.return(403);
        }

    } catch (e) {
        request.error(`Authorization error: ${e}`);
        request.return(500);
    }
}

// Export all functions for both NJS (needs default export) and tests (can destructure)
export default {
    checkAuthorization,
    mockIntrospect,
    extractBearerToken,
    parsePathArray,
    extractFhirContext,
    buildOpaRequest
};
