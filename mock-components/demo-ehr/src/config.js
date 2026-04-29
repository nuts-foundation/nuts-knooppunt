// Absolute upstream URLs. Kept available so the frontend can embed them as
// FHIR Reference values in resources sent to other parties (those URLs must be
// publicly resolvable, not point at this app's local proxy).
export const config = {
    mcsdQueryBaseURL: process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL,
    fhirBaseURL: process.env.REACT_APP_FHIR_BASE_URL,
    fhirStu3BaseURL: process.env.REACT_APP_FHIR_STU3_BASE_URL,
    organizationURA: process.env.REACT_APP_ORGANIZATION_URA,
};

// Relative paths the SPA uses for its own fetch() calls. The backend (server.js
// in production, setupProxy.js under `npm start`) proxies these to the
// configured upstreams and enforces an allowlist of operations. PUBLIC_URL is
// baked at build time by CRA and is empty when serving at the host root.
const publicUrl = process.env.PUBLIC_URL || '';
export const apiBase = {
    fhir: `${publicUrl}/api/fhir`,
    fhirStu3: `${publicUrl}/api/fhir-stu3`,
    mcsd: `${publicUrl}/api/mcsd`,
    knooppunt: `${publicUrl}/api/knooppunt`,
    dynamicProxy: `${publicUrl}/api/dynamic-proxy`,
};
