// Shared allowlist for demo-ehr's API proxies. Used by both `server.js`
// (production) and `src/setupProxy.js` (CRA dev server) so dev and prod enforce
// the same operations against each upstream.
//
// Each entry: { method, path } where path is a RegExp matched against the
// request path AFTER the /api/<prefix> stripping.

const FHIR_R4 = [
  // Patient
  { method: 'GET', path: /^\/Patient(\?.*)?$/ },
  { method: 'GET', path: /^\/Patient\/[^/]+$/ },
  { method: 'POST', path: /^\/Patient$/ },
  { method: 'PUT', path: /^\/Patient\/[^/]+$/ },
  { method: 'DELETE', path: /^\/Patient\/[^/]+$/ },
  // Medication
  { method: 'GET', path: /^\/MedicationRequest(\?.*)?$/ },
  { method: 'GET', path: /^\/MedicationDispense(\?.*)?$/ },
  // Practitioner
  { method: 'GET', path: /^\/Practitioner(\?.*)?$/ },
  { method: 'POST', path: /^\/Practitioner$/ },
  // Consent
  { method: 'GET', path: /^\/Consent(\?.*)?$/ },
  { method: 'POST', path: /^\/Consent$/ },
  { method: 'PUT', path: /^\/Consent\/[^/]+$/ },
  { method: 'DELETE', path: /^\/Consent\/[^/]+$/ },
];

const FHIR_STU3 = [
  // Bundle POST at root (used by bgzApi.generateBGZ)
  { method: 'POST', path: /^\/?$/ },
  // Task
  { method: 'GET', path: /^\/Task(\?.*)?$/ },
  { method: 'POST', path: /^\/Task$/ },
  { method: 'GET', path: /^\/Task\/[^/]+$/ },
  { method: 'DELETE', path: /^\/Task\/[^/]+$/ },
  // Patient
  { method: 'GET', path: /^\/Patient(\?.*)?$/ },
  { method: 'POST', path: /^\/Patient$/ },
  { method: 'DELETE', path: /^\/Patient\/[^/]+$/ },
  // BGZ visualization queries (read-only)
  { method: 'GET', path: /^\/Coverage(\?.*)?$/ },
  { method: 'GET', path: /^\/Observation(\?.*)?$/ },
  { method: 'GET', path: /^\/Condition(\?.*)?$/ },
  { method: 'GET', path: /^\/NutritionOrder(\?.*)?$/ },
  { method: 'GET', path: /^\/Flag(\?.*)?$/ },
  { method: 'GET', path: /^\/AllergyIntolerance(\?.*)?$/ },
  { method: 'GET', path: /^\/MedicationStatement(\?.*)?$/ },
  { method: 'GET', path: /^\/MedicationRequest(\?.*)?$/ },
  { method: 'GET', path: /^\/DeviceUseStatement(\?.*)?$/ },
  { method: 'GET', path: /^\/Immunization(\?.*)?$/ },
  { method: 'GET', path: /^\/Procedure(\?.*)?$/ },
  { method: 'GET', path: /^\/Encounter(\?.*)?$/ },
  { method: 'GET', path: /^\/ImmunizationRecommendation(\?.*)?$/ },
  { method: 'GET', path: /^\/DeviceRequest(\?.*)?$/ },
  { method: 'GET', path: /^\/Appointment(\?.*)?$/ },
  // Generic instance delete used by PatientPage's BGZ cleanup
  { method: 'DELETE', path: /^\/[A-Za-z]+\/[^/]+$/ },
];

const MCSD = [
  { method: 'GET', path: /^\/Organization(\?.*)?$/ },
  { method: 'GET', path: /^\/Organization\/[^/]+$/ },
  { method: 'GET', path: /^\/HealthcareService(\?.*)?$/ },
  { method: 'GET', path: /^\/Endpoint\/[^/]+$/ },
];

const KNOOPPUNT = [
  { method: 'GET', path: /^\/nvi\/DocumentReference(\?.*)?$/ },
];

// Dynamic proxy is only used to POST a Task to a peer's notification endpoint.
const DYNAMIC = [
  { method: 'POST', path: /^\/Task$/ },
];

function makeGate(name, rules) {
  return function (req, res, next) {
    const ok = rules.some((r) => r.method === req.method && r.path.test(req.url));
    if (!ok) {
      console.warn(`[${name}] denied ${req.method} ${req.url}`);
      return res.status(403).json({
        error: `${req.method} ${req.url} not permitted by ${name} allowlist`,
      });
    }
    next();
  };
}

module.exports = {
  FHIR_R4,
  FHIR_STU3,
  MCSD,
  KNOOPPUNT,
  DYNAMIC,
  makeGate,
};
