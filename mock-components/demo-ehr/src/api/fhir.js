// Headers for GET/DELETE requests (no Content-Type)
export const headers = {
    'Accept': 'application/fhir+json',
    'Cache-Control': 'no-cache',
};

// Headers for POST/PUT/PATCH requests (with Content-Type)
export const headersWithContentType = {
    'Accept': 'application/fhir+json',
    'Content-Type': 'application/fhir+json',
    'Cache-Control': 'no-cache',
};