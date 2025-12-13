import React from 'react';
import { useAuth } from '../AuthProvider';

function HomePage() {
  const { user, isLoading, isAuthenticated, login, logout } = useAuth();

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
                Logged in as: {user.profile.name ? `${user.profile.name} (${user.profile.sub})` : user.profile.sub}
              </span>
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
              the Nuts Knooppunt for secure authentication.
            </p>
            <p>
              To access patient records and other features, please log in using
              your credentials.
            </p>
            <button onClick={login} className="button">
              Login with Knooppunt
            </button>
          </div>
        ) : (
          <div>
            <div className="welcome-section" style={{ marginBottom: '20px' }}>
              <h2>Welcome back{user.profile.name ? `, ${user.profile.name}` : ''}!</h2>
              <p>You have successfully logged in to the Demo EHR system.</p>
            </div>

            <div className="dashboard">
              <div className="card">
                <h3>📋 Patient Records</h3>
                <p>
                  Access and manage patient health records securely through the
                  Nuts Knooppunt infrastructure.
                </p>
                <a href="/patients" className="button" style={{ marginTop: '15px', display: 'inline-block' }}>
                  View Patients
                </a>
              </div>

              <div className="card">
                <h3>📝 Patient Consents</h3>
                <p>Manage consent records that grant or deny access to patient data for organizations.</p>
                <a href="/consents" className="button" style={{ marginTop: '15px', display: 'inline-block' }}>
                  Manage Consents
                </a>
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

              <div className="card">
                <h3>🔐 Your Session</h3>
                <div className="info-grid">
                  <div className="info-item">
                    <span className="info-label">Subject:</span>
                    <span className="info-value">{user.profile.sub}</span>
                  </div>
                  <div className="info-item">
                    <span className="info-label">Client ID:</span>
                    <span className="info-value">{user.profile.client_id}</span>
                  </div>
                  <div className="info-item">
                    <span className="info-label">Scopes:</span>
                    <span className="info-value">{user.scopes?.join(', ')}</span>
                  </div>
                  <div className="info-item">
                    <span className="info-label">Expires:</span>
                    <span className="info-value">
                      {new Date(user.expires_at * 1000).toLocaleString()}
                    </span>
                  </div>
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
