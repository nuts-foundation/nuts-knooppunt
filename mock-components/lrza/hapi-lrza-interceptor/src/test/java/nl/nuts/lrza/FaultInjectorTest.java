package nl.nuts.lrza;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

class FaultInjectorTest {

    private final FaultInjector faults = new FaultInjector();

    @Test
    void nothingArmedFailsNothing() {
        assertTrue(faults.take("GET").isEmpty());
    }

    @Test
    void anArmedFailureFailsTheNextRequestsUntilUsedUp() {
        faults.arm(429, 2, 1, null);

        final FaultInjector.Fault first = faults.take("GET").orElseThrow();
        assertEquals(429, first.status());
        assertEquals(1, first.retryAfter());
        assertEquals(1, faults.current().orElseThrow().remaining());
        assertEquals(429, faults.take("PUT").orElseThrow().status());
        assertTrue(faults.take("GET").isEmpty());
        assertTrue(faults.current().isEmpty());
    }

    @Test
    void aFailureForOneMethodLeavesTheOthersAlone() {
        faults.arm(503, 1, null, "post");

        assertTrue(faults.take("GET").isEmpty());
        assertTrue(faults.take("PUT").isEmpty());
        assertEquals(503, faults.take("POST").orElseThrow().status());
        assertTrue(faults.take("POST").isEmpty());
    }

    @Test
    void disarmClearsIt() {
        faults.arm(503, 5, null, null);
        faults.disarm();

        assertTrue(faults.take("GET").isEmpty());
    }

    @Test
    void onlyErrorsCanBeArmed() {
        assertThrows(IllegalArgumentException.class, () -> faults.arm(200, 1, null, null));
        assertThrows(IllegalArgumentException.class, () -> faults.arm(429, 0, null, null));
        assertThrows(IllegalArgumentException.class, () -> faults.arm(429, 1, -1, null));
    }
}
