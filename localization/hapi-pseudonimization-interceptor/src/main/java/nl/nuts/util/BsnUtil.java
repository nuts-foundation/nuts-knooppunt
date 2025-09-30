package nl.nuts.util;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import lombok.NoArgsConstructor;
import nl.nuts.PseudonimizationExecutionException;

@NoArgsConstructor
public class BsnUtil {

    private static final String PSEUDONYM_PREFIX = "ps-";
    private static final String TOKEN_PREFIX = "token-";
    private static final int MIN_PSEUDONYM_LENGTH = PSEUDONYM_PREFIX.length() + 1;

    /**
     * Converts a transport token to a pseudonym format.
     * This extracts the core BSN information and creates a consistent pseudonym (ignoring nonce).
     *
     * @param token The transport token in format "token-{audience}-{transformedBSN}-{nonce}"
     * @return A pseudonym in format "ps-{audience}-{transformedBSN}"
     * @throws PseudonimizationExecutionException if the token format is invalid
     */
    public String transportTokenToPseudonym(final String token) throws PseudonimizationExecutionException {
        // Parse token components
        final TokenComponents components = parseTokenComponents(token);

        // Generate consistent pseudonym using the transformed BSN and audience (deterministic)
        return String.format("ps-%s-%s", components.audience, components.transformedBSN);
    }

    /**
     * Converts a pseudonym back to transport token format.
     * This reverses the transportTokenToPseudonym transformation.
     *
     * @param pseudonym The pseudonym in format "ps-{audience}-{transformedBSN}"
     * @param audience The target audience for the new transport token
     * @return A transport token in format "token-{audience}-{transformedBSN}-{nonce}"
     * @throws PseudonimizationExecutionException if the pseudonym format is invalid
     * @throws Exception if token creation fails
     */
    public String pseudonymToTransportToken(final String pseudonym, final String audience)
            throws PseudonimizationExecutionException {
        // Parse the pseudonym format manually to handle audience names with hyphens
        if (pseudonym.length() < MIN_PSEUDONYM_LENGTH || !pseudonym.startsWith(PSEUDONYM_PREFIX)) {
            throw new PseudonimizationExecutionException("invalid pseudonym format");
        }

        // Find the last hyphen to separate audience from transformedBSN
        final String afterPrefix = pseudonym.substring(PSEUDONYM_PREFIX.length());
        final int lastHyphen = afterPrefix.lastIndexOf("-");

        if (lastHyphen == -1 || lastHyphen == 0) {
            throw new PseudonimizationExecutionException("invalid pseudonym format");
        }

        final String pseudonymHolder = afterPrefix.substring(0, lastHyphen);
        final String transformedBSN = afterPrefix.substring(lastHyphen + 1);

        // Reverse the XOR to get original BSN
        final int key = generateSimpleKey(pseudonymHolder);
        final String originalBSN = decodeXOR(transformedBSN, key);

        // Create new token with the target audience
        return createTransportToken(originalBSN, audience);
    }

    /**
     * Creates a transport token from BSN and audience using simple XOR transformation.
     *
     * @param bsn The social security number or other identifier
     * @param audience The identifier for the organization/audience receiving the token
     * @return A transport token in format "token-{audience}-{transformedBSN}-{nonce}"
     * @throws Exception if token generation fails
     */
    private String createTransportToken(final String bsn, final String audience) {
        // Generate key and transform BSN
        final int key = generateSimpleKey(audience);
        final String transformedBSN = encodeXOR(bsn, key);

        // Add random nonce to make each token unique
        final String nonce = generateRandomNonce();

        return String.format("token-%s-%s-%s", audience, transformedBSN, nonce);
    }

    /**
     * Generates a simple numeric key from the audience string using SHA256.
     */
    private int generateSimpleKey(final String audience) {
        try {
            final MessageDigest digest = MessageDigest.getInstance("SHA-256");
            final byte[] hash = digest.digest(audience.getBytes(StandardCharsets.UTF_8));

            // Convert first 4 bytes to unsigned int, then ensure positive int range
            final int hashValue = ByteBuffer.wrap(hash, 0, 4).getInt();
            return hashValue >>> 1; // Unsigned right shift to ensure positive int (31 bits)
        } catch (final NoSuchAlgorithmException e) {
            throw new RuntimeException("SHA-256 algorithm not available", e);
        }
    }

    /**
     * Generates a random nonce to make transport tokens unique.
     */
    private String generateRandomNonce() {
        final byte[] bytes = new byte[4];
        new SecureRandom().nextBytes(bytes);
        return bytesToHex(bytes);
    }

    /**
     * XOR encodes a plaintext string and returns a hex-encoded result.
     */
    private String encodeXOR(final String plaintext, final int key) {
        if (plaintext == null || plaintext.isEmpty()) {
            return "";
        }

        final byte[] inputBytes = plaintext.getBytes(StandardCharsets.UTF_8);
        final byte[] keyBytes = ByteBuffer.allocate(4).putInt(key).array();
        final byte[] result = new byte[inputBytes.length];

        for (int i = 0; i < inputBytes.length; i++) {
            result[i] = (byte) (inputBytes[i] ^ keyBytes[i % 4]);
        }

        return bytesToHex(result);
    }

    /**
     * Decodes a hex-encoded XOR string and returns the plaintext result.
     */
    private String decodeXOR(final String hexEncoded, final int key) throws PseudonimizationExecutionException {
        if (hexEncoded == null || hexEncoded.isEmpty()) {
            return "";
        }

        final byte[] inputBytes = hexToBytes(hexEncoded);
        final byte[] keyBytes = ByteBuffer.allocate(4).putInt(key).array();
        final byte[] result = new byte[inputBytes.length];

        for (int i = 0; i < inputBytes.length; i++) {
            result[i] = (byte) (inputBytes[i] ^ keyBytes[i % 4]);
        }

        return new String(result, StandardCharsets.UTF_8);
    }

    /**
     * Parses token components from a transport token.
     */
    private TokenComponents parseTokenComponents(final String token) throws PseudonimizationExecutionException {
        if (token.length() < TOKEN_PREFIX.length() + 1 || !token.startsWith(TOKEN_PREFIX)) {
            throw new PseudonimizationExecutionException("invalid token format");
        }

        // Split by hyphens and parse components
        final String afterPrefix = token.substring(TOKEN_PREFIX.length());
        final String[] parts = afterPrefix.split("-");

        // We need at least 3 parts: audience, transformedBSN, and nonce
        if (parts.length < 3) {
            throw new PseudonimizationExecutionException("invalid token format");
        }

        // Get transformedBSN (second-to-last part)
        final String transformedBSN = parts[parts.length - 2];

        // Reconstruct audience (all parts except the last two: transformedBSN and nonce)
        final StringBuilder audienceBuilder = new StringBuilder();
        for (int i = 0; i < parts.length - 2; i++) {
            if (i > 0) {
                audienceBuilder.append("-");
            }
            audienceBuilder.append(parts[i]);
        }
        final String audience = audienceBuilder.toString();

        return new TokenComponents(audience, transformedBSN);
    }

    /**
     * Converts byte array to hex string.
     */
    private String bytesToHex(final byte[] bytes) {
        final StringBuilder hexString = new StringBuilder();
        for (final byte b : bytes) {
            final String hex = Integer.toHexString(0xff & b);
            if (hex.length() == 1) {
                hexString.append('0');
            }
            hexString.append(hex);
        }
        return hexString.toString();
    }

    /**
     * Converts hex string to byte array.
     */
    private byte[] hexToBytes(final String hex) throws PseudonimizationExecutionException {
        if (hex.length() % 2 != 0) {
            throw new PseudonimizationExecutionException("invalid hex encoding");
        }

        final byte[] bytes = new byte[hex.length() / 2];
        for (int i = 0; i < bytes.length; i++) {
            try {
                bytes[i] = (byte) Integer.parseInt(hex.substring(2 * i, 2 * i + 2), 16);
            } catch (final NumberFormatException e) {
                throw new PseudonimizationExecutionException("invalid hex encoding: " + e.getMessage());
            }
        }
        return bytes;
    }

    /**
     * Helper class to hold token components.
     */
    private static class TokenComponents {

        final String audience;
        final String transformedBSN;

        TokenComponents(final String audience, final String transformedBSN) {
            this.audience = audience;
            this.transformedBSN = transformedBSN;
        }
    }
}