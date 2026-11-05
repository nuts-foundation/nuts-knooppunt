package nl.nuts.lrza;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import ca.uhn.fhir.rest.api.RestOperationTypeEnum;
import ca.uhn.fhir.rest.server.exceptions.BaseServerResponseException;
import ca.uhn.fhir.rest.server.exceptions.MethodNotAllowedException;
import org.hl7.fhir.r4.model.Bundle;
import org.hl7.fhir.r4.model.Endpoint;
import org.junit.jupiter.api.Test;

class RequestRulesTest {

    @Test
    void aDeleteIsNotAllowed() {
        assertThrows(MethodNotAllowedException.class, () -> RequestRules.check(RestOperationTypeEnum.DELETE, null));
    }

    @Test
    void anUpdateNeedsIfMatch() {
        final BaseServerResponseException exception = assertThrows(BaseServerResponseException.class,
                () -> RequestRules.check(RestOperationTypeEnum.UPDATE, null));
        assertEquals(428, exception.getStatusCode());

        assertEquals(428, assertThrows(BaseServerResponseException.class,
                () -> RequestRules.check(RestOperationTypeEnum.UPDATE, " ")).getStatusCode());
        assertDoesNotThrow(() -> RequestRules.check(RestOperationTypeEnum.UPDATE, "W/\"1\""));
    }

    @Test
    void otherOperationsPass() {
        assertDoesNotThrow(() -> RequestRules.check(RestOperationTypeEnum.CREATE, null));
        assertDoesNotThrow(() -> RequestRules.check(RestOperationTypeEnum.READ, null));
        assertDoesNotThrow(() -> RequestRules.check(RestOperationTypeEnum.TRANSACTION, null));
        assertDoesNotThrow(() -> RequestRules.check(null, null));
    }

    @Test
    void aTransactionFollowsTheSameRulesPerEntry() {
        final Bundle valid = new Bundle().setType(Bundle.BundleType.TRANSACTION);
        valid.addEntry().setResource(new Endpoint()).getRequest().setMethod(Bundle.HTTPVerb.POST).setUrl("Endpoint");
        valid.addEntry().setResource(new Endpoint()).getRequest().setMethod(Bundle.HTTPVerb.PUT).setUrl("Endpoint/1").setIfMatch("W/\"1\"");
        assertDoesNotThrow(() -> RequestRules.check(valid));

        final Bundle withoutIfMatch = valid.copy();
        withoutIfMatch.getEntry().get(1).getRequest().setIfMatch(null);
        final BaseServerResponseException exception = assertThrows(BaseServerResponseException.class, () -> RequestRules.check(withoutIfMatch));
        assertEquals(428, exception.getStatusCode());
        assertTrue(exception.getMessage().contains("Bundle.entry[1]"), exception.getMessage());

        final Bundle withDelete = valid.copy();
        withDelete.addEntry().getRequest().setMethod(Bundle.HTTPVerb.DELETE).setUrl("Endpoint/1");
        assertThrows(MethodNotAllowedException.class, () -> RequestRules.check(withDelete));
    }
}
