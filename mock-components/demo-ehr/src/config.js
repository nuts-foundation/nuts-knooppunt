// Public upstream URLs. Used when embedding values into FHIR Reference fields
// of resources sent to other parties (those URLs must be publicly resolvable,
// not point at this app's local proxy).
import { baseUrl, runtimeConfig } from './runtimeConfig';

export const config = {
    mcsdQueryBaseURL: runtimeConfig.mcsdQueryBaseURL,
    fhirBaseURL: runtimeConfig.fhirBaseURL,
    fhirStu3BaseURL: runtimeConfig.fhirStu3BaseURL,
    organizationURA: runtimeConfig.organizationURA,
};

// Relative paths the SPA uses for its own fetch() calls. The backend (server.js
// in production, setupProxy.js under `npm start`) proxies these to the
// configured upstreams and enforces an allowlist of operations.
export const apiBase = {
    fhir: `${baseUrl}/api/fhir`,
    fhirStu3: `${baseUrl}/api/fhir-stu3`,
    mcsd: `${baseUrl}/api/mcsd`,
    knooppunt: `${baseUrl}/api/knooppunt`,
    dynamicProxy: `${baseUrl}/api/dynamic-proxy`,
};
