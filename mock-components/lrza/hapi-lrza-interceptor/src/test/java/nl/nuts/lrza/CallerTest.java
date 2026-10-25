package nl.nuts.lrza;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import ca.uhn.fhir.rest.server.exceptions.InvalidRequestException;
import org.hl7.fhir.r4.model.Identifier;
import org.junit.jupiter.api.Test;

class CallerTest {

    @Test
    void theHeaderOverridesTheConfiguredCaller() {
        final Caller caller = Caller.resolve("urn:oid:2.16.528.1.1007.3.3|00000030", "http://kvk.nl|90001234");

        assertEquals(new Caller(Caller.URA_SYSTEM, "00000030"), caller);
        assertTrue(caller.isCareProvider());
    }

    @Test
    void withoutHeaderTheConfiguredCallerCalls() {
        final Caller caller = Caller.resolve(null, "http://kvk.nl|90001234");

        assertEquals(new Caller("http://kvk.nl", "90001234"), caller);
        assertFalse(caller.isCareProvider());
        assertEquals("http://kvk.nl|90001234", caller.toString());
    }

    @Test
    void withNeitherTheCallerIsUnknown() {
        assertNull(Caller.resolve(" ", null));
    }

    @Test
    void aCallerIsASystemValueToken() {
        assertThrows(InvalidRequestException.class, () -> Caller.resolve("90001234", null));
        assertThrows(InvalidRequestException.class, () -> Caller.resolve("http://kvk.nl|", null));
        assertThrows(InvalidRequestException.class, () -> Caller.resolve(null, "|90001234"));
    }

    @Test
    void matchesAnIdentifierOnSystemAndValue() {
        final Caller caller = new Caller("http://kvk.nl", "90001234");

        assertTrue(caller.matches(new Identifier().setSystem("http://kvk.nl").setValue("90001234")));
        assertFalse(caller.matches(new Identifier().setSystem("http://kvk.nl").setValue("90009999")));
        assertFalse(caller.matches(new Identifier().setSystem(Caller.URA_SYSTEM).setValue("90001234")));
        assertFalse(caller.matches(null));
    }
}
