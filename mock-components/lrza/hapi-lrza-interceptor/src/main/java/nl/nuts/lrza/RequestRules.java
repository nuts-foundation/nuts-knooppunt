package nl.nuts.lrza;

import ca.uhn.fhir.rest.api.RestOperationTypeEnum;
import ca.uhn.fhir.rest.server.exceptions.BaseServerResponseException;
import ca.uhn.fhir.rest.server.exceptions.MethodNotAllowedException;
import ca.uhn.fhir.rest.server.exceptions.UnclassifiedServerFailureException;
import org.hl7.fhir.r4.model.Bundle;

/**
 * How the LRZA wants writes made: a resource is withdrawn by a status change rather than deleted, and an
 * update names the version it replaces (If-Match), so concurrent writers don't overwrite each other.
 */
final class RequestRules {

    static final int PRECONDITION_REQUIRED = 428;

    private RequestRules() {
    }

    /** Checks a single (non-transaction) request. */
    static void check(final RestOperationTypeEnum operation, final String ifMatch) {
        if (operation == null) {
            return;
        }
        switch (operation) {
            case DELETE -> throw deleteNotAllowed("");
            case UPDATE -> {
                if (ifMatch == null || ifMatch.isBlank()) {
                    throw ifMatchRequired("");
                }
            }
            default -> {
            }
        }
    }

    /** Checks every entry of a transaction or batch. */
    static void check(final Bundle bundle) {
        for (int i = 0; i < bundle.getEntry().size(); i++) {
            final Bundle.BundleEntryRequestComponent request = bundle.getEntry().get(i).getRequest();
            final String where = String.format("Bundle.entry[%d]: ", i);
            if (request.getMethod() == Bundle.HTTPVerb.DELETE) {
                throw deleteNotAllowed(where);
            }
            if (request.getMethod() == Bundle.HTTPVerb.PUT && !request.hasIfMatch()) {
                throw ifMatchRequired(where);
            }
        }
    }

    private static BaseServerResponseException deleteNotAllowed(final String where) {
        return new MethodNotAllowedException(where
                + "The LRZA does not delete resources: withdraw one by updating its status (active=false, status=inactive or off)");
    }

    private static BaseServerResponseException ifMatchRequired(final String where) {
        return new UnclassifiedServerFailureException(PRECONDITION_REQUIRED, where
                + "An update in the LRZA names the version it replaces: set If-Match (Bundle.entry.request.ifMatch in a transaction)");
    }
}
