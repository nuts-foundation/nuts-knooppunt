package nl.nuts.lrza;

import ca.uhn.fhir.jpa.api.dao.DaoRegistry;
import ca.uhn.fhir.jpa.api.dao.IFhirResourceDao;
import ca.uhn.fhir.jpa.searchparam.SearchParameterMap;
import ca.uhn.fhir.model.api.IQueryParameterType;
import ca.uhn.fhir.rest.api.server.RequestDetails;
import ca.uhn.fhir.rest.param.ReferenceParam;
import ca.uhn.fhir.rest.param.TokenParam;
import ca.uhn.fhir.rest.server.exceptions.ResourceGoneException;
import ca.uhn.fhir.rest.server.exceptions.ResourceNotFoundException;
import java.util.ArrayList;
import java.util.List;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.r4.model.IdType;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Resource;

/**
 * Reads the partition through HAPI's DAOs, with the request being authorized: its tenant selects the
 * partition, and inside a transaction the reads see what earlier entries wrote. Searches are synchronous,
 * so they never answer from HAPI's search cache (a mandate revoked a moment ago is revoked).
 */
final class DaoResourceLookup implements ResourceLookup {

    private final DaoRegistry daoRegistry;
    private final RequestDetails requestDetails;

    DaoResourceLookup(final DaoRegistry daoRegistry, final RequestDetails requestDetails) {
        this.daoRegistry = daoRegistry;
        this.requestDetails = requestDetails;
    }

    @Override
    public Resource read(final String resourceType, final String id) {
        return read(new IdType(resourceType, id));
    }

    @Override
    public Resource readVersion(final String resourceType, final String id, final String versionId) {
        return read(new IdType(resourceType, id, versionId));
    }

    private Resource read(final IdType id) {
        if (!daoRegistry.isResourceTypeSupported(id.getResourceType())) {
            return null;
        }
        try {
            return (Resource) daoRegistry.getResourceDao(id.getResourceType()).read(id, requestDetails);
        } catch (ResourceNotFoundException | ResourceGoneException e) {
            return null;
        }
    }

    @Override
    public List<Organization> organizationsByIdentifier(final String system, final String value) {
        return search(Organization.class, "Organization", Organization.SP_IDENTIFIER, new TokenParam(system, value));
    }

    @Override
    public List<OrganizationAffiliation> affiliationsOf(final String organizationId) {
        return search(OrganizationAffiliation.class, "OrganizationAffiliation", OrganizationAffiliation.SP_PRIMARY_ORGANIZATION,
                new ReferenceParam("Organization/" + organizationId));
    }

    private <T extends Resource> List<T> search(final Class<T> type, final String resourceType, final String parameter,
                                                final IQueryParameterType value) {
        final IFhirResourceDao<?> dao = daoRegistry.getResourceDao(resourceType);
        final SearchParameterMap map = SearchParameterMap.newSynchronous(parameter, value);
        final List<T> result = new ArrayList<>();
        for (final IBaseResource resource : dao.search(map, requestDetails).getAllResources()) {
            if (type.isInstance(resource)) {
                result.add(type.cast(resource));
            }
        }
        return result;
    }
}
