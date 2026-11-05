package nl.nuts.lrza;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.context.support.IValidationSupport;
import java.util.List;
import org.hl7.fhir.instance.model.api.IBaseResource;

/**
 * Resolves a version-pinned canonical ({@code http://hl7.org/fhir/StructureDefinition/Endpoint|4.0.1},
 * as the IG's dependency packages write their target profiles) to the one version loaded. HAPI's
 * supports only resolve the bare URL, so without this every such reference is an error.
 */
final class VersionedCanonicalValidationSupport implements IValidationSupport {

    private final FhirContext fhirContext;
    private final List<IValidationSupport> delegates;

    VersionedCanonicalValidationSupport(final FhirContext fhirContext, final List<IValidationSupport> delegates) {
        this.fhirContext = fhirContext;
        this.delegates = delegates;
    }

    @Override
    public FhirContext getFhirContext() {
        return fhirContext;
    }

    @Override
    public IBaseResource fetchStructureDefinition(final String url) {
        final int separator = url == null ? -1 : url.indexOf('|');
        if (separator < 0) {
            return null;
        }
        final String unversioned = url.substring(0, separator);
        for (final IValidationSupport delegate : delegates) {
            final IBaseResource definition = delegate.fetchStructureDefinition(unversioned);
            if (definition != null) {
                return definition;
            }
        }
        return null;
    }

    @Override
    public String getName() {
        return "versioned canonical resolution";
    }
}
