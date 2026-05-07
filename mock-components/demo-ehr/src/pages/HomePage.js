import React from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../AuthProvider';
import CredentialStatusCard from '../components/CredentialStatusCard';

const ORG_CREDENTIAL_TYPES = [
  {
    type: 'HealthcareOrganizationCredential',
    label: '🏥 Healthcare Organization Credential',
    requestable: false,
  },
  {
    type: 'HealthcareProfessionalDelegationCredential',
    label: '🪪 Healthcare Professional Delegation Credential',
  },
  {
    type: 'HealthcareProviderRoleTypeCredential',
    label: '🩺 Healthcare Provider Role Type Credential',
  },
];

const buildOrgCredentialDetails = ({ type, walletDid, ura }) => {
  if (type === 'HealthcareProfessionalDelegationCredential') {
    // BSN intentionally empty: practitioner has no BSN, but the AET stub
    // expects the field to be present.
    return { did: walletDid, bsn: '', ura };
  }
  return undefined;
};

function HomePage() {
  const {
    user,
    isLoading,
    isAuthenticated,
    isDevUser,
    login,
    devLogin,
    switchDevUra,
    devLoginEnabled,
    logout,
  } = useAuth();

  const handleDevLogin = () => {
    const ura = window.prompt('Care organization URA (leave empty for default 00000666):', '');
    devLogin(ura == null ? '' : ura);
  };

  const handleSwitchUra = () => {
    const current = user && (user.ura || user.sub) ? (user.ura || user.sub) : '';
    const ura = window.prompt('Switch URA:', current);
    if (ura == null) return;
    switchDevUra(ura);
  };

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="app-container">
      <header className="header">
        <div className="header-content">
          <div>
            <h1>🏥 Demo EHR</h1>
            <div className="header-subtitle">Electronic Health Record System</div>
          </div>
          {isAuthenticated && (
            <div className="user-info">
              <span className="user-name">
                Logged in as: {user.name || user.sub}
              </span>
              {isDevUser && (
                <button
                  onClick={handleSwitchUra}
                  className="button button-secondary"
                  title="Switch the URA of the dev session"
                >
                  Switch URA
                </button>
              )}
              <button onClick={logout} className="button button-secondary">
                Logout
              </button>
            </div>
          )}
        </div>
      </header>

      <main className="main-content">
        {!isAuthenticated ? (
          <div className="welcome-section">
            <h2>Welcome to Demo EHR</h2>
            <p>
              This is a demonstration Electronic Health Record system that uses
              Dezi for secure authentication.
            </p>
            <p>
              To access patient records and other features, please log in using
              your credentials.
            </p>
            <button onClick={login} className="button">
              Login with Dezi
            </button>
            {devLoginEnabled && (
              <button
                onClick={handleDevLogin}
                className="button button-secondary"
                style={{ marginLeft: '10px' }}
                title="Bypass OIDC for local development"
              >
                Dev login (skip OIDC)
              </button>
            )}
          </div>
        ) : (
          <div>
            <div className="welcome-section" style={{ marginBottom: '20px' }}>
              <h2>Welcome back{user.name ? `, ${user.name}` : ''}!</h2>
              <p>You have successfully logged in to the Demo EHR system.</p>
            </div>

            <div className="dashboard">
              <div className="card">
                <h3>📋 Patient Records</h3>
                <p>
                  Access and manage patient health records securely through the
                  Dezi authentication infrastructure.
                </p>
                <Link to="/patients" className="button" style={{ marginTop: '15px', display: 'inline-block' }}>
                  View Patients
                </Link>
              </div>

              <div className="card">
                <h3>📝 Patient Consents</h3>
                <p>Manage consent records that grant or deny access to patient data for organizations.</p>
                <Link to="/consents" className="button" style={{ marginTop: '15px', display: 'inline-block' }}>
                  Manage Consents
                </Link>
              </div>

              <div className="card">
                <h3>🔄 Data Exchange</h3>
                <p>
                  Share and receive patient data with other healthcare providers
                  in the Nuts network.
                </p>
                <p style={{ marginTop: '15px', fontSize: '14px', color: '#999' }}>
                  (Feature coming soon)
                </p>
              </div>

              <CredentialStatusCard
                title="🛂 LDN Identity"
                description="Verifiable Credentials that prove this care organization's role in the Landelijk Dekkend Netwerk."
                ura={user.ura || user.abonnee_nummer || user.sub}
                types={ORG_CREDENTIAL_TYPES}
                buildCredentialDetails={buildOrgCredentialDetails}
              />

              <div className="card">
                <h3>🔐 Your Session</h3>
                <div className="info-grid">
                  <div className="info-item">
                    <span className="info-label">Subject:</span>
                    <span className="info-value">{user.sub}</span>
                  </div>
                  {user.dezi_nummer && (
                    <div className="info-item">
                      <span className="info-label">Dezi Number:</span>
                      <span className="info-value">{user.dezi_nummer}</span>
                    </div>
                  )}
                  {user.rol_naam && (
                    <div className="info-item">
                      <span className="info-label">Role:</span>
                      <span className="info-value">{user.rol_naam}</span>
                    </div>
                  )}
                  {user.abonnee_naam && (
                    <div className="info-item">
                      <span className="info-label">Organization:</span>
                      <span className="info-value">{user.abonnee_naam}</span>
                    </div>
                  )}
                  {user.verklaring_id && (
                    <div className="info-item">
                      <span className="info-label">Verklaring ID:</span>
                      <span className="info-value" style={{ fontSize: '12px' }}>{user.verklaring_id}</span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

export default HomePage;
