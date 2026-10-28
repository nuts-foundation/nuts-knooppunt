package nl.nuts.lrza;

import static nl.nuts.lrza.Directory.careProvider;
import static nl.nuts.lrza.Directory.location;
import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.rest.server.exceptions.UnprocessableEntityException;
import com.nimbusds.jose.JOSEObjectType;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.JWSObject;
import com.nimbusds.jose.Payload;
import com.nimbusds.jose.crypto.ECDSASigner;
import com.nimbusds.jose.util.Base64;
import com.nimbusds.jose.util.X509CertUtils;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.interfaces.ECPrivateKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.util.Date;
import java.util.List;
import java.util.function.Consumer;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Location;
import org.hl7.fhir.r4.model.Provenance;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.Signature;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class ProvenanceVerifierTest {

    private static final FhirContext FHIR = FhirContext.forR4Cached();

    private final Directory directory = new Directory();
    private final OwnerResolver owners = new OwnerResolver(directory);
    private final WriteRules rules = new WriteRules(FHIR, directory, owners, new Mandates(directory, owners, Date::new));
    private final ProvenanceVerifier verifier = new ProvenanceVerifier(FHIR, rules, owners);
    private Location target;

    @BeforeEach
    void setUp() {
        directory.put(careProvider("sunflower", "00000020"));
        directory.put(careProvider("care2cure", "00000030"));
        target = location("north", "sunflower");
        target.setId("Location/north/_history/2");
        target.getMeta().setVersionId("2").setSource("http://example.org/somewhere");
        directory.put(target);
    }

    @Test
    void aSignatureByTheCareProviderOverTheTargetVerifies() throws Exception {
        assertDoesNotThrow(() -> verifier.verify(signed(target, "00000020", provenance -> {
        })));
    }

    @Test
    void anUnsignedProvenanceNeedsNoVerification() {
        final Provenance provenance = new Provenance();
        provenance.addTarget(new Reference("Location/north/_history/2"));

        assertDoesNotThrow(() -> verifier.verify(provenance));
    }

    @Test
    void aSignatureOverAnotherVersionDoesNotVerify() throws Exception {
        final Location tampered = target.copy();
        tampered.setName("Something else");

        final UnprocessableEntityException exception = assertThrows(UnprocessableEntityException.class,
                () -> verifier.verify(signed(tampered, "00000020", provenance -> {
                })));
        assertTrue(exception.getMessage().contains("does not match"), exception.getMessage());
    }

    @Test
    void aTamperedSignatureDoesNotVerify() throws Exception {
        final Provenance provenance = signed(target, "00000020", p -> {
        });
        final String compact = new String(provenance.getSignatureFirstRep().getData(), StandardCharsets.US_ASCII);
        // A character in the middle of the signature: the last one also carries padding bits.
        final int at = compact.length() - 20;
        final char flipped = compact.charAt(at) == 'A' ? 'B' : 'A';
        provenance.getSignatureFirstRep().setData((compact.substring(0, at) + flipped + compact.substring(at + 1))
                .getBytes(StandardCharsets.US_ASCII));

        assertThrows(UnprocessableEntityException.class, () -> verifier.verify(provenance));
    }

    @Test
    void theSignerMustBeTheCareProviderOfTheTarget() throws Exception {
        final UnprocessableEntityException exception = assertThrows(UnprocessableEntityException.class,
                () -> verifier.verify(signed(target, "00000030", p -> {
                })));
        assertTrue(exception.getMessage().contains("not the care provider"), exception.getMessage());
    }

    @Test
    void onlyTheStaticCanonicalizationIsSupported() throws Exception {
        assertThrows(UnprocessableEntityException.class, () -> verifier.verify(signed(target, "00000020",
                provenance -> provenance.getSignatureFirstRep().setTargetFormat("application/fhir+json"))));
    }

    @Test
    void theSignerCanBeNamedByItsOrganization() throws Exception {
        assertDoesNotThrow(() -> verifier.verify(signed(target, "00000020",
                provenance -> provenance.getSignatureFirstRep().setWho(new Reference("Organization/sunflower")))));
    }

    /** A Provenance as the Knooppunt makes it (component/lrza/provenance.go), signed with the test key. */
    private Provenance signed(final Location signedVersion, final String signerUra, final Consumer<Provenance> change) throws Exception {
        final byte[] canonical = Canonicalizer.canonicalize(FHIR.newJsonParser().encodeResourceToString(signedVersion));
        final JWSHeader header = new JWSHeader.Builder(JWSAlgorithm.ES256)
                .type(JOSEObjectType.JOSE)
                .x509CertChain(List.of(Base64.encode(X509CertUtils.parse(resource("signer-cert.pem")).getEncoded())))
                .customParam("canon", Canonicalizer.STATIC_CANONICALIZATION)
                .build();
        final JWSObject jws = new JWSObject(header, new Payload(canonical));
        jws.sign(new ECDSASigner(privateKey()));
        final String detached = jws.serialize(true);

        final Provenance provenance = new Provenance();
        provenance.addTarget(new Reference("Location/north/_history/2"));
        provenance.addSignature(new Signature()
                .setWho(new Reference().setIdentifier(new Identifier().setSystem(Caller.URA_SYSTEM).setValue(signerUra)))
                .setTargetFormat("application/fhir+json;canonicalization=" + Canonicalizer.STATIC_CANONICALIZATION)
                .setSigFormat(ProvenanceVerifier.JOSE)
                .setData(detached.getBytes(StandardCharsets.US_ASCII)));
        change.accept(provenance);
        return provenance;
    }

    private static ECPrivateKey privateKey() throws Exception {
        final String pem = resource("signer-key.pem")
                .replaceAll("-----(BEGIN|END) PRIVATE KEY-----", "").replaceAll("\\s", "");
        return (ECPrivateKey) KeyFactory.getInstance("EC")
                .generatePrivate(new PKCS8EncodedKeySpec(java.util.Base64.getDecoder().decode(pem)));
    }

    private static String resource(final String name) throws Exception {
        try (InputStream in = ProvenanceVerifierTest.class.getResourceAsStream("/" + name)) {
            return new String(in.readAllBytes(), StandardCharsets.US_ASCII);
        }
    }
}
