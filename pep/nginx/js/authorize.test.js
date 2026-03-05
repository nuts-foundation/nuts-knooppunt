/**
 * Unit tests for authorize.js
 * Run with: npm test (from pep/nginx/js directory)
 */

import authorize from './authorize.js';
import {jest} from '@jest/globals';

const {
    extractBearerToken,
    getTokenType,
    parseQueryParams,
    normalizeClaimValue,
    buildPDPRequest,
    validateDPoP
} = authorize;

function createMockRequest(overrides = {}) {
    const {variables, headersIn, method, ...rest} = overrides;
    const effectiveMethod = method || 'GET';
    return {
        headersIn: {...headersIn},
        variables: {
            request_uri: '',
            request_method: effectiveMethod,
            ...variables
        },
        uri: '',
        method: effectiveMethod,
        requestText: '',
        log: jest.fn(),
        error: jest.fn(),
        warn: jest.fn(),
        return: jest.fn(),
        subrequest: jest.fn(),
        ...rest
    };
}

// Helper to create mock subrequest response (njs style)
function createMockSubrequestResponse(status, body) {
    return {
        status,
        responseText: typeof body === 'string' ? body : JSON.stringify(body)
    };
}

describe('extractBearerToken', () => {
    test('extracts valid bearer token', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer abc123'}
        });
        expect(extractBearerToken(request)).toBe('abc123');
    });

    test('extracts DPoP token', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'DPoP xyz789'}
        });
        expect(extractBearerToken(request)).toBe('xyz789');
    });

    test('returns null when Authorization header is missing', () => {
        const request = createMockRequest();
        expect(extractBearerToken(request)).toBeNull();
    });

    test('returns null for invalid format', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'Basic abc123'}
        });
        expect(extractBearerToken(request)).toBeNull();
    });

    test('handles case-insensitive Bearer keyword', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'BEARER token123'}
        });
        expect(extractBearerToken(request)).toBe('token123');
    });

    test('handles case-insensitive DPoP keyword', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'DPOP token456'}
        });
        expect(extractBearerToken(request)).toBe('token456');
    });
});

describe('getTokenType', () => {
    test('returns bearer for Bearer token', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer abc123'}
        });
        expect(getTokenType(request)).toBe('bearer');
    });

    test('returns dpop for DPoP token', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'DPoP xyz789'}
        });
        expect(getTokenType(request)).toBe('dpop');
    });

    test('returns null when Authorization header is missing', () => {
        const request = createMockRequest();
        expect(getTokenType(request)).toBeNull();
    });

    test('returns null for invalid format', () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'Basic abc123'}
        });
        expect(getTokenType(request)).toBeNull();
    });
});

describe('normalizeClaimValue', () => {
    test('converts string values unchanged', () => {
        expect(normalizeClaimValue('hello')).toBe('hello');
    });

    test('preserves numbers as-is', () => {
        expect(normalizeClaimValue(12345)).toBe(12345);
        expect(normalizeClaimValue(0)).toBe(0);
        expect(normalizeClaimValue(-42)).toBe(-42);
    });

    test('preserves booleans as-is', () => {
        expect(normalizeClaimValue(true)).toBe(true);
        expect(normalizeClaimValue(false)).toBe(false);
    });

    test('converts null to empty string', () => {
        expect(normalizeClaimValue(null)).toBe('');
    });

    test('converts undefined to empty string', () => {
        expect(normalizeClaimValue(undefined)).toBe('');
    });

    test('converts plain objects to JSON string', () => {
        const obj = {key: 'value', nested: {a: 1}};
        expect(normalizeClaimValue(obj)).toBe('{"key":"value","nested":{"a":1}}');
    });

    test('preserves arrays as arrays', () => {
        const arr = ['a', 'b', 'c'];
        expect(normalizeClaimValue(arr)).toEqual(['a', 'b', 'c']);
        expect(Array.isArray(normalizeClaimValue(arr))).toBe(true);
    });
});

describe('parseQueryParams', () => {
    test('parses simple query params', () => {
        expect(parseQueryParams('_id=123&status=active')).toEqual({
            '_id': ['123'],
            'status': ['active']
        });
    });

    test('handles multiple values for same key', () => {
        expect(parseQueryParams('status=active&status=pending')).toEqual({
            'status': ['active', 'pending']
        });
    });

    test('handles empty query string', () => {
        expect(parseQueryParams('')).toEqual({});
    });

    test('handles null/undefined', () => {
        expect(parseQueryParams(null)).toEqual({});
        expect(parseQueryParams(undefined)).toEqual({});
    });

    test('decodes URL-encoded values', () => {
        expect(parseQueryParams('name=John%20Doe&filter=type%3DPatient')).toEqual({
            'name': ['John Doe'],
            'filter': ['type=Patient']
        });
    });

    test('throws on malformed percent-encoding', () => {
        // Invalid percent-encoding should throw URIError for explicit 400 response
        expect(() => parseQueryParams('patient=%ZZ&valid=ok')).toThrow(URIError);
    });
});

describe('buildPDPRequest', () => {
    const originalEnv = process.env;

    beforeEach(() => {
        process.env = {
            ...originalEnv,
            DATA_HOLDER_ORGANIZATION_URA: '00000666',
            DATA_HOLDER_FACILITY_TYPE: 'Z3'
        };
    });

    afterEach(() => {
        process.env = originalEnv;
    });

    test('builds complete PDPInput structure', () => {
        // Introspection claims use the exact names the PDP expects (defined by PD constraint ids)
        const introspection = {
            active: true,
            client_id: 'did:nuts:client123',
            scope: 'bgz eoverdracht',
            subject_id: 'practitioner-456',
            subject_role: '01.015',
            subject_organization_id: '00000020',
            subject_organization: 'Requesting Hospital',
            subject_facility_type: 'Z3'
        };
        const request = createMockRequest({
            variables: {
                request_uri: '/fhir/Patient/patient-123?_include=Patient:organization',
                request_method: 'GET'
            }
        });

        const result = buildPDPRequest(introspection, request);

        expect(result).toEqual({
            input: {
                subject: {
                    active: true,
                    client_id: 'did:nuts:client123',
                    scope: 'bgz eoverdracht',
                    subject_id: 'practitioner-456',
                    subject_role: '01.015',
                    subject_organization_id: '00000020',
                    subject_organization: 'Requesting Hospital',
                    subject_facility_type: 'Z3'
                },
                request: {
                    method: 'GET',
                    protocol: 'HTTP/1.1',
                    path: '/Patient/patient-123', // /fhir/ prefix is stripped
                    query_params: {
                        '_include': ['Patient:organization']
                    },
                    header: {},
                    body: ''
                },
                context: {
                    connection_type_code: "hl7-fhir-rest",
                    data_holder_organization_id: '00000666',
                    data_holder_facility_type: 'Z3',
                    patient_bsn: ''
                }
            }
        });
    });

    test('uses request.uri as fallback', () => {
        const introspection = {active: true, client_id: 'test'};
        const request = createMockRequest({
            uri: '/fhir/Patient/123',
            method: 'GET',
            variables: {}
        });

        const result = buildPDPRequest(introspection, request);

        // /fhir/ prefix is stripped from path
        expect(result.input.request.path).toBe('/Patient/123');
    });

    test('prefers request.variables over request object', () => {
        const introspection = {active: true, client_id: 'test'};
        const request = createMockRequest({
            method: 'GET',
            uri: '/fallback',
            variables: {
                request_uri: '/fhir/Patient',
                request_method: 'POST'
            }
        });

        const result = buildPDPRequest(introspection, request);

        expect(result.input.request.method).toBe('POST');
        // /fhir/ prefix is stripped from path
        expect(result.input.request.path).toBe('/Patient');
    });

    test('passes request body for POST search', () => {
        const introspection = {active: true, client_id: 'test'};
        const searchBody = 'patient=Patient/123&_include=Observation:subject';
        const request = createMockRequest({
            uri: '/fhir/Observation/_search',
            method: 'POST',
            requestText: searchBody
        });

        const result = buildPDPRequest(introspection, request);

        expect(result.input.request.method).toBe('POST');
        expect(result.input.request.path).toBe('/Observation/_search');
        expect(result.input.request.body).toBe(searchBody);
    });
});

describe('validateDPoP', () => {
    test('returns valid when no cnf claim present', async () => {
        const request = createMockRequest();
        const introspection = {active: true};

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(true);
    });

    test('returns valid when cnf has no jkt', async () => {
        const request = createMockRequest();
        const introspection = {active: true, cnf: {}};

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(true);
    });

    test('returns invalid when cnf.jkt present but DPoP header missing', async () => {
        const request = createMockRequest({
            headersIn: {'Authorization': 'DPoP token123'}
        });
        const introspection = {
            active: true,
            cnf: {jkt: 'thumbprint123'}
        };

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(false);
        expect(result.reason).toBe('DPoP header required but missing');
    });

    test('calls Nuts node validation endpoint with correct payload', async () => {
        const mockSubrequest = jest.fn().mockResolvedValue(
            createMockSubrequestResponse(200, {valid: true})
        );
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP token123',
                'DPoP': 'dpop-proof-jwt',
                'Host': 'example.com'
            },
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });
        const introspection = {
            active: true,
            cnf: {jkt: 'thumbprint123'}
        };

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(true);
        expect(mockSubrequest).toHaveBeenCalledWith('/_dpop_validate', {
            method: 'POST',
            body: JSON.stringify({
                dpop_proof: 'dpop-proof-jwt',
                method: 'GET',
                thumbprint: 'thumbprint123',
                token: 'token123',
                url: 'https://example.com/fhir/Patient/123'
            })
        });
    });

    test('returns invalid when validation endpoint returns non-200', async () => {
        const mockSubrequest = jest.fn().mockResolvedValue(
            createMockSubrequestResponse(400, 'invalid_dpop')
        );
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP token123',
                'DPoP': 'dpop-proof-jwt',
                'Host': 'example.com'
            },
            variables: {
                request_uri: '/fhir/Patient',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });
        const introspection = {
            active: true,
            cnf: {jkt: 'thumbprint123'}
        };

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(false);
        expect(result.reason).toContain('DPoP validation returned 400');
    });

    test('returns invalid when validation response has valid: false', async () => {
        const mockSubrequest = jest.fn().mockResolvedValue(
            createMockSubrequestResponse(200, {valid: false, reason: 'expired'})
        );
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP token123',
                'DPoP': 'dpop-proof-jwt',
                'Host': 'example.com'
            },
            variables: {
                request_uri: '/fhir/Patient',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });
        const introspection = {
            active: true,
            cnf: {jkt: 'thumbprint123'}
        };

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(false);
        expect(result.reason).toBe('expired');
    });

    test('handles subrequest errors gracefully', async () => {
        const mockSubrequest = jest.fn().mockRejectedValue(new Error('Network error'));
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP token123',
                'DPoP': 'dpop-proof-jwt',
                'Host': 'example.com'
            },
            variables: {
                request_uri: '/fhir/Patient',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });
        const introspection = {
            active: true,
            cnf: {jkt: 'thumbprint123'}
        };

        const result = await validateDPoP(request, introspection);

        expect(result.valid).toBe(false);
        expect(result.reason).toContain('DPoP validation error');
    });
});

describe('checkAuthorization integration', () => {
    const {checkAuthorization} = authorize;
    const originalEnv = process.env;

    beforeEach(() => {
        process.env = {
            ...originalEnv,
            DATA_HOLDER_ORGANIZATION_URA: '00000666',
            DATA_HOLDER_FACILITY_TYPE: 'Z3',
            NUTS_NODE_HOST: 'nuts-node',
            NUTS_NODE_INTERNAL_PORT: '8081',
            KNOOPPUNT_PDP_HOST: 'knooppunt',
            KNOOPPUNT_PDP_PORT: '8081'
        };
    });

    afterEach(() => {
        process.env = originalEnv;
    });

    test('returns 401 when no Authorization header', async () => {
        const request = createMockRequest();

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(401);
        expect(request.error).toHaveBeenCalledWith('Missing or invalid Authorization header');
    });

    test('returns 401 when token introspection returns inactive', async () => {
        const mockSubrequest = jest.fn().mockResolvedValue(
            createMockSubrequestResponse(200, {active: false})
        );
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer test-token'},
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(401);
        expect(request.error).toHaveBeenCalledWith('Token is not active');
    });

    test('returns 502 when introspection fails', async () => {
        const mockSubrequest = jest.fn().mockResolvedValue(
            createMockSubrequestResponse(500, {})
        );
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer test-token'},
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(502);
    });

    test('returns 200 when PDP allows', async () => {
        const mockSubrequest = jest.fn()
            // First call: introspection
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                active: true,
                client_id: 'did:nuts:test',
                scope: 'bgz'
            }))
            // Second call: PDP
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                allow: true
            }));
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer test-token'},
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(200);
        expect(request.log).toHaveBeenCalledWith('Access ALLOWED by PDP');
    });

    test('returns 403 when PDP denies', async () => {
        const mockSubrequest = jest.fn()
            // First call: introspection
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                active: true,
                client_id: 'did:nuts:test',
                scope: 'bgz'
            }))
            // Second call: PDP
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                allow: false,
                results: {
                    "bgz": {
                        allow: false,
                        reasons: [{code: 'not_allowed', description: 'No consent'}]
                    }
                }
            }));
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer test-token'},
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(403);
        expect(request.warn).toHaveBeenCalled();
    });

    test('returns 502 when PDP response is malformed', async () => {
        const mockSubrequest = jest.fn()
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                active: true,
                client_id: 'did:nuts:test',
                scope: 'bgz'
            }))
            // PDP returns malformed response (allow is string instead of boolean)
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                result: {allow: 'true'}
            }));
        const request = createMockRequest({
            headersIn: {'Authorization': 'Bearer test-token'},
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(502);
        expect(request.error).toHaveBeenCalledWith('Malformed PDP response: missing allow boolean');
    });

    test('returns 401 when DPoP validation fails', async () => {
        const mockSubrequest = jest.fn()
            // First call: introspection (token has cnf.jkt)
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                active: true,
                client_id: 'did:nuts:test',
                cnf: {jkt: 'thumbprint123'}
            }))
            // Second call: DPoP validation fails
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                valid: false,
                reason: 'invalid signature'
            }));
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP test-token',
                'DPoP': 'dpop-proof',
                'Host': 'example.com'
            },
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(401);
        expect(request.error).toHaveBeenCalledWith('DPoP validation failed: invalid signature');
    });

    test('returns 401 when DPoP scheme used without DPoP header (RFC 9449)', async () => {
        // RFC 9449: If using DPoP authorization scheme, DPoP header is required
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'DPoP test-token'
                // Note: No 'DPoP' header
            }
        });

        await checkAuthorization(request);

        expect(request.return).toHaveBeenCalledWith(401);
        expect(request.error).toHaveBeenCalledWith('DPoP authorization scheme requires DPoP header');
    });

    test('blocks DPoP-bound token presented as Bearer (attack scenario)', async () => {
        // Security model test: An attacker who steals a DPoP-bound token
        // cannot use it by simply changing Authorization scheme to Bearer.
        // The token's cnf.jkt binding is intrinsic and revealed by introspection.

        const mockSubrequest = jest.fn()
            // Introspection returns cnf.jkt even when token presented as Bearer
            .mockResolvedValueOnce(createMockSubrequestResponse(200, {
                active: true,
                client_id: 'did:nuts:victim',
                scope: 'bgz',
                cnf: {jkt: 'victim-key-thumbprint'}  // Token is DPoP-bound
            }));

        // Attacker presents stolen token as Bearer (no DPoP header)
        const request = createMockRequest({
            headersIn: {
                'Authorization': 'Bearer stolen-dpop-bound-token'
                // Note: No 'DPoP' header - attacker doesn't have victim's private key
            },
            variables: {
                request_uri: '/fhir/Patient/123',
                request_method: 'GET'
            },
            subrequest: mockSubrequest
        });

        await checkAuthorization(request);

        // Request MUST be blocked - DPoP proof required for tokens with cnf.jkt
        expect(request.return).toHaveBeenCalledWith(401);
        expect(request.error).toHaveBeenCalledWith('DPoP validation failed: DPoP header required but missing');
    });
});
