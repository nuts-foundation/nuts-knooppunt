import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../AuthProvider';

function CallbackPage() {
  const navigate = useNavigate();
  const { handleCallback } = useAuth();
  const [error, setError] = useState(null);

  useEffect(() => {
    handleCallback()
      .then(() => {
        navigate('/');
      })
      .catch((err) => {
        console.error('Callback error:', err);
        setError(err.message);
      });
  }, [handleCallback, navigate]);

  return (
    <div className="callback-container">
      <div className="callback-content">
        <h2>Processing Login...</h2>
        {!error ? (
          <>
            <div className="spinner"></div>
            <p>Please wait while we complete your authentication.</p>
          </>
        ) : (
          <div className="error-message">
            <strong>Authentication Error</strong>
            <p>{error}</p>
            <button onClick={() => navigate('/')} className="button" style={{ marginTop: '15px' }}>
              Return to Home
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

export default CallbackPage;

