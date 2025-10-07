package nl.nuts.util;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import nl.nuts.PseudonimizationExecutionException;
import org.junit.jupiter.api.Test;

class BsnUtilTest {

    private final BsnUtil bsnUtil= new BsnUtil();

    @Test
    void testTransportTokenToPseudonym_validToken() {
        // Given a valid transport token
        final String token = "token-hospital-abc123-def456";

        // When converting to pseudonym
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token, "nvi");

        // Then the pseudonym should have correct format (ps-{audience}-{transformedBSN})
        assertNotNull(pseudonym);
        assertTrue(pseudonym.startsWith("ps-"));
        assertEquals("ps-nvi-abc123", pseudonym);
    }

    @Test
    void testTransportTokenToPseudonym_audienceWithHyphens() {
        // Given a token with audience containing hyphens
        final String token = "token-hospital-north-east-abc123-def456";

        // When converting to pseudonym
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token, "nvi");
    
        // Then the audience should be preserved correctly
        assertEquals("ps-nvi-abc123", pseudonym);
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_noPrefix() {
        // Given a token without proper prefix
        final String token = "invalid-hospital-abc123-def456";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token, "nvi");
        });
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_tooShort() {
        // Given a token that's too short
        final String token = "token-a";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token, "nvi");
        });
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_notEnoughParts() {
        // Given a token without enough parts
        final String token = "token-hospital-abc123";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token, "nvi");
        });
    }

    @Test
    void testPseudonymToTransportToken_validPseudonym() throws Exception {
        // Given a valid pseudonym
        final String pseudonym = "ps-hospital-6d656469";
        final String audience = "clinic";

        // When converting to transport token
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, audience);

        // Then the token should have correct format
        assertNotNull(token);
        assertTrue(token.startsWith("token-"));
        assertTrue(token.contains("clinic"));
        // Token should have 4 parts: prefix, audience, transformedBSN, nonce
        final String[] parts = token.split("-");
        assertTrue(parts.length >= 3);
    }

    @Test
    void testPseudonymToTransportToken_invalidFormat_noPrefix() {
        // Given a pseudonym without proper prefix
        final String pseudonym = "invalid-hospital-abc123";
        final String audience = "clinic";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testPseudonymToTransportToken_invalidFormat_tooShort() {
        // Given a pseudonym that's too short
        final String pseudonym = "ps-a";
        final String audience = "clinic";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testPseudonymToTransportToken_invalidFormat_noHyphen() {
        // Given a pseudonym without required hyphens
        final String pseudonym = "ps-nohyphen";
        final String audience = "clinic";

        // When/Then converting should throw PseudonimizationExecutionException
        assertThrows(PseudonimizationExecutionException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testRoundTrip_sameAudienceShouldProduceSamePseudonym() throws Exception {
        // Given two different tokens with the same audience and BSN but different nonces
        final String token1 = "token-hospital-abc123-nonce1";
        final String token2 = "token-hospital-abc123-nonce2";

        // When converting both to pseudonyms
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token1, "nvi");
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token2, "nvi");

        // Then pseudonyms should be identical (nonce is ignored)
        assertEquals(pseudonym1, pseudonym2);
    }

    @Test
    void testPseudonymToTransportToken_differentAudience() throws Exception {
        // Given a pseudonym with one audience
        final String pseudonym = "ps-hospital-6d656469";
        final String newAudience = "clinic";

        // When converting to token with different audience
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, newAudience);

        // Then the token should contain the new audience
        assertTrue(token.contains(newAudience));
        assertTrue(token.startsWith("token-" + newAudience + "-"));
    }

    @Test
    void testPseudonymToTransportToken_audienceWithHyphens() throws Exception {
        // Given a pseudonym with audience containing hyphens
        final String pseudonym = "ps-hospital-north-east-6d656469";
        final String newAudience = "clinic-south-west";

        // When converting to token
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, newAudience);

        // Then the token should be valid and contain the new audience
        assertNotNull(token);
        assertTrue(token.contains(newAudience));
    }

    @Test
    void testTransportTokenToPseudonym_consistency() {
        // Given the same token
        final String token = "token-hospital-abc123-def456";

        // When converting multiple times
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token, "nvi");
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token, "nvi");

        // Then results should be identical (deterministic)
        assertEquals(pseudonym1, pseudonym2);
    }

    @Test
    void testPseudonymToTransportToken_uniqueNonces() throws Exception {
        // Given the same pseudonym
        final String pseudonym = "ps-hospital-6d656469";
        final String audience = "clinic";

        // When converting multiple times
        final String token1 = bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        final String token2 = bsnUtil.pseudonymToTransportToken(pseudonym, audience);

        // Then tokens should be different (due to random nonces)
        assertNotEquals(token1, token2);

        // But both should convert to the same pseudonym
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token1, "nvi");
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token2, "nvi");
        assertEquals(pseudonym1, pseudonym2);
    }

    @Test
    void testCompleteWorkflow() throws Exception {
        // Test the complete federated health workflow from the architecture diagram
        final String bsn = "123456789";

        // LOCALIZATION: Knooppunt A registers DocumentReference
        // Step 1: Knooppunt A creates transport token with audience="nvi"
        final String tokenFromA = bsnUtil.createTransportToken(bsn, "nvi");

        // Step 2: NVI converts to pseudonym for storage (shared across orgs)
        final String sharedPseudonym = bsnUtil.transportTokenToPseudonym(tokenFromA, "nvi");

        // SEARCH: Knooppunt B queries for same BSN
        // Step 3: Knooppunt B creates transport token with audience="nvi" (same as A)
        final String tokenFromB = bsnUtil.createTransportToken(bsn, "nvi");

        // Step 4: NVI converts B's token to pseudonym (should match stored one)
        final String searchPseudonym = bsnUtil.transportTokenToPseudonym(tokenFromB, "nvi");
        assertEquals(sharedPseudonym, searchPseudonym,
            "Search pseudonym mismatch: got " + searchPseudonym + ", want " + sharedPseudonym);

        // Step 5: NVI creates org-specific token for Knooppunt B
        final String tokenForB = bsnUtil.pseudonymToTransportToken(sharedPseudonym, "knooppunt-b");

        // Step 6: Knooppunt B extracts BSN from their org-specific token
        final String extractedBSN = bsnUtil.bsnFromTransportToken(tokenForB);

        // Verify extraction works and format is preserved
        assertFalse(extractedBSN.isEmpty(), "Complete workflow failed: got empty BSN");
        assertEquals(bsn, extractedBSN, "Extracted BSN should match original BSN");
    }
}