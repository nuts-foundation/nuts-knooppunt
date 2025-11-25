import React from 'react';
import { AuthProvider } from './AuthProvider';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import HomePage from './pages/HomePage';
import CallbackPage from './pages/CallbackPage';
import PatientsPage from './pages/PatientsPage';
import ConsentsPage from './pages/ConsentsPage';
import './App.css';

function App() {
  return (
    <AuthProvider>
      <Router>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/callback" element={<CallbackPage />} />
          <Route path="/patients" element={<PatientsPage />} />
          <Route path="/consents" element={<ConsentsPage />} />
        </Routes>
      </Router>
    </AuthProvider>
  );
}

export default App;
