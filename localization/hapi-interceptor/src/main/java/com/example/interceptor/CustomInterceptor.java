package com.example.fhirserver.interceptor;

import org.hl7.fhir.instance.model.api.IBaseResource;
import org.springframework.stereotype.Component;

import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import org.hl7.fhir.instance.model.api.IBaseResource;
import ca.uhn.fhir.rest.server.servlet.ServletRequestDetails;

import org.hl7.fhir.r4.model.*;

import com.example.fhirserver.pseudonyms.*;

@Component
@Interceptor
public class CustomInterceptor
{
  @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
  public void resourceCreated(ServletRequestDetails requestDetails, IBaseResource newResource)
  {
    System.out.println("YourInterceptor.resourceCreated");
    if (newResource instanceof DomainResource) {
      deTokenizePsuedonym((DomainResource) newResource);
    }
  }

  @Hook(value = Pointcut.SERVER_OUTGOING_RESPONSE)
  public void handleResponse(ServletRequestDetails requestDetails, IBaseResource resource) {
    String requestorURA = requestDetails.getHeader("X-Requestor-URA");

    if (resource instanceof DomainResource) {
      tokenizePsuedonym((DomainResource) resource, requestorURA);
    } else if (resource instanceof Bundle) {
      Bundle bundle = (Bundle) resource;
      for (Bundle.BundleEntryComponent entry : bundle.getEntry()) {
        if (entry.getResource() instanceof DomainResource) {
          tokenizePsuedonym((DomainResource) entry.getResource(), requestorURA);
        }
      }
    }
  }

  // Tokenize resources before sending them out so externally we communicate BSN-tokens specifically created for the requestor
  public void tokenizePsuedonym(DomainResource resource, String requestorURA) {
    org.hl7.fhir.r4.model.Identifier identifier = null;
    if (resource instanceof DocumentReference) {
      System.out.println("tokenizePs: DocumentReference found, checking subject identifier.");
      identifier = ((DocumentReference) resource).getSubject().getIdentifier();
    }

    if (identifier != null) {
      System.out.println("Identifier found: " + identifier.getSystem() + " | " + identifier.getValue());
      if ("http://example.com/pseudoBSN".equals(identifier.getSystem())) {
        System.out.println("Updating identifier system from pseudoBSN to BSNToken.");
        identifier.setSystem("http://example.com/BSNToken");
        String token = psuedonymToToken(identifier.getValue(), requestorURA);
        identifier.setValue(token);
      } else {
        System.out.println("Identifier is of type: " + identifier.getSystem() + " - no changes made.");
      }
    }
  }

  // De-tokenize resources before storing them so internally we have the local pseudoBSN
  public void deTokenizePsuedonym(DomainResource resource) {
    org.hl7.fhir.r4.model.Identifier identifier = null;
    if (resource instanceof DocumentReference) {
      System.out.println("deTokenizePs: DocumentReference found, checking subject identifier.");
      identifier = ((DocumentReference) resource).getSubject().getIdentifier();
    }

    if (identifier != null) {
      System.out.println("Identifier found: " + identifier.getSystem() + " | " + identifier.getValue());

      if ("http://example.com/BSNToken".equals(identifier.getSystem())) {
        System.out.println("Updating identifier system from BSN to pseudoBSN.");
        identifier.setSystem("http://example.com/pseudoBSN");
        String psuedonym = tokenToPsuedonym(identifier.getValue());
        identifier.setValue(psuedonym);
      } else {
        System.out.println("Identifier is of type: " + identifier.getSystem() + " - no changes made.");
      }
    }
  }

  public String tokenToPsuedonym(String token) {
    PseudoniemenServiceClient client = new PseudoniemenServiceClient("http://host.docker.internal:8082");;

    ExchangeTokenRequest request = new ExchangeTokenRequest(
        token,
        "ORGANISATION_PSEUDO",
        "localization",
        "NVI");

    try {
      ExchangeTokenResponse response = client.exchangeToken(request);
      System.out.println("Received pseudonym: " + response.identifier.value);
      return response.identifier.value;
    } catch (Exception e) {
      e.printStackTrace();
      return "error-pseudonym";
    }
  }

  public String psuedonymToToken(String psuedonym, String requestorURA) {
    PseudoniemenServiceClient client = new PseudoniemenServiceClient("http://host.docker.internal:8082");;

    GetTokenRequest request = new GetTokenRequest(
        new com.example.fhirserver.pseudonyms.Identifier(psuedonym, "ORGANISATION_PSEUDO"),
        requestorURA,
        "localization",
        "NVI");
    try {
      GetTokenResponse response = client.getToken(request);
      System.out.println("Received token: " + response.token);
      return response.token;
    } catch (Exception e) {
      e.printStackTrace();
      return "error-token";
    }
  }
}



