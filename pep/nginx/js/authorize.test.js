/**
 * Unit tests for authorize.js
 * Run with: npm test (from pep/nginx/js directory)
 */

// Import functions from authorize.js
import authorize from './authorize.js';
import { jest } from '@jest/globals';

const { extractBearerToken, parsePathArray, extractFhirContext, buildOpaRequest } = authorize;

// Mock NGINX request object
function createMockRequest(overrides = {}) {
    return {
        headersIn: {},
        variables: {
            request_uri: '',
            request_method: 'GET'
        },
        method: 'GET',
        requestText: '',
        log: jest.fn(),
        error: jest.fn(),
        warn: jest.fn(),
        return: jest.fn(),
        subrequest: jest.fn(),
        ...overrides
    };
}

describe('extractBearerToken', () => {
    test('extracts valid bearer token', () => {
        const request = createMockRequest({
            headersIn: { 'Authorization': 'Bearer abc123' }
        });
        expect(extractBearerToken(request)).toBe('abc123');
    });

    test('returns null when Authorization header is missing', () => {
        const request = createMockRequest();
        expect(extractBearerToken(request)).toBeNull();
    });

    test('returns null for invalid format', () => {
        const request = createMockRequest({
            headersIn: { 'Authorization': 'Basic abc123' }
        });
        expect(extractBearerToken(request)).toBeNull();
    });

    test('handles case-insensitive Bearer keyword', () => {
        const request = createMockRequest({
            headersIn: { 'Authorization': 'BEARER token123' }
        });
        expect(extractBearerToken(request)).toBe('token123');
    });
});

describe('parsePathArray', () => {
    test('parses FHIR resource path', () => {
        expect(parsePathArray('/fhir/Patient/patient-123')).toEqual(['fhir', 'Patient', 'patient-123']);
    });

    test('parses path without leading slash', () => {
        expect(parsePathArray('fhir/Patient/123')).toEqual(['fhir', 'Patient', '123']);
    });

    test('removes query string', () => {
        expect(parsePathArray('/fhir/Patient?_id=123')).toEqual(['fhir', 'Patient']);
    });

    test('handles empty path', () => {
        expect(parsePathArray('')).toEqual([]);
    });

    test('handles root path', () => {
        expect(parsePathArray('/')).toEqual([]);
    });
});

describe('extractFhirContext', () => {
    test('extracts resource type and ID', () => {
        const result = extractFhirContext('/fhir/Patient/patient-123');
        expect(result.resourceType).toBe('Patient');
        expect(result.resourceId).toBe('patient-123');
    });

    test('extracts resource type without ID', () => {
        const result = extractFhirContext('/fhir/Observation');
        expect(result.resourceType).toBe('Observation');
        expect(result.resourceId).toBeNull();
    });

    test('handles search operations', () => {
        const result = extractFhirContext('/fhir/Patient/_search');
        expect(result.resourceType).toBe('Patient');
        expect(result.resourceId).toBe('_search');
    });

    test('returns null for non-FHIR paths', () => {
        const result = extractFhirContext('/health');
        expect(result.resourceType).toBeNull();
        expect(result.resourceId).toBeNull();
    });
});

describe('buildOpaRequest', () => {
    // Save and restore process.env
    const originalEnv = process.env;

    beforeEach(() => {
        process.env = {
            ...originalEnv,
            DATA_HOLDER_ORGANIZATION_URA: '00000666',
            DATA_HOLDER_FACILITY_TYPE: 'Z3',
            REQUESTING_FACILITY_TYPE: 'Z3',
            PURPOSE_OF_USE: 'TREAT',
            EVENT_CODE: 'GGC002'
        };
    });

    afterEach(() => {
        process.env = originalEnv;
    });

    test('builds complete OPA request', () => {
        const tokenClaims = {
            sub: 'user123',
            requesting_organization_ura: '00000020',
            requesting_uzi_role_code: '01.015',
            requesting_practitioner_identifier: '123456789',
            patient_bsn: '900186021'
        };
        const fhirContext = {
            resourceType: 'Patient',
            resourceId: 'patient-123'
        };
        const request = createMockRequest({
            variables: {
                request_uri: '/fhir/Patient/patient-123',
                request_method: 'GET'
            }
        });

        const result = buildOpaRequest(tokenClaims, fhirContext, request);

        expect(result).toEqual({
            input: {
                method: 'GET',
                path: ['fhir', 'Patient', 'patient-123'],
                requesting_organization_ura: '00000020',
                requesting_uzi_role_code: '01.015',
                requesting_practitioner_identifier: '123456789',
                requesting_facility_type: 'Z3',
                data_holder_organization_ura: '00000666',
                data_holder_facility_type: 'Z3',
                patient_bsn: '900186021',
                resource_type: 'Patient',
                resource_id: 'patient-123',
                purpose_of_use: 'TREAT',
                event_code: 'GGC002'
            }
        });
    });

    test('handles missing optional fields', () => {
        const tokenClaims = {
            sub: 'user123'
        };
        const fhirContext = {
            resourceType: 'Observation',
            resourceId: null
        };
        const request = createMockRequest({
            variables: {
                request_uri: '/fhir/Observation',
                request_method: 'POST'
            }
        });

        const result = buildOpaRequest(tokenClaims, fhirContext, request);

        expect(result.input.requesting_practitioner_identifier).toBeNull();
        expect(result.input.requesting_organization_ura).toBeNull();
        expect(result.input.patient_bsn).toBeNull();
        expect(result.input.resource_id).toBeNull();
    });

    test('prefers request.variables.request_method (NGINX canonical source) over request.method (NJS convenience property)', () => {
        const tokenClaims = { sub: 'user', role: 'practitioner' };
        const fhirContext = { resourceType: 'Patient', resourceId: null };
        const request = createMockRequest({
            method: 'GET',
            variables: {
                request_uri: '/fhir/Patient',
                request_method: 'POST'  // Should override request.method
            }
        });

        const result = buildOpaRequest(tokenClaims, fhirContext, request);

        expect(result.input.method).toBe('POST');
    });
});
