import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../AuthProvider';
import { patientApi } from '../api/patientApi';
import { medicationApi } from '../api/medicationApi';
import { nviApi } from '../api/nviApi';

function PatientPage() {
  const { patientId } = useParams();
  const navigate = useNavigate();
  const { isAuthenticated, logout, user } = useAuth();

  const [patient, setPatient] = useState(null);
  const [medicationRequests, setMedicationRequests] = useState([]);
  const [medicationDispenses, setMedicationDispenses] = useState([]);
  const [careNetworkOrganizations, setCareNetworkOrganizations] = useState([]);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [medicationRequestsLoading, setMedicationRequestsLoading] = useState(true);
  const [medicationDispensesLoading, setMedicationDispensesLoading] = useState(true);
  const [medicationRequestsError, setMedicationRequestsError] = useState(null);
  const [medicationDispensesError, setMedicationDispensesError] = useState(null);
  const [careNetworkLoading, setCareNetworkLoading] = useState(true);
  const [careNetworkError, setCareNetworkError] = useState(null);

  useEffect(() => {
    if (isAuthenticated && patientId) {
      loadPatientData();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, patientId]);

  const loadPatientData = async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch patient details
      const response = await fetch(`${require('../config').config.fhirBaseURL}/Patient/${patientId}`, {
        method: 'GET',
        headers: {
          'Accept': 'application/fhir+json',
          'Content-Type': 'application/fhir+json',
          'Cache-Control': 'no-cache',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch patient: ${response.statusText}`);
      }

      const patientData = await response.json();
      setPatient(patientData);

      // Load medication data in parallel
      loadMedicationRequests(patientId);
      loadMedicationDispenses(patientId);

      // Load care network organizations
      const patientBSN = patientApi.getByBSN(patientData);
      if (patientBSN) {
        loadCareNetwork(patientBSN);
      } else {
        setCareNetworkLoading(false);
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const loadMedicationRequests = async (patientId) => {
    setMedicationRequestsLoading(true);
    setMedicationRequestsError(null);
    try {
      const data = await medicationApi.getMedicationRequests(patientId);
      setMedicationRequests(data);
    } catch (err) {
      setMedicationRequestsError(err.message);
    } finally {
      setMedicationRequestsLoading(false);
    }
  };

  const loadMedicationDispenses = async (patientId) => {
    setMedicationDispensesLoading(true);
    setMedicationDispensesError(null);
    try {
      const data = await medicationApi.getMedicationDispenses(patientId);
      setMedicationDispenses(data);
    } catch (err) {
      setMedicationDispensesError(err.message);
    } finally {
      setMedicationDispensesLoading(false);
    }
  };

  const loadCareNetwork = async (bsn) => {
    setCareNetworkLoading(true);
    setCareNetworkError(null);
    try {
      // Extract abonnee_nummer from the OIDC token (stored in 'sub' claim)
      const abonneeNummer = user?.profile?.sub;
      const organizations = await nviApi.searchOrganizationsByPatient(bsn, abonneeNummer);
      setCareNetworkOrganizations(organizations);
    } catch (err) {
      setCareNetworkError(err.message);
    } finally {
      setCareNetworkLoading(false);
    }
  };

  const formatDate = (dateString) => {
    if (!dateString) return '-';
    try {
      return new Date(dateString).toLocaleDateString('nl-NL');
    } catch {
      return dateString;
    }
  };

  const getGenderIcon = (gender) => {
    switch (gender?.toLowerCase()) {
      case 'male': return '♂️';
      case 'female': return '♀️';
      default: return '⚪';
    }
  };

  const calculateAge = (birthDate) => {
    if (!birthDate) return '-';
    try {
      const today = new Date();
      const birth = new Date(birthDate);
      const age = Math.floor((today - birth) / (365.25 * 24 * 60 * 60 * 1000));
      return age;
    } catch {
      return '-';
    }
  };

  if (!isAuthenticated) {
    return (
      <div className="app-container">
        <div className="loading">Please log in to view patient details.</div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Loading patient information...</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="loading-container">
            <div className="spinner"></div>
            <p>Loading patient data...</p>
          </div>
        </main>
      </div>
    );
  }

  if (error) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Error loading patient</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="error-container">
            <div className="error-message">
              <strong>Error loading patient</strong>
              <p>{error}</p>
              <button onClick={() => navigate('/patients')} className="button" style={{ marginTop: '15px' }}>
                ← Back to Patients
              </button>
            </div>
          </div>
        </main>
      </div>
    );
  }

  if (!patient) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Patient not found</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="error-container">
            <div className="error-message">
              <strong>Patient not found</strong>
              <p>The patient with ID "{patientId}" could not be found.</p>
              <button onClick={() => navigate('/patients')} className="button" style={{ marginTop: '15px' }}>
                ← Back to Patients
              </button>
            </div>
          </div>
        </main>
      </div>
    );
  }

  const patientName = patientApi.formatName(patient);
  const patientBSN = patientApi.getByBSN(patient);
  const patientGender = patientApi.formatGender(patient);
  const patientBirthDate = patientApi.formatBirthDate(patient);
  const patientAge = calculateAge(patientBirthDate);

  return (
    <div className="app-container">
      <header className="header">
        <div className="header-content">
          <div>
            <h1>🏥 Demo EHR - Patient Details</h1>
            <div className="header-subtitle">{patientName}</div>
          </div>
          <button onClick={logout} className="button button-secondary">
            Logout
          </button>
        </div>
      </header>

      <main className="main-content">
        <div style={{ marginBottom: '20px' }}>
          <button onClick={() => navigate('/patients')} className="button button-secondary">
            ← Back to Patients
          </button>
        </div>

        <div className="patient-split-layout">
          {/* Left side - Patient Details */}
          <div className="patient-details-section">
            <div className="card">
              <h2 style={{ marginTop: 0 }}>Patient Information</h2>

              <div className="patient-detail-grid">
                <div className="patient-detail-item">
                  <div className="patient-detail-label">Name</div>
                  <div className="patient-detail-value">{patientName}</div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">BSN</div>
                  <div className="patient-detail-value">
                    {patientBSN ? (
                      <span className="bsn-badge">{patientBSN}</span>
                    ) : (
                      <span className="text-muted">-</span>
                    )}
                  </div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Gender</div>
                  <div className="patient-detail-value">
                    <span className="gender-badge">
                      {getGenderIcon(patientGender)} {patientGender}
                    </span>
                  </div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Birth Date</div>
                  <div className="patient-detail-value">{formatDate(patientBirthDate)}</div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Age</div>
                  <div className="patient-detail-value">{patientAge} years</div>
                </div>
              </div>

              {patient.address && patient.address.length > 0 && (
                <>
                  <h3 style={{ marginTop: '30px', marginBottom: '15px' }}>Address</h3>
                  {patient.address.map((addr, idx) => (
                    <div key={idx} className="patient-detail-grid">
                      {addr.line && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Street</div>
                          <div className="patient-detail-value">{addr.line.join(', ')}</div>
                        </div>
                      )}
                      {addr.city && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">City</div>
                          <div className="patient-detail-value">{addr.city}</div>
                        </div>
                      )}
                      {addr.postalCode && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Postal Code</div>
                          <div className="patient-detail-value">{addr.postalCode}</div>
                        </div>
                      )}
                      {addr.country && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Country</div>
                          <div className="patient-detail-value">{addr.country}</div>
                        </div>
                      )}
                    </div>
                  ))}
                </>
              )}

              {patient.telecom && patient.telecom.length > 0 && (
                <>
                  <h3 style={{ marginTop: '30px', marginBottom: '15px' }}>Contact</h3>
                  <div className="patient-detail-grid">
                    {patient.telecom.map((contact, idx) => (
                      <div key={idx} className="patient-detail-item">
                        <div className="patient-detail-label">{contact.system || 'Contact'}</div>
                        <div className="patient-detail-value">{contact.value}</div>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>

            {/* Care Network Card */}
            <div className="card" style={{ marginTop: '20px' }}>
              <h2 style={{ marginTop: 0 }}>🏥 Care Network</h2>
              <p style={{ color: '#666', fontSize: '14px', marginBottom: '15px' }}>
                Organizations that have data for this patient
              </p>

              {careNetworkLoading ? (
                <div className="loading-container" style={{ padding: '30px' }}>
                  <div className="spinner"></div>
                  <p>Loading care network...</p>
                </div>
              ) : careNetworkError ? (
                <div className="error-message">
                  <strong>Error loading care network</strong>
                  <p>{careNetworkError}</p>
                </div>
              ) : !patientBSN ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No BSN available for this patient</p>
                </div>
              ) : careNetworkOrganizations.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No organizations found in care network</p>
                </div>
              ) : (
                <div className="care-network-list">
                  {careNetworkOrganizations.map((org, idx) => (
                    <div key={org.ura || idx} className="care-network-item">
                      <div className="care-network-item-header">
                        <span className="care-network-org-name">{org.name}</span>
                        <span className="care-network-doc-count">
                          {org.documentCount} document{org.documentCount !== 1 ? 's' : ''}
                        </span>
                      </div>
                      <div className="care-network-ura">
                        <span className="care-network-ura-label">URA:</span>
                        <span className="care-network-ura-value">{nviApi.formatURA(org.ura)}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Right side - FHIR Resources */}
          <div className="patient-resources-section">
            {/* Medication Requests */}
            <div className="card" style={{ marginBottom: '20px' }}>
              <h2 style={{ marginTop: 0 }}>💊 Medication Requests</h2>

              {medicationRequestsLoading ? (
                <div className="loading-container">
                  <div className="spinner"></div>
                  <p>Loading medication requests...</p>
                </div>
              ) : medicationRequestsError ? (
                <div className="error-message">
                  <strong>Error loading medication requests</strong>
                  <p>{medicationRequestsError}</p>
                </div>
              ) : medicationRequests.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No medication requests found</p>
                </div>
              ) : (
                <div className="resource-table-container">
                  <table className="resource-table">
                    <thead>
                      <tr>
                        <th>Medication</th>
                        <th>Dosage</th>
                        <th>Status</th>
                        <th>Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      {medicationRequests.map((req) => (
                        <tr key={req.id}>
                          <td className="medication-name">{medicationApi.formatMedication(req)}</td>
                          <td>{medicationApi.formatDosage(req)}</td>
                          <td>
                            <span className={`status-badge ${medicationApi.getStatusClass(req.status)}`}>
                              {req.status || 'unknown'}
                            </span>
                          </td>
                          <td>{medicationApi.formatDate(req.authoredOn)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* Medication Dispenses */}
            <div className="card">
              <h2 style={{ marginTop: 0 }}>💊 Medication Dispenses</h2>

              {medicationDispensesLoading ? (
                <div className="loading-container">
                  <div className="spinner"></div>
                  <p>Loading medication dispenses...</p>
                </div>
              ) : medicationDispensesError ? (
                <div className="error-message">
                  <strong>Error loading medication dispenses</strong>
                  <p>{medicationDispensesError}</p>
                </div>
              ) : medicationDispenses.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No medication dispenses found</p>
                </div>
              ) : (
                <div className="resource-table-container">
                  <table className="resource-table">
                    <thead>
                      <tr>
                        <th>Medication</th>
                        <th>Dosage</th>
                        <th>Status</th>
                        <th>Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      {medicationDispenses.map((disp) => (
                        <tr key={disp.id}>
                          <td className="medication-name">{medicationApi.formatMedication(disp)}</td>
                          <td>{medicationApi.formatDosage(disp)}</td>
                          <td>
                            <span className={`status-badge ${medicationApi.getStatusClass(disp.status)}`}>
                              {disp.status || 'unknown'}
                            </span>
                          </td>
                          <td>{medicationApi.formatDate(disp.whenHandedOver || disp.whenPrepared)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}

export default PatientPage;

