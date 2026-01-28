import { getIssuerHostname } from '@/lib/utils';

export const dynamic = 'force-dynamic';

export default function Home() {
  // Derive base URL from ISSUER_HOSTNAME (the single source of truth)
  const hostname = getIssuerHostname();
  const isLocalhost = hostname.startsWith('localhost');
  const protocol = isLocalhost ? 'http' : 'https';
  const baseUrl = `${protocol}://${hostname}`;

  return (
    <div className="min-h-screen bg-gray-100">
      {/* Header */}
      <header className="bg-white shadow-sm">
        <div className="max-w-4xl mx-auto px-4 py-6">
          <div className="flex items-center space-x-4">
            <div className="w-12 h-12 bg-blue-600 rounded-lg flex items-center justify-center">
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
                  d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                />
              </svg>
            </div>
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Vektis VC Issuer</h1>
              <p className="text-sm text-gray-500">
                OID4VCI Credential Issuance Service
              </p>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-4xl mx-auto px-4 py-8">
        {/* Status Card */}
        <div className="bg-white rounded-xl shadow-sm p-6 mb-6">
          <div className="flex items-center space-x-3 mb-4">
            <div className="w-3 h-3 bg-green-500 rounded-full animate-pulse"></div>
            <span className="text-green-700 font-medium">Service Online</span>
          </div>
          <p className="text-gray-600">
            This service issues HealthcareProviderTypeCredentials using the OpenID for Verifiable
            Credential Issuance (OID4VCI) protocol.
          </p>
        </div>

        {/* Endpoints Card */}
        <div className="bg-white rounded-xl shadow-sm p-6 mb-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">
            Discovery Endpoints
          </h2>
          <div className="space-y-3">
            <EndpointLink
              label="Credential Issuer Metadata"
              url={`${baseUrl}/.well-known/openid-credential-issuer`}
            />
            <EndpointLink
              label="OAuth2 Authorization Server Metadata (RFC 8414)"
              url={`${baseUrl}/.well-known/oauth-authorization-server`}
            />
            <EndpointLink
              label="OpenID Configuration"
              url={`${baseUrl}/.well-known/openid-configuration`}
            />
            <EndpointLink
              label="DID Document"
              url={`${baseUrl}/.well-known/did.json`}
            />
          </div>
        </div>

        {/* API Endpoints Card */}
        <div className="bg-white rounded-xl shadow-sm p-6 mb-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">
            OID4VCI Endpoints
          </h2>
          <div className="space-y-3">
            <EndpointInfo
              method="GET"
              path="/api/oidc4vci/authorize"
              description="Authorization endpoint - initiates credential issuance flow"
            />
            <EndpointInfo
              method="POST"
              path="/api/oidc4vci/token"
              description="Token endpoint - exchanges authorization code for access token"
            />
            <EndpointInfo
              method="POST"
              path="/api/oidc4vci/credential"
              description="Credential endpoint - issues signed HealthcareProviderTypeCredential"
            />
          </div>
        </div>

        {/* Supported Credentials Card */}
        <div className="bg-white rounded-xl shadow-sm p-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">
            Supported Credentials
          </h2>
          <div className="border rounded-lg p-4">
            <div className="flex items-start space-x-4">
              <div className="w-12 h-12 bg-blue-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <svg
                  className="w-6 h-6 text-blue-600"
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
              <div>
                <h3 className="font-semibold text-gray-900">HealthcareProviderTypeCredential</h3>
                <p className="text-sm text-gray-600 mt-1">
                  Organization identification credential containing organization name and healthcare provider type.
                </p>
                <div className="flex flex-wrap gap-2 mt-3">
                  <span className="px-2 py-1 bg-gray-100 text-gray-600 text-xs rounded">
                    jwt_vc_json
                  </span>
                  <span className="px-2 py-1 bg-gray-100 text-gray-600 text-xs rounded">
                    EdDSA
                  </span>
                  <span className="px-2 py-1 bg-gray-100 text-gray-600 text-xs rounded">
                    did:web
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="max-w-4xl mx-auto px-4 py-8 text-center text-sm text-gray-500">
        <p>Mock VC Issuer for Nuts Foundation PoC</p>
        <p className="mt-1">
          <a
            href="https://github.com/nuts-foundation/nuts-knooppunt/issues/196"
            className="text-blue-600 hover:underline"
            target="_blank"
            rel="noopener noreferrer"
          >
            Issue #196
          </a>
        </p>
      </footer>
    </div>
  );
}

function EndpointLink({ label, url }: { label: string; url: string }) {
  return (
    <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
      <span className="text-gray-700">{label}</span>
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        className="text-blue-600 hover:text-blue-800 text-sm font-mono truncate ml-4"
      >
        {url.replace(/^https?:\/\//, '')}
      </a>
    </div>
  );
}

function EndpointInfo({
  method,
  path,
  description,
}: {
  method: string;
  path: string;
  description: string;
}) {
  const methodColors: Record<string, string> = {
    GET: 'bg-green-100 text-green-700',
    POST: 'bg-blue-100 text-blue-700',
    PUT: 'bg-yellow-100 text-yellow-700',
    DELETE: 'bg-red-100 text-red-700',
  };

  return (
    <div className="p-3 bg-gray-50 rounded-lg">
      <div className="flex items-center space-x-3 mb-1">
        <span
          className={`px-2 py-0.5 text-xs font-medium rounded ${methodColors[method] || 'bg-gray-100 text-gray-700'}`}
        >
          {method}
        </span>
        <code className="text-sm font-mono text-gray-800">{path}</code>
      </div>
      <p className="text-sm text-gray-600 ml-12">{description}</p>
    </div>
  );
}
