export const config = {
    mcsdQueryBaseURL: process.env.REACT_APP_FHIR_MCSD_QUERY_BASE_URL,
    fhirBaseURL: process.env.REACT_APP_FHIR_BASE_URL,
    knooppuntURL: process.env.REACT_APP_KNOOPPUNT_URL || 'http://localhost:8081',
    organizationURA: process.env.REACT_APP_ORGANIZATION_URA,
}