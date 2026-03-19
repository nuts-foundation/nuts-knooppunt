// Local development configuration.
// In Docker, mount your own copy of this file over /app/public/env-config.js.
window._env_ = {
  AUTH_BASE_URL: 'http://localhost:8090',
  FHIR_BASE_URL: 'http://localhost:7050/fhir/sunflower-patients',
  FHIR_STU3_BASE_URL: 'http://localhost:7060/fhir',
  FHIR_MCSD_QUERY_BASE_URL: 'http://localhost:8080/fhir',
  ORGANIZATION_URA: '',
};

