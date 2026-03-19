const env = window._env_ || {};

export const config = {
    mcsdQueryBaseURL: env.FHIR_MCSD_QUERY_BASE_URL || process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL,
    fhirBaseURL: env.FHIR_BASE_URL || process.env.REACT_APP_FHIR_BASE_URL,
    fhirStu3BaseURL: env.FHIR_STU3_BASE_URL || process.env.REACT_APP_FHIR_STU3_BASE_URL,
    organizationURA: env.ORGANIZATION_URA || process.env.REACT_APP_ORGANIZATION_URA,
}