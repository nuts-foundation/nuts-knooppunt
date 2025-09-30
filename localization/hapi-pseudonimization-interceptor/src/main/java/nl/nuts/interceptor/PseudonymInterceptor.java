package nl.nuts.interceptor;

import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import ca.uhn.fhir.rest.api.server.IPreResourceShowDetails;
import ca.uhn.fhir.rest.server.servlet.ServletRequestDetails;
import java.util.List;
import lombok.extern.slf4j.Slf4j;
import nl.nuts.util.BsnUtil;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.r4.model.DocumentReference;
import org.springframework.stereotype.Component;

@Component
@Interceptor
@Slf4j
public class PseudonymInterceptor {

    private static final String PSEUDO_BSN_SYSTEM = System.getenv().getOrDefault(
            "PSEUDO_BSN_SYSTEM", "http://example.com/pseudoBSN");
    private static final String BSN_TOKEN_SYSTEM = System.getenv().getOrDefault(
            "BSN_TOKEN_SYSTEM", "http://example.com/BSNToken");
    private static final String REQUESTOR_URA_HEADER = "X-Requestor-URA";

    private final BsnUtil bsnUtil;

    public PseudonymInterceptor() {
        this.bsnUtil = new BsnUtil();
        log.info("Initialized PseudonymInterceptor with BsnUtil");
    }

    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void resourceCreated(final ServletRequestDetails requestDetails, final IBaseResource newResource) {
        if (newResource instanceof final DocumentReference documentReference) {
            log.debug("Intercepting DocumentReference resource creation");
            deTokenizePseudonym(documentReference);
        }
    }

    @Hook(Pointcut.STORAGE_PRESHOW_RESOURCES)
    public void handleResponse(final IPreResourceShowDetails requestDetails) {
        final List<IBaseResource> allResources = requestDetails.getAllResources();
        for (final IBaseResource aResoruce : allResources) {
            if (aResoruce instanceof final DocumentReference documentReference) {
                tokenizePseudonym(documentReference, "requestorURA");
            }
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

    private void deTokenizePseudonym(final DocumentReference docRef) {
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
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token);
        log.debug("Converted token to pseudonym: {}", pseudonym);
        return pseudonym;
    }

    private String pseudonymToToken(final String pseudonym, final String requestorURA) {
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, requestorURA);
        log.debug("Converted pseudonym to token: {}", token);
        return token;
    }
}



