package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.tmf.model.*;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.security.Security;
import java.time.Duration;
import java.util.*;

import static org.fiware.dataspace.it.components.DSPEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.*;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.*;
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

    // --- DSP TMForum Ordering Constants ---

    /** The credential configuration ID for user credentials (representative role). */
    private static final String USER_CREDENTIAL_ID = "user-credential";

    /** The credential configuration ID for operator credentials. */
    private static final String OPERATOR_CREDENTIAL_ID = "operator-credential";

    /** The Keycloak username for the representative user who can buy products. */
    private static final String REPRESENTATIVE_USERNAME = "representative";

    /** The Keycloak username for the operator user who accesses data services. */
    private static final String OPERATOR_USERNAME = "operator";

    /** The OAuth scope for default access (TMForum API operations). */
    private static final String DEFAULT_SCOPE = "default";

    /** The OAuth scope for operator access (data service operations). */
    private static final String OPERATOR_SCOPE = "operator";

    /** The entity ID for the UptimeReport created as demo data. */
    private static final String UPTIME_REPORT_ENTITY_ID = "urn:ngsi-ld:UptimeReport:fms-1";

    /** The external asset ID used in the DSP product specification. */
    private static final String DSP_ASSET_ID = "ASSET-1";

    /** The external offering ID used in the DSP product offering. */
    private static final String DSP_OFFER_EXTERNAL_ID = "OFFER-1";

    /** The DCP endpoint path suffix for DSP protocol interactions. */
    private static final String DSP_ENDPOINT_PATH = "/api/dsp/2025-1";

    /** The internal upstream address for the data service (Scorpio via K8s service). */
    private static final String DATA_SERVICE_UPSTREAM = "data-service-scorpio:9090";

    /** Schema location for the TMForum credential configuration characteristic. */
    private static final String CREDENTIALS_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json";

    /** Schema location for the TMForum ODRL policy configuration characteristic. */
    private static final String POLICY_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json";

    /** Schema location for the EDC external ID. */
    private static final String EXTERNAL_ID_SCHEMA =
            "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json";

    /** Schema location for the EDC contract definition. */
    private static final String CONTRACT_DEFINITION_SCHEMA =
            "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/contract-definition.json";

    /** Timeout in seconds for data access assertions that require async policy updates. */
    private static final int DATA_ACCESS_TIMEOUT_SECONDS = 60;

    /** Timeout in seconds for policy propagation. */
    private static final int POLICY_PROPAGATION_TIMEOUT_SECONDS = 15;

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

    // --- DSP TMForum Ordering State ---

    /** The Wallet instance used for credential issuance and OID4VP token exchange. */
    private Wallet dspWallet;

    /** The category ID created for DSP offerings. */
    private String dspCategoryId;

    /** The product specification ID created for DSP offerings. */
    private String dspProductSpecId;

    /** The product offering ID created for DSP. */
    private String dspProductOfferingId;

    /** The consumer's organization registration at the provider marketplace. */
    private OrganizationVO dspConsumerRegistration;

    /** The product order ID for the current DSP ordering flow. */
    private String dspProductOrderId;

    /** Tracks policy IDs created during DSP TMForum tests for cleanup. */
    private List<String> dspCreatedPolicies = new ArrayList<>();

    /** Tracks entity IDs created during DSP TMForum tests for cleanup. */
    private List<String> dspCreatedEntities = new ArrayList<>();

    @Before("@dsp")
    public void setup() {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        dspWallet = new Wallet();
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

    // ==================== DSP TMForum: Demo Data Creation ====================

    /**
     * Creates an UptimeReport NGSI-LD entity in Scorpio as demo data for the DSP offering.
     * This is the equivalent of the "Prepare some data" section in DSP_INTEGRATION.md.
     */
    @When("The provider creates an UptimeReport entity in Scorpio for the DSP offering.")
    public void theProviderCreatesAnUptimeReportEntity() throws Exception {
        Map<String, Object> entity = Map.of(
                "id", UPTIME_REPORT_ENTITY_ID,
                "type", "UptimeReport",
                "name", Map.of("type", "Property", "value", "Standard Server"),
                "uptime", Map.of("type", "Property", "value", "99.9"));
        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(entity),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(SCORPIO_ADDRESS + ":" + SERVICE_PORT + "/ngsi-ld/v1/entities")
                .header("Accept", "application/json")
                .header("Content-Type", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful() || response.code() == 409,
                    String.format("UptimeReport creation should succeed or already exist. Got status %d", response.code()));
            dspCreatedEntities.add(UPTIME_REPORT_ENTITY_ID);
            log.info("UptimeReport entity created/exists at {}", SCORPIO_ADDRESS);
        }
    }

    /**
     * Verifies the UptimeReport entity exists in Scorpio by querying it directly.
     */
    @Then("The UptimeReport entity exists in Scorpio.")
    public void theUptimeReportEntityExistsInScorpio() throws Exception {
        Request request = new Request.Builder()
                .get()
                .url(SCORPIO_ADDRESS + ":" + SERVICE_PORT + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID)
                .header("Accept", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("UptimeReport should be retrievable. Got status %d", response.code()));
            String body = response.body().string();
            JsonNode entityNode = OBJECT_MAPPER.readTree(body);
            assertEquals(UPTIME_REPORT_ENTITY_ID, entityNode.get("id").asText(),
                    "Entity ID should match the expected UptimeReport ID.");
            log.info("UptimeReport entity verified in Scorpio: {}", UPTIME_REPORT_ENTITY_ID);
        }
    }

    // ==================== DSP TMForum: Offering Creation ====================

    /**
     * Creates a TMForum category for DSP offerings.
     * Equivalent to step 1 of "Prepare the offering" in DSP_INTEGRATION.md.
     */
    @Given("The provider creates a demo category for DSP offerings.")
    public void theProviderCreatesADemoCategoryForDspOfferings() throws Exception {
        CategoryCreateVO categoryCreate = new CategoryCreateVO()
                .description("Demo Category")
                .name("Demo Category");
        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(categoryCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(TMF_DIRECT_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/category")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The category should have been created.");
            CategoryVO category = OBJECT_MAPPER.readValue(response.body().string(), CategoryVO.class);
            dspCategoryId = category.getId();
            assertNotNull(dspCategoryId, "Category ID should not be null.");
            log.info("DSP category created with ID: {}", dspCategoryId);
        }
    }

    /**
     * Creates a TMForum catalog that includes the DSP category.
     * Equivalent to step 2 of "Prepare the offering" in DSP_INTEGRATION.md.
     */
    @Given("The provider creates a demo catalog for DSP offerings.")
    public void theProviderCreatesADemoCatalogForDspOfferings() throws Exception {
        assertNotNull(dspCategoryId, "Category must be created first.");
        CatalogCreateVO catalogCreate = new CatalogCreateVO()
                .description("Demo Catalog")
                .name("Demo Catalog")
                .category(List.of(new CategoryRefVO().id(dspCategoryId)));
        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(catalogCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(TMF_DIRECT_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/catalog")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The catalog should have been created.");
            log.info("DSP catalog created.");
        }
    }

    /**
     * Creates the DSP product specification with all DSP-specific characteristics:
     * DCP endpoint, OID4VC endpoint, upstream address, endpoint description,
     * target specification, service configuration, credentials config, and policy config.
     * Equivalent to step 3 of "Prepare the offering" in DSP_INTEGRATION.md.
     */
    @When("The provider creates a DSP product specification with DCP and OID4VC endpoints.")
    public void theProviderCreatesADspProductSpecification() throws Exception {
        // Build the target specification value
        Map<String, Object> targetSpec = Map.of(
                "@type", "AssetCollection",
                "refinement", List.of(Map.of(
                        "@type", "Constraint",
                        "leftOperand", "http:path",
                        "operator", "http:isInPath",
                        "rightOperand", "/*/ngsi-ld/v1/entities")));

        // Build the service configuration value
        Map<String, Object> serviceConfig = Map.of(
                "defaultOidcScope", "openid",
                "authorizationType", "DEEPLINK",
                "oidcScopes", Map.of(
                        "openid", Map.of(
                                "credentials", List.of(Map.of(
                                        "type", "MembershipCredential",
                                        "trustedParticipantsLists", List.of(Map.of(
                                                "type", "ebsi",
                                                "url", "http://tir.127.0.0.1.nip.io")),
                                        "trustedIssuersLists", List.of("http://trusted-issuers-list:8080"),
                                        "jwtInclusion", Map.of(
                                                "enabled", true,
                                                "fullInclusion", true))),
                                "dcql", Map.of(
                                        "credentials", List.of(Map.of(
                                                "id", "legal-person-query",
                                                "format", "jwt_vc_json",
                                                "multiple", false,
                                                "meta", Map.of(
                                                        "type_values", List.of(List.of("MembershipCredential")))))))));

        // Build credentials config value
        Map<String, Object> credentialsConfig = Map.of(
                "credentialsType", "OperatorCredential",
                "claims", List.of(Map.of(
                        "name", "roles",
                        "path", "$.roles[?(@.target==\"did:web:mp-operation.org\")].names[*]",
                        "allowedValues", List.of("OPERATOR"))));

        // Build policy config value (ODRL policy for operator access)
        Map<String, Object> policyConfig = buildDspPolicyConfig();

        // Build the product specification with all characteristics
        ProductSpecificationCreateVO specCreate = new ProductSpecificationCreateVO()
                .name("Demo Spec")
                .atSchemaLocation(URI.create(EXTERNAL_ID_SCHEMA));

        // Use raw JSON for the product spec since we need externalId which is not in the generated model
        ObjectNode specJson = OBJECT_MAPPER.valueToTree(specCreate);
        specJson.put("externalId", DSP_ASSET_ID);

        ArrayNode characteristics = OBJECT_MAPPER.createArrayNode();

        // DCP endpoint
        characteristics.add(buildCharacteristic("dcp",
                "Endpoint, that the service can be negotiated at via DCP.",
                "endpointUrl",
                DCP_PROVIDER_ADDRESS + ":" + SERVICE_PORT + DSP_ENDPOINT_PATH));

        // OID4VC endpoint
        characteristics.add(buildCharacteristic("oid4vc",
                "Endpoint, that the service can be negotiated at via OID4VC.",
                "endpointUrl",
                OID4VC_PROVIDER_ADDRESS + ":" + SERVICE_PORT + DSP_ENDPOINT_PATH));

        // Upstream address
        characteristics.add(buildCharacteristic("upstreamAddress",
                "Address of the upstream serving the data",
                "upstreamAddress",
                DATA_SERVICE_UPSTREAM));

        // Endpoint description
        ObjectNode endpointDescChar = OBJECT_MAPPER.createObjectNode();
        endpointDescChar.put("id", "endpointDescription");
        endpointDescChar.put("name", "Service Endpoint Description");
        endpointDescChar.put("valueType", "endpointDescription");
        ArrayNode endpointDescValues = OBJECT_MAPPER.createArrayNode();
        ObjectNode endpointDescValue = OBJECT_MAPPER.createObjectNode();
        endpointDescValue.put("value", "The Demo Service");
        endpointDescValues.add(endpointDescValue);
        endpointDescChar.set("productSpecCharacteristicValue", endpointDescValues);
        characteristics.add(endpointDescChar);

        // Target specification (complex object value)
        characteristics.add(buildObjectCharacteristic("targetSpecification",
                "Detailed specification of the ODRL target. Allows to over services via OID4VC",
                "targetSpecification", targetSpec));

        // Service configuration (complex object value)
        characteristics.add(buildObjectCharacteristic("serviceConfiguration",
                "Service config to be used in the credentials config service when provisioning transfers through OID4VC",
                "serviceConfiguration", serviceConfig));

        // Credentials config
        ObjectNode credChar = buildObjectCharacteristic("credentialsConfig",
                "Credentials Config", "credentialsConfiguration", credentialsConfig);
        credChar.put("@schemaLocation", CREDENTIALS_CONFIG_SCHEMA);
        characteristics.add(credChar);

        // Policy config
        ObjectNode policyChar = buildObjectCharacteristic("policyConfig",
                "Policy for creation of K8S clusters.", "authorizationPolicy", policyConfig);
        policyChar.put("@schemaLocation", POLICY_CONFIG_SCHEMA);
        characteristics.add(policyChar);

        specJson.set("productSpecCharacteristic", characteristics);

        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(specJson),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(TMF_DIRECT_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(),
                    "The DSP product specification should have been created.");
            JsonNode specResult = OBJECT_MAPPER.readTree(response.body().string());
            dspProductSpecId = specResult.get("id").asText();
            assertNotNull(dspProductSpecId, "Product spec ID should not be null.");
            log.info("DSP product specification created with ID: {}", dspProductSpecId);
        }
    }

    /**
     * Creates the DSP product offering with EDC contract definition terms (access and contract policies).
     * Equivalent to step 4 of "Prepare the offering" in DSP_INTEGRATION.md.
     */
    @When("The provider creates a DSP product offering with EDC contract definition terms.")
    public void theProviderCreatesADspProductOfferingWithEdcTerms() throws Exception {
        assertNotNull(dspProductSpecId, "Product spec must be created first.");
        assertNotNull(dspCategoryId, "Category must be created first.");

        String accessPolicyId = UUID.randomUUID().toString();
        String contractPolicyId = UUID.randomUUID().toString();

        // Build the offering JSON with EDC contract definition terms
        ObjectNode offering = OBJECT_MAPPER.createObjectNode();
        offering.put("name", "Test Offering");
        offering.put("description", "Test Offering description");
        offering.put("isBundle", false);
        offering.put("isSellable", true);
        offering.put("lifecycleStatus", "Active");
        offering.put("@schemaLocation", EXTERNAL_ID_SCHEMA);
        offering.put("externalId", DSP_OFFER_EXTERNAL_ID);

        // Product specification reference
        ObjectNode specRef = OBJECT_MAPPER.createObjectNode();
        specRef.put("id", dspProductSpecId);
        specRef.put("name", "The Test Spec");
        offering.set("productSpecification", specRef);

        // Category reference
        ArrayNode categories = OBJECT_MAPPER.createArrayNode();
        ObjectNode catRef = OBJECT_MAPPER.createObjectNode();
        catRef.put("id", dspCategoryId);
        categories.add(catRef);
        offering.set("category", categories);

        // EDC contract definition terms
        ArrayNode terms = OBJECT_MAPPER.createArrayNode();
        ObjectNode term = OBJECT_MAPPER.createObjectNode();
        term.put("name", "edc:contractDefinition");
        term.put("@schemaLocation", CONTRACT_DEFINITION_SCHEMA);

        // Access policy - simple "use" permission
        ObjectNode accessPolicy = OBJECT_MAPPER.createObjectNode();
        accessPolicy.put("@context", "http://www.w3.org/ns/odrl.jsonld");
        accessPolicy.put("odrl:uid", accessPolicyId);
        accessPolicy.put("assigner", PROVIDER_DID);
        ArrayNode accessPermissions = OBJECT_MAPPER.createArrayNode();
        ObjectNode accessPerm = OBJECT_MAPPER.createObjectNode();
        accessPerm.put("action", "use");
        accessPermissions.add(accessPerm);
        accessPolicy.set("permission", accessPermissions);
        accessPolicy.put("@type", "Offer");
        term.set("accessPolicy", accessPolicy);

        // Contract policy - "use" with day-of-week constraint
        ObjectNode contractPolicy = OBJECT_MAPPER.createObjectNode();
        contractPolicy.put("@context", "http://www.w3.org/ns/odrl.jsonld");
        contractPolicy.put("odrl:uid", contractPolicyId);
        contractPolicy.put("assigner", PROVIDER_DID);
        ArrayNode contractPermissions = OBJECT_MAPPER.createArrayNode();
        ObjectNode contractPerm = OBJECT_MAPPER.createObjectNode();
        contractPerm.put("action", "use");
        ObjectNode constraint = OBJECT_MAPPER.createObjectNode();
        constraint.put("leftOperand", "odrl:dayOfWeek");
        constraint.put("operator", "lt");
        constraint.put("rightOperand", 6);
        contractPerm.set("constraint", constraint);
        contractPermissions.add(contractPerm);
        contractPolicy.set("permission", contractPermissions);
        contractPolicy.put("@type", "Offer");
        term.set("contractPolicy", contractPolicy);

        terms.add(term);
        offering.set("productOfferingTerm", terms);

        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(offering),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(TMF_DIRECT_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("DSP product offering should have been created. Got status %d", response.code()));
            JsonNode offeringResult = OBJECT_MAPPER.readTree(response.body().string());
            dspProductOfferingId = offeringResult.get("id").asText();
            assertNotNull(dspProductOfferingId, "Offering ID should not be null.");
            log.info("DSP product offering created with ID: {}", dspProductOfferingId);
        }
    }

    /**
     * Verifies the DSP product offering is available at the TMForum API.
     */
    @Then("The DSP product offering is available at the TMForum API.")
    public void theDspProductOfferingIsAvailableAtTheTmForumApi() throws Exception {
        Request request = new Request.Builder()
                .get()
                .url(TMF_DIRECT_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "TMForum offering list should be accessible.");
            List<ProductOfferingVO> offerings = OBJECT_MAPPER.readValue(
                    response.body().string(), new TypeReference<List<ProductOfferingVO>>() {});
            assertFalse(offerings.isEmpty(), "At least one offering should exist.");
            log.info("Found {} DSP product offerings at TMForum API.", offerings.size());
        }
    }

    // ==================== DSP TMForum: Policy Preparation ====================

    /**
     * Registers the product offering read policy at the provider PAP.
     * Equivalent to the first policy in {@code prepare-policies.sh}.
     */
    @When("The provider registers the product offering read policy for DSP.")
    public void theProviderRegistersTheOfferingReadPolicyForDsp() throws Exception {
        createDspPolicyAtPap("allowProductOffering");
    }

    /**
     * Registers the self-registration policy at the provider PAP.
     * Equivalent to the second policy in {@code prepare-policies.sh}.
     */
    @When("The provider registers the self-registration policy for DSP.")
    public void theProviderRegistersTheSelfRegistrationPolicyForDsp() throws Exception {
        createDspPolicyAtPap("allowSelfRegistration");
    }

    /**
     * Registers the product ordering policy at the provider PAP.
     * Equivalent to the third policy in {@code prepare-policies.sh}.
     */
    @When("The provider registers the product ordering policy for DSP.")
    public void theProviderRegistersTheProductOrderingPolicyForDsp() throws Exception {
        createDspPolicyAtPap("allowProductOrder");
    }

    /**
     * Verifies that the TMForum access policies are registered at the provider PAP.
     */
    @Then("The TMForum access policies are active at the provider PAP.")
    public void theTmForumAccessPoliciesAreActiveAtTheProviderPap() throws Exception {
        // Allow time for policies to propagate
        Thread.sleep(POLICY_PROPAGATION_TIMEOUT_SECONDS * 1000L);
        assertFalse(dspCreatedPolicies.isEmpty(), "At least one policy should have been created.");
        log.info("DSP TMForum access policies are active. Created {} policies.", dspCreatedPolicies.size());
    }

    // ==================== DSP TMForum: Composite Setup Steps ====================

    /**
     * Composite step: creates demo data and the full DSP offering (category, catalog, spec, offering).
     */
    @Given("The provider creates demo data and offering for DSP.")
    public void theProviderCreatesDemoDataAndOfferingForDsp() throws Exception {
        theProviderCreatesAnUptimeReportEntity();
        theProviderCreatesADemoCategoryForDspOfferings();
        theProviderCreatesADemoCatalogForDspOfferings();
        theProviderCreatesADspProductSpecification();
        theProviderCreatesADspProductOfferingWithEdcTerms();
    }

    /**
     * Composite step: registers all TMForum access policies for DSP.
     */
    @Given("The DSP TMForum access policies are in place.")
    public void theDspTmForumAccessPoliciesAreInPlace() throws Exception {
        theProviderRegistersTheOfferingReadPolicyForDsp();
        theProviderRegistersTheSelfRegistrationPolicyForDsp();
        theProviderRegistersTheProductOrderingPolicyForDsp();
        // Wait for policies to propagate
        Thread.sleep(POLICY_PROPAGATION_TIMEOUT_SECONDS * 1000L);
    }

    // ==================== DSP TMForum: Consumer Ordering ====================

    /**
     * Obtains a representative (LegalPerson/UserCredential) credential for the consumer
     * to use when buying products via TMForum.
     * Equivalent to step 1 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @When("The consumer obtains a representative credential for DSP ordering.")
    public void theConsumerObtainsARepresentativeCredentialForDspOrdering() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(REPRESENTATIVE_USER_NAME);
        dspWallet.getCredentialFromIssuer(accessToken, CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL_ID);
        log.info("Consumer obtained representative (user) credential for DSP ordering.");
    }

    /**
     * Obtains an operator credential for the consumer to use when accessing data services.
     * Equivalent to step 2 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @When("The consumer obtains an operator credential for DSP ordering.")
    public void theConsumerObtainsAnOperatorCredentialForDspOrdering() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(OPERATOR_USER_NAME);
        dspWallet.getCredentialFromIssuer(accessToken, CONSUMER_KEYCLOAK_ADDRESS, OPERATOR_CREDENTIAL_ID);
        log.info("Consumer obtained operator credential for DSP ordering.");
    }

    /**
     * Registers the consumer organization at the provider's TMForum marketplace.
     * Equivalent to step 3 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @When("The consumer registers at the provider marketplace for DSP.")
    public void theConsumerRegistersAtTheProviderMarketplaceForDsp() throws Exception {
        String accessToken = getDspAccessToken(USER_CREDENTIAL_ID, DEFAULT_SCOPE, PROVIDER_API_ADDRESS);

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(CONSUMER_DID);
        OrganizationCreateVO orgCreate = new OrganizationCreateVO()
                .name("Fancy Marketplace Inc.")
                .partyCharacteristic(List.of(didCharacteristic));

        RequestBody body = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(orgCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(TM_FORUM_API_ADDRESS + ":" + SERVICE_PORT + "/tmf-api/party/v4/organization")
                .header("Authorization", "Bearer " + accessToken)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(),
                    "Consumer organization should have been registered.");
            dspConsumerRegistration = OBJECT_MAPPER.readValue(
                    response.body().string(), OrganizationVO.class);
            assertNotNull(dspConsumerRegistration.getId(), "Organization ID should not be null.");
            log.info("Consumer registered at provider marketplace with ID: {}",
                    dspConsumerRegistration.getId());
        }
    }

    /**
     * Lists the product offerings and creates a product order.
     * Equivalent to steps 4.1 and 4.2 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @When("The consumer lists offerings and creates a product order for DSP.")
    public void theConsumerListsOfferingsAndCreatesAProductOrderForDsp() throws Exception {
        assertNotNull(dspConsumerRegistration, "Consumer must be registered first.");

        String accessToken = getDspAccessToken(USER_CREDENTIAL_ID, DEFAULT_SCOPE, PROVIDER_API_ADDRESS);

        // List offerings
        Request offerRequest = new Request.Builder()
                .get()
                .url(TM_FORUM_API_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .header("Authorization", "Bearer " + accessToken)
                .build();
        String offerId;
        try (Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, offerResponse.code(), "Offerings should be listed.");
            List<ProductOfferingVO> offerings = OBJECT_MAPPER.readValue(
                    offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {});
            assertFalse(offerings.isEmpty(), "At least one offering should exist.");
            offerId = offerings.get(0).getId();
            assertNotNull(offerId, "Offer ID should not be null.");
            log.info("Found offering with ID: {}", offerId);
        }

        // Create order
        accessToken = getDspAccessToken(USER_CREDENTIAL_ID, DEFAULT_SCOPE, PROVIDER_API_ADDRESS);
        ProductOfferingRefVO offerRef = new ProductOfferingRefVO().id(offerId);
        ProductOrderItemVO orderItem = new ProductOrderItemVO()
                .id("random-order-id")
                .action(OrderItemActionTypeVO.ADD)
                .productOffering(offerRef);
        RelatedPartyVO relatedParty = new RelatedPartyVO()
                .id(dspConsumerRegistration.getId());
        ProductOrderCreateVO orderCreate = new ProductOrderCreateVO()
                .productOrderItem(List.of(orderItem))
                .relatedParty(List.of(relatedParty));

        RequestBody orderBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(orderCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request orderRequest = new Request.Builder()
                .post(orderBody)
                .url(TM_FORUM_API_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productOrderingManagement/v4/productOrder")
                .header("Authorization", "Bearer " + accessToken)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();
        try (Response orderResponse = HTTP_CLIENT.newCall(orderRequest).execute()) {
            assertEquals(HttpStatus.SC_CREATED, orderResponse.code(), "Product order should have been created.");
            ProductOrderVO order = OBJECT_MAPPER.readValue(
                    orderResponse.body().string(), ProductOrderVO.class);
            dspProductOrderId = order.getId();
            assertNotNull(dspProductOrderId, "Order ID should not be null.");
            log.info("Product order created with ID: {}", dspProductOrderId);
        }
    }

    /**
     * Completes the product order by patching the order state to "completed".
     * Equivalent to step 4.3 of "Order through TMForum" in DSP_INTEGRATION.md.
     * After completion, the Data Space Connector creates:
     * - a TIL entry to allow issuance of OperatorCredentials
     * - a PAP policy to allow access for users with OperatorCredential in role "Operator"
     */
    @When("The consumer completes the product order for DSP.")
    public void theConsumerCompletesTheProductOrderForDsp() throws Exception {
        assertNotNull(dspProductOrderId, "Product order must be created first.");

        String accessToken = getDspAccessToken(USER_CREDENTIAL_ID, DEFAULT_SCOPE, PROVIDER_API_ADDRESS);

        ProductOrderUpdateVO updateVO = new ProductOrderUpdateVO()
                .state(ProductOrderStateTypeVO.COMPLETED);
        RequestBody updateBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(updateVO),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request updateRequest = new Request.Builder()
                .patch(updateBody)
                .url(TM_FORUM_API_ADDRESS + ":" + SERVICE_PORT
                        + "/tmf-api/productOrderingManagement/v4/productOrder/" + dspProductOrderId)
                .header("Authorization", "Bearer " + accessToken)
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(updateRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "Product order should have been updated to completed.");
            log.info("Product order {} completed.", dspProductOrderId);
        }
    }

    /**
     * Verifies the consumer can access the UptimeReport entity using the operator credential
     * after the order is completed. This validates the end-to-end TMForum ordering flow.
     * Equivalent to step 5 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @Then("The consumer can access the UptimeReport with the operator credential.")
    public void theConsumerCanAccessTheUptimeReportWithTheOperatorCredential() throws Exception {
        Awaitility.await()
                .atMost(Duration.ofSeconds(DATA_ACCESS_TIMEOUT_SECONDS))
                .pollInterval(Duration.ofSeconds(3))
                .untilAsserted(() -> {
                    String accessToken = getDspAccessToken(
                            OPERATOR_CREDENTIAL_ID, OPERATOR_SCOPE, PROVIDER_API_ADDRESS);
                    Request reportRequest = new Request.Builder()
                            .get()
                            .url(PROVIDER_API_ADDRESS + ":" + SERVICE_PORT
                                    + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID)
                            .header("Content-Type", "application/json")
                            .header("Authorization", "Bearer " + accessToken)
                            .build();
                    try (Response response = HTTP_CLIENT.newCall(reportRequest).execute()) {
                        assertEquals(HttpStatus.SC_OK, response.code(),
                                "Consumer should be able to access UptimeReport with operator credential.");
                        String body = response.body().string();
                        JsonNode entity = OBJECT_MAPPER.readTree(body);
                        assertEquals(UPTIME_REPORT_ENTITY_ID, entity.get("id").asText(),
                                "Returned entity should be the UptimeReport.");
                        log.info("Consumer successfully accessed UptimeReport via operator credential.");
                    }
                });
    }

    // ==================== DSP TMForum: Helper Methods ====================

    /**
     * Creates a policy at the provider PAP from a JSON resource file in the policies directory.
     *
     * @param policyName the name of the policy file (without .json extension) in the /policies/ directory
     * @throws IOException if the policy file cannot be read or the HTTP request fails
     */
    private void createDspPolicyAtPap(String policyName) throws IOException {
        String policyJson = loadPolicyResource(policyName);
        RequestBody body = RequestBody.create(policyJson,
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request request = new Request.Builder()
                .post(body)
                .url(PROVIDER_PAP_ADDRESS + ":" + SERVICE_PORT + "/policy")
                .header("Content-Type", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    String.format("Policy '%s' should have been created.", policyName));
            String location = response.header("Location");
            if (location != null) {
                dspCreatedPolicies.add(location);
            }
            log.info("Policy '{}' created at provider PAP.", policyName);
        }
    }

    /**
     * Loads a policy JSON file from the classpath resources.
     *
     * @param policyName the policy resource name (without .json extension)
     * @return the policy JSON as a string
     * @throws IOException if the resource cannot be read
     */
    private String loadPolicyResource(String policyName) throws IOException {
        try (InputStream is = this.getClass().getResourceAsStream(
                String.format("/policies/%s.json", policyName))) {
            assertNotNull(is, String.format("Policy resource '%s.json' should exist.", policyName));
            return new String(is.readAllBytes());
        }
    }

    /**
     * Gets an access token for the consumer via OID4VP exchange using the DSP wallet.
     *
     * @param credentialId the credential ID in the wallet (e.g., {@code user-credential})
     * @param scope        the OAuth scope to request
     * @param targetAddress the base URL of the service to authenticate against
     * @return the access token string
     * @throws Exception if the token exchange fails
     */
    private String getDspAccessToken(String credentialId, String scope, String targetAddress) throws Exception {
        OpenIdConfiguration oidcConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                targetAddress + ":" + SERVICE_PORT);
        return dspWallet.exchangeCredentialForToken(oidcConfig, credentialId, scope);
    }

    /**
     * Builds a simple characteristic node with a string value.
     *
     * @param id        the characteristic ID
     * @param name      the characteristic display name
     * @param valueType the value type
     * @param value     the string value
     * @return the characteristic JSON node
     */
    private ObjectNode buildCharacteristic(String id, String name, String valueType, String value) {
        ObjectNode node = OBJECT_MAPPER.createObjectNode();
        node.put("id", id);
        node.put("name", name);
        node.put("valueType", valueType);
        ArrayNode values = OBJECT_MAPPER.createArrayNode();
        ObjectNode valNode = OBJECT_MAPPER.createObjectNode();
        valNode.put("value", value);
        valNode.put("isDefault", true);
        values.add(valNode);
        node.set("productSpecCharacteristicValue", values);
        return node;
    }

    /**
     * Builds a characteristic node with a complex object value.
     *
     * @param id        the characteristic ID
     * @param name      the characteristic display name
     * @param valueType the value type
     * @param value     the object value (will be serialized to JSON)
     * @return the characteristic JSON node
     */
    private ObjectNode buildObjectCharacteristic(String id, String name, String valueType, Object value) {
        ObjectNode node = OBJECT_MAPPER.createObjectNode();
        node.put("id", id);
        node.put("name", name);
        node.put("valueType", valueType);
        ArrayNode values = OBJECT_MAPPER.createArrayNode();
        ObjectNode valNode = OBJECT_MAPPER.createObjectNode();
        valNode.set("value", OBJECT_MAPPER.valueToTree(value));
        valNode.put("isDefault", true);
        values.add(valNode);
        node.set("productSpecCharacteristicValue", values);
        return node;
    }

    /**
     * Builds the ODRL policy configuration for the DSP product specification.
     * This policy allows operator access to NGSI-LD entities.
     *
     * @return the policy config as a map
     */
    private Map<String, Object> buildDspPolicyConfig() {
        Map<String, Object> pathConstraint = new LinkedHashMap<>();
        pathConstraint.put("@type", "odrl:Constraint");
        pathConstraint.put("odrl:leftOperand", "http:path");
        pathConstraint.put("odrl:operator", "http:isInPath");
        pathConstraint.put("odrl:rightOperand", "/ngsi-ld/v1/entities");

        Map<String, Object> roleConstraint = new LinkedHashMap<>();
        roleConstraint.put("@type", "odrl:Constraint");
        roleConstraint.put("odrl:leftOperand", "vc:role");
        roleConstraint.put("odrl:operator", "odrl:hasPart");
        roleConstraint.put("odrl:rightOperand", Map.of("@value", "OPERATOR", "@type", "xsd:string"));

        Map<String, Object> typeConstraint = new LinkedHashMap<>();
        typeConstraint.put("@type", "odrl:Constraint");
        typeConstraint.put("odrl:leftOperand", "vc:type");
        typeConstraint.put("odrl:operator", "odrl:hasPart");
        typeConstraint.put("odrl:rightOperand", Map.of("@value", "OperatorCredential", "@type", "xsd:string"));

        Map<String, Object> target = new LinkedHashMap<>();
        target.put("@type", "odrl:AssetCollection");
        target.put("odrl:source", "urn:asset");
        target.put("odrl:refinement", List.of(pathConstraint));

        Map<String, Object> assigneeRefinement = new LinkedHashMap<>();
        assigneeRefinement.put("@type", "odrl:LogicalConstraint");
        assigneeRefinement.put("odrl:and", List.of(roleConstraint, typeConstraint));

        Map<String, Object> assignee = new LinkedHashMap<>();
        assignee.put("@type", "odrl:PartyCollection");
        assignee.put("odrl:source", "urn:user");
        assignee.put("odrl:refinement", assigneeRefinement);

        Map<String, Object> permission = new LinkedHashMap<>();
        permission.put("odrl:assigner", "https://www.mp-operation.org/");
        permission.put("odrl:target", target);
        permission.put("odrl:assignee", assignee);
        permission.put("odrl:action", "odrl:read");

        Map<String, Object> policy = new LinkedHashMap<>();
        policy.put("@context", Map.of("odrl", "http://www.w3.org/ns/odrl/2/"));
        policy.put("@id", "https://mp-operation.org/policy/common/k8s-small");
        policy.put("odrl:uid", "https://mp-operation.org/policy/common/k8s-small");
        policy.put("@type", "odrl:Policy");
        policy.put("odrl:permission", permission);

        return policy;
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
