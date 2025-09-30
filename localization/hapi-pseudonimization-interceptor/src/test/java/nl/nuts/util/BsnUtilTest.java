package nl.nuts.util;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

class BsnUtilTest {

    private final BsnUtil bsnUtil= new BsnUtil();

    @Test
    void testTransportTokenToPseudonym_validToken() {
        // Given a valid transport token
        final String token = "token-hospital-abc123-def456";

        // When converting to pseudonym
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token);

        // Then the pseudonym should have correct format (ps-{audience}-{transformedBSN})
        assertNotNull(pseudonym);
        assertTrue(pseudonym.startsWith("ps-"));
        assertEquals("ps-hospital-abc123", pseudonym);
    }

    @Test
    void testTransportTokenToPseudonym_audienceWithHyphens() {
        // Given a token with audience containing hyphens
        final String token = "token-hospital-north-east-abc123-def456";

        // When converting to pseudonym
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token);
    
        // Then the audience should be preserved correctly
        assertEquals("ps-hospital-north-east-abc123", pseudonym);
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_noPrefix() {
        // Given a token without proper prefix
        final String token = "invalid-hospital-abc123-def456";

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token);
        });
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_tooShort() {
        // Given a token that's too short
        final String token = "token-a";

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token);
        });
    }

    @Test
    void testTransportTokenToPseudonym_invalidFormat_notEnoughParts() {
        // Given a token without enough parts
        final String token = "token-hospital-abc123";

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.transportTokenToPseudonym(token);
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

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testPseudonymToTransportToken_invalidFormat_tooShort() {
        // Given a pseudonym that's too short
        final String pseudonym = "ps-a";
        final String audience = "clinic";

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testPseudonymToTransportToken_invalidFormat_noHyphen() {
        // Given a pseudonym without required hyphens
        final String pseudonym = "ps-nohyphen";
        final String audience = "clinic";

        // When/Then converting should throw IllegalArgumentException
        assertThrows(IllegalArgumentException.class, () -> {
            bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        });
    }

    @Test
    void testRoundTrip_tokenToPseudonymBackToToken() throws Exception {
        // Given a pseudonym (representing a stored identifier)
        final String originalPseudonym = "ps-hospital-6d656469";

        // When converting to token for an audience
        final String token1 = bsnUtil.pseudonymToTransportToken(originalPseudonym, "clinic");

        // Then converting that token back to pseudonym
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token1);

        // When converting the pseudonym to another token for the same audience
        final String token2 = bsnUtil.pseudonymToTransportToken(pseudonym1, "clinic");

        // Then both tokens should decode to the same pseudonym
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token2);
        assertEquals(pseudonym1, pseudonym2);
    }

    @Test
    void testRoundTrip_sameAudienceShouldProduceSamePseudonym() throws Exception {
        // Given two different tokens with the same audience and BSN but different nonces
        final String token1 = "token-hospital-abc123-nonce1";
        final String token2 = "token-hospital-abc123-nonce2";

        // When converting both to pseudonyms
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token1);
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token2);

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
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token);
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token);

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
        final String pseudonym1 = bsnUtil.transportTokenToPseudonym(token1);
        final String pseudonym2 = bsnUtil.transportTokenToPseudonym(token2);
        assertEquals(pseudonym1, pseudonym2);
    }
}