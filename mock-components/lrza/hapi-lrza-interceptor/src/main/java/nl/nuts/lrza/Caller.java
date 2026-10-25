package nl.nuts.lrza;

import ca.uhn.fhir.rest.server.exceptions.InvalidRequestException;
import org.hl7.fhir.r4.model.Identifier;

/**
 * Who a request comes from. The real LRZA knows this from the client certificate (mTLS); the mock takes
 * it from configuration: LRZA_CALLER for every request, which a test overrides per request with the
 * X-Mock-Caller header. Both are a FHIR token, {@code system|value}: a vendor's KvK number
 * ({@code http://kvk.nl|90001234}) or a care provider's URA ({@code urn:oid:2.16.528.1.1007.3.3|00000030}).
 */
public record Caller(String system, String value) {

    public static final String URA_SYSTEM = "urn:oid:2.16.528.1.1007.3.3";

    /**
     * The caller of a request: the header when set, else the configured default. Null when neither is
     * set, which leaves the request unauthenticated.
     */
    static Caller resolve(final String header, final String configured) {
        if (header != null && !header.isBlank()) {
            return parse(header, "X-Mock-Caller");
        }
        if (configured != null && !configured.isBlank()) {
            return parse(configured, "LRZA_CALLER");
        }
        return null;
    }

    static Caller parse(final String token, final String source) {
        final int separator = token.indexOf('|');
        if (separator <= 0 || separator == token.length() - 1) {
            throw new InvalidRequestException(String.format("%s must be a token system|value, got '%s'", source, token));
        }
        return new Caller(token.substring(0, separator).trim(), token.substring(separator + 1).trim());
    }

    /** A care provider, identified by its URA, rather than an IT vendor acting on its behalf. */
    boolean isCareProvider() {
        return URA_SYSTEM.equals(system);
    }

    boolean matches(final Identifier identifier) {
        return identifier != null && system.equals(identifier.getSystem()) && value.equals(identifier.getValue());
    }

    @Override
    public String toString() {
        return system + "|" + value;
    }
}
