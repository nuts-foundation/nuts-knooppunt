package nl.nuts.lrza;

import java.util.List;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Resource;

/**
 * The reads the LRZA rules need from the partition. In HAPI they go through the DAOs
 * ({@link DaoResourceLookup}); tests give an in-memory directory.
 */
interface ResourceLookup {

    /** The current version of a resource, or null when it doesn't exist. */
    Resource read(String resourceType, String id);

    /** A specific version of a resource, or null when it doesn't exist. */
    Resource readVersion(String resourceType, String id, String versionId);

    /** The Organizations carrying an identifier. */
    List<Organization> organizationsByIdentifier(String system, String value);

    /** The OrganizationAffiliations whose organization is Organization/{organizationId}. */
    List<OrganizationAffiliation> affiliationsOf(String organizationId);
}
