import { NextRequest, NextResponse } from 'next/server';
import prisma from '@/lib/prisma';
import { getOrganizationById } from '@/lib/mock-data/organizations';
import { jsonResponse } from '@/lib/utils';

/**
 * Callback endpoint after e-Herkenning authentication
 * Updates the authorization request with selected organization and redirects to wallet
 */
export async function GET(req: NextRequest) {
  const searchParams = req.nextUrl.searchParams;

  const state = searchParams.get('state');
  const orgId = searchParams.get('org');

  if (!state) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'state is required' },
      { status: 400 }
    );
  }

  if (!orgId) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'org is required' },
      { status: 400 }
    );
  }

  // Find the authorization request
  const authRequest = await prisma.authorizationRequest.findUnique({
    where: { state },
  });

  if (!authRequest) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request not found' },
      { status: 400 }
    );
  }

  // Check if expired
  if (new Date() > authRequest.expiresAt) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request has expired' },
      { status: 400 }
    );
  }

  // Check if already used
  if (authRequest.isUsed) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request has already been used' },
      { status: 400 }
    );
  }

  // Get the selected organization
  const organization = getOrganizationById(orgId);
  if (!organization) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Organization not found' },
      { status: 400 }
    );
  }

  // Update authorization request with authenticated organization
  await prisma.authorizationRequest.update({
    where: { state },
    data: {
      authenticatedOrg: JSON.stringify({
        id: organization.id,
        name: organization.name,
        type: organization.type,
        typeLabel: organization.typeLabel,
        agbCode: organization.agbCode,
        uraNumber: organization.uraNumber,
      }),
    },
  });

  // Build form POST to redirect with authorization code
  const redirectUri = authRequest.redirectUri;
  const code = authRequest.generatedCode;
  const authState = authRequest.state;

  // Return an HTML page with a form that auto-submits
  const html = `
    <!DOCTYPE html>
    <html lang="en">
      <head>
        <title>Redirecting...</title>
        <style>
          body {
            font-family: system-ui, -apple-system, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: #f5f5f5;
          }
          .container {
            background: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            text-align: center;
          }
          button {
            margin-top: 1rem;
            padding: 0.75rem 1.5rem;
            font-size: 1rem;
            background: #0070f3;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
          }
          button:hover {
            background: #0051cc;
          }
        </style>
      </head>
      <body>
        <div class="container">
          <p>Redirecting...</p>
          <form id="redirectForm" method="POST" action="${redirectUri}">
            <input type="hidden" name="code" value="${code}" />
            ${authState ? `<input type="hidden" name="state" value="${authState}" />` : ''}
            <button type="submit">Click here if you are not automatically redirected</button>
          </form>
        </div>
        <script>
          document.getElementById('redirectForm').submit();
        </script>
      </body>
    </html>
  `;

  return new NextResponse(html, {
    status: 200,
    headers: {
      'Content-Type': 'text/html',
    },
  });
}
