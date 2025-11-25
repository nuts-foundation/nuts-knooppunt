import React, {useEffect, useState} from 'react';
import {useAuth} from '../AuthProvider';
import {consentApi} from '../api/consentApi';
import {patientApi} from '../api/patientApi';
import {organizationApi} from '../api/organizationApi';

function ConsentsPage() {
    const {isAuthenticated, logout} = useAuth();
    const [consents, setConsents] = useState([]);
    const [organizations, setOrganizations] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [showModal, setShowModal] = useState(false);
    const [editingConsent, setEditingConsent] = useState(null);
    const [saving, setSaving] = useState(false);
    const [formError, setFormError] = useState(null);
    const [deleteError, setDeleteError] = useState(null);
    const [deleting, setDeleting] = useState(false);
    const [patients, setPatients] = useState([]);

    const emptyForm = () => ({
        status: 'active',
        patientReference: '',
        provisionType: 'permit',
        provisionActorsOrgURAs: [],
        dateTime: new Date().toISOString().slice(0, 16), // for datetime-local
        categoryCodes: []
    });
    const [form, setForm] = useState(emptyForm());

    useEffect(() => {
        if (!isAuthenticated) return;
        const load = async () => {
            try {
                setLoading(true);
                const [orgs, pats, cons] = await Promise.all([
                    organizationApi.list(),
                    patientApi.list(),
                    consentApi.list()
                ]);
                setOrganizations(orgs);
                setPatients(pats);
                setConsents(cons);
            } catch (e) {
                setError(e.message);
            } finally {
                setLoading(false);
            }
        };
        load();
    }, [isAuthenticated]);

    const openNew = () => {
        setEditingConsent(null);
        setForm(emptyForm());
        setShowModal(true);
    };

    const openEdit = (consent) => {
        setEditingConsent(consent);
        setForm({...consentApi.toEditable(consent), dateTime: (consent.dateTime || '').slice(0, 16)});
        setShowModal(true);
    };

    const closeModal = () => {
        setShowModal(false);
        setEditingConsent(null);
        setFormError(null);
        setDeleteError(null);
        setForm(emptyForm());
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        setFormError(null);
        if (!form.patientReference) {
            setFormError('Patient is required');
            return;
        }
        if ((form.provisionActorsOrgURAs || []).length === 0) {
            setFormError('At least one provision actor organization is required');
            return;
        }
        setSaving(true);
        try {
            if (editingConsent) {
                // Build resource from form and keep same id
                const resource = consentApi.toResource(form);
                resource.id = editingConsent.id;
                const updated = await consentApi.update(editingConsent.id, resource);
                setConsents(prev => prev.map(c => c.id === updated.id ? updated : c));
            } else {
                const created = await consentApi.create(form);
                setConsents(prev => [created, ...prev]);
            }
            closeModal();
        } catch (e) {
            setFormError(e.message);
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async () => {
        if (!editingConsent) return;
        if (!window.confirm('Delete this consent?')) return;
        setDeleteError(null);
        setDeleting(true);
        try {
            await consentApi.delete(editingConsent.id);
            setConsents(prev => prev.filter(c => c.id !== editingConsent.id));
            closeModal();
        } catch (e) {
            setDeleteError(e.message);
        } finally {
            setDeleting(false);
        }
    };

    const toggleArrayValue = (field, value) => {
        setForm(f => {
            const arr = new Set(f[field]);
            if (arr.has(value)) arr.delete(value); else arr.add(value);
            return {...f, [field]: Array.from(arr)};
        });
    };

    const formatDateTime = (dt) => {
        if (!dt) return '-';
        try {
            return new Date(dt).toLocaleString();
        } catch {
            return dt;
        }
    };

    if (!isAuthenticated) return <div className="loading">Please log in to manage consents.</div>;

    return (
        <div className="app-container">
            <header className="header">
                <div className="header-content">
                    <div>
                        <h1>📝 Patient Consents</h1>
                        <div className="header-subtitle">Manage patient consent records</div>
                    </div>
                    <button onClick={logout} className="button button-secondary">Logout</button>
                </div>
            </header>
            <main className="main-content">
                <div style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    flexWrap: 'wrap',
                    gap: '10px'
                }}>
                    <h2 style={{margin: 0}}>Consents</h2>
                    <div style={{display: 'flex', gap: '10px'}}>
                        <button className="button" onClick={openNew}>➕ New Consent</button>
                        <a className="button button-secondary" href="/patients">← Patients</a>
                    </div>
                </div>
                {loading ? (
                    <div className="loading-container">
                        <div className="spinner"/>
                        <p>Loading consents...</p></div>
                ) : error ? (
                    <div className="error-message">{error}</div>
                ) : consents.length === 0 ? (
                    <div className="empty-state">
                        <p>No consents found.</p>
                        <button className="button" onClick={openNew}>Create one</button>
                    </div>
                ) : (
                    <div className="patients-table-container" style={{marginTop: '20px'}}>
                        <table className="patients-table">
                            <thead>
                            <tr>
                                <th>Status</th>
                                <th>Patient</th>
                                <th>Granted to</th>
                                <th>Date</th>
                            </tr>
                            </thead>
                            <tbody>
                            {consents.map(c => {
                                const patientId = c.patient?.reference?.replace('Patient/', '');
                                const patient = patients.find(p => p.id === patientId);
                                const bsn = patient ? patientApi.getByBSN(patient) : null;
                                return (
                                    <tr key={c.id} style={{cursor: 'pointer'}} onClick={() => openEdit(c)}
                                        title="Edit consent">
                                        <td>{c.status}</td>
                                        <td>{bsn || patientId || '-'}</td>
                                        <td>{(c.provision?.actor || []).map(a => a.reference.identifier.value).join(', ') || '-'}</td>
                                        <td>{formatDateTime(c.dateTime)}</td>
                                    </tr>
                                );
                            })}
                            </tbody>
                        </table>
                    </div>
                )}

                {showModal && (
                    <div className="modal-overlay">
                        <div className="modal" style={{maxWidth: '640px'}}>
                            <h3 style={{marginTop: 0}}>{editingConsent ? 'Edit Consent' : 'New Consent'}</h3>
                            <form onSubmit={handleSubmit} className="new-patient-form">
                                <div className="form-row">
                                    <label>Status</label>
                                    <select value={form.status}
                                            onChange={e => setForm(f => ({...f, status: e.target.value}))}>
                                        <option value="active">active</option>
                                        <option value="inactive">inactive</option>
                                        <option value="draft">draft</option>
                                    </select>
                                </div>
                                <div className="form-row">
                                    <label>Patient</label>
                                    <select value={form.patientReference}
                                            onChange={e => setForm(f => ({...f, patientReference: e.target.value}))}
                                            required>
                                        <option value="">-- select patient --</option>
                                        {patients.map(p => (
                                            <option key={p.id}
                                                    value={`Patient/${p.id}`}>{patientApi.formatName(p)} ({p.id})</option>
                                        ))}
                                    </select>
                                </div>
                                <div className="form-row">
                                    <label>Grant consent to</label>
                                    <div className="checkbox-grid" style={{
                                        display: 'grid',
                                        gap: '6px',
                                        gridTemplateColumns: 'repeat(auto-fill, minmax(220px,1fr))'
                                    }}>
                                        {organizations.map(o => {
                                            const ura = o.identifier[0].value;
                                            return (
                                                <label key={o.id} style={{fontWeight: 'normal'}}>
                                                    <input type="checkbox"
                                                           checked={form.provisionActorsOrgURAs.includes(ura)}
                                                           onChange={() => toggleArrayValue('provisionActorsOrgURAs', ura)}/> {o.name || ura}
                                                </label>
                                            );
                                        })}
                                    </div>
                                </div>
                                <div className="form-row">
                                    <label>Date/Time</label>
                                    <input type="datetime-local" value={form.dateTime}
                                           onChange={e => setForm(f => ({...f, dateTime: e.target.value}))}/>
                                </div>
                                <div className="form-row">
                                    <label>Category Codes (comma separated codes)</label>
                                    <input type="text" value={form.categoryCodes.map(c => c.code).join(',')}
                                           onChange={e => {
                                               const codes = e.target.value.split(',').map(c => c.trim()).filter(Boolean);
                                               setForm(f => ({...f, categoryCodes: codes.map(code => ({code}))}));
                                           }} placeholder="e.g. 34133-9, 64292-6"/>
                                </div>
                                {formError && <div className="form-error">{formError}</div>}
                                {deleteError && <div className="form-error">{deleteError}</div>}
                                <div className="form-actions" style={{justifyContent: 'space-between'}}>
                                    <div style={{display: 'flex', gap: '8px'}}>
                                        <button type="button" className="button button-secondary" onClick={closeModal}
                                                disabled={saving || deleting}>Cancel
                                        </button>
                                        <button type="submit" className="button"
                                                disabled={saving || deleting}>{saving ? 'Saving...' : (editingConsent ? 'Save Changes' : 'Create')}</button>
                                    </div>
                                    {editingConsent && (
                                        <button type="button" className="button button-danger" onClick={handleDelete}
                                                disabled={saving || deleting}>{deleting ? 'Deleting...' : '🗑 Delete'}</button>
                                    )}f
                                </div>
                            </form>
                        </div>
                    </div>
                )}
            </main>
        </div>
    );
}

export default ConsentsPage;
