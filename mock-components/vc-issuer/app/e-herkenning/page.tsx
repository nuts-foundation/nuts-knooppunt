'use client';

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { getOrganizationTypeOptions, type CareOrganizationType } from '@/lib/vektis/care-organization-types';

function EHerkenningContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const state = searchParams.get('state');

  const [isLoading, setIsLoading] = useState(false);
  const [step, setStep] = useState<'login' | 'select' | 'consent'>('login');
  const [error, setError] = useState<string | null>(null);

  // Get organization type options from the centralized list
  const organizationTypeOptions = getOrganizationTypeOptions();

  const [selectedType, setSelectedType] = useState<CareOrganizationType>(organizationTypeOptions[0].code);
  const [selectedTypeLabel, setSelectedTypeLabel] = useState(organizationTypeOptions[0].label);

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

  const handleSubmit = () => {
    setStep('consent');
  };

  const handleTypeChange = (type: string) => {
    const typeObj = organizationTypeOptions.find(t => t.code === type);
    setSelectedType(type as CareOrganizationType);
    setSelectedTypeLabel(typeObj?.label || type);
  };

  const handleConsent = async () => {
    if (!state) return;

    setIsLoading(true);

    // Send only the healthcare provider type
    const callbackUrl = `/api/oidc4vci/authorize/callback?state=${encodeURIComponent(state)}&organizationType=${encodeURIComponent(selectedType)}`;

    router.push(callbackUrl);
  };

  const handleBack = () => {
    if (step === 'consent') {
      setStep('select');
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

            {/* Step 2: Select Healthcare Provider Type */}
            {step === 'select' && (
              <div>
                <h2 className="text-lg font-semibold text-gray-900 mb-2">
                  Zorgaanbiedertype Selecteren
                </h2>
                <p className="text-gray-600 mb-4">
                  Selecteer het type zorgaanbieder.
                </p>

                {/* Suggested Organization Types Info Box */}
                <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-4">
                  <div className="flex">
                    <svg
                      className="w-5 h-5 text-blue-500 mr-2 shrink-0 mt-0.5"
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
                    <div className="text-sm text-blue-700 w-full">
                      <p className="font-medium mb-2">Veelgebruikte zorgaanbiedertypen:</p>
                      <div className="grid grid-cols-2 gap-2">
                        <button
                          type="button"
                          onClick={() => handleTypeChange('A1')}
                          className="text-left px-3 py-2 bg-white border border-blue-200 rounded hover:bg-blue-100 hover:border-blue-300 transition-colors"
                        >
                          <strong>A1</strong> - Apotheek
                        </button>
                        <button
                          type="button"
                          onClick={() => handleTypeChange('H1')}
                          className="text-left px-3 py-2 bg-white border border-blue-200 rounded hover:bg-blue-100 hover:border-blue-300 transition-colors"
                        >
                          <strong>H1</strong> - Huisartsinstelling
                        </button>
                        <button
                          type="button"
                          onClick={() => handleTypeChange('V4')}
                          className="text-left px-3 py-2 bg-white border border-blue-200 rounded hover:bg-blue-100 hover:border-blue-300 transition-colors"
                        >
                          <strong>V4</strong> - Ziekenhuis
                        </button>
                        <button
                          type="button"
                          onClick={() => handleTypeChange('R5')}
                          className="text-left px-3 py-2 bg-white border border-blue-200 rounded hover:bg-blue-100 hover:border-blue-300 transition-colors"
                        >
                          <strong>R5</strong> - Verpleeghuis
                        </button>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Zorgaanbiedertype *
                    </label>
                    <select
                      value={selectedType}
                      onChange={(e) => handleTypeChange(e.target.value)}
                      className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent"
                    >
                      {organizationTypeOptions.map((type) => (
                        <option key={type.code} value={type.code}>
                          {type.code} - {type.label}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div className="pt-2">
                    <button
                      onClick={handleSubmit}
                      className="w-full py-3 px-4 bg-orange-500 text-white font-medium rounded-lg hover:bg-orange-600 transition-colors"
                    >
                      Doorgaan
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Step 3: Consent */}
            {step === 'consent' && (
              <div>
                <h2 className="text-lg font-semibold text-gray-900 mb-2">
                  Bevestig Credential Aanvraag
                </h2>
                <p className="text-gray-600 mb-4">
                  U staat op het punt een HealthcareProviderTypeCredential aan te maken voor:
                </p>

                <div className="bg-gray-50 rounded-lg p-4 mb-6">
                  <dl className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <dt className="text-gray-500">Zorgaanbiedertype:</dt>
                      <dd className="text-gray-900 font-medium">
                        {selectedType} - {selectedTypeLabel}
                      </dd>
                    </div>
                  </dl>
                </div>

                <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
                  <div className="flex">
                    <svg
                      className="w-5 h-5 text-blue-500 mr-2 shrink-0"
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
