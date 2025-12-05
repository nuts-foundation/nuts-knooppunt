import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../AuthProvider';
import { patientApi } from '../api/patientApi';
import { medicationApi } from '../api/medicationApi';
import { nviApi } from '../api/nviApi';
import { healthcareServiceApi } from '../api/healthcareServiceApi';
import { organizationApi } from '../api/organizationApi';
import { taskApi } from '../api/taskApi';

function PatientPage() {
  const { patientId } = useParams();
  const navigate = useNavigate();
  const { isAuthenticated, logout, user, practitionerId } = useAuth();

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
  const [showEOverdrachtModal, setShowEOverdrachtModal] = useState(false);
  const [eOverdrachtStep, setEOverdrachtStep] = useState('services'); // 'services', 'organizations', 'departments', or 'confirmation'
  const [selectedServiceGroup, setSelectedServiceGroup] = useState(null);
  const [selectedOrganization, setSelectedOrganization] = useState(null);
  const [selectedDepartment, setSelectedDepartment] = useState(null);
  const [healthcareServices, setHealthcareServices] = useState([]);
  const [groupedHealthcareServices, setGroupedHealthcareServices] = useState([]);
  const [healthcareServicesLoading, setHealthcareServicesLoading] = useState(false);
  const [healthcareServicesError, setHealthcareServicesError] = useState(null);
  const [organizations, setOrganizations] = useState([]);
  const [organizationsLoading, setOrganizationsLoading] = useState(false);
  const [organizationsError, setOrganizationsError] = useState(null);
  const [departments, setDepartments] = useState([]);
  const [departmentsLoading, setDepartmentsLoading] = useState(false);
  const [departmentsError, setDepartmentsError] = useState(null);
  const [taskCreating, setTaskCreating] = useState(false);
  const [taskSuccess, setTaskSuccess] = useState(false);
  const [taskError, setTaskError] = useState(null);
  const [eOverdrachtTasks, setEOverdrachtTasks] = useState([]);
  const [eOverdrachtTasksLoading, setEOverdrachtTasksLoading] = useState(true);
  const [eOverdrachtTasksError, setEOverdrachtTasksError] = useState(null);
  const [taskOrganizations, setTaskOrganizations] = useState({});
  const [expandedTaskJson, setExpandedTaskJson] = useState(null);

  useEffect(() => {
    if (isAuthenticated && patientId) {
      loadPatientData();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, patientId]);

  useEffect(() => {
    if (showEOverdrachtModal) {
      loadHealthcareServices();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showEOverdrachtModal]);

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

      // Load eOvedracht tasks
      loadEOverdrachtTasks(patientId);
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

  const loadEOverdrachtTasks = async (patientId) => {
    setEOverdrachtTasksLoading(true);
    setEOverdrachtTasksError(null);
    try {
      const tasks = await taskApi.getEOverdrachtTasks(patientId);
      setEOverdrachtTasks(tasks);

      // Load organization details for each task
      const orgMap = {};
      for (const task of tasks) {
        if (task.owner?.reference) {
          try {
            // Extract organization ID from reference (format: Organization/id or full URL)
            const ref = task.owner.reference;
            const orgId = ref.includes('/') ? ref.split('/').pop() : ref;

            // Fetch organization details
            let org = await organizationApi.getById(orgId);
            if (org) {
              // Traverse up the partOf hierarchy to find the root organization
              let rootOrg = org;
              const visited = new Set(); // Prevent infinite loops

              while (rootOrg.partOf && rootOrg.partOf.reference) {
                const parentRef = rootOrg.partOf.reference;
                const parentId = parentRef.includes('/') ? parentRef.split('/').pop() : parentRef;

                // Check for circular references
                if (visited.has(parentId)) {
                  console.warn('Circular partOf reference detected for organization:', parentId);
                  break;
                }
                visited.add(parentId);

                try {
                  const parentOrg = await organizationApi.getById(parentId);
                  if (parentOrg) {
                    rootOrg = parentOrg;
                  } else {
                    break;
                  }
                } catch (err) {
                  console.warn('Failed to load parent organization:', parentId, err);
                  break;
                }
              }

              // Store the root organization
              orgMap[orgId] = rootOrg;
            }
          } catch (err) {
            console.warn('Failed to load organization for task:', task.id, err);
          }
        }
      }
      setTaskOrganizations(orgMap);
    } catch (err) {
      setEOverdrachtTasksError(err.message);
    } finally {
      setEOverdrachtTasksLoading(false);
    }
  };

  const loadHealthcareServices = async () => {
    setHealthcareServicesLoading(true);
    setHealthcareServicesError(null);
    try {
      const services = await healthcareServiceApi.list();
      setHealthcareServices(services);
      // Group services by name
      const grouped = healthcareServiceApi.groupByName(services);
      setGroupedHealthcareServices(grouped);
    } catch (err) {
      setHealthcareServicesError(err.message);
    } finally {
      setHealthcareServicesLoading(false);
    }
  };

  const loadOrganizationsForServiceGroup = async (serviceGroup) => {
    setOrganizationsLoading(true);
    setOrganizationsError(null);
    try {
      // Extract organization IDs from providedBy references
      const orgIds = healthcareServiceApi.getOrganizationIds(serviceGroup.services);
      if (orgIds.length === 0) {
        setOrganizations([]);
        return;
      }
      // Fetch organizations by IDs
      const orgs = await organizationApi.getByIds(orgIds);

      // Traverse to root organizations (with URA and no partOf)
      const rootOrgMap = new Map();
      for (const org of orgs) {
        try {
          let currentOrg = org;
          const visited = new Set([currentOrg.id]); // Prevent infinite loops

          // Traverse up the partOf hierarchy until we find a root organization
          while (currentOrg) {
            const hasURA = organizationApi.getURA(currentOrg);
            const hasPartOf = currentOrg.partOf && currentOrg.partOf.reference;

            // Check if this is a root organization (has URA and no partOf)
            if (hasURA && !hasPartOf) {
              rootOrgMap.set(currentOrg.id, currentOrg);
              break;
            }

            // If has partOf, traverse up
            if (hasPartOf) {
              const parentRef = currentOrg.partOf.reference;
              const parentId = parentRef.includes('/') ? parentRef.split('/').pop() : parentRef;

              // Check for circular references
              if (visited.has(parentId)) {
                console.warn('Circular partOf reference detected for organization:', parentId);
                break;
              }
              visited.add(parentId);

              try {
                const parentOrg = await organizationApi.getById(parentId);
                if (parentOrg) {
                  currentOrg = parentOrg;
                } else {
                  break;
                }
              } catch (err) {
                console.warn('Failed to load parent organization:', parentId, err);
                break;
              }
            } else {
              // Has no partOf but also no URA - stop here
              break;
            }
          }
        } catch (err) {
          console.warn('Failed to traverse organization hierarchy:', org.id, err);
        }
      }

      // Set deduplicated root organizations
      setOrganizations(Array.from(rootOrgMap.values()));
    } catch (err) {
      setOrganizationsError(err.message);
    } finally {
      setOrganizationsLoading(false);
    }
  };

  const loadDepartments = async (organizationId) => {
    setDepartmentsLoading(true);
    setDepartmentsError(null);
    try {
      const deps = await organizationApi.getSubOrganizations(organizationId);
      setDepartments(deps);
    } catch (err) {
      setDepartmentsError(err.message);
    } finally {
      setDepartmentsLoading(false);
    }
  };

  const handleServiceSelect = async (group) => {
    setSelectedServiceGroup(group);
    setEOverdrachtStep('organizations');
    await loadOrganizationsForServiceGroup(group);
  };

  const handleOrganizationSelect = async (org) => {
    setSelectedOrganization(org);
    setEOverdrachtStep('departments');
    await loadDepartments(org.id);
  };

  const handleCloseModal = () => {
    setShowEOverdrachtModal(false);
    // Reset state when modal closes
    setTimeout(() => {
      setEOverdrachtStep('services');
      setSelectedServiceGroup(null);
      setSelectedOrganization(null);
      setSelectedDepartment(null);
      setOrganizations([]);
      setOrganizationsError(null);
      setDepartments([]);
      setDepartmentsError(null);
      setTaskError(null);
      setTaskSuccess(false);
    }, 300); // Delay to allow modal animation to complete
  };

  const handleBackToServices = () => {
    setEOverdrachtStep('services');
    setSelectedServiceGroup(null);
    setSelectedOrganization(null);
    setOrganizations([]);
    setOrganizationsError(null);
    setDepartments([]);
    setDepartmentsError(null);
  };

  const handleBackToOrganizations = () => {
    setEOverdrachtStep('organizations');
    setSelectedOrganization(null);
    setDepartments([]);
    setDepartmentsError(null);
    setTaskError(null);
    setTaskSuccess(false);
  };

  const handleDepartmentSelect = (department) => {
    setSelectedDepartment(department);
    setEOverdrachtStep('confirmation');
    setTaskError(null);
    setTaskSuccess(false);
  };

  const handleBackToDepartments = () => {
    setEOverdrachtStep('departments');
    setSelectedDepartment(null);
    setTaskError(null);
    setTaskSuccess(false);
  };

  const handleConfirmTask = async () => {
    setTaskCreating(true);
    setTaskError(null);
    setTaskSuccess(false);

    try {
      // Get practitioner ID from auth context
      if (!practitionerId) {
        throw new Error('Practitioner ID not found. Please log out and log back in.');
      }

      // Get user information for display
      const userName = user?.profile?.name || user?.profile?.email || 'Unknown User';

      // Step 1: Create the eOverdracht Task
      console.log('Creating eOverdracht Task...');
      const createdTask = await taskApi.createEOverdrachtTask(
        patientId,
        patientName,
        practitionerId,
        userName,
        selectedDepartment.id,
        organizationApi.formatName(selectedDepartment)
      );

      console.log('Created Task:', createdTask);

      // Step 2: Find endpoint and notify receiving party
      console.log('Finding endpoint for organization:', selectedDepartment.id);
      const endpoint = await organizationApi.getEndpoint(selectedDepartment.id);

      if (!endpoint) {
        // No endpoint found - delete task and fail
        console.error('No endpoint found for organization');
        await taskApi.deleteTask(createdTask.id);
        throw new Error('No eOverdracht notification endpoint found for the selected organization');
      }

      console.log('Found endpoint:', endpoint.address);
      console.log('Sending notification to receiving party...');

      try {
        await taskApi.notifyReceivingParty(createdTask, endpoint);
        console.log('Notification sent successfully');
      } catch (notifyErr) {
        // Notification failed - delete task and fail
        console.error('Failed to notify receiving party:', notifyErr);
        await taskApi.deleteTask(createdTask.id);
        throw new Error(`Failed to notify receiving party: ${notifyErr.message}`);
      }

      setTaskSuccess(true);

      // Reload tasks list
      loadEOverdrachtTasks(patientId);

      // Show success message for 2 seconds, then close modal
      setTimeout(() => {
        handleCloseModal();
      }, 2000);
    } catch (err) {
      console.error('Error creating Task:', err);
      setTaskError(err.message);
    } finally {
      setTaskCreating(false);
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
        <div style={{ marginBottom: '20px', display: 'flex', gap: '10px' }}>
          <button onClick={() => navigate('/patients')} className="button button-secondary">
            ← Back to Patients
          </button>
          <button onClick={() => setShowEOverdrachtModal(true)} className="button">
            eOvedracht
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

            {/* eOvedracht Tasks Card */}
            <div className="card" style={{ marginTop: '20px' }}>
              <h2 style={{ marginTop: 0 }}>📋 eOvedracht Tasks</h2>
              <p style={{ color: '#666', fontSize: '14px', marginBottom: '15px' }}>
                Care handover tasks for this patient
              </p>

              {eOverdrachtTasksLoading ? (
                <div className="loading-container" style={{ padding: '30px' }}>
                  <div className="spinner"></div>
                  <p>Loading eOvedracht tasks...</p>
                </div>
              ) : eOverdrachtTasksError ? (
                <div className="error-message">
                  <strong>Error loading eOvedracht tasks</strong>
                  <p>{eOverdrachtTasksError}</p>
                </div>
              ) : eOverdrachtTasks.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No eOvedracht tasks found</p>
                </div>
              ) : (
                <div className="care-network-list">
                  {eOverdrachtTasks.map((task, idx) => {
                    // Extract organization ID from task owner reference
                    const orgRef = task.owner?.reference;
                    const orgId = orgRef ? (orgRef.includes('/') ? orgRef.split('/').pop() : orgRef) : null;
                    const organization = orgId ? taskOrganizations[orgId] : null;

                    return (
                      <div key={task.id || idx} className="care-network-item">
                        <div className="care-network-item-header">
                          <span className="care-network-org-name">
                            {task.owner?.display || 'Unknown Organization'}
                          </span>
                          <span className={`status-badge ${task.status === 'in-progress' ? 'status-active' : task.status === 'completed' ? 'status-inactive' : ''}`}>
                            {task.status || 'unknown'}
                          </span>
                        </div>

                        {/* Organization Details */}
                        {organization && (
                          <div style={{ marginTop: '10px', fontSize: '13px', color: '#666' }}>
                            {organizationApi.getURA(organization) && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>URA:</strong> {organizationApi.getURA(organization)}
                              </div>
                            )}
                            {organizationApi.formatAddress(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Address:</strong> {organizationApi.formatAddress(organization)}
                              </div>
                            )}
                            {organizationApi.formatType(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Type:</strong> {organizationApi.formatType(organization)}
                              </div>
                            )}
                            {organizationApi.formatTelecomString(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Contact:</strong> {organizationApi.formatTelecomString(organization)}
                              </div>
                            )}
                          </div>
                        )}

                        {/* Requester Information */}
                        {task.requester?.agent?.display && (
                          <div style={{ fontSize: '13px', color: '#666', marginTop: '8px' }}>
                            <strong>Requested by:</strong> {task.requester.agent.display}
                          </div>
                        )}

                        {/* Created Date */}
                        {task.authoredOn && (
                          <div style={{ fontSize: '13px', color: '#666', marginTop: '5px' }}>
                            <strong>Created:</strong> {new Date(task.authoredOn).toLocaleDateString('nl-NL', {
                              year: 'numeric',
                              month: 'long',
                              day: 'numeric',
                              hour: '2-digit',
                              minute: '2-digit'
                            })}
                          </div>
                        )}

                        {/* Task ID */}
                        {task.id && (
                          <div
                            onClick={() => setExpandedTaskJson(expandedTaskJson === task.id ? null : task.id)}
                            style={{
                              fontSize: '12px',
                              color: '#2563eb',
                              marginTop: '5px',
                              cursor: 'pointer',
                              padding: '4px 8px',
                              backgroundColor: '#f0f4ff',
                              borderRadius: '4px',
                              display: 'inline-block',
                              transition: 'all 0.2s'
                            }}
                            onMouseEnter={(e) => {
                              e.currentTarget.style.backgroundColor = '#dbeafe';
                              e.currentTarget.style.textDecoration = 'underline';
                            }}
                            onMouseLeave={(e) => {
                              e.currentTarget.style.backgroundColor = '#f0f4ff';
                              e.currentTarget.style.textDecoration = 'none';
                            }}
                          >
                            Task ID: {task.id} {expandedTaskJson === task.id ? '▼' : '▶'}
                          </div>
                        )}

                        {/* Task JSON Display */}
                        {expandedTaskJson === task.id && (
                          <div style={{ marginTop: '10px', backgroundColor: '#f8f9fa', borderRadius: '4px', padding: '10px' }}>
                            <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#333' }}>
                              Task JSON:
                            </div>
                            <pre style={{
                              backgroundColor: '#2d2d2d',
                              color: '#f8f8f2',
                              padding: '12px',
                              borderRadius: '4px',
                              fontSize: '12px',
                              overflow: 'auto',
                              maxHeight: '400px',
                              margin: 0,
                              fontFamily: 'Monaco, Consolas, "Courier New", monospace'
                            }}>
                              {JSON.stringify(task, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>
                    );
                  })}
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

        {/* eOvedracht Modal */}
        {showEOverdrachtModal && (
          <div className="modal-overlay" onClick={handleCloseModal}>
            <div className="modal" style={{ maxWidth: '1000px' }} onClick={(e) => e.stopPropagation()}>
              {eOverdrachtStep === 'services' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>Step 1: Select Healthcare Service</h3>

                  <div style={{ marginTop: '20px' }}>
                    {healthcareServicesLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading healthcare services...</p>
                      </div>
                    ) : healthcareServicesError ? (
                      <div className="error-message">
                        <strong>Error loading healthcare services</strong>
                        <p>{healthcareServicesError}</p>
                      </div>
                    ) : groupedHealthcareServices.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No healthcare services found</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Service Name</th>
                              <th>Count</th>
                              <th>Type(s)</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {groupedHealthcareServices.map((group, idx) => {
                              const types = group.types !== '-' ? group.types.split(', ') : ['-'];

                              return (
                                <tr key={`${group.name}-${idx}`}>
                                  <td>
                                    <strong>{group.name}</strong>
                                  </td>
                                  <td>
                                    <span style={{ color: '#666', fontSize: '14px' }}>
                                      {group.count} {group.count === 1 ? 'service' : 'services'}
                                    </span>
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, tidx) => (
                                          <li key={tidx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td>
                                    <span className={`status-badge ${group.hasActive ? 'status-active' : 'status-inactive'}`}>
                                      {group.hasActive ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleServiceSelect(group)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : eOverdrachtStep === 'organizations' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>
                    Step 2: Select Organization
                    {selectedServiceGroup && (
                      <div style={{ fontSize: '14px', fontWeight: 'normal', color: '#666', marginTop: '8px' }}>
                        Service: {selectedServiceGroup.name}
                      </div>
                    )}
                  </h3>

                  <div style={{ marginTop: '20px' }}>
                    {organizationsLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading organizations...</p>
                      </div>
                    ) : organizationsError ? (
                      <div className="error-message">
                        <strong>Error loading organizations</strong>
                        <p>{organizationsError}</p>
                      </div>
                    ) : organizations.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No organizations found for this service</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Name</th>
                              <th>URA</th>
                              <th>Type</th>
                              <th>Address</th>
                              <th>Contact</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {organizations.map((org) => {
                              const ura = organizationApi.getURA(org);
                              const address = organizationApi.formatAddress(org);
                              const type = organizationApi.formatType(org);
                              const types = type !== '-' ? type.split(', ') : ['-'];
                              const telecom = organizationApi.formatTelecomString(org);

                              return (
                                <tr key={org.id}>
                                  <td>
                                    <strong>{organizationApi.formatName(org)}</strong>
                                  </td>
                                  <td>
                                    {ura ? (
                                      <span className="bsn-badge">{ura}</span>
                                    ) : (
                                      <span style={{ color: '#999' }}>-</span>
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, idx) => (
                                          <li key={idx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>{address}</td>
                                  <td style={{ fontSize: '13px' }}>{telecom}</td>
                                  <td>
                                    <span className={`status-badge ${organizationApi.isActive(org) ? 'status-active' : 'status-inactive'}`}>
                                      {organizationApi.isActive(org) ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleOrganizationSelect(org)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToServices}
                    >
                      ← Back to Services
                    </button>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : eOverdrachtStep === 'departments' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>
                    Step 3: Select Department
                    {selectedOrganization && (
                      <div style={{ fontSize: '14px', fontWeight: 'normal', color: '#666', marginTop: '8px' }}>
                        Organization: {organizationApi.formatName(selectedOrganization)}
                      </div>
                    )}
                  </h3>

                  <div style={{ marginTop: '20px' }}>
                    {departmentsLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading departments...</p>
                      </div>
                    ) : departmentsError ? (
                      <div className="error-message">
                        <strong>Error loading departments</strong>
                        <p>{departmentsError}</p>
                      </div>
                    ) : departments.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No departments found for this organization</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Department Name</th>
                              <th>URA</th>
                              <th>Type</th>
                              <th>Address</th>
                              <th>Contact</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {departments.map((dept) => {
                              const ura = organizationApi.getURA(dept);
                              const address = organizationApi.formatAddress(dept);
                              const type = organizationApi.formatType(dept);
                              const types = type !== '-' ? type.split(', ') : ['-'];
                              const telecom = organizationApi.formatTelecomString(dept);

                              return (
                                <tr key={dept.id}>
                                  <td>
                                    <strong>{organizationApi.formatName(dept)}</strong>
                                  </td>
                                  <td>
                                    {ura ? (
                                      <span className="bsn-badge">{ura}</span>
                                    ) : (
                                      <span style={{ color: '#999' }}>-</span>
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, idx) => (
                                          <li key={idx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>{address}</td>
                                  <td style={{ fontSize: '13px' }}>{telecom}</td>
                                  <td>
                                    <span className={`status-badge ${organizationApi.isActive(dept) ? 'status-active' : 'status-inactive'}`}>
                                      {organizationApi.isActive(dept) ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleDepartmentSelect(dept)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToOrganizations}
                    >
                      ← Back to Organizations
                    </button>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <h3 style={{ marginTop: 0 }}>Confirm eOvedracht Task Creation</h3>

                  <div style={{ marginTop: '20px' }}>
                    <div style={{ padding: '20px', backgroundColor: '#f8f9fa', borderRadius: '8px', border: '1px solid #dee2e6' }}>
                      <h4 style={{ marginTop: 0, marginBottom: '20px', color: '#495057' }}>Summary</h4>

                      <div style={{ display: 'grid', gap: '15px' }}>
                        <div>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Patient
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {patientName}
                          </div>
                          <div style={{ fontSize: '13px', color: '#6c757d' }}>
                            ID: {patientId}
                          </div>
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Healthcare Service
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedServiceGroup?.name}
                          </div>
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Organization
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedOrganization && organizationApi.formatName(selectedOrganization)}
                          </div>
                          {selectedOrganization && organizationApi.getURA(selectedOrganization) && (
                            <div style={{ fontSize: '13px', color: '#6c757d' }}>
                              URA: {organizationApi.getURA(selectedOrganization)}
                            </div>
                          )}
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Department
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedDepartment && organizationApi.formatName(selectedDepartment)}
                          </div>
                          {selectedDepartment && organizationApi.getURA(selectedDepartment) && (
                            <div style={{ fontSize: '13px', color: '#6c757d' }}>
                              URA: {organizationApi.getURA(selectedDepartment)}
                            </div>
                          )}
                          {selectedDepartment && (
                            <div style={{ fontSize: '13px', color: '#6c757d', marginTop: '5px' }}>
                              {organizationApi.formatAddress(selectedDepartment)}
                            </div>
                          )}
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Requested By
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {user?.profile?.name || user?.profile?.email || 'Unknown User'}
                          </div>
                        </div>
                      </div>

                      <div style={{ marginTop: '20px', padding: '15px', backgroundColor: '#fff3cd', border: '1px solid #ffc107', borderRadius: '6px' }}>
                        <div style={{ fontSize: '14px', color: '#856404' }}>
                          <strong>Note:</strong> By confirming, an eOvedracht Task will be created with status "in-progress" for the care handover process.
                        </div>
                      </div>
                    </div>

                    {/* Task creation feedback */}
                    {taskSuccess && (
                      <div style={{ marginTop: '20px', padding: '15px', backgroundColor: '#d4edda', border: '1px solid #c3e6cb', borderRadius: '8px', color: '#155724' }}>
                        <strong>Success!</strong> eOvedracht Task has been created successfully. Closing...
                      </div>
                    )}

                    {taskError && (
                      <div className="error-message" style={{ marginTop: '20px' }}>
                        <strong>Error creating Task</strong>
                        <p>{taskError}</p>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToDepartments}
                      disabled={taskCreating}
                    >
                      ← Back to Departments
                    </button>
                    <div style={{ display: 'flex', gap: '10px' }}>
                      <button
                        type="button"
                        className="button button-secondary"
                        onClick={handleCloseModal}
                        disabled={taskCreating}
                      >
                        Cancel
                      </button>
                      <button
                        type="button"
                        className="button"
                        onClick={handleConfirmTask}
                        disabled={taskCreating}
                        style={{
                          backgroundColor: taskCreating ? '#6c757d' : '#28a745',
                          borderColor: taskCreating ? '#6c757d' : '#28a745'
                        }}
                      >
                        {taskCreating ? 'Creating Task...' : 'Confirm & Create Task'}
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

export default PatientPage;

