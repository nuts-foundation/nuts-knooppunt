import React, { useState, useEffect } from 'react';
import { useAuth } from '../AuthProvider';
import { useNavigate } from 'react-router-dom';
import { patientApi } from '../api/patientApi';

function PatientsPage() {
  const { isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();
  const [patients, setPatients] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [searchResults, setSearchResults] = useState(null);
  const [searching, setSearching] = useState(false);
  const [showNewPatient, setShowNewPatient] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState(null);
  const [form, setForm] = useState({
    bsn: '',
    given: '',
    family: '',
    prefix: '',
    birthDate: '',
    gender: 'unknown',
  });
  const [editingPatient, setEditingPatient] = useState(null);
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState(null);

  useEffect(() => {
    if (isAuthenticated) {
      loadPatients();
    }
  }, [isAuthenticated]);

  // Search by BSN when search term changes
  useEffect(() => {
    const searchByBSN = async () => {
      const trimmed = searchTerm.trim();

      // Check if search term looks like a BSN (contains only digits)
      const isBSNLike = /^\d+$/.test(trimmed);

      if (isBSNLike && trimmed.length > 0) {
        setSearching(true);
        try {
          const results = await patientApi.searchByBSN(trimmed);
          setSearchResults(results);
        } catch (err) {
          console.error('Error searching by BSN:', err);
          setSearchResults([]);
        } finally {
          setSearching(false);
        }
      } else {
        // Clear search results if not a BSN
        setSearchResults(null);
      }
    };

    // Debounce the search
    const timeoutId = setTimeout(searchByBSN, 300);
    return () => clearTimeout(timeoutId);
  }, [searchTerm]);

  const loadPatients = async () => {
    setLoading(true);
    setError(null);
    try {
      const patientData = await patientApi.list();
      setPatients(patientData);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  // Use search results if BSN search was performed, otherwise use client-side filter
  const filteredPatients = searchResults !== null ? searchResults : patients.filter(patient => {
    if (!searchTerm) return true;

    const name = patientApi.formatName(patient).toLowerCase();
    const bsn = patientApi.getByBSN(patient) || '';
    const search = searchTerm.toLowerCase();

    return name.includes(search) || bsn.includes(search);
  });

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

  const resetForm = () => setForm({ bsn: '', given: '', family: '', prefix: '', birthDate: '', gender: 'unknown' });

  const startEdit = (patient) => {
    setEditingPatient(patient);
    setForm(patientApi.toForm(patient));
    setShowNewPatient(true); // reuse modal for edit
  };
  const isEditMode = !!editingPatient;

  const handleCreate = async (e) => {
    e.preventDefault();
    setCreateError(null);
    if (!form.given.trim() || !form.family.trim() || !form.birthDate) {
      setCreateError('Given name, family name and birth date are required.');
      return;
    }
    if (isEditMode) {
      setEditing(true);
      try {
        const updated = await patientApi.update(editingPatient.id, {
          bsn: form.bsn || null,
          given: form.given.trim().split(/\s+/),
          family: form.family.trim(),
          prefix: form.prefix.trim() ? form.prefix.trim().split(/\s+/) : [],
          birthDate: form.birthDate,
          gender: form.gender,
        });
        setPatients(prev => prev.map(p => p.id === updated.id ? updated : p));
        resetForm();
        setShowNewPatient(false);
        setEditingPatient(null);
      } catch (err) {
        setCreateError(err.message);
      } finally {
        setEditing(false);
      }
      return;
    }
    // create new
    setCreating(true);
    try {
      const created = await patientApi.create({
        bsn: form.bsn || null,
        given: form.given.trim().split(/\s+/),
        family: form.family.trim(),
        prefix: form.prefix.trim() ? form.prefix.trim().split(/\s+/) : [],
        birthDate: form.birthDate,
        gender: form.gender,
      });
      setPatients(prev => [created, ...prev]);
      resetForm();
      setShowNewPatient(false);
    } catch (err) {
      setCreateError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async () => {
    if (!editingPatient) return;
    if (!window.confirm('Are you sure you want to delete this patient? This action cannot be undone.')) return;
    setDeleteError(null);
    setDeleting(true);
    try {
      await patientApi.delete(editingPatient.id);
      setPatients(prev => prev.filter(p => p.id !== editingPatient.id));
      cancelModal();
    } catch (err) {
      setDeleteError(err.message);
    } finally {
      setDeleting(false);
    }
  };

  const cancelModal = () => {
    setShowNewPatient(false);
    setCreateError(null);
    setEditingPatient(null);
    resetForm();
  };

  // Helper to generate random patient form data
  const generateRandomForm = () => {
    const maleNames = [
      'Jan','Pieter','Bram','Sven','Lars','Noah','Daan','Luuk','Thijs','Jelle','Koen','Ruben','Gijs','Hugo','Mats','Timo','Bas','Niels','Olaf','Kyan','Floris','Sem','Joris','Tijn'
    ];
    const femaleNames = [
      'Emma','Sophie','Noor','Anna','Eva','Mila','Sara','Lina','Zoë','Fleur','Julia','Lotte','Iris','Nina','Esmee','Tess','Hanna','Jade','Maud','Ilse','Vera','Lois','Liv','Elin'
    ];
    const neutralNames = [
      'Alex','Sam','Robin','Jesse','Sky','Taylor','Jamie','Charlie','Bo','Casey','Puck','Quinn','Riley','Rowan','Saar','Morris','Fenna','Mika','Nik','Noa'
    ];
    const familyNames = [
      'Jansen','de Vries','Bakker','Visser','Mulder','Kok','Meijer','Koster','Bos','Smits','de Boer','Willems','Dijkstra','van Dijk','van Dam','de Graaf','Hoekstra','Post','Kuipers','Verbeek','Peeters','Dekker','van Leeuwen','Hendriks'
    ];
    const prefixes = ['van','van der','de','van den','in het','op den',''];
    const genders = ['male','female','other','unknown'];

    const gender = genders[Math.floor(Math.random() * genders.length)];
    let givenPool;
    if (gender === 'male') givenPool = maleNames; else if (gender === 'female') givenPool = femaleNames; else givenPool = neutralNames.concat(maleNames, femaleNames);

    const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];
    const given = [pick(givenPool)];
    // 30% chance of second given name
    if (Math.random() < 0.3) {
      const second = pick(givenPool.filter(n => !given.includes(n)));
      if (second) given.push(second);
    }
    const family = pick(familyNames);
    const prefixRaw = pick(prefixes);
    const prefix = prefixRaw ? prefixRaw : '';
    // Random birth date between 1940-01-01 and 2020-12-31
    const start = new Date(1940, 0, 1).getTime();
    const end = new Date(2020, 11, 31).getTime();
    const birthMillis = start + Math.random() * (end - start);
    const birthDate = new Date(birthMillis).toISOString().slice(0, 10);
    // Random BSN 9 digits
    const bsn = String(Math.floor(100000000 + Math.random() * 900000000));

    return {
      bsn,
      given: given.join(' '),
      family,
      prefix,
      birthDate,
      gender,
    };
  };

  const openNewPatient = () => {
    setForm(generateRandomForm());
    setEditingPatient(null);
    setShowNewPatient(true);
  };

  if (!isAuthenticated) {
    return (
      <div className="app-container">
        <div className="loading">Please log in to view patients.</div>
      </div>
    );
  }

  return (
    <div className="app-container">
      <header className="header">
        <div className="header-content">
          <div>
            <h1>🏥 Demo EHR - Patients</h1>
            <div className="header-subtitle">Patient Overview</div>
          </div>
          <button onClick={logout} className="button button-secondary">
            Logout
          </button>
        </div>
      </header>

      <main className="main-content">
        <div className="patients-header">
          <h2>Patients Overview</h2>
          <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
            <div className="search-box">
              <input
                type="text"
                placeholder="🔍 Search by name or BSN..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="search-input"
              />
              {searching && <span style={{ marginLeft: '10px', color: '#666' }}>Searching...</span>}
            </div>
            <button type="button" className="button" onClick={openNewPatient}>
              ➕ New Patient
            </button>
          </div>
        </div>

        {loading ? (
          <div className="loading-container">
            <div className="spinner"></div>
            <p>Loading patients...</p>
          </div>
        ) : error ? (
          <div className="error-container">
            <div className="error-message">
              <strong>Error loading patients</strong>
              <p>{error}</p>
              <button onClick={loadPatients} className="button" style={{ marginTop: '15px' }}>
                Retry
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="patients-count">
              {filteredPatients.length} patient{filteredPatients.length !== 1 ? 's' : ''} found
              {searchTerm && searchResults !== null && ' (searched by BSN in FHIR server)'}
              {searchTerm && searchResults === null && ` (filtered from ${patients.length})`}
            </div>

            {filteredPatients.length === 0 ? (
              <div className="empty-state">
                <p>No patients found</p>
                {searchTerm && (
                  <button onClick={() => setSearchTerm('')} className="button">
                    Clear search
                  </button>
                )}
              </div>
            ) : (
              <div className="patients-table-container">
                <table className="patients-table">
                  <thead>
                    <tr>
                      <th>BSN</th>
                      <th>Name</th>
                      <th>Gender</th>
                      <th>Birth Date</th>
                      <th>Age</th>
                      <th style={{ width: '100px' }}>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredPatients.map((patient) => {
                      const bsn = patientApi.getByBSN(patient);
                      const name = patientApi.formatName(patient);
                      const birthDate = patientApi.formatBirthDate(patient);
                      const gender = patientApi.formatGender(patient);

                      // Calculate age
                      let age = '-';
                      if (birthDate) {
                        const today = new Date();
                        const birth = new Date(birthDate);
                        age = Math.floor((today - birth) / (365.25 * 24 * 60 * 60 * 1000));
                      }

                      return (
                        <tr key={patient.id} onClick={() => navigate(`/patients/${patient.id}`)} style={{ cursor: 'pointer' }} title="Click to view details">
                          <td className="bsn-cell">
                            {bsn ? (
                              <span className="bsn-badge">{bsn}</span>
                            ) : (
                              <span className="text-muted">-</span>
                            )}
                          </td>
                          <td className="name-cell">{name}</td>
                          <td className="gender-cell">
                            <span className="gender-badge">
                              {getGenderIcon(gender)} {gender}
                            </span>
                          </td>
                          <td>{formatDate(birthDate)}</td>
                          <td>{age}</td>
                          <td>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                startEdit(patient);
                              }}
                              className="button button-secondary"
                              style={{ padding: '6px 12px', fontSize: '13px' }}
                              title="Edit patient"
                            >
                              ✏️ Edit
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}

        {showNewPatient && (
          <div className="modal-overlay">
            <div className="modal">
              <h3 style={{ marginTop: 0 }}>{isEditMode ? 'Edit Patient' : 'Add New Patient'}</h3>
              <form onSubmit={handleCreate} className="new-patient-form">
                <div className="form-row">
                  <label>BSN (optional)</label>
                  <input
                    type="text"
                    value={form.bsn}
                    maxLength={9}
                    onChange={(e) => setForm(f => ({ ...f, bsn: e.target.value.replace(/\D/g, '') }))}
                    placeholder="9 digits"
                  />
                </div>
                <div className="form-row">
                  <label>Given Name(s)</label>
                  <input
                    type="text"
                    value={form.given}
                    required
                    onChange={(e) => setForm(f => ({ ...f, given: e.target.value }))}
                    placeholder="e.g. John Robert"
                  />
                </div>
                <div className="form-row">
                  <label>Family Name</label>
                  <input
                    type="text"
                    value={form.family}
                    required
                    onChange={(e) => setForm(f => ({ ...f, family: e.target.value }))}
                    placeholder="e.g. Doe"
                  />
                </div>
                <div className="form-row">
                  <label>Prefix(es)</label>
                  <input
                    type="text"
                    value={form.prefix}
                    onChange={(e) => setForm(f => ({ ...f, prefix: e.target.value }))}
                    placeholder="e.g. van der"
                  />
                </div>
                <div className="form-row">
                  <label>Birth Date</label>
                  <input
                    type="date"
                    value={form.birthDate}
                    required
                    onChange={(e) => setForm(f => ({ ...f, birthDate: e.target.value }))}
                  />
                </div>
                <div className="form-row">
                  <label>Gender</label>
                  <select
                    value={form.gender}
                    onChange={(e) => setForm(f => ({ ...f, gender: e.target.value }))}
                  >
                    <option value="male">Male</option>
                    <option value="female">Female</option>
                    <option value="other">Other</option>
                    <option value="unknown">Unknown</option>
                  </select>
                </div>
                {createError && <div className="form-error">{createError}</div>}
                {deleteError && <div className="form-error">{deleteError}</div>}
                <div className="form-actions">
                  {isEditMode && (
                    <button type="button" className="button button-danger" onClick={handleDelete} disabled={creating || editing || deleting}>
                      {deleting ? 'Deleting...' : '🗑 Delete'}
                    </button>
                  )}
                  {!isEditMode && (
                    <button type="button" className="button button-secondary" onClick={() => setForm(generateRandomForm())} disabled={creating || editing}>
                      🎲 Randomize
                    </button>
                  )}
                  <button type="button" className="button button-secondary" onClick={cancelModal} disabled={creating || editing || deleting}>Cancel</button>
                  <button type="submit" className="button" disabled={creating || editing || deleting}>{(creating || editing) ? 'Saving...' : (isEditMode ? 'Save Changes' : 'Create')}</button>
                </div>
              </form>
            </div>
          </div>
        )}

        <div style={{ marginTop: '30px' }}>
          <a href="/" className="button button-secondary">
            ← Back to Dashboard
          </a>
        </div>
      </main>
    </div>
  );
}

export default PatientsPage;
