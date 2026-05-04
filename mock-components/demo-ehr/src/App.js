import React from 'react';
import { AuthProvider } from './AuthProvider';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { baseUrl } from './runtimeConfig';
import HomePage from './pages/HomePage';
import PatientsPage from './pages/PatientsPage';
import PatientPage from './pages/PatientPage';
import PatientContextLaunchPage from './pages/PatientContextLaunchPage';
import ConsentsPage from './pages/ConsentsPage';
import './App.css';

function App() {
  return (
    <AuthProvider>
      <Router basename={baseUrl || undefined}>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/patients" element={<PatientsPage />} />
          <Route path="/patients/:patientId" element={<PatientPage />} />
          <Route path="/patients/:patientId/context-launch" element={<PatientContextLaunchPage />} />
          <Route path="/consents" element={<ConsentsPage />} />
        </Routes>
      </Router>
    </AuthProvider>
  );
}

export default App;
