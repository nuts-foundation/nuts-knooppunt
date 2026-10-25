package nl.nuts.lrza;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.rest.server.exceptions.UnprocessableEntityException;
import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSObject;
import com.nimbusds.jose.JWSVerifier;
import com.nimbusds.jose.Payload;
import com.nimbusds.jose.crypto.factories.DefaultJWSVerifierFactory;
import com.nimbusds.jose.util.Base64;
import com.nimbusds.jose.util.X509CertUtils;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.cert.X509Certificate;
import java.text.ParseException;
import java.util.List;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.Provenance;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.Resource;
import org.hl7.fhir.r4.model.Signature;

/**
 * Verifies the signatures of a Provenance, as the LRZA does for signed registrations
 * (https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/signing.html): each is a
 * detached JWS, with its certificate chain in x5c, over the json#static canonical form of the target
 * version, made by the care provider that target belongs to. Certificate trust (UZI/PKIoverheid) is not
 * checked: the mock has no trust anchors.
 */
final class ProvenanceVerifier {

    static final String JOSE = "application/jose";

    private final FhirContext fhirContext;
    private final WriteRules rules;
    private final OwnerResolver owners;

    ProvenanceVerifier(final FhirContext fhirContext, final WriteRules rules, final OwnerResolver owners) {
        this.fhirContext = fhirContext;
        this.rules = rules;
        this.owners = owners;
    }

    /**
     * @throws UnprocessableEntityException when a signature does not verify
     */
    void verify(final Provenance provenance) {
        if (!provenance.hasSignature()) {
            return;
        }
        final Resource target = rules.target(provenance);
        final OwnerResolver.Owner careProvider = owners.ownerOf(target);
        final byte[] canonical;
        try {
            canonical = Canonicalizer.canonicalize(fhirContext.newJsonParser().encodeResourceToString(target));
        } catch (IOException e) {
            throw new IllegalStateException("HAPI encoded invalid JSON", e);
        }
        for (int i = 0; i < provenance.getSignature().size(); i++) {
            verify(provenance.getSignature().get(i), String.format("Provenance.signature[%d]", i), canonical, careProvider);
        }
    }

    private void verify(final Signature signature, final String path, final byte[] canonical, final OwnerResolver.Owner careProvider) {
        if (!JOSE.equals(signature.getSigFormat())) {
            throw invalid(path + ".sigFormat", "sigFormat must be " + JOSE);
        }
        final String targetFormat = signature.getTargetFormat();
        if (targetFormat == null || !targetFormat.replace(" ", "").contains("canonicalization=" + Canonicalizer.STATIC_CANONICALIZATION)) {
            throw invalid(path + ".targetFormat", "targetFormat must name the canonicalization " + Canonicalizer.STATIC_CANONICALIZATION);
        }
        final String signerUra = uraOf(signature.getWho());
        if (signerUra == null) {
            throw invalid(path + ".who", "who must identify the care provider by URA");
        }
        if (!signerUra.equals(careProvider.ura())) {
            throw invalid(path + ".who", String.format("the signer (URA %s) is not the care provider the target belongs to (URA %s)",
                    signerUra, careProvider.ura()));
        }
        if (!signature.hasData()) {
            throw invalid(path + ".data", "data is missing");
        }
        final JWSObject jws;
        try {
            jws = JWSObject.parse(new String(signature.getData(), StandardCharsets.US_ASCII), new Payload(canonical));
        } catch (ParseException e) {
            throw invalid(path + ".data", "data is not a detached compact JWS: " + e.getMessage());
        }
        final List<Base64> chain = jws.getHeader().getX509CertChain();
        if (chain == null || chain.isEmpty()) {
            throw invalid(path + ".data", "the JWS header has no x5c certificate chain");
        }
        final X509Certificate certificate = X509CertUtils.parse(chain.get(0).decode());
        if (certificate == null) {
            throw invalid(path + ".data", "the first x5c certificate does not parse");
        }
        final boolean valid;
        try {
            final JWSVerifier verifier = new DefaultJWSVerifierFactory().createJWSVerifier(jws.getHeader(), certificate.getPublicKey());
            valid = jws.verify(verifier);
        } catch (JOSEException e) {
            throw invalid(path + ".data", "the signature can't be verified: " + e.getMessage());
        }
        if (!valid) {
            throw invalid(path + ".data", "the signature does not match the target's canonical form (" + Canonicalizer.STATIC_CANONICALIZATION + ")");
        }
    }

    /** The URA of a care provider reference: its identifier, or else the URA of the Organization it points to. */
    private String uraOf(final Reference who) {
        if (who == null || who.isEmpty()) {
            return null;
        }
        if (who.hasIdentifier() && Caller.URA_SYSTEM.equals(who.getIdentifier().getSystem())) {
            return who.getIdentifier().getValue();
        }
        final Organization organization = owners.readOrganization(who);
        if (organization == null) {
            return null;
        }
        return organization.getIdentifier().stream()
                .filter(identifier -> Caller.URA_SYSTEM.equals(identifier.getSystem()))
                .map(identifier -> identifier.getValue())
                .findFirst().orElse(null);
    }

    private UnprocessableEntityException invalid(final String expression, final String message) {
        return new UnprocessableEntityException(fhirContext, Outcomes.error("invalid", expression, message));
    }
}
