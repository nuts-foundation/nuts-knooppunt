/**
 * OID4VCI Issuer Metadata
 */

export interface CredentialIssuerMetadata {
  credential_issuer: string;
  authorization_servers?: string[];
  credential_endpoint: string;
  display?: Array<{
    name: string;
    locale?: string;
    logo?: {
      uri: string;
      alt_text?: string;
    };
  }>;
  credential_configurations_supported: Record<string, CredentialConfiguration>;
}

export interface CredentialConfiguration {
  format: string;
  scope?: string;
  cryptographic_binding_methods_supported?: string[];
  credential_signing_alg_values_supported?: string[];
  credential_definition: {
    type: string[];
    credentialSubject?: Record<string, CredentialSubjectField>;
  };
  display?: Array<{
    name: string;
    locale?: string;
    description?: string;
    background_color?: string;
    text_color?: string;
  }>;
}

export interface CredentialSubjectField {
  mandatory?: boolean;
  display?: Array<{
    name: string;
    locale?: string;
  }>;
}

export function getCredentialIssuerMetadata(baseUrl: string, issuerDid: string): CredentialIssuerMetadata {
  return {
    credential_issuer: issuerDid,
    authorization_servers: [baseUrl],
    credential_endpoint: `${baseUrl}/api/oidc4vci/credential`,
    display: [
      {
        name: 'Vektis Credential Issuer',
        locale: 'nl-NL',
      },
      {
        name: 'Vektis Credential Issuer',
        locale: 'en-US',
      },
    ],
    credential_configurations_supported: {
      HealthcareProviderTypeCredential: {
        format: 'jwt_vc_json',
        scope: 'HealthcareProviderTypeCredential',
        cryptographic_binding_methods_supported: ['did:web', 'did:jwk'],
        credential_signing_alg_values_supported: ['EdDSA'],
        credential_definition: {
          type: ['VerifiableCredential', 'HealthcareProviderTypeCredential'],
          credentialSubject: {
            organizationType: {
              mandatory: true,
              display: [
                { name: 'Organisatie Type', locale: 'nl-NL' },
                { name: 'Organization Type', locale: 'en-US' },
              ],
            },
          },
        },
        display: [
          {
            name: 'Vektis Organisatie Credential',
            locale: 'nl-NL',
            description: 'Een credential dat de organisatie-identificatie bevat',
            background_color: '#12107c',
            text_color: '#FFFFFF',
          },
          {
            name: 'Vektis Organization Credential',
            locale: 'en-US',
            description: 'A credential containing organization identification',
            background_color: '#12107c',
            text_color: '#FFFFFF',
          },
        ],
      },
    },
  };
}

/**
 * OAuth 2.0 Authorization Server Metadata (RFC 8414)
 * https://www.rfc-editor.org/rfc/rfc8414.html
 */
export interface AuthorizationServerMetadata {
  // RFC 8414 required fields
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;

  // RFC 8414 recommended/optional fields
  jwks_uri?: string;
  registration_endpoint?: string;
  scopes_supported?: string[];
  response_types_supported: string[];
  response_modes_supported?: string[];
  grant_types_supported: string[];
  token_endpoint_auth_methods_supported: string[];
  token_endpoint_auth_signing_alg_values_supported?: string[];
  service_documentation?: string;
  ui_locales_supported?: string[];
  op_policy_uri?: string;
  op_tos_uri?: string;
  revocation_endpoint?: string;
  revocation_endpoint_auth_methods_supported?: string[];
  introspection_endpoint?: string;
  introspection_endpoint_auth_methods_supported?: string[];
  code_challenge_methods_supported?: string[];

  // OID4VCI extensions
  authorization_details_types_supported?: string[];
  pre_authorized_grant_anonymous_access_supported?: boolean;
  client_id_schemes_supported?: string[];
}

export function getAuthorizationServerMetadata(baseUrl: string): AuthorizationServerMetadata {
  return {
    // RFC 8414 required fields
    issuer: baseUrl,
    authorization_endpoint: `${baseUrl}/api/oidc4vci/authorize`,
    token_endpoint: `${baseUrl}/api/oidc4vci/token`,

    // RFC 8414 recommended fields
    response_types_supported: ['code'],
    grant_types_supported: ['authorization_code'],
    token_endpoint_auth_methods_supported: ['none'],
    code_challenge_methods_supported: ['S256'],
    scopes_supported: ['HealthcareProviderTypeCredential'],

    // OID4VCI extensions
    authorization_details_types_supported: ['openid_credential'],
    pre_authorized_grant_anonymous_access_supported: false,
    client_id_schemes_supported: ['entity_id'],
  };
}
