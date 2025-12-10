import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../AuthProvider';
import { patientApi } from '../api/patientApi';
import {config} from "../config";

function PatientContextLaunchPage() {
  const { patientId } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { isAuthenticated, logout } = useAuth();

  const [patient, setPatient] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Get parameters from URL
  const launchUrl = searchParams.get('launchUrl');
  const workflowTaskId = searchParams.get('workflow');
  const bsn = searchParams.get('patient');

  useEffect(() => {
    if (isAuthenticated && patientId) {
      loadPatient();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, patientId]);

  const loadPatient = async () => {
    setLoading(true);
    setError(null);
    try {
      const url = `${config.fhirBaseURL}/Patient/${patientId}`;
      const res = await fetch(url, {
        headers: {
          'Accept': 'application/fhir+json',
        },
      });

      if (!res.ok) {
        throw new Error(`Failed to fetch patient: ${res.statusText}`);
      }

      const patientData = await res.json();
      setPatient(patientData);
    } catch (err) {
      console.error('Error loading patient:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const calculateAge = (birthDate) => {
    if (!birthDate) return null;
    const today = new Date();
    const birth = new Date(birthDate);
    let age = today.getFullYear() - birth.getFullYear();
    const monthDiff = today.getMonth() - birth.getMonth();
    if (monthDiff < 0 || (monthDiff === 0 && today.getDate() < birth.getDate())) {
      age--;
    }
    return age;
  };

  const formatDate = (dateString) => {
    if (!dateString) return '-';
    try {
      return new Date(dateString).toLocaleDateString('nl-NL');
    } catch {
      return dateString;
    }
  };

  if (!isAuthenticated) {
    return (
      <div className="app-container">
        <div className="loading">Please log in to view this page.</div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="app-container">
        <div className="loading-container">
          <div className="spinner"></div>
          <p>Loading patient information...</p>
        </div>
      </div>
    );
  }

  if (error || !patient) {
    return (
      <div className="app-container">
        <div className="error-container">
          <div className="error-message">
            <strong>Error loading patient</strong>
            <p>{error || 'Patient not found'}</p>
            <button onClick={() => navigate(`/patients/${patientId}`)} className="button" style={{ marginTop: '15px' }}>
              ← Back to Patient
            </button>
          </div>
        </div>
      </div>
    );
  }

  const patientName = patientApi.formatName(patient);
  const patientGender = patientApi.formatGender(patient);
  const patientBirthDate = patientApi.formatBirthDate(patient);
  const patientAge = calculateAge(patientBirthDate);
  const patientBSN = patientApi.getByBSN(patient);

  // Build iframe URL with parameters
  const iframeUrl = launchUrl ?
    `${launchUrl}?${new URLSearchParams({
      ...(workflowTaskId && { workflow: workflowTaskId }),
      ...(bsn && { patient: bsn })
    }).toString()}` : null;

  return (
    <div className="app-container">
      <header className="header">
        <div className="header-content">
          <div>
            <h1>🏥 Demo EHR - Patient Context Launch</h1>
            <div className="header-subtitle">{patientName}</div>
          </div>
          <button onClick={logout} className="button button-secondary">
            Logout
          </button>
        </div>
      </header>

      {/* Patient Details Bar */}
      <div style={{
        backgroundColor: '#fff',
        borderBottom: '1px solid #dee2e6',
        padding: '12px 30px',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }}>
        <button
          onClick={() => navigate(`/patients/${patientId}`)}
          className="button button-secondary"
          style={{ padding: '6px 14px' }}
        >
          ← Back to Patient
        </button>

        <div style={{ display: 'flex', gap: '20px', alignItems: 'center' }}>
          {patientBSN && (
            <div>
              <span style={{ fontSize: '12px', color: '#6c757d' }}>BSN: </span>
              <span style={{ fontSize: '14px', fontWeight: 500 }}>{patientBSN}</span>
            </div>
          )}

          <div>
            <span style={{ fontSize: '12px', color: '#6c757d' }}>Birth: </span>
            <span style={{ fontSize: '14px', fontWeight: 500 }}>
              {formatDate(patientBirthDate)} {patientAge && `(${patientAge}y)`}
            </span>
          </div>

          <div>
            <span style={{ fontSize: '12px', color: '#6c757d' }}>Gender: </span>
            <span style={{ fontSize: '14px', fontWeight: 500, textTransform: 'capitalize' }}>
              {patientGender}
            </span>
          </div>
        </div>
      </div>

      {/* Context Launch Iframe */}
      <div style={{
        height: 'calc(100vh - 140px)',
        backgroundColor: '#f8f9fa',
        display: 'flex',
        flexDirection: 'column'
      }}>
        {iframeUrl ? (
          <iframe
            src={iframeUrl}
            style={{
              width: '100%',
              height: '100%',
              border: 'none'
            }}
            title="Receiving Application"
          />
        ) : (
          <div style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100%',
            color: '#6c757d'
          }}>
            <div>
              <p>No launch URL provided</p>
              <button
                onClick={() => navigate(`/patients/${patientId}`)}
                className="button"
                style={{ marginTop: '15px' }}
              >
                ← Back to Patient
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default PatientContextLaunchPage;
