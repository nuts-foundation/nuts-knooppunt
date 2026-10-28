package nl.nuts.lrza;

import java.util.Date;
import java.util.function.Supplier;
import org.hl7.fhir.r4.model.CodeableConcept;
import org.hl7.fhir.r4.model.Coding;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Period;

/**
 * The mandates care providers grant IT vendors: an OrganizationAffiliation the care provider registered
 * itself, with organization = the care provider, participatingOrganization = the vendor, and as code the
 * kind of mandate (nl-gf-authorization-type-cs).
 */
final class Mandates {

    static final String AUTHORIZATION_TYPE_SYSTEM = "http://fhir.generiekefuncties.nl/CodeSystem/nl-gf-authorization-type-cs";

    /** What a vendor may do for a care provider, from nothing to everything. */
    enum Mandate {
        NONE(null),
        /** lrza-endpoint-admin: the care provider's Endpoints, and the links to them. */
        ENDPOINT_ADMIN("lrza-endpoint-admin"),
        /** lrza-careprovider-admin: all of the care provider's addressing resources. */
        CAREPROVIDER_ADMIN("lrza-careprovider-admin");

        final String code;

        Mandate(final String code) {
            this.code = code;
        }

        static Mandate ofCode(final String code) {
            for (final Mandate mandate : values()) {
                if (mandate.code != null && mandate.code.equals(code)) {
                    return mandate;
                }
            }
            return NONE;
        }
    }

    private final ResourceLookup lookup;
    private final OwnerResolver owners;
    private final Supplier<Date> now;

    Mandates(final ResourceLookup lookup, final OwnerResolver owners, final Supplier<Date> now) {
        this.lookup = lookup;
        this.owners = owners;
        this.now = now;
    }

    /** The widest mandate the care provider has granted the vendor. */
    Mandate granted(final OwnerResolver.Owner careProvider, final Caller vendor) {
        if (careProvider.organizationId() == null) {
            return Mandate.NONE;
        }
        Mandate result = Mandate.NONE;
        for (final OrganizationAffiliation affiliation : lookup.affiliationsOf(careProvider.organizationId())) {
            if (!isCurrent(affiliation) || !isParticipant(affiliation, vendor)) {
                continue;
            }
            for (final CodeableConcept code : affiliation.getCode()) {
                for (final Coding coding : code.getCoding()) {
                    if (AUTHORIZATION_TYPE_SYSTEM.equals(coding.getSystem())) {
                        final Mandate mandate = Mandate.ofCode(coding.getCode());
                        if (mandate.compareTo(result) > 0) {
                            result = mandate;
                        }
                    }
                }
            }
        }
        return result;
    }

    /** Active (absent counts as active, as the Knooppunt reads it) and within its period. */
    private boolean isCurrent(final OrganizationAffiliation affiliation) {
        if (affiliation.hasActive() && !affiliation.getActive()) {
            return false;
        }
        final Period period = affiliation.getPeriod();
        final Date at = now.get();
        if (period.hasStart() && period.getStart().after(at)) {
            return false;
        }
        return !period.hasEnd() || !period.getEnd().before(at);
    }

    /** Whether the vendor is the participatingOrganization, by identifier or by its Organization. */
    private boolean isParticipant(final OrganizationAffiliation affiliation, final Caller vendor) {
        if (!affiliation.hasParticipatingOrganization()) {
            return false;
        }
        if (affiliation.getParticipatingOrganization().hasIdentifier()) {
            return vendor.matches(affiliation.getParticipatingOrganization().getIdentifier());
        }
        final Organization organization = owners.readOrganization(affiliation.getParticipatingOrganization());
        if (organization == null) {
            return false;
        }
        for (final Identifier identifier : organization.getIdentifier()) {
            if (vendor.matches(identifier)) {
                return true;
            }
        }
        return false;
    }
}
