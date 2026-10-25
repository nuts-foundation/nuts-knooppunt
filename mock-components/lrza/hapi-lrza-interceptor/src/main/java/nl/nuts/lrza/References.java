package nl.nuts.lrza;

import org.hl7.fhir.r4.model.Reference;

/** Parses literal references: {@code Type/id}, optionally versioned or absolute. */
final class References {

    /** The resource a literal reference points to; versionId is null for an unversioned reference. */
    record Target(String resourceType, String id, String versionId) {
    }

    private References() {
    }

    /** The target of a literal reference, or null when it has none (a logical or contained reference). */
    static Target parse(final Reference reference) {
        return reference == null || !reference.hasReference() ? null : parse(reference.getReference());
    }

    static Target parse(final String reference) {
        if (reference == null || reference.isBlank() || reference.startsWith("#") || reference.startsWith("urn:")) {
            return null;
        }
        String path = reference;
        final int query = path.indexOf('?');
        if (query >= 0) {
            path = path.substring(0, query);
        }
        final String[] segments = path.replaceAll("/+$", "").split("/");
        final int n = segments.length;
        if (n >= 4 && "_history".equals(segments[n - 2])) {
            return target(segments[n - 4], segments[n - 3], segments[n - 1]);
        }
        if (n >= 2) {
            return target(segments[n - 2], segments[n - 1], null);
        }
        return null;
    }

    private static Target target(final String resourceType, final String id, final String versionId) {
        if (resourceType.isEmpty() || id.isEmpty() || !Character.isUpperCase(resourceType.charAt(0))) {
            return null;
        }
        return new Target(resourceType, id, versionId);
    }
}
