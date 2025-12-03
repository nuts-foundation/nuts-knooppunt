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
 * Mock token introspection for testing (format: bearer-<ura>-<uzi_role>-<practitioner_id>-<bsn>)
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

        // Mock token format: bearer-<ura>-<uzi_role>-<practitioner_id>-<bsn>
        // Example: bearer-00000020-01.015-123456789-900186021
        const parts = token.split('-');

        if (parts.length < 5 || parts[0] !== 'bearer') {
            request.return(200, JSON.stringify({
                active: false
            }));
            return;
        }

        // Return RFC 7662 compliant introspection response
        request.return(200, JSON.stringify({
            active: true,
            sub: 'mock-user',
            requesting_organization_ura: parts[1],
            requesting_uzi_role_code: parts[2],
            requesting_practitioner_identifier: parts[3],
            patient_bsn: parts[4],
            scope: 'patient_example'
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
        interactionType: null,
        resourceType: null,
        resourceId: null
    };
    
    // Only support resource reads for now
    context.interactionType = "read"

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
 * Build OPA request for PDP with clear field names matching XACML/Mitz terminology
 * @param {Object} tokenClaims - Claims from introspected token
 * @param {Object} fhirContext - FHIR resource context
 * @param {Object} request - NGINX request object
 * @returns {Object} - OPA-compliant request for PDP
 */
function buildOpaRequest(tokenClaims, fhirContext, request) {
    const uri = request.variables.request_uri || '';

    return {
        input: {
            scope: tokenClaims.scope,
            // HTTP Request context
            method: request.variables.request_method || request.method,
            path: parsePathArray(uri),

            // REQUESTING PARTY (who is asking for data)
            requesting_organization_ura: tokenClaims.requesting_organization_ura || null,
            requesting_uzi_role_code: tokenClaims.requesting_uzi_role_code || null,
            requesting_practitioner_identifier: tokenClaims.requesting_practitioner_identifier || null,
            // TODO: Facility type is a property of the organization (URA), not directly provided by clients.
            // Once authn/authz IGs are finalized, determine how to properly resolve this value.
            // Hardcoded for single-org reference implementation until architecture is defined.
            requesting_facility_type: process.env.REQUESTING_FACILITY_TYPE || 'Z3',

            // DATA HOLDER PARTY (who has the data being requested)
            data_holder_organization_ura: process.env.DATA_HOLDER_ORGANIZATION_URA || null,
            data_holder_facility_type: process.env.DATA_HOLDER_FACILITY_TYPE || 'Z3',

            // PATIENT/RESOURCE CONTEXT
            patient_bsn: tokenClaims.patient_bsn || null,
            interaction_type: fhirContext.interactionType,
            resource_type: fhirContext.resourceType,
            resource_id: fhirContext.resourceId,

            // PURPOSE OF USE
            purpose_of_use: process.env.PURPOSE_OF_USE || 'treatment'
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

        request.log(`Token parsed: requesting_org=${tokenClaims.requesting_organization_ura}, ` +
              `patient_bsn=${tokenClaims.patient_bsn}`);

        // Step 3: Extract FHIR context from request
        const fhirContext = extractFhirContext(request.variables.request_uri || '');

        request.log(`FHIR context: resourceType=${fhirContext.resourceType}, ` +
              `resourceId=${fhirContext.resourceId}`);

        // Step 4: Build OPA request for PDP
        const opaRequest = buildOpaRequest(tokenClaims, fhirContext, request);

        // Step 5: Call Knooppunt PDP
        const pdpRequestOpts = {
            method: 'POST',
            body: JSON.stringify(opaRequest)
        };

        const opaRequestInput = opaRequest.input;
        request.log(`Calling PDP: requesting_org=${opaRequestInput.requesting_organization_ura}, ` +
              `data_holder=${opaRequestInput.data_holder_organization_ura}, ` +
              `patient_bsn=${opaRequestInput.patient_bsn}, resource=${opaRequestInput.resource_type}`);

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
