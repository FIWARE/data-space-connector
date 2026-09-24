package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.asn1.x500.RDN;
import org.bouncycastle.asn1.x500.X500Name;
import org.bouncycastle.asn1.x500.style.BCStyle;
import org.bouncycastle.asn1.x500.style.IETFUtils;
import org.bouncycastle.cert.jcajce.JcaX509CertificateHolder;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.IssuerCredential;
import org.fiware.dataspace.it.components.model.IssuerConfiguration;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.TrustedIssuer;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.ByteArrayInputStream;
import java.security.Security;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.time.Duration;
import java.util.Base64;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Steps for the eIDAS deployment ({@code mvn clean integration-test -Ptest,eidas,eidas-test}).
 *
 * <p>The flow is the standard OID4VP exchange, so the provider-side steps are reused from
 * {@link StandardStepDefinitions}. What is specific to eIDAS, and therefore what these steps assert,
 * is the shape of the credential and the trust decision made on it: the issuer is a {@code did:elsi}
 * DID, the certificate chain travels with the credential in the {@code x5c} header, and the verifier
 * only issues a token once that chain validates against its trust list.
 *
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class EidasStepDefinitions extends StepDefintions {

    private static final String USER_CREDENTIAL = "user-credential";
    private static final String DEFAULT_SCOPE = "default";
    private static final String ENERGY_REPORT_ENTITY_ID = "urn:ngsi-ld:EnergyReport:fms-1";

    /**
     * Header parameter carrying the signing time. Since ETSI TS 119 182-1 v1.2 JAdES uses the
     * registered {@code iat} claim; before that it was the JAdES-specific {@code sigT}.
     */
    private static final String JADES_SIGNING_TIME_HEADER = "iat";
    private static final String LEGACY_JADES_SIGNING_TIME_HEADER = "sigT";

    private static final String X5C_HEADER = "x5c";
    private static final String ISSUER_CLAIM = "iss";

    /** The leaf, the intermediate and the root of the test CA. */
    private static final int EXPECTED_CHAIN_LENGTH = 3;

    private static final Duration DATA_ACCESS_TIMEOUT = Duration.ofSeconds(60);

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private Wallet employeeWallet;

    /**
     * Prepares a fresh wallet and a trusted-issuers list that knows the {@code did:elsi} issuer.
     * <p>
     * {@link StandardStepDefinitions} does the same for the standard local deployment, but registers
     * the {@code did:web} consumer - which is not the issuer here - so that hook excludes {@code @eidas}.
     */
    @Before("@eidas")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        employeeWallet = new Wallet();
        cleanUpPolicies(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS);
        deleteEnergyReport();
        cleanUpTIL();
        registerElsiIssuerAtTil();
        // give the verifier time to pick the new trusted-issuers entry up
        Thread.sleep(3001);
    }

    /**
     * Removes the energy report so the scenario that creates it starts from a clean state - the
     * provider rejects a second creation of the same entity id.
     */
    private void deleteEnergyReport() {
        Request deletion = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ENTITY_ID)
                .delete()
                .build();
        try (Response response = HTTP_CLIENT.newCall(deletion).execute()) {
            log.debug("Deleted the energy report - code {}", response.code());
        } catch (Exception e) {
            log.debug("No energy report to delete: {}", e.getMessage());
        }
    }

    private void registerElsiIssuerAtTil() throws Exception {
        TrustedIssuer elsiIssuer = new TrustedIssuer(EidasEnvironment.CONSUMER_DID,
                List.of(new IssuerCredential("UserCredential", List.of())));
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(elsiIssuer),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request create = new Request.Builder()
                .post(body)
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer")
                .build();
        try (Response response = HTTP_CLIENT.newCall(create).execute()) {
            log.debug("Registered the did:elsi issuer - code {}", response.code());
        }
    }

    // --- trust anchor ---

    @Then("The did:elsi issuer is registered at the trust anchor.")
    public void elsiIssuerIsRegisteredAtTrustAnchor() throws Exception {
        Request request = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + EidasEnvironment.CONSUMER_DID)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    "The did:elsi issuer should be registered at the trust anchor.");
        }
    }

    @Given("The did:elsi issuer is trusted by the provider.")
    public void elsiIssuerIsTrustedByProvider() throws Exception {
        Request request = new Request.Builder()
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer/" + EidasEnvironment.CONSUMER_DID)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    "The did:elsi issuer should be on the provider's trusted-issuers list.");
        }
    }

    // --- credential issuance ---

    @Given("The eIDAS consumer Keycloak credential issuer is configured.")
    public void eidasIssuerIsConfigured() throws Exception {
        IssuerConfiguration issuerConfiguration =
                employeeWallet.getIssuerConfiguration(EidasEnvironment.CONSUMER_KEYCLOAK_ADDRESS);
        assertNotNull(issuerConfiguration, "The issuer configuration should be returned.");
        assertTrue(issuerConfiguration.getCredentialConfigurationsSupported().containsKey(USER_CREDENTIAL),
                "The issuer should support the user-credential configuration.");
    }

    @When("The eIDAS consumer employee receives a user credential.")
    public void eidasEmployeeReceivesCredential() throws Exception {
        String accessToken = EidasEnvironment.loginToConsumerKeycloak(EidasEnvironment.TEST_USER_NAME);
        employeeWallet.getCredentialFromIssuer(
                accessToken, EidasEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
        assertNotNull(employeeWallet.getStoredCredential(USER_CREDENTIAL),
                "The user credential should be stored in the wallet.");
    }

    // --- the eIDAS specific assertions ---

    @Then("The credential is issued by the did:elsi issuer.")
    public void credentialIsIssuedByElsiIssuer() throws Exception {
        JsonNode payload = decodeSegment(storedCredential(), 1);
        assertEquals(EidasEnvironment.CONSUMER_DID, payload.path(ISSUER_CLAIM).asText(),
                "The credential should name the did:elsi issuer in its iss claim.");
    }

    @Then("The credential carries the eIDAS certificate chain in its x5c header.")
    public void credentialCarriesCertificateChain() throws Exception {
        JsonNode header = decodeSegment(storedCredential(), 0);
        JsonNode x5c = header.path(X5C_HEADER);
        assertTrue(x5c.isArray() && !x5c.isEmpty(),
                "The credential header should carry an x5c certificate chain - without it the verifier "
                        + "has nothing to validate against the trust list.");
    }

    @Then("The signing certificate's organizationIdentifier matches the did:elsi issuer.")
    public void organizationIdentifierMatchesIssuer() throws Exception {
        X500Name subject = new JcaX509CertificateHolder(certificateAt(0)).getSubject();
        RDN[] organizationIdentifiers = subject.getRDNs(BCStyle.ORGANIZATION_IDENTIFIER);
        assertEquals(1, organizationIdentifiers.length,
                "The leaf certificate should carry exactly one organizationIdentifier (OID 2.5.4.97), "
                        + "but its subject was: " + subject);

        String organizationIdentifier =
                IETFUtils.valueToString(organizationIdentifiers[0].getFirst().getValue());
        assertEquals(EidasEnvironment.ORGANIZATION_IDENTIFIER, organizationIdentifier,
                "The organizationIdentifier of the signing certificate has to match the identifier of "
                        + "the did:elsi issuer - that binding is what makes the DID verifiable.");
    }

    @Then("The credential is signed as JAdES.")
    public void credentialIsSignedAsJades() throws Exception {
        JsonNode header = decodeSegment(storedCredential(), 0);
        assertTrue(header.has(JADES_SIGNING_TIME_HEADER) || header.has(LEGACY_JADES_SIGNING_TIME_HEADER),
                "A JAdES baseline-B signature carries the signing time in its protected header, "
                        + "but the header only had: " + header.fieldNames());
    }

    @Then("The certificate chain of the credential is complete.")
    public void certificateChainIsComplete() throws Exception {
        JsonNode x5c = decodeSegment(storedCredential(), 0).path(X5C_HEADER);
        assertEquals(EXPECTED_CHAIN_LENGTH, x5c.size(),
                "The chain should contain the leaf, the intermediate and the root of the test CA.");

        X509Certificate leaf = certificateAt(0);
        X509Certificate intermediate = certificateAt(1);
        assertEquals(intermediate.getSubjectX500Principal(), leaf.getIssuerX500Principal(),
                "The leaf should be issued by the intermediate it is shipped with.");
        leaf.verify(intermediate.getPublicKey());
    }

    // --- data access ---

    @Then("The eIDAS credential can be exchanged for an access token.")
    public void eidasCredentialCanBeExchangedForToken() throws Exception {
        String accessToken = getAccessToken();
        assertNotNull(accessToken, "The verifier should issue an access token for the eIDAS credential.");
        assertFalse(accessToken.isEmpty(), "The access token should not be empty.");
    }

    @Then("The eIDAS consumer employee can access the EnergyReport with the access token.")
    public void eidasEmployeeCanAccessEnergyReport() {
        Awaitility.await()
                .atMost(DATA_ACCESS_TIMEOUT)
                .untilAsserted(() -> {
                    Request request = new Request.Builder()
                            .get()
                            .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS
                                    + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ENTITY_ID)
                            .addHeader("Authorization", "Bearer " + getAccessToken())
                            .addHeader("Accept", MediaType.APPLICATION_JSON)
                            .build();
                    try (Response response = HTTP_CLIENT.newCall(request).execute()) {
                        assertEquals(HttpStatus.SC_OK, response.code(),
                                "The EnergyReport should be accessible with a token obtained from the "
                                        + "eIDAS credential.");
                    }
                });
    }

    private String getAccessToken() throws Exception {
        OpenIdConfiguration openIdConfiguration =
                MPOperationsEnvironment.getOpenIDConfiguration(MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        return employeeWallet.exchangeCredentialForToken(openIdConfiguration, USER_CREDENTIAL, DEFAULT_SCOPE);
    }

    // --- helpers ---

    private String storedCredential() {
        String credential = employeeWallet.getStoredCredential(USER_CREDENTIAL);
        assertNotNull(credential, "A credential has to be issued before it can be inspected.");
        return credential;
    }

    /**
     * Decodes one segment of a compact JWS into JSON.
     *
     * @param jws     the compact serialization
     * @param segment {@code 0} for the protected header, {@code 1} for the payload
     */
    private JsonNode decodeSegment(String jws, int segment) throws Exception {
        String[] segments = jws.split("\\.");
        assertTrue(segments.length > segment, "The credential should be a compact JWS.");
        return MAPPER.readTree(Base64.getUrlDecoder().decode(segments[segment]));
    }

    /**
     * Returns the certificate at the given position of the credential's {@code x5c} chain, where
     * {@code 0} is the signing certificate.
     */
    private X509Certificate certificateAt(int position) throws Exception {
        JsonNode x5c = decodeSegment(storedCredential(), 0).path(X5C_HEADER);
        assertTrue(x5c.size() > position, "The x5c chain should contain a certificate at " + position);
        byte[] der = Base64.getDecoder().decode(x5c.get(position).asText());
        return (X509Certificate) CertificateFactory.getInstance("X.509")
                .generateCertificate(new ByteArrayInputStream(der));
    }
}
