package nl.nuts.interceptor;

import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import ca.uhn.fhir.rest.server.servlet.ServletRequestDetails;
import nl.nuts.pseudonyms.ExchangeTokenRequest;
import nl.nuts.pseudonyms.ExchangeTokenResponse;
import nl.nuts.pseudonyms.GetTokenRequest;
import nl.nuts.pseudonyms.GetTokenResponse;
import nl.nuts.pseudonyms.Identifier;
import nl.nuts.pseudonyms.PseudoniemenServiceClient;
import lombok.extern.slf4j.Slf4j;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.r4.model.Bundle;
import org.hl7.fhir.r4.model.DocumentReference;
import org.springframework.stereotype.Component;

@Component
@Interceptor
@Slf4j
public class PseudonymInterceptor {
    private static final String PSEUDONYM_SERVICE_URL = System.getenv().getOrDefault(
            "PSEUDONYM_SERVICE_URL", "http://host.docker.internal:8082");
    private static final String PSEUDO_BSN_SYSTEM = System.getenv().getOrDefault(
            "PSEUDO_BSN_SYSTEM", "http://example.com/pseudoBSN");
    private static final String BSN_TOKEN_SYSTEM = System.getenv().getOrDefault(
            "BSN_TOKEN_SYSTEM", "http://example.com/BSNToken");
    private static final String IDENTIFIER_TYPE = System.getenv().getOrDefault(
            "IDENTIFIER_TYPE", "ORGANISATION_PSEUDO");
    private static final String SCOPE = System.getenv().getOrDefault("SCOPE", "localization");
    private static final String ORGANISATION = System.getenv().getOrDefault("ORGANISATION", "NVI");
    private static final String REQUESTOR_URA_HEADER = "X-Requestor-URA";

    private final PseudoniemenServiceClient client;

    public PseudonymInterceptor() {
        this.client = new PseudoniemenServiceClient(PSEUDONYM_SERVICE_URL);
        log.info("Initialized PseudonymInterceptor with service URL: {}", PSEUDONYM_SERVICE_URL);
    }

    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void resourceCreated(final ServletRequestDetails requestDetails, final IBaseResource newResource) {
        log.debug("Intercepting resource creation");
        if (newResource instanceof DocumentReference || newResource instanceof Bundle) {
            deTokenizePseudonym(newResource);
        }
    }

    @Hook(Pointcut.SERVER_OUTGOING_RESPONSE)
    public void handleResponse(final ServletRequestDetails requestDetails, final IBaseResource resource) {
        final String requestorURA = requestDetails.getHeader(REQUESTOR_URA_HEADER);

        if (resource instanceof final DocumentReference documentReference) {
            tokenizePseudonym(documentReference, requestorURA);
        } else if (resource instanceof Bundle) {
            final Bundle bundle = (Bundle) resource;
            bundle.getEntry().stream()
                    .map(Bundle.BundleEntryComponent::getResource)
                    .filter(DocumentReference.class::isInstance)
                    .map(DocumentReference.class::cast)
                    .forEach(document -> tokenizePseudonym(document, requestorURA));
        }
    }

    private void tokenizePseudonym(final DocumentReference resource, final String requestorURA) {
        final org.hl7.fhir.r4.model.Identifier identifier = resource.getSubject().getIdentifier();

        if (identifier == null) {
            return;
        }

        log.debug("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());

        if (PSEUDO_BSN_SYSTEM.equals(identifier.getSystem())) {
            log.info("Converting pseudoBSN to BSNToken");
            final String token = pseudonymToToken(identifier.getValue(), requestorURA);
            identifier.setSystem(BSN_TOKEN_SYSTEM);
            identifier.setValue(token);
        }
    }

    private void deTokenizePseudonym(final IBaseResource resource) {
        if (!(resource instanceof DocumentReference)) {
            return;
        }

        final DocumentReference docRef = (DocumentReference) resource;
        final org.hl7.fhir.r4.model.Identifier identifier = docRef.getSubject().getIdentifier();

        if (identifier == null) {
            return;
        }

        log.debug("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());

        if (BSN_TOKEN_SYSTEM.equals(identifier.getSystem())) {
            log.info("Converting BSNToken to pseudoBSN");
            final String pseudonym = tokenToPseudonym(identifier.getValue());
            identifier.setSystem(PSEUDO_BSN_SYSTEM);
            identifier.setValue(pseudonym);
        }
    }

    private String tokenToPseudonym(final String token) {
        final ExchangeTokenRequest request = new ExchangeTokenRequest(token, IDENTIFIER_TYPE, SCOPE, ORGANISATION);

        try {
            final ExchangeTokenResponse response = client.exchangeToken(request);
            log.info("Received pseudonym: {}", response.getIdentifier().getValue());
            return response.getIdentifier().getValue();
        } catch (final Exception e) {
            log.error("Failed to exchange token for pseudonym", e);
            return "error-pseudonym";
        }
    }

    private String pseudonymToToken(final String pseudonym, final String requestorURA) {
        final Identifier identifier = new Identifier(pseudonym, IDENTIFIER_TYPE);
        final GetTokenRequest request = new GetTokenRequest(identifier, requestorURA, SCOPE, ORGANISATION);

        try {
            final GetTokenResponse response = client.getToken(request);
            log.info("Received token: {}", response.getToken());
            return response.getToken();
        } catch (final Exception e) {
            log.error("Failed to get token for pseudonym", e);
            return "error-token";
        }
    }
}



