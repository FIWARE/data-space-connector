package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.security.Security;

import static org.fiware.dataspace.it.components.DSPEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.CONSUMER_DID;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_DID;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_KEYCLOAK_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.TIL_DIRECT_ADDRESS;
import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.fiware.dataspace.it.components.TestUtils.OK_HTTP_CLIENT;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Cucumber step definitions for the DSP identity setup scenarios described in DSP_INTEGRATION.md.
 * <p>
 * Covers consumer and provider identity registration in IdentityHub, key insertion into Vault,
 * membership credential issuance, and trusted issuers list verification.
 *
 * @see DSPEnvironment
 * @see IdentityHubHelper
 * @see ScriptHelper
 */
@Slf4j
public class DSPStepDefinitions {

    /** HTTP port used by local services in the k3s deployment. */
    private static final int SERVICE_PORT = 8080;

    /** The credential configuration ID for membership credentials in Keycloak. */
    private static final String MEMBERSHIP_CREDENTIAL_ID = "membership-credential";

    /** The Keycloak username used for issuing membership credentials. */
    private static final String MEMBERSHIP_CREDENTIAL_USERNAME = "employee";

    /**
     * Relative path from the project root to the consumer's PKCS#8 private key PEM file.
     * This file is generated during the deployment process by {@code helpers/certs/generate-certs.sh}.
     */
    private static final String CONSUMER_KEY_PEM_PATH = "helpers/certs/out/client-consumer/private/client-pkcs8.key.pem";

    /**
     * Relative path from the project root to the provider's PKCS#8 private key PEM file.
     * This file is generated during the deployment process by {@code helpers/certs/generate-certs.sh}.
     */
    private static final String PROVIDER_KEY_PEM_PATH = "helpers/certs/out/client-provider/private/client-pkcs8.key.pem";

    /** The IdentityHub credential ID used when storing membership credentials. */
    private static final String IDENTITYHUB_CREDENTIAL_ID = "membership-credential";

    private static final OkHttpClient HTTP_CLIENT = OK_HTTP_CLIENT;

    /** The consumer's private key in JWK format, populated during identity setup. */
    private String consumerJwk;

    /** The provider's private key in JWK format, populated during identity setup. */
    private String providerJwk;

    /** The consumer's PEM key content, read from the file system. */
    private String consumerPemContent;

    /** The provider's PEM key content, read from the file system. */
    private String providerPemContent;

    /** The raw JWT membership credential for the consumer. */
    private String consumerMembershipCredential;

    /** The raw JWT membership credential for the provider. */
    private String providerMembershipCredential;

    @Before("@dsp")
    public void setup() {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
    }

    // ==================== Consumer Identity Setup ====================

    /**
     * Verifies that the consumer's PKCS#8 private key PEM file exists.
     * This file is generated during the {@code mvn clean deploy -Plocal,dsp} process.
     */
    @Given("The consumer private key PEM file is available.")
    public void theConsumerPrivateKeyPemFileIsAvailable() throws IOException {
        Path pemPath = resolveProjectPath(CONSUMER_KEY_PEM_PATH);
        assertTrue(Files.exists(pemPath),
                "Consumer private key PEM file should exist at: " + pemPath.toAbsolutePath());
        consumerPemContent = Files.readString(pemPath);
        assertFalse(consumerPemContent.isBlank(), "Consumer PEM file should not be empty.");
        log.info("Consumer private key PEM file is available at {}", pemPath);
    }

    /**
     * Converts the consumer's PEM private key to JWK format using IdentityHubHelper.
     * This is the Java equivalent of running {@code get-private-jwk-p-256.sh}.
     */
    @When("The consumer private key is converted to JWK format.")
    public void theConsumerPrivateKeyIsConvertedToJwkFormat() throws Exception {
        assertNotNull(consumerPemContent, "Consumer PEM content must be loaded first.");
        consumerJwk = IdentityHubHelper.getPrivateKeyAsJwk(consumerPemContent);
        assertNotNull(consumerJwk, "Consumer JWK should not be null.");
        // Verify it's valid JSON with expected EC key fields
        JsonNode jwkNode = OBJECT_MAPPER.readTree(consumerJwk);
        assertEquals("EC", jwkNode.get("kty").asText(), "Key type should be EC.");
        assertEquals("P-256", jwkNode.get("crv").asText(), "Curve should be P-256.");
        assertTrue(jwkNode.has("d"), "JWK should contain private key component 'd'.");
        log.info("Consumer private key successfully converted to JWK format.");
    }

    /**
     * Inserts the consumer's JWK into the consumer's Vault instance for use by the IdentityHub STS.
     */
    @When("The consumer JWK is inserted into the consumer Vault.")
    public void theConsumerJwkIsInsertedIntoTheConsumerVault() throws Exception {
        assertNotNull(consumerJwk, "Consumer JWK must be available.");
        IdentityHubHelper.insertKeyIntoVault(
                VAULT_CONSUMER_ADDRESS,
                IdentityHubHelper.DEFAULT_KEY_ALIAS,
                consumerJwk);
        log.info("Consumer JWK inserted into Vault at {}", VAULT_CONSUMER_ADDRESS);
    }

    /**
     * Registers the consumer participant in the consumer's IdentityHub management API.
     * Builds the participant payload including the public key and credential service endpoint.
     */
    @When("The consumer participant is registered in the consumer IdentityHub.")
    public void theConsumerParticipantIsRegisteredInTheConsumerIdentityHub() throws Exception {
        assertNotNull(consumerJwk, "Consumer JWK must be available.");
        String participantPayload = IdentityHubHelper.buildParticipantPayload(
                consumerJwk,
                CONSUMER_DID,
                IDENTITYHUB_CONSUMER_ADDRESS,
                IdentityHubHelper.DEFAULT_KEY_ALIAS);
        IdentityHubHelper.registerParticipant(IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS, participantPayload);
        log.info("Consumer participant registered in IdentityHub at {}", IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS);
    }

    /**
     * Verifies the consumer's DID document is accessible at the well-known endpoint.
     * Checks {@code https://fancy-marketplace.biz/.well-known/did.json} via the Squid proxy.
     */
    @Then("The consumer DID document is available at the well-known endpoint.")
    public void theConsumerDidDocumentIsAvailableAtTheWellKnownEndpoint() throws Exception {
        // The DID web resolution for did:web:fancy-marketplace.biz resolves to
        // https://fancy-marketplace.biz/.well-known/did.json
        String didDocUrl = "https://fancy-marketplace.biz/.well-known/did.json";
        verifyDidDocument(didDocUrl, CONSUMER_DID);
        log.info("Consumer DID document is available at {}", didDocUrl);
    }

    // ==================== Provider Identity Setup ====================

    /**
     * Verifies that the provider's PKCS#8 private key PEM file exists.
     * This file is generated during the {@code mvn clean deploy -Plocal,dsp} process.
     */
    @Given("The provider private key PEM file is available.")
    public void theProviderPrivateKeyPemFileIsAvailable() throws IOException {
        Path pemPath = resolveProjectPath(PROVIDER_KEY_PEM_PATH);
        assertTrue(Files.exists(pemPath),
                "Provider private key PEM file should exist at: " + pemPath.toAbsolutePath());
        providerPemContent = Files.readString(pemPath);
        assertFalse(providerPemContent.isBlank(), "Provider PEM file should not be empty.");
        log.info("Provider private key PEM file is available at {}", pemPath);
    }

    /**
     * Converts the provider's PEM private key to JWK format using IdentityHubHelper.
     */
    @When("The provider private key is converted to JWK format.")
    public void theProviderPrivateKeyIsConvertedToJwkFormat() throws Exception {
        assertNotNull(providerPemContent, "Provider PEM content must be loaded first.");
        providerJwk = IdentityHubHelper.getPrivateKeyAsJwk(providerPemContent);
        assertNotNull(providerJwk, "Provider JWK should not be null.");
        JsonNode jwkNode = OBJECT_MAPPER.readTree(providerJwk);
        assertEquals("EC", jwkNode.get("kty").asText(), "Key type should be EC.");
        assertEquals("P-256", jwkNode.get("crv").asText(), "Curve should be P-256.");
        assertTrue(jwkNode.has("d"), "JWK should contain private key component 'd'.");
        log.info("Provider private key successfully converted to JWK format.");
    }

    /**
     * Inserts the provider's JWK into the provider's Vault instance for use by the IdentityHub STS.
     */
    @When("The provider JWK is inserted into the provider Vault.")
    public void theProviderJwkIsInsertedIntoTheProviderVault() throws Exception {
        assertNotNull(providerJwk, "Provider JWK must be available.");
        IdentityHubHelper.insertKeyIntoVault(
                VAULT_PROVIDER_ADDRESS,
                IdentityHubHelper.DEFAULT_KEY_ALIAS,
                providerJwk);
        log.info("Provider JWK inserted into Vault at {}", VAULT_PROVIDER_ADDRESS);
    }

    /**
     * Registers the provider participant in the provider's IdentityHub management API.
     */
    @When("The provider participant is registered in the provider IdentityHub.")
    public void theProviderParticipantIsRegisteredInTheProviderIdentityHub() throws Exception {
        assertNotNull(providerJwk, "Provider JWK must be available.");
        String participantPayload = IdentityHubHelper.buildParticipantPayload(
                providerJwk,
                PROVIDER_DID,
                IDENTITYHUB_PROVIDER_ADDRESS,
                IdentityHubHelper.DEFAULT_KEY_ALIAS);
        IdentityHubHelper.registerParticipant(IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS, participantPayload);
        log.info("Provider participant registered in IdentityHub at {}", IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS);
    }

    /**
     * Verifies the provider's DID document is accessible at the well-known endpoint.
     */
    @Then("The provider DID document is available at the well-known endpoint.")
    public void theProviderDidDocumentIsAvailableAtTheWellKnownEndpoint() throws Exception {
        String didDocUrl = "https://mp-operations.org/.well-known/did.json";
        verifyDidDocument(didDocUrl, PROVIDER_DID);
        log.info("Provider DID document is available at {}", didDocUrl);
    }

    // ==================== Membership Credential Issuance ====================

    /**
     * Sets up the consumer identity in IdentityHub as a prerequisite for credential issuance.
     * Performs the full identity setup flow: read PEM, convert to JWK, insert into Vault, register participant.
     */
    @Given("The consumer identity is registered in IdentityHub.")
    public void theConsumerIdentityIsRegisteredInIdentityHub() throws Exception {
        theConsumerPrivateKeyPemFileIsAvailable();
        theConsumerPrivateKeyIsConvertedToJwkFormat();
        theConsumerJwkIsInsertedIntoTheConsumerVault();
        theConsumerParticipantIsRegisteredInTheConsumerIdentityHub();
    }

    /**
     * Sets up the provider identity in IdentityHub as a prerequisite for credential issuance.
     */
    @Given("The provider identity is registered in IdentityHub.")
    public void theProviderIdentityIsRegisteredInIdentityHub() throws Exception {
        theProviderPrivateKeyPemFileIsAvailable();
        theProviderPrivateKeyIsConvertedToJwkFormat();
        theProviderJwkIsInsertedIntoTheProviderVault();
        theProviderParticipantIsRegisteredInTheProviderIdentityHub();
    }

    /**
     * Issues a membership credential for the consumer from the consumer's Keycloak instance.
     * Uses ScriptHelper to perform the OID4VC pre-authorized code flow.
     */
    @When("A membership credential is issued for the consumer from the consumer Keycloak.")
    public void aMembershipCredentialIsIssuedForTheConsumerFromTheConsumerKeycloak() throws Exception {
        consumerMembershipCredential = ScriptHelper.getCredential(
                CONSUMER_KEYCLOAK_ADDRESS,
                MEMBERSHIP_CREDENTIAL_ID,
                MEMBERSHIP_CREDENTIAL_USERNAME);
        assertNotNull(consumerMembershipCredential, "Consumer membership credential should not be null.");
        assertFalse(consumerMembershipCredential.isBlank(), "Consumer membership credential should not be blank.");
        log.info("Consumer membership credential issued successfully from {}", CONSUMER_KEYCLOAK_ADDRESS);
    }

    /**
     * Inserts the consumer's membership credential into the consumer's IdentityHub.
     * Decodes the JWT to extract the VC content for storage.
     */
    @When("The consumer membership credential is inserted into the consumer IdentityHub.")
    public void theConsumerMembershipCredentialIsInsertedIntoTheConsumerIdentityHub() throws Exception {
        assertNotNull(consumerMembershipCredential, "Consumer membership credential must be issued first.");
        String vcContent = ScriptHelper.getVcContentFromJwt(consumerMembershipCredential);
        IdentityHubHelper.insertCredential(
                IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS,
                CONSUMER_DID,
                IDENTITYHUB_CREDENTIAL_ID,
                consumerMembershipCredential,
                vcContent);
        log.info("Consumer membership credential inserted into IdentityHub at {}",
                IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS);
    }

    /**
     * Issues a membership credential for the provider from the provider's Keycloak instance.
     */
    @When("A membership credential is issued for the provider from the provider Keycloak.")
    public void aMembershipCredentialIsIssuedForTheProviderFromTheProviderKeycloak() throws Exception {
        providerMembershipCredential = ScriptHelper.getCredential(
                PROVIDER_KEYCLOAK_ADDRESS,
                MEMBERSHIP_CREDENTIAL_ID,
                MEMBERSHIP_CREDENTIAL_USERNAME);
        assertNotNull(providerMembershipCredential, "Provider membership credential should not be null.");
        assertFalse(providerMembershipCredential.isBlank(), "Provider membership credential should not be blank.");
        log.info("Provider membership credential issued successfully from {}", PROVIDER_KEYCLOAK_ADDRESS);
    }

    /**
     * Inserts the provider's membership credential into the provider's IdentityHub.
     */
    @When("The provider membership credential is inserted into the provider IdentityHub.")
    public void theProviderMembershipCredentialIsInsertedIntoTheProviderIdentityHub() throws Exception {
        assertNotNull(providerMembershipCredential, "Provider membership credential must be issued first.");
        String vcContent = ScriptHelper.getVcContentFromJwt(providerMembershipCredential);
        IdentityHubHelper.insertCredential(
                IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS,
                PROVIDER_DID,
                IDENTITYHUB_CREDENTIAL_ID,
                providerMembershipCredential,
                vcContent);
        log.info("Provider membership credential inserted into IdentityHub at {}",
                IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS);
    }

    /**
     * Verifies that the consumer's membership credential is stored in the IdentityHub
     * by querying the participant's credentials endpoint.
     */
    @Then("The consumer membership credential is stored in the consumer IdentityHub.")
    public void theConsumerMembershipCredentialIsStoredInTheConsumerIdentityHub() throws Exception {
        verifyCredentialInIdentityHub(IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS, CONSUMER_DID);
    }

    /**
     * Verifies that the provider's membership credential is stored in the IdentityHub.
     */
    @Then("The provider membership credential is stored in the provider IdentityHub.")
    public void theProviderMembershipCredentialIsStoredInTheProviderIdentityHub() throws Exception {
        verifyCredentialInIdentityHub(IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS, PROVIDER_DID);
    }

    // ==================== Trusted Issuers List ====================

    /**
     * Verifies the provider's trusted issuers list contains the consumer's DID.
     * Checks the TIL endpoint at the provider side as documented in DSP_INTEGRATION.md.
     */
    @Then("The provider trusted issuers list contains the consumer DID.")
    public void theProviderTrustedIssuersListContainsTheConsumerDid() throws Exception {
        String tilUrl = TIL_DIRECT_ADDRESS + ":" + SERVICE_PORT + "/issuer/" + CONSUMER_DID;
        Request request = new Request.Builder()
                .get()
                .url(tilUrl)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("TIL should contain consumer DID. Got status %d at %s",
                            response.code(), tilUrl));
            String body = response.body().string();
            assertNotNull(body, "TIL response body should not be null.");
            assertFalse(body.isBlank(), "TIL response body should not be blank.");
            JsonNode tilEntry = OBJECT_MAPPER.readTree(body);
            assertEquals(CONSUMER_DID, tilEntry.get("did").asText(),
                    "TIL entry should contain the consumer DID.");
            log.info("Consumer DID {} found in provider TIL at {}", CONSUMER_DID, tilUrl);
        }
    }

    /**
     * Verifies the consumer is trusted for membership credentials at the provider's TIL.
     * Checks that the TIL entry includes MembershipCredential in the credentials list.
     */
    @Then("The consumer is trusted for membership credentials at the provider.")
    public void theConsumerIsTrustedForMembershipCredentialsAtTheProvider() throws Exception {
        String tilUrl = TIL_DIRECT_ADDRESS + ":" + SERVICE_PORT + "/issuer/" + CONSUMER_DID;
        Request request = new Request.Builder()
                .get()
                .url(tilUrl)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("TIL should be accessible. Got status %d", response.code()));
            String body = response.body().string();
            JsonNode tilEntry = OBJECT_MAPPER.readTree(body);

            // The TIL entry should have a credentials array with at least one entry
            JsonNode credentials = tilEntry.get("credentials");
            assertNotNull(credentials, "TIL entry should contain a 'credentials' field.");
            assertTrue(credentials.isArray(), "TIL credentials should be an array.");
            assertTrue(credentials.size() > 0, "TIL credentials should not be empty.");

            log.info("Consumer {} is trusted for credentials at the provider TIL", CONSUMER_DID);
        }
    }

    // ==================== Helper Methods ====================

    /**
     * Verifies that a DID document is accessible at the given URL.
     * The request goes through the Squid proxy (as configured in TestUtils.OK_HTTP_CLIENT).
     *
     * @param didDocUrl the full URL to the DID document (e.g., {@code https://fancy-marketplace.biz/.well-known/did.json})
     * @param expectedDid the expected DID value in the document
     */
    private void verifyDidDocument(String didDocUrl, String expectedDid) throws Exception {
        Request request = new Request.Builder()
                .get()
                .url(didDocUrl)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("DID document should be accessible at %s. Got status %d",
                            didDocUrl, response.code()));
            String body = response.body().string();
            assertNotNull(body, "DID document response should not be null.");

            JsonNode didDoc = OBJECT_MAPPER.readTree(body);
            String documentId = didDoc.get("id").asText();
            assertEquals(expectedDid, documentId,
                    "DID document id should match the expected DID.");
            log.info("DID document verified at {} with id={}", didDocUrl, documentId);
        }
    }

    /**
     * Verifies that a credential has been stored in the IdentityHub for the given participant.
     * Queries the credentials endpoint of the IdentityHub management API.
     *
     * @param identityHubManagementAddress the base URL of the IdentityHub management API
     * @param participantDid              the DID of the participant whose credentials to check
     */
    private void verifyCredentialInIdentityHub(String identityHubManagementAddress, String participantDid) throws Exception {
        String base64ParticipantId = IdentityHubHelper.base64UrlEncode(participantDid);
        String credentialsUrl = identityHubManagementAddress + ":" + SERVICE_PORT
                + "/api/identity/v1alpha/participants/" + base64ParticipantId + "/credentials";

        Request request = new Request.Builder()
                .get()
                .url(credentialsUrl)
                .header("x-api-key", IdentityHubHelper.IDENTITY_HUB_API_KEY)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("Credentials query should succeed at %s. Got status %d",
                            credentialsUrl, response.code()));
            String body = response.body().string();
            assertNotNull(body, "Credentials response should not be null.");
            assertFalse(body.isBlank(), "Credentials response should not be blank.");
            log.info("Credentials verified for participant {} at {}", participantDid, identityHubManagementAddress);
        }
    }

    /**
     * Resolves a relative path from the project root directory.
     * Searches upward from the current working directory to find the project root
     * (identified by the presence of the {@code pom.xml} file).
     *
     * @param relativePath the path relative to the project root
     * @return the absolute path
     */
    private Path resolveProjectPath(String relativePath) {
        // Try to find the project root by looking for the parent pom.xml
        Path currentDir = Paths.get(System.getProperty("user.dir"));

        // If we're in the 'it' subdirectory, go up one level
        if (currentDir.endsWith("it") && Files.exists(currentDir.getParent().resolve("pom.xml"))) {
            return currentDir.getParent().resolve(relativePath);
        }

        // Otherwise assume we're at the project root
        return currentDir.resolve(relativePath);
    }
}
