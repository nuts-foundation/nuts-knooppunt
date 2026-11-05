package nl.nuts.lrza;

import java.util.Locale;
import java.util.Optional;

/**
 * Failures a test arms, to see the Knooppunt cope with the LRZA's 429 (with Retry-After) and 5xx: the
 * next {@code count} non-operator requests, optionally only those with one HTTP method, get the armed
 * status instead of being processed.
 */
final class FaultInjector {

    /** An armed failure; retryAfter (seconds) and method are optional. */
    record Fault(int status, int remaining, Integer retryAfter, String method) {

        boolean appliesTo(final String requestMethod) {
            return method == null || method.equalsIgnoreCase(requestMethod);
        }
    }

    private Fault armed;

    synchronized Fault arm(final int status, final int count, final Integer retryAfter, final String method) {
        if (status < 400 || status > 599) {
            throw new IllegalArgumentException("status must be an HTTP error status (400-599), got " + status);
        }
        if (count < 1) {
            throw new IllegalArgumentException("count must be at least 1, got " + count);
        }
        if (retryAfter != null && retryAfter < 0) {
            throw new IllegalArgumentException("retryAfter must not be negative, got " + retryAfter);
        }
        final String normalizedMethod = method == null || method.isBlank() ? null : method.toUpperCase(Locale.ROOT);
        armed = new Fault(status, count, retryAfter, normalizedMethod);
        return armed;
    }

    synchronized void disarm() {
        armed = null;
    }

    synchronized Optional<Fault> current() {
        return Optional.ofNullable(armed);
    }

    /** Takes one failure for a request with this method, if one is armed for it. */
    synchronized Optional<Fault> take(final String requestMethod) {
        if (armed == null || !armed.appliesTo(requestMethod)) {
            return Optional.empty();
        }
        final Fault taken = armed;
        armed = taken.remaining() > 1
                ? new Fault(taken.status(), taken.remaining() - 1, taken.retryAfter(), taken.method())
                : null;
        return Optional.of(taken);
    }
}
