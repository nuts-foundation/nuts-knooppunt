'use client';

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

interface Organization {
  id: string;
  name: string;
  type: string;
  typeLabel: string;
  agbCode: string;
  uraNumber: string;
}

const mockOrganizations: Organization[] = [
  {
    id: 'org-1',
    name: 'Apotheek De Zonnehoek',
    type: 'pharmacy',
    typeLabel: 'Apotheek',
    agbCode: '06010713',
    uraNumber: '32475534',
  },
  {
    id: 'org-2',
    name: 'Huisartsenpraktijk Centrum',
    type: 'general_practice',
    typeLabel: 'Huisartsenpraktijk',
    agbCode: '01234567',
    uraNumber: '12345678',
  },
  {
    id: 'org-3',
    name: 'Ziekenhuis Oost',
    type: 'hospital',
    typeLabel: 'Ziekenhuis',
    agbCode: '98765432',
    uraNumber: '87654321',
  },
  {
    id: 'org-4',
    name: 'Verpleeghuis De Rusthoeve',
    type: 'care_home',
    typeLabel: 'Verpleeghuis',
    agbCode: '11223344',
    uraNumber: '44332211',
  },
];

function EHerkenningContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const state = searchParams.get('state');

  const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [step, setStep] = useState<'login' | 'select' | 'consent'>('login');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!state) {
      setError('Ongeldige sessie. Probeer het opnieuw vanuit uw wallet.');
    }
  }, [state]);

  const handleLogin = () => {
    setIsLoading(true);
    // Simulate login delay
    setTimeout(() => {
      setStep('select');
      setIsLoading(false);
    }, 1500);
  };

  const handleOrgSelect = (org: Organization) => {
    setSelectedOrg(org);
    setStep('consent');
  };

  const handleConsent = async () => {
    if (!selectedOrg || !state) return;

    setIsLoading(true);

    // Redirect to callback with state and selected org
    const callbackUrl = `/api/oidc4vci/authorize/callback?state=${encodeURIComponent(state)}&org=${encodeURIComponent(selectedOrg.id)}`;
    router.push(callbackUrl);
  };

  const handleBack = () => {
    if (step === 'consent') {
      setStep('select');
      setSelectedOrg(null);
    } else if (step === 'select') {
      setStep('login');
    }
  };

  if (error) {
    return (
      <div className="min-h-screen bg-gray-100 flex items-center justify-center p-4">
        <div className="max-w-md w-full bg-white rounded-xl shadow-lg p-8">
          <div className="text-center">
            <div className="text-red-500 text-6xl mb-4">!</div>
            <h1 className="text-xl font-bold text-gray-900 mb-2">Fout</h1>
            <p className="text-gray-600">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-100 flex items-center justify-center p-4">
      <div className="max-w-md w-full">
        {/* Header */}
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-orange-500 rounded-full mb-4">
            <svg
              className="w-8 h-8 text-white"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"
              />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-gray-900">eHerkenning</h1>
          <p className="text-sm text-gray-500 mt-1">Mock Authenticatie Service</p>
        </div>

        {/* Card */}
        <div className="bg-white rounded-xl shadow-lg overflow-hidden">
          {/* Progress indicator */}
          <div className="bg-gray-50 px-6 py-3 border-b">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
                    step === 'login'
                      ? 'bg-orange-500 text-white'
                      : 'bg-green-500 text-white'
                  }`}
                >
                  {step === 'login' ? '1' : '✓'}
                </div>
                <span className="text-sm text-gray-600">Inloggen</span>
              </div>
              <div className="w-8 h-0.5 bg-gray-300" />
              <div className="flex items-center space-x-2">
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
                    step === 'select'
                      ? 'bg-orange-500 text-white'
                      : step === 'consent'
                      ? 'bg-green-500 text-white'
                      : 'bg-gray-300 text-gray-600'
                  }`}
                >
                  {step === 'consent' ? '✓' : '2'}
                </div>
                <span className="text-sm text-gray-600">Selecteer</span>
              </div>
              <div className="w-8 h-0.5 bg-gray-300" />
              <div className="flex items-center space-x-2">
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
                    step === 'consent'
                      ? 'bg-orange-500 text-white'
                      : 'bg-gray-300 text-gray-600'
                  }`}
                >
                  3
                </div>
                <span className="text-sm text-gray-600">Bevestig</span>
              </div>
            </div>
          </div>

          <div className="p-6">
            {/* Step 1: Login */}
            {step === 'login' && (
              <div className="text-center">
                <h2 className="text-lg font-semibold text-gray-900 mb-2">
                  Inloggen met eHerkenning
                </h2>
                <p className="text-gray-600 mb-6">
                  Log in om namens uw organisatie een Vektis credential aan te vragen.
                </p>
                <button
                  onClick={handleLogin}
                  disabled={isLoading}
                  className="w-full py-3 px-4 bg-orange-500 text-white font-medium rounded-lg hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {isLoading ? (
                    <span className="flex items-center justify-center">
                      <svg
                        className="animate-spin -ml-1 mr-3 h-5 w-5 text-white"
                        fill="none"
                        viewBox="0 0 24 24"
                      >
                        <circle
                          className="opacity-25"
                          cx="12"
                          cy="12"
                          r="10"
                          stroke="currentColor"
                          strokeWidth="4"
                        />
                        <path
                          className="opacity-75"
                          fill="currentColor"
                          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                        />
                      </svg>
                      Inloggen...
                    </span>
                  ) : (
                    'Inloggen met eHerkenning'
                  )}
                </button>
              </div>
            )}

            {/* Step 2: Select Organization */}
            {step === 'select' && (
              <div>
                <h2 className="text-lg font-semibold text-gray-900 mb-2">
                  Selecteer Organisatie
                </h2>
                <p className="text-gray-600 mb-4">
                  Kies de organisatie waarvoor u een credential wilt aanvragen.
                </p>
                <div className="space-y-2">
                  {mockOrganizations.map((org) => (
                    <button
                      key={org.id}
                      onClick={() => handleOrgSelect(org)}
                      className="w-full p-4 text-left border-2 border-gray-200 rounded-lg hover:border-orange-500 hover:bg-orange-50 transition-colors"
                    >
                      <div className="font-medium text-gray-900">{org.name}</div>
                      <div className="text-sm text-gray-500 mt-1">
                        {org.typeLabel} | AGB: {org.agbCode}
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Step 3: Consent */}
            {step === 'consent' && selectedOrg && (
              <div>
                <h2 className="text-lg font-semibold text-gray-900 mb-2">
                  Bevestig Credential Aanvraag
                </h2>
                <p className="text-gray-600 mb-4">
                  U staat op het punt een VektisOrgCredential aan te maken voor:
                </p>

                <div className="bg-gray-50 rounded-lg p-4 mb-6">
                  <h3 className="font-semibold text-gray-900 mb-3">
                    {selectedOrg.name}
                  </h3>
                  <dl className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <dt className="text-gray-500">Type:</dt>
                      <dd className="text-gray-900 font-medium">
                        {selectedOrg.typeLabel}
                      </dd>
                    </div>
                    <div className="flex justify-between">
                      <dt className="text-gray-500">AGB Code:</dt>
                      <dd className="text-gray-900 font-medium">
                        {selectedOrg.agbCode}
                      </dd>
                    </div>
                    <div className="flex justify-between">
                      <dt className="text-gray-500">URA Nummer:</dt>
                      <dd className="text-gray-900 font-medium">
                        {selectedOrg.uraNumber}
                      </dd>
                    </div>
                  </dl>
                </div>

                <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
                  <div className="flex">
                    <svg
                      className="w-5 h-5 text-blue-500 mr-2 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    <p className="text-sm text-blue-700">
                      Dit credential wordt toegevoegd aan uw wallet en kan worden
                      gebruikt om uw organisatie te identificeren.
                    </p>
                  </div>
                </div>

                <div className="flex gap-3">
                  <button
                    onClick={handleBack}
                    className="flex-1 py-3 px-4 border border-gray-300 text-gray-700 font-medium rounded-lg hover:bg-gray-50 transition-colors"
                  >
                    Terug
                  </button>
                  <button
                    onClick={handleConsent}
                    disabled={isLoading}
                    className="flex-1 py-3 px-4 bg-green-500 text-white font-medium rounded-lg hover:bg-green-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    {isLoading ? (
                      <span className="flex items-center justify-center">
                        <svg
                          className="animate-spin -ml-1 mr-3 h-5 w-5 text-white"
                          fill="none"
                          viewBox="0 0 24 24"
                        >
                          <circle
                            className="opacity-25"
                            cx="12"
                            cy="12"
                            r="10"
                            stroke="currentColor"
                            strokeWidth="4"
                          />
                          <path
                            className="opacity-75"
                            fill="currentColor"
                            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                          />
                        </svg>
                        Bezig...
                      </span>
                    ) : (
                      'Credential Aanmaken'
                    )}
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <p className="text-center text-xs text-gray-500 mt-4">
          Dit is een mock authenticatie service voor demonstratiedoeleinden.
        </p>
      </div>
    </div>
  );
}

export default function EHerkenningPage() {
  return (
    <Suspense
      fallback={
        <div className="min-h-screen bg-gray-100 flex items-center justify-center">
          <div className="animate-spin h-8 w-8 border-4 border-orange-500 border-t-transparent rounded-full"></div>
        </div>
      }
    >
      <EHerkenningContent />
    </Suspense>
  );
}
