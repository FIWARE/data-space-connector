package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import io.cucumber.java.Before;
import io.cucumber.java.an.E;
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
import org.bouncycastle.jcajce.provider.asymmetric.ec.KeyFactorySpi;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.DataAddress;
import org.fiware.dataspace.it.components.model.DcpCredential;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.IdResponse;
import org.fiware.dataspace.tmf.model.*;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.PrivateKey;
import java.security.Security;
import java.time.Duration;
import java.util.*;

import static org.fiware.dataspace.it.components.DSPEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.*;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.*;
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
public class DSPStepDefinitions extends StepDefintions {

    /**
     * HTTP port used by local services in the k3s deployment.
     */
//    private static final int SERVICE_PORT = 8080;

    /**
     * The credential configuration ID for membership credentials in Keycloak.
     */
    private static final String MEMBERSHIP_CREDENTIAL_ID = "membership-credential";

    /**
     * The Keycloak username used for issuing membership credentials.
     */
    private static final String MEMBERSHIP_CREDENTIAL_USERNAME = "employee";

    // --- DSP TMForum Ordering Constants ---

    /**
     * The credential configuration ID for user credentials (representative role).
     */
    private static final String USER_CREDENTIAL_ID = "user-credential";

    /**
     * The credential configuration ID for operator credentials.
     */
    private static final String OPERATOR_CREDENTIAL_ID = "operator-credential";

    /**
     * The Keycloak username for the representative user who can buy products.
     */
    private static final String REPRESENTATIVE_USERNAME = "representative";

    /**
     * The Keycloak username for the operator user who accesses data services.
     */
    private static final String OPERATOR_USERNAME = "operator";

    /**
     * The OAuth scope for default access (TMForum API operations).
     */
    private static final String DEFAULT_SCOPE = "default";

    /**
     * The OAuth scope for operator access (data service operations).
     */
    private static final String OPERATOR_SCOPE = "operator";

    /**
     * The entity ID for the UptimeReport created as demo data.
     */
    private static final String UPTIME_REPORT_ENTITY_ID = "urn:ngsi-ld:UptimeReport:fms-1";

    /**
     * The external asset ID used in the DSP product specification.
     */
    private static final String DSP_ASSET_ID = "ASSET-1";

    /**
     * The external offering ID used in the DSP product offering.
     */
    private static final String DSP_OFFER_EXTERNAL_ID = "OFFER-1";

    /**
     * The DCP endpoint path suffix for DSP protocol interactions.
     */
    private static final String DSP_ENDPOINT_PATH = "/api/dsp/2025-1";

    /**
     * The internal upstream address for the data service (Scorpio via K8s service).
     */
    private static final String DATA_SERVICE_UPSTREAM = "data-service-scorpio:9090";

    /**
     * Schema location for the TMForum credential configuration characteristic.
     */
    private static final String CREDENTIALS_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json";

    /**
     * Schema location for the TMForum ODRL policy configuration characteristic.
     */
    private static final String POLICY_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json";

    /**
     * Schema location for the EDC external ID.
     */
    private static final String EXTERNAL_ID_SCHEMA =
            "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json";

    /**
     * Schema location for the EDC contract definition.
     */
    private static final String CONTRACT_DEFINITION_SCHEMA =
            "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/contract-definition.json";

    /**
     * Timeout in seconds for data access assertions that require async policy updates.
     */
    private static final int DATA_ACCESS_TIMEOUT_SECONDS = 60;

    /**
     * Timeout in seconds for policy propagation.
     */
    private static final int POLICY_PROPAGATION_TIMEOUT_SECONDS = 15;

    /**
     * The IdentityHub credential ID used when storing membership credentials.
     */
    private static final String IDENTITYHUB_CREDENTIAL_ID = "membership-credential";

    private static final OkHttpClient HTTP_CLIENT = OK_HTTP_CLIENT;

    /**
     * The consumer's private key in JWK format, populated during identity setup.
     */
    private String consumerJwk;

    /**
     * The provider's private key in JWK format, populated during identity setup.
     */
    private String providerJwk;

    /**
     * The consumer's PEM key content, read from the file system.
     */
    private String consumerPemContent;

    /**
     * The provider's PEM key content, read from the file system.
     */
    private String providerPemContent;

    /**
     * The raw JWT membership credential for the consumer.
     */
    private String consumerMembershipCredential;

    /**
     * The raw JWT membership credential for the provider.
     */
    private String providerMembershipCredential;

    // --- DSP TMForum Ordering State ---

    /**
     * The Wallet instance used for credential issuance and OID4VP token exchange.
     */
    private Wallet dspWallet;

    /**
     * The category ID created for DSP offerings.
     */
    private String dspCategoryId;

    /**
     * The product specification ID created for DSP offerings.
     */
    private String dspProductSpecId;

    /**
     * The product offering ID created for DSP.
     */
    private String dspProductOfferingId;

    /**
     * The consumer's organization registration at the provider marketplace.
     */
    private OrganizationVO dspConsumerRegistration;

    /**
     * The product order ID for the current DSP ordering flow.
     */
    private String dspProductOrderId;

    /**
     * Tracks policy IDs created during DSP TMForum tests for cleanup.
     */
    private List<String> dspCreatedPolicies = new ArrayList<>();

    /**
     * Tracks entity IDs created during DSP TMForum tests for cleanup.
     */
    private List<String> dspCreatedEntities = new ArrayList<>();

    // --- DCP Protocol Flow State ---

    /**
     * The DCP catalog response JSON, populated during catalog request.
     */
    private JsonNode dcpCatalogResponse;

    /**
     * The ID from the DCP negotiation.
     */
    private String dcpNegotiationId;

    /**
     * The ID from the OID4VC negotiation.
     */
    private String oid4vcNegotiationId;


    /**
     * The contract agreement ID from a finalized DCP negotiation.
     */
    private String dcpAgreementId;

    /**
     * The contract agreement ID from a finalized OID4VC negotiation.
     */
    private String oid4vcAgreementId;

    /**
     * The transfer process ID from a started DCP transfer.
     */
    private String dcpTransferId;

    /**
     * The EDR data address (endpoint + token) from a completed DCP transfer.
     */
    private DataAddress dcpDataAddress;

    // --- OID4VC Protocol Flow State ---

    /**
     * The OID4VC catalog response JSON, populated during catalog request.
     */
    private JsonNode oid4vcCatalogResponse;

    /**
     * The transfer process ID from a started OID4VC transfer.
     */
    private String oid4vcTransferId;

    /**
     * The EDR data address (endpoint + token) from a completed OID4VC transfer.
     */
    private DataAddress oid4vcDataAddress;

    /**
     * The OID4VP access token obtained by exchanging a membership credential at the OID4VC endpoint.
     */
    private String oid4vcAccessToken;

    /**
     * The OpenID configuration retrieved from the OID4VC transfer endpoint.
     */
    private OpenIdConfiguration oid4vcEndpointOidcConfig;

    /**
     * The scope used for OID4VP authentication in OID4VC flows.
     */
    private static final String OPENID_SCOPE = "openid";

    /**
     * Setup hook executed before each {@code @dsp} scenario.
     * Initializes cryptographic providers, creates a fresh Wallet instance,
     * and cleans up stale DSP resources from previous runs.
     */
    @Before("@dsp")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        dspWallet = new Wallet();
        log.debug("DSP test setup: cleaning up stale resources.");
        try {
            cleanDspResources();
        } catch (Exception e) {
            log.warn("Error during DSP pre-test cleanup: {}", e.getMessage());
        }
        prepareTil();
        // give the system time to propagate the cleanups
        Thread.sleep(1000);
    }

    /**
     * Cleans up all resources created during DSP tests.
     * Each cleanup method handles its own exceptions to ensure partial failures
     * do not prevent other resources from being cleaned.
     */
    private void cleanDspResources() throws Exception {
        cleanUpDspTMForum();
        cleanUpDspPolicies();
        cleanUpDspEntities();
        cleanUpDspDcatCatalogs();
        cleanUpDspAgreements();
        cleanUpTIL();
        cleanUpDspVaultKeys();
        cleanUpDspIdentityHubParticipants();
    }

    /**
     * Cleans up TMForum resources at the provider's direct TMForum API.
     * Removes orders, offerings, specifications, catalogs, categories, and organizations.
     */
    private void cleanUpDspTMForum() {
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productOrderingManagement/v4/productOrder", "DSP orders");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/productOffering", "DSP offerings");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/productSpecification", "DSP specs");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/catalog", "DSP catalogs");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/category", "DSP categories");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/party/v4/organization", "DSP organizations");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/quote/v4/quote", "DSP quotes");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/agreementManagement/v4/agreement", "DSP agreements");
    }

    private void cleanUpDspPolicies() {
        cleanUpPolicies(PROVIDER_PAP_ADDRESS);
    }

    /**
     * Cleans up entities created in Scorpio during DSP tests.
     */
    private void cleanUpDspEntities() {
        for (String entityId : dspCreatedEntities) {
            try {
                Request deleteRequest = new Request.Builder()
                        .delete()
                        .url(SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + entityId)
                        .build();
                try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                    log.debug("Deleted entity {}: status={}", entityId, resp.code());
                }
            } catch (Exception e) {
                log.warn("Failed to clean up entity {}: {}", entityId, e.getMessage());
            }
        }
        dspCreatedEntities.clear();
    }

    /**
     * Cleans up DCAT catalogs from the provider's Rainbow instance.
     */
    private void cleanUpDspDcatCatalogs() {
        try {
            Request listRequest = new Request.Builder()
                    .get()
                    .url(RAINBOW_DIRECT_ADDRESS + "/api/v1/catalogs")
                    .build();
            try (Response response = HTTP_CLIENT.newCall(listRequest).execute()) {
                okhttp3.ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    return;
                }
                String bodyString = responseBody.string();
                List<java.util.Map<String, Object>> catalogs;
                try {
                    catalogs = OBJECT_MAPPER.readValue(bodyString,
                            new TypeReference<List<java.util.Map<String, Object>>>() {
                            });
                } catch (Exception e) {
                    log.warn("Could not parse DCAT catalogs: {}", e.getMessage());
                    return;
                }
                for (java.util.Map<String, Object> catalog : catalogs) {
                    Object id = catalog.get("id");
                    if (id == null) continue;
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(RAINBOW_DIRECT_ADDRESS + "/api/v1/catalogs/" + id)
                            .build();
                    try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.debug("Deleted DCAT catalog {}: status={}", id, resp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up DCAT catalogs: {}", e.getMessage());
        }
    }

    /**
     * Cleans up agreements from the provider's Rainbow instance.
     */
    private void cleanUpDspAgreements() {
        try {
            Request listRequest = new Request.Builder()
                    .get()
                    .url(RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements")
                    .build();
            try (Response response = HTTP_CLIENT.newCall(listRequest).execute()) {
                okhttp3.ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    return;
                }
                String bodyString = responseBody.string();
                List<java.util.Map<String, Object>> agreements;
                try {
                    agreements = OBJECT_MAPPER.readValue(bodyString,
                            new TypeReference<List<java.util.Map<String, Object>>>() {
                            });
                } catch (Exception e) {
                    log.warn("Could not parse agreements: {}", e.getMessage());
                    return;
                }
                for (java.util.Map<String, Object> agreement : agreements) {
                    Object id = agreement.get("agreementId");
                    if (id == null) id = agreement.get("id");
                    if (id == null) continue;
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements/" + id)
                            .build();
                    try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.debug("Deleted agreement {}: status={}", id, resp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up agreements: {}", e.getMessage());
        }
    }

    /**
     * Cleans up the consumer's TIL entry at the provider and restores baseline trust configuration.
     */
    private void cleanUpDspTIL() {
        try {
            Request deleteRequest = new Request.Builder()
                    .delete()
                    .url(TIL_DIRECT_ADDRESS + "/issuer/" + CONSUMER_DID)
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                log.debug("TIL cleanup for consumer DID: status={}", resp.code());
            }

            // Restore baseline TIL entry
            String tilPayload = OBJECT_MAPPER.writeValueAsString(
                    java.util.Map.of(
                            "did", CONSUMER_DID,
                            "credentials", java.util.List.of(
                                    java.util.Map.of(
                                            "credentialsType", "UserCredential",
                                            "claims", java.util.List.of()))));
            RequestBody body = RequestBody.create(tilPayload,
                    okhttp3.MediaType.parse("application/json"));
            Request postRequest = new Request.Builder()
                    .post(body)
                    .url(TIL_DIRECT_ADDRESS + "/issuer")
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(postRequest).execute()) {
                log.debug("TIL baseline restore: status={}", resp.code());
            }
        } catch (Exception e) {
            log.warn("Failed to clean up DSP TIL entries: {}", e.getMessage());
        }
    }

    /**
     * Cleans up keys inserted into the consumer and provider Vault instances during DSP tests.
     */
    private void cleanUpDspVaultKeys() {
        deleteVaultKey(VAULT_CONSUMER_ADDRESS, IdentityHubHelper.DEFAULT_KEY_ALIAS);
        deleteVaultKey(VAULT_PROVIDER_ADDRESS, IdentityHubHelper.DEFAULT_KEY_ALIAS);
    }

    /**
     * Deletes a key from a Vault instance.
     *
     * @param vaultAddress the base URL of the Vault instance
     * @param keyAlias     the alias of the key to delete
     */
    private void deleteVaultKey(String vaultAddress, String keyAlias) {
        try {
            Request deleteRequest = new Request.Builder()
                    .delete()
                    .url(vaultAddress + "/v1/secret/data/" + keyAlias)
                    .addHeader("X-Vault-Token", IdentityHubHelper.VAULT_TOKEN)
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                log.debug("Vault key cleanup at {}/{}: status={}", vaultAddress, keyAlias, resp.code());
            }
        } catch (Exception e) {
            log.warn("Failed to delete Vault key {}/{}: {}", vaultAddress, keyAlias, e.getMessage());
        }
    }

    /**
     * Cleans up participants registered in the consumer and provider IdentityHub instances.
     * Uses the IdentityHub management API to list and delete participants.
     */
    private void cleanUpDspIdentityHubParticipants() {
        deleteIdentityHubParticipant(IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS, CONSUMER_DID);
        deleteIdentityHubParticipant(IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS, PROVIDER_DID);
    }

    /**
     * Deletes a participant from an IdentityHub management API.
     *
     * @param managementAddress the base URL of the IdentityHub management API
     * @param participantDid    the DID of the participant to delete
     */
    private void deleteIdentityHubParticipant(String managementAddress, String participantDid) {
        try {
            // URL-encode the DID for the path parameter
            String encodedDid = Base64.getEncoder().encodeToString(participantDid.getBytes(StandardCharsets.UTF_8));

            // get all credentials, participant removal does not delete them
            Request getCreds = new Request.Builder()
                    .get()
                    .url(managementAddress
                            + "/api/identity/v1alpha/participants/" + encodedDid + "/credentials")
                    .addHeader("x-api-key", IdentityHubHelper.IDENTITY_HUB_API_KEY)
                    .build();
            List<String> credentialIds = new ArrayList<>();
            try (Response resp = HTTP_CLIENT.newCall(getCreds).execute()) {
                OBJECT_MAPPER.readValue(resp.body().string(), new TypeReference<List<DcpCredential>>() {
                        })
                        .stream()
                        .map(DcpCredential::getId)
                        .forEach(credentialIds::add);
            }
            credentialIds
                    .forEach(credentialId -> {
                        Request credDelete = new Request.Builder()
                                .delete()
                                .url(managementAddress
                                        + "/api/identity/v1alpha/participants/" + encodedDid + "/credentials/" + credentialId)
                                .addHeader("x-api-key", IdentityHubHelper.IDENTITY_HUB_API_KEY)
                                .build();
                        try (Response resp = HTTP_CLIENT.newCall(credDelete).execute()) {
                            log.debug("Deleted credential {} - code {}", credentialId, resp.code());
                        } catch (IOException e) {
                            log.warn("Failed to delete credential {}", credentialId, e);
                        }
                    });

            Request deleteRequest = new Request.Builder()
                    .delete()
                    .url(managementAddress
                            + "/api/identity/v1alpha/participants/" + encodedDid)
                    .addHeader("x-api-key", IdentityHubHelper.IDENTITY_HUB_API_KEY)
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                log.debug("IdentityHub participant cleanup at {} {}: status={}", managementAddress, participantDid, resp.code());
            }
        } catch (Exception e) {
            log.warn("Failed to delete IdentityHub participant {}: {}", participantDid, e.getMessage());
        }
    }


    // ==================== Central Marketplace Step Definitions ====================
    // Step definitions for @central scenarios will be added in Step 8.

    // ==================== Consumer Identity Setup ====================

    /**
     * Retrieves the consumer's PKCS#8 private key from the cert-manager {@code signing-key}
     * Kubernetes secret in the {@code consumer} namespace.
     */
    @Given("The consumer private key is retrieved from the Kubernetes signing-key secret.")
    public void theConsumerPrivateKeyIsRetrievedFromTheKubernetesSecret() throws Exception {
        consumerPemContent = KubernetesHelper.fetchTlsKeyFromSecret(CONSUMER_NAMESPACE, CONSUMER_SIGNING_KEY_SECRET_NAME);
        assertFalse(consumerPemContent.isBlank(), "Consumer PEM content from Kubernetes secret should not be empty.");
        log.debug("Consumer private key retrieved from Kubernetes secret '{}' in namespace '{}'.",
                CONSUMER_SIGNING_KEY_SECRET_NAME, CONSUMER_NAMESPACE);
    }

    /**
     * Converts the consumer's PEM private key to JWK format using IdentityHubHelper.
     * This is the Java equivalent of running {@code get-private-jwk-from-k8s-secret.sh}.
     */
    @When("The consumer private key is converted to JWK format.")
    public void theConsumerPrivateKeyIsConvertedToJwkFormat() throws Exception {
        assertNotNull(consumerPemContent, "Consumer PEM content must be available.");
        PrivateKey privateKey = IdentityHubHelper.loadPrivateKeyFromPemContent(consumerPemContent);

        consumerJwk = IdentityHubHelper.asJWK(privateKey);
        // Verify it's valid JSON with expected EC key fields
        JsonNode jwkNode = OBJECT_MAPPER.readTree(consumerJwk);
        assertEquals("EC", jwkNode.get("kty").asText(), "Key type should be EC.");
        assertEquals("P-256", jwkNode.get("crv").asText(), "Curve should be P-256.");
        assertTrue(jwkNode.has("d"), "JWK should contain private key component 'd'.");
        log.debug("Consumer private key successfully converted to JWK format.");
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
        log.debug("Consumer JWK inserted into Vault at {}", VAULT_CONSUMER_ADDRESS);
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
        log.debug("Consumer participant registered in IdentityHub at {}", IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS);
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
        log.debug("Consumer DID document is available at {}", didDocUrl);
    }

    // ==================== Provider Identity Setup ====================

    /**
     * Retrieves the provider's PKCS#8 private key from the cert-manager {@code signing-key}
     * Kubernetes secret in the {@code provider} namespace.
     */
    @Given("The provider private key is retrieved from the Kubernetes signing-key secret.")
    public void theProviderPrivateKeyIsRetrievedFromTheKubernetesSecret() throws Exception {
        providerPemContent = KubernetesHelper.fetchTlsKeyFromSecret(PROVIDER_NAMESPACE, PROVIDER_SIGNING_KEY_SECRET_NAME);
        assertFalse(providerPemContent.isBlank(), "Provider PEM content from Kubernetes secret should not be empty.");
        log.debug("Provider private key retrieved from Kubernetes secret '{}' in namespace '{}'.",
                PROVIDER_SIGNING_KEY_SECRET_NAME, PROVIDER_NAMESPACE);
    }

    /**
     * Converts the provider's PEM private key to JWK format using IdentityHubHelper.
     */
    @When("The provider private key is converted to JWK format.")
    public void theProviderPrivateKeyIsConvertedToJwkFormat() throws Exception {
        assertNotNull(providerPemContent, "Provider PEM content must be available.");
        PrivateKey privateKey = IdentityHubHelper.loadPrivateKeyFromPemContent(providerPemContent);
        providerJwk = IdentityHubHelper.asJWK(privateKey);
        assertNotNull(providerJwk, "Provider JWK should not be null.");
        JsonNode jwkNode = OBJECT_MAPPER.readTree(providerJwk);
        assertEquals("EC", jwkNode.get("kty").asText(), "Key type should be EC.");
        assertEquals("P-256", jwkNode.get("crv").asText(), "Curve should be P-256.");
        assertTrue(jwkNode.has("d"), "JWK should contain private key component 'd'.");
        log.debug("Provider private key successfully converted to JWK format.");
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
        log.debug("Provider JWK inserted into Vault at {}", VAULT_PROVIDER_ADDRESS);
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
        log.debug("Provider participant registered in IdentityHub at {}", IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS);
    }

    /**
     * Verifies the provider's DID document is accessible at the well-known endpoint.
     */
    @Then("The provider DID document is available at the well-known endpoint.")
    public void theProviderDidDocumentIsAvailableAtTheWellKnownEndpoint() throws Exception {
        String didDocUrl = "https://mp-operations.org/.well-known/did.json";
        verifyDidDocument(didDocUrl, PROVIDER_DID);
        log.debug("Provider DID document is available at {}", didDocUrl);
    }

    // ==================== Membership Credential Issuance ====================

    /**
     * Sets up the consumer identity in IdentityHub as a prerequisite for credential issuance.
     * Performs the full identity setup flow: fetch key from k8s, convert to JWK, insert into Vault, register participant.
     */
    @Given("The consumer identity is registered in IdentityHub.")
    public void theConsumerIdentityIsRegisteredInIdentityHub() throws Exception {
        theConsumerPrivateKeyIsRetrievedFromTheKubernetesSecret();
        theConsumerPrivateKeyIsConvertedToJwkFormat();
        theConsumerJwkIsInsertedIntoTheConsumerVault();
        theConsumerParticipantIsRegisteredInTheConsumerIdentityHub();
    }

    /**
     * Sets up the provider identity in IdentityHub as a prerequisite for credential issuance.
     */
    @Given("The provider identity is registered in IdentityHub.")
    public void theProviderIdentityIsRegisteredInIdentityHub() throws Exception {
        theProviderPrivateKeyIsRetrievedFromTheKubernetesSecret();
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
        log.debug("Consumer membership credential issued successfully from {}", CONSUMER_KEYCLOAK_ADDRESS);
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
        log.debug("Consumer membership credential inserted into IdentityHub at {}",
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
        log.debug("Provider membership credential issued successfully from {}", PROVIDER_KEYCLOAK_ADDRESS);
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
        log.debug("Provider membership credential inserted into IdentityHub at {}",
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
        String tilUrl = TIL_DIRECT_ADDRESS + "/issuer/" + CONSUMER_DID;
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
            log.debug("Consumer DID {} found in provider TIL at {}", CONSUMER_DID, tilUrl);
        }
    }

    /**
     * Verifies the consumer is trusted for membership credentials at the provider's TIL.
     * Checks that the TIL entry includes MembershipCredential in the credentials list.
     */
    @Then("The consumer is trusted for membership credentials at the provider.")
    public void theConsumerIsTrustedForMembershipCredentialsAtTheProvider() throws Exception {
        String tilUrl = TIL_DIRECT_ADDRESS + "/issuer/" + CONSUMER_DID;
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

            log.debug("Consumer {} is trusted for credentials at the provider TIL", CONSUMER_DID);
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
                .url(SCORPIO_ADDRESS + "/ngsi-ld/v1/entities")
                .header("Accept", "application/json")
                .header("Content-Type", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful() || response.code() == 409,
                    String.format("UptimeReport creation should succeed or already exist. Got status %d", response.code()));
            dspCreatedEntities.add(UPTIME_REPORT_ENTITY_ID);
            log.debug("UptimeReport entity created/exists at {}", SCORPIO_ADDRESS);
        }
    }

    /**
     * Verifies the UptimeReport entity exists in Scorpio by querying it directly.
     */
    @Then("The UptimeReport entity exists in Scorpio.")
    public void theUptimeReportEntityExistsInScorpio() throws Exception {
        Request request = new Request.Builder()
                .get()
                .url(SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID)
                .header("Accept", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("UptimeReport should be retrievable. Got status %d", response.code()));
            String body = response.body().string();
            JsonNode entityNode = OBJECT_MAPPER.readTree(body);
            assertEquals(UPTIME_REPORT_ENTITY_ID, entityNode.get("id").asText(),
                    "Entity ID should match the expected UptimeReport ID.");
            log.debug("UptimeReport entity verified in Scorpio: {}", UPTIME_REPORT_ENTITY_ID);
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
                .url(TMF_DIRECT_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/category")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The category should have been created.");
            CategoryVO category = OBJECT_MAPPER.readValue(response.body().string(), CategoryVO.class);
            dspCategoryId = category.getId();
            assertNotNull(dspCategoryId, "Category ID should not be null.");
            log.debug("DSP category created with ID: {}", dspCategoryId);
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
                .url(TMF_DIRECT_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/catalog")
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The catalog should have been created.");
            log.debug("DSP catalog created.");
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
                                                "url", "https://tir.127.0.0.1.nip.io")),
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
                DCP_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH));

        // OID4VC endpoint
        characteristics.add(buildCharacteristic("oid4vc",
                "Endpoint, that the service can be negotiated at via OID4VC.",
                "endpointUrl",
                OID4VC_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH));

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
                .url(TMF_DIRECT_ADDRESS
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
            log.debug("DSP product specification created with ID: {}", dspProductSpecId);
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
                .url(TMF_DIRECT_ADDRESS
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
            log.debug("DSP product offering created with ID: {}", dspProductOfferingId);
        }
    }

    /**
     * Verifies the DSP product offering is available at the TMForum API.
     */
    @Then("The DSP product offering is available at the TMForum API.")
    public void theDspProductOfferingIsAvailableAtTheTmForumApi() throws Exception {
        Request request = new Request.Builder()
                .get()
                .url(TMF_DIRECT_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "TMForum offering list should be accessible.");
            List<ProductOfferingVO> offerings = OBJECT_MAPPER.readValue(
                    response.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                    });
            assertFalse(offerings.isEmpty(), "At least one offering should exist.");
            log.debug("Found {} DSP product offerings at TMForum API.", offerings.size());
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
        log.debug("DSP TMForum access policies are active. Created {} policies.", dspCreatedPolicies.size());
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
        log.debug("Consumer obtained representative (user) credential for DSP ordering.");
    }

    /**
     * Obtains an operator credential for the consumer to use when accessing data services.
     * Equivalent to step 2 of "Order through TMForum" in DSP_INTEGRATION.md.
     */
    @When("The consumer obtains an operator credential for DSP ordering.")
    public void theConsumerObtainsAnOperatorCredentialForDspOrdering() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(OPERATOR_USER_NAME);
        dspWallet.getCredentialFromIssuer(accessToken, CONSUMER_KEYCLOAK_ADDRESS, OPERATOR_CREDENTIAL_ID);
        log.debug("Consumer obtained operator credential for DSP ordering.");
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
                .url(TM_FORUM_API_ADDRESS + "/tmf-api/party/v4/organization")
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
            log.debug("Consumer registered at provider marketplace with ID: {}",
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
                .url(TM_FORUM_API_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .header("Authorization", "Bearer " + accessToken)
                .build();
        String offerId;
        try (Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, offerResponse.code(), "Offerings should be listed.");
            List<ProductOfferingVO> offerings = OBJECT_MAPPER.readValue(
                    offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                    });
            assertFalse(offerings.isEmpty(), "At least one offering should exist.");
            offerId = offerings.get(0).getId();
            assertNotNull(offerId, "Offer ID should not be null.");
            log.debug("Found offering with ID: {}", offerId);
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
                .url(TM_FORUM_API_ADDRESS
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
            log.debug("Product order created with ID: {}", dspProductOrderId);
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
                .url(TM_FORUM_API_ADDRESS
                        + "/tmf-api/productOrderingManagement/v4/productOrder/" + dspProductOrderId)
                .header("Authorization", "Bearer " + accessToken)
                .header("accept", "application/json;charset=utf-8")
                .header("Content-Type", "application/json;charset=utf-8")
                .build();
        try (Response response = HTTP_CLIENT.newCall(updateRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "Product order should have been updated to completed.");
            log.debug("Product order {} completed.", dspProductOrderId);
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
                            .url(PROVIDER_API_ADDRESS
                                    + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID)
                            .header("Accept", "*/*")
                            .header("Authorization", "Bearer " + accessToken)
                            .build();
                    try (Response response = HTTP_CLIENT.newCall(reportRequest).execute()) {
                        assertEquals(HttpStatus.SC_OK, response.code(),
                                "Consumer should be able to access UptimeReport with operator credential.");
                        String body = response.body().string();
                        JsonNode entity = OBJECT_MAPPER.readTree(body);
                        assertEquals(UPTIME_REPORT_ENTITY_ID, entity.get("id").asText(),
                                "Returned entity should be the UptimeReport.");
                        log.debug("Consumer successfully accessed UptimeReport via operator credential.");
                    }
                });
    }

    // ==================== DSP DCP Protocol Flow ====================

    /**
     * Requests the provider catalog via the DCP management API.
     * Equivalent to step 1 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     * <p>
     * Uses the DCP management API address and the DCP provider DSP endpoint.
     */
    @When("The consumer requests the provider catalog via the DCP management API.")
    public void theConsumerRequestsTheProviderCatalogViaDcp() throws Exception {
        String counterPartyAddress = DCP_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        dcpCatalogResponse = DSPManagementHelper.requestCatalog(
                DCP_MANAGEMENT_API_ADDRESS,
                PROVIDER_DID,
                counterPartyAddress);
        assertNotNull(dcpCatalogResponse, "DCP catalog response should not be null.");
        log.debug("DCP catalog response received: {}", dcpCatalogResponse.toString().substring(0,
                Math.min(dcpCatalogResponse.toString().length(), 500)));
    }

    /**
     * Verifies the DCP catalog contains at least one dataset with the expected DSP asset ID.
     * The catalog is a DCAT structure; datasets appear under {@code dcat:dataset}.
     */
    @Then("The DCP catalog contains at least one dataset with the DSP asset.")
    public void theDcpCatalogContainsAtLeastOneDataset() {
        assertNotNull(dcpCatalogResponse, "DCP catalog response must have been retrieved first.");

        // Look for datasets in the catalog - the EDC returns them under "dcat:dataset"
        JsonNode datasets = dcpCatalogResponse.path("dcat:dataset");
        if (datasets.isMissingNode()) {
            // Also try "@graph" or direct array
            datasets = dcpCatalogResponse.path("dataset");
        }

        boolean hasDataset;
        if (datasets.isArray()) {
            hasDataset = datasets.size() > 0;
        } else if (!datasets.isMissingNode()) {
            // Single dataset (not wrapped in array)
            hasDataset = true;
        } else {
            hasDataset = false;
        }

        assertTrue(hasDataset, "DCP catalog should contain at least one dataset. Catalog: "
                + dcpCatalogResponse.toString().substring(0,
                Math.min(dcpCatalogResponse.toString().length(), 1000)));
        log.debug("DCP catalog contains dataset(s) with the DSP asset.");
    }

    /**
     * Starts a contract negotiation via the DCP management API.
     * Equivalent to step 2 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     * <p>
     * Extracts the offer ID from the catalog response and uses the contract policy
     * documented in DSP_INTEGRATION.md (dayOfWeek constraint).
     */
    @When("The consumer starts a contract negotiation via the DCP management API.")
    public void theConsumerStartsNegotiationViaDcp() throws Exception {
        // First ensure we have a catalog; request one if needed
        if (dcpCatalogResponse == null) {
            theConsumerRequestsTheProviderCatalogViaDcp();
        }

        // Extract the offer ID from the catalog
        String offerId = extractOfferIdFromCatalog(dcpCatalogResponse);
        assertNotNull(offerId, "Should be able to extract an offer ID from the DCP catalog.");

        // Build the contract policy matching DSP_INTEGRATION.md
        ObjectNode policy = buildDcpContractPolicy(offerId);

        String counterPartyAddress = DCP_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        IdResponse negotiationResponse = DSPManagementHelper.startNegotiation(
                DCP_MANAGEMENT_API_ADDRESS,
                counterPartyAddress,
                PROVIDER_DID,
                offerId,
                DSP_ASSET_ID,
                policy);
        assertNotNull(negotiationResponse, "Negotiation start response should not be null.");
        log.debug("DCP negotiation started: {}", negotiationResponse);
        dcpNegotiationId = negotiationResponse.getId();
    }

    /**
     * Waits for the DCP negotiation to reach the "FINALIZED" state and extracts the agreement ID.
     * Equivalent to steps 3-4 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     */
    @When("The consumer waits for the DCP negotiation to be finalized.")
    public void theConsumerWaitsForDcpNegotiationFinalized() throws Exception {
        dcpAgreementId = DSPManagementHelper.waitForNegotiationFinalized(DCP_MANAGEMENT_API_ADDRESS, dcpNegotiationId);
        assertNotNull(dcpAgreementId, "DCP agreement ID should not be null after negotiation is finalized.");
        log.debug("DCP negotiation finalized with agreement ID: {}", dcpAgreementId);
    }

    /**
     * Verifies that a valid contract agreement ID was obtained from the DCP negotiation.
     */
    @Then("The DCP negotiation yields a valid contract agreement ID.")
    public void theDcpNegotiationYieldsAValidAgreementId() {
        assertNotNull(dcpAgreementId, "DCP agreement ID should have been set during negotiation.");
        assertFalse(dcpAgreementId.isBlank(), "DCP agreement ID should not be blank.");
        log.debug("Verified DCP agreement ID: {}", dcpAgreementId);
    }

    @Given("The consumer identity is properly setup.")
    public void setupConsumerIdentity() throws Exception {
        theConsumerPrivateKeyIsRetrievedFromTheKubernetesSecret();
        theConsumerPrivateKeyIsConvertedToJwkFormat();
        theConsumerJwkIsInsertedIntoTheConsumerVault();
        theConsumerParticipantIsRegisteredInTheConsumerIdentityHub();
        theConsumerDidDocumentIsAvailableAtTheWellKnownEndpoint();
        aMembershipCredentialIsIssuedForTheConsumerFromTheConsumerKeycloak();
        theConsumerMembershipCredentialIsInsertedIntoTheConsumerIdentityHub();
    }

    @Given("The provider identity is properly setup.")
    public void setupProviderIdentity() throws Exception {
        theProviderPrivateKeyIsRetrievedFromTheKubernetesSecret();
        theProviderPrivateKeyIsConvertedToJwkFormat();
        theProviderJwkIsInsertedIntoTheProviderVault();
        theProviderParticipantIsRegisteredInTheProviderIdentityHub();
        theProviderDidDocumentIsAvailableAtTheWellKnownEndpoint();
        aMembershipCredentialIsIssuedForTheProviderFromTheProviderKeycloak();
        theProviderMembershipCredentialIsInsertedIntoTheProviderIdentityHub();
    }

    @Given("The provider catalog is properly setup.")
    public void setupProviderCatalog() throws Exception {
        theProviderCreatesADemoCategoryForDspOfferings();
        theProviderCreatesADemoCatalogForDspOfferings();
        theProviderCreatesADspProductSpecification();
        theProviderCreatesADspProductOfferingWithEdcTerms();
    }

    /**
     * Ensures the consumer has a finalized DCP contract agreement (prerequisite for transfer).
     * If no agreement exists yet, runs the full negotiation flow.
     */
    @Given("The consumer has a finalized DCP contract agreement.")
    public void theConsumerHasAFinalizedDcpContractAgreement() throws Exception {
        if (dcpAgreementId == null || dcpAgreementId.isBlank()) {
            theConsumerRequestsTheProviderCatalogViaDcp();
            theConsumerStartsNegotiationViaDcp();
            theConsumerWaitsForDcpNegotiationFinalized();
        }
        assertNotNull(dcpAgreementId, "A finalized DCP contract agreement ID is required.");
        log.debug("Consumer has finalized DCP contract agreement: {}", dcpAgreementId);
    }

    /**
     * Starts a transfer process via the DCP management API using the agreement from negotiation.
     * Equivalent to step 5 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     */
    @When("The consumer starts a transfer process via the DCP management API.")
    public void theConsumerStartsTransferProcessViaDcp() throws Exception {
        String counterPartyAddress = DCP_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        IdResponse transferResponse = DSPManagementHelper.startTransferProcess(
                DCP_MANAGEMENT_API_ADDRESS,
                DSP_ASSET_ID,
                PROVIDER_DID,
                counterPartyAddress,
                dcpAgreementId,
                DSPManagementHelper.TRANSFER_TYPE_HTTP_DATA_PULL);
        assertNotNull(transferResponse, "Transfer process start response should not be null.");
        dcpTransferId = transferResponse.getId();
    }

    /**
     * Waits for the DCP transfer process to reach the "STARTED" state and extracts the transfer ID.
     * Equivalent to steps 6-7 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     */
    @When("The consumer waits for the DCP transfer process to start.")
    public void theConsumerWaitsForDcpTransferStarted() throws Exception {
        DSPManagementHelper.waitForTransferStarted(DCP_MANAGEMENT_API_ADDRESS, dcpTransferId);
        assertNotNull(dcpTransferId, "DCP transfer ID should not be null after transfer starts.");
        log.debug("DCP transfer process started with ID: {}", dcpTransferId);
    }

    /**
     * Retrieves the EDR data address (endpoint URL and access token) from the DCP management API.
     * Equivalent to step 7 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     */
    @Then("The consumer retrieves the EDR data address from the DCP management API.")
    public void theConsumerRetrievesEdrDataAddressViaDcp() throws Exception {
        assertNotNull(dcpTransferId, "A started DCP transfer ID is required to retrieve the data address.");
        dcpDataAddress = DSPManagementHelper.getDataAddress(DCP_MANAGEMENT_API_ADDRESS, dcpTransferId);
        assertNotNull(dcpDataAddress, "DCP data address should not be null.");
        assertNotNull(dcpDataAddress.getEndpoint(), "DCP data address endpoint should not be null.");
        assertNotNull(dcpDataAddress.getToken(), "DCP data address token should not be null.");
        log.debug("DCP data address retrieved: endpoint={}", dcpDataAddress.getEndpoint());
    }

    /**
     * Accesses the UptimeReport entity via the DCP transfer endpoint using the provisioned token.
     * Equivalent to step 8 of "Order through DSP" > "DCP" in DSP_INTEGRATION.md.
     */
    @Then("The consumer accesses the UptimeReport entity via the DCP transfer endpoint.")
    public void theConsumerAccessesUptimeReportViaDcpTransfer() throws Exception {
        assertNotNull(dcpDataAddress, "DCP data address must have been retrieved first.");
        String entityUrl = dcpDataAddress.getEndpoint()
                + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID;
        Request request = new Request.Builder()
                .get()
                .url(entityUrl)
                .addHeader("Accept", "*/*")
                .addHeader("Authorization", "Bearer " + dcpDataAddress.getToken())
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(org.apache.http.HttpStatus.SC_OK, response.code(),
                    String.format("Should be able to access UptimeReport via DCP transfer endpoint. Got status %d",
                            response.code()));
            String body = response.body().string();
            JsonNode entity = OBJECT_MAPPER.readTree(body);
            assertEquals(UPTIME_REPORT_ENTITY_ID, entity.get("id").asText(),
                    "Returned entity ID should match the UptimeReport.");
            log.debug("Successfully accessed UptimeReport via DCP transfer endpoint at {}", entityUrl);
        }
    }

    // ==================== OID4VC Protocol Flow ====================

    /**
     * Verifies the OID4VC catalog contains at least one dataset with the expected DSP asset ID.
     * The catalog is a DCAT structure; datasets appear under {@code dcat:dataset}.
     */
    @Then("The OID4VC catalog contains at least one dataset with the DSP asset.")
    public void theOID4VCCatalogContainsAtLeastOneDataset() {
        assertNotNull(oid4vcCatalogResponse, "OID4VC catalog response must have been retrieved first.");

        // Look for datasets in the catalog - the EDC returns them under "dcat:dataset"
        JsonNode datasets = oid4vcCatalogResponse.path("dcat:dataset");
        if (datasets.isMissingNode()) {
            // Also try "@graph" or direct array
            datasets = oid4vcCatalogResponse.path("dataset");
        }

        boolean hasDataset;
        if (datasets.isArray()) {
            hasDataset = datasets.size() > 0;
        } else if (!datasets.isMissingNode()) {
            // Single dataset (not wrapped in array)
            hasDataset = true;
        } else {
            hasDataset = false;
        }

        assertTrue(hasDataset, "OID4VC catalog should contain at least one dataset. Catalog: "
                + oid4vcCatalogResponse.toString().substring(0,
                Math.min(oid4vcCatalogResponse.toString().length(), 1000)));
        log.debug("OID4VC catalog contains dataset(s) with the DSP asset.");
    }

    /**
     * Starts a contract negotiation via the OID4VC management API.
     */
    @When("The consumer starts a contract negotiation via the OID4VC management API.")
    public void theConsumerStartsNegotiationViaOid4vc() throws Exception {
        // First ensure we have a catalog; request one if needed
        if (oid4vcCatalogResponse == null) {
            theConsumerRequestsTheProviderCatalogViaOid4vc();
        }

        // Extract the offer ID from the catalog
        String offerId = extractOfferIdFromCatalog(oid4vcCatalogResponse);
        assertNotNull(offerId, "Should be able to extract an offer ID from the OID4VC catalog.");

        // Build the contract policy matching DSP_INTEGRATION.md
        ObjectNode policy = buildDcpContractPolicy(offerId);

        String counterPartyAddress = OID4VC_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        IdResponse negotiationResponse = DSPManagementHelper.startNegotiation(
                OID4VC_MANAGEMENT_API_ADDRESS,
                counterPartyAddress,
                PROVIDER_DID,
                offerId,
                DSP_ASSET_ID,
                policy);
        assertNotNull(negotiationResponse, "Negotiation start response should not be null.");
        log.debug("OID4VC negotiation started: {}", negotiationResponse);
        oid4vcNegotiationId = negotiationResponse.getId();
    }

    /**
     * Waits for the OID4VC negotiation to reach the "FINALIZED" state and extracts the agreement ID.
     */
    @When("The consumer waits for the OID4VC negotiation to be finalized.")
    public void theConsumerWaitsForOid4VcNegotiationFinalized() throws Exception {
        oid4vcAgreementId = DSPManagementHelper.waitForNegotiationFinalized(OID4VC_MANAGEMENT_API_ADDRESS, oid4vcNegotiationId);
        assertNotNull(oid4vcAgreementId, "OID4VC agreement ID should not be null after negotiation is finalized.");
        log.debug("OID4VC negotiation finalized with agreement ID: {}", oid4vcAgreementId);
    }

    /**
     * Verifies that a valid contract agreement ID was obtained from the DCP negotiation.
     */
    @Then("The OID4VC negotiation yields a valid contract agreement ID.")
    public void theOid4VcNegotiationYieldsAValidAgreementId() {
        assertNotNull(oid4vcAgreementId, "OID4VC agreement ID should have been set during negotiation.");
        assertFalse(oid4vcAgreementId.isBlank(), "OID4VC agreement ID should not be blank.");
        log.debug("Verified OID4VC agreement ID: {}", oid4vcAgreementId);
    }

    /**
     * Requests the provider catalog via the OID4VC management API.
     * <p>
     * Uses the OID4VC management API address and the OID4VC provider DSP endpoint.
     */
    @When("The consumer requests the provider catalog via the OID4VC management API.")
    public void theConsumerRequestsTheProviderCatalogViaOid4vc() throws Exception {
        String counterPartyAddress = OID4VC_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        oid4vcCatalogResponse = DSPManagementHelper.requestCatalog(
                OID4VC_MANAGEMENT_API_ADDRESS,
                PROVIDER_DID,
                counterPartyAddress);
        assertNotNull(oid4vcCatalogResponse, "OID4VC catalog response should not be null.");
        log.debug("OID4VC catalog response received: {}", oid4vcCatalogResponse.toString().substring(0,
                Math.min(oid4vcCatalogResponse.toString().length(), 500)));
    }

    /**
     * Starts a transfer process via the OID4VC management API, reusing the agreement from DCP.
     * Equivalent to step 2 of "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     * <p>
     * As documented, the OID4VC transfer includes a {@code dataDestination} with type
     * {@code HttpProxy} and reuses the agreement negotiated via DCP.
     */
    @When("The consumer starts a transfer process via the OID4VC management API.")
    public void theConsumerStartsTransferProcessViaOid4vc() throws Exception {
        assertNotNull(oid4vcAgreementId, "A finalized OID4VC contract agreement is required for OID4VC transfer.");
        String counterPartyAddress = OID4VC_PROVIDER_ADDRESS + DSP_ENDPOINT_PATH;
        IdResponse transferResponse = DSPManagementHelper.startTransferProcessWithDataDestination(
                OID4VC_MANAGEMENT_API_ADDRESS,
                DSP_ASSET_ID,
                PROVIDER_DID,
                counterPartyAddress,
                oid4vcAgreementId,
                DSPManagementHelper.TRANSFER_TYPE_HTTP_DATA_PULL);
        assertNotNull(transferResponse, "OID4VC transfer process start response should not be null.");
        log.debug("OID4VC transfer process started: {}", transferResponse);
        oid4vcTransferId = transferResponse.getId();
    }

    /**
     * Waits for the OID4VC transfer process to reach the "STARTED" state and extracts the transfer ID.
     * Equivalent to steps 3-4 of "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     */
    @When("The consumer waits for the OID4VC transfer process to start.")
    public void theConsumerWaitsForOid4vcTransferStarted() throws Exception {
        DSPManagementHelper.waitForTransferStarted(OID4VC_MANAGEMENT_API_ADDRESS, oid4vcTransferId);
        assertNotNull(oid4vcTransferId, "OID4VC transfer ID should not be null after transfer starts.");
        log.debug("OID4VC transfer process started with ID: {}", oid4vcTransferId);
    }

    /**
     * Retrieves the EDR data address (endpoint URL and access token) from the OID4VC management API.
     * Equivalent to step 4 of "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     */
    @Then("The consumer retrieves the EDR data address from the OID4VC management API.")
    public void theConsumerRetrievesEdrDataAddressViaOid4vc() throws Exception {
        assertNotNull(oid4vcTransferId, "A started OID4VC transfer ID is required to retrieve the data address.");
        oid4vcDataAddress = DSPManagementHelper.getDataAddress(OID4VC_MANAGEMENT_API_ADDRESS, oid4vcTransferId);
        assertNotNull(oid4vcDataAddress, "OID4VC data address should not be null.");
        assertNotNull(oid4vcDataAddress.getEndpoint(), "OID4VC data address endpoint should not be null.");
        log.debug("OID4VC data address retrieved: endpoint={}", oid4vcDataAddress.getEndpoint());
    }

    /**
     * Ensures the consumer has a started OID4VC transfer with an endpoint.
     * If no transfer exists yet, runs the full OID4VC transfer setup flow (reusing the DCP agreement).
     */
    @Given("The consumer has a started OID4VC transfer with an endpoint.")
    public void theConsumerHasAStartedOid4vcTransferWithEndpoint() throws Exception {
        if (oid4vcDataAddress == null) {
            // Ensure DCP agreement exists first
            theConsumerHasAFinalizedDcpContractAgreement();
            theConsumerStartsTransferProcessViaOid4vc();
            theConsumerWaitsForOid4vcTransferStarted();
            theConsumerRetrievesEdrDataAddressViaOid4vc();
        }
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        assertNotNull(oid4vcDataAddress.getEndpoint(), "OID4VC endpoint must be available.");
        log.debug("Consumer has started OID4VC transfer with endpoint: {}", oid4vcDataAddress.getEndpoint());
    }

    /**
     * Ensures the consumer has a finalized OID4VC contract agreement (prerequisite for transfer).
     * If no agreement exists yet, runs the full negotiation flow.
     */
    @Given("The consumer has a finalized OID4VC contract agreement.")
    public void theConsumerHasAFinalizedOid4VcContractAgreement() throws Exception {
        if (oid4vcAgreementId == null || oid4vcAgreementId.isBlank()) {
            theConsumerRequestsTheProviderCatalogViaOid4vc();
            theConsumerStartsNegotiationViaOid4vc();
            theConsumerWaitsForOid4VcNegotiationFinalized();
        }
        assertNotNull(oid4vcAgreementId, "A finalized OID4VC contract agreement ID is required.");
        log.debug("Consumer has finalized OID4VC contract agreement: {}", oid4vcAgreementId);
    }

    /**
     * Sends an unauthenticated request to the OID4VC transfer endpoint.
     * Equivalent to step 5 of "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     * The request should be rejected (401 or 403).
     */
    @When("The consumer requests the UptimeReport without authentication via the OID4VC endpoint.")
    public void theConsumerRequestsUptimeReportWithoutAuthViaOid4vc() throws Exception {
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        // The unauthenticated request result is verified in the Then step.
        // We just log that we're about to make the request.
        log.debug("Sending unauthenticated request to OID4VC endpoint: {}", oid4vcDataAddress.getEndpoint());
    }

    /**
     * Verifies that the unauthenticated request to the OID4VC transfer endpoint is rejected.
     * The service should respond with 401 Unauthorized or 403 Forbidden.
     */
    @Then("The unauthenticated OID4VC request is rejected.")
    public void theUnauthenticatedOid4vcRequestIsRejected() throws Exception {
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        String entityUrl = oid4vcDataAddress.getEndpoint()
                + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID;
        Request request = new Request.Builder()
                .get()
                .url(entityUrl)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            int statusCode = response.code();
            assertTrue(statusCode == HttpStatus.SC_UNAUTHORIZED || statusCode == HttpStatus.SC_FORBIDDEN,
                    String.format("Unauthenticated request should be rejected (401 or 403). Got status %d", statusCode));
            log.debug("Unauthenticated OID4VC request correctly rejected with status {} at {}", statusCode, entityUrl);
        }
    }

    /**
     * Requests the OpenID configuration from the OID4VC transfer endpoint.
     * Equivalent to step 6 of "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     */
    @When("The consumer requests the openid-configuration from the OID4VC transfer endpoint.")
    public void theConsumerRequestsOpenidConfigFromOid4vcEndpoint() throws Exception {
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        String oidcUrl = oid4vcDataAddress.getEndpoint() + "/.well-known/openid-configuration";
        Request request = new Request.Builder()
                .get()
                .url(oidcUrl)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("OpenID configuration should be accessible at %s. Got status %d",
                            oidcUrl, response.code()));
            String body = response.body().string();
            oid4vcEndpointOidcConfig = OBJECT_MAPPER.readValue(body, OpenIdConfiguration.class);
            assertNotNull(oid4vcEndpointOidcConfig, "OpenID configuration should be parsed successfully.");
            log.debug("OpenID configuration retrieved from OID4VC endpoint at {}", oidcUrl);
        }
    }

    /**
     * Verifies the OpenID configuration response contains a token endpoint.
     */
    @Then("The openid-configuration response contains a token endpoint.")
    public void theOpenidConfigContainsATokenEndpoint() {
        assertNotNull(oid4vcEndpointOidcConfig, "OpenID configuration must have been retrieved first.");
        assertNotNull(oid4vcEndpointOidcConfig.getTokenEndpoint(),
                "OpenID configuration should contain a token_endpoint.");
        assertFalse(oid4vcEndpointOidcConfig.getTokenEndpoint().isBlank(),
                "Token endpoint should not be blank.");
        log.debug("OpenID configuration token endpoint: {}", oid4vcEndpointOidcConfig.getTokenEndpoint());
    }

    /**
     * Obtains a membership credential from the consumer's Keycloak for OID4VC access.
     * Equivalent to the first part of step 7 in "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md:
     * {@code ./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io membership-credential employee}
     * <p>
     * Stores the credential in the DSP wallet for subsequent OID4VP token exchange.
     */
    @When("The consumer obtains a membership credential for OID4VC access.")
    public void theConsumerObtainsMembershipCredentialForOid4vcAccess() throws Exception {
        // Login as the employee user and store the credential in the wallet for OID4VP exchange
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(
                MEMBERSHIP_CREDENTIAL_USERNAME + "@consumer.org");
        dspWallet.getCredentialFromIssuer(accessToken, CONSUMER_KEYCLOAK_ADDRESS, MEMBERSHIP_CREDENTIAL_ID);
        assertNotNull(dspWallet.getStoredCredential(MEMBERSHIP_CREDENTIAL_ID),
                "Membership credential should be stored in the wallet.");
        log.debug("Consumer obtained membership credential for OID4VC access.");
    }

    /**
     * Exchanges the membership credential for an access token via OID4VP at the OID4VC transfer endpoint.
     * Equivalent to the second part of step 7 in "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md:
     * {@code ./doc/scripts/get_access_token_oid4vp.sh ${ENDPOINT} $MEMBERSHIP_CREDENTIAL openid}
     */
    @When("The consumer exchanges the membership credential for an access token via OID4VP at the OID4VC endpoint.")
    public void theConsumerExchangesMembershipCredentialForTokenViaOid4vp() throws Exception {
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        oid4vcAccessToken = ScriptHelper.getAccessTokenViaOid4vp(
                oid4vcDataAddress.getEndpoint(),
                MEMBERSHIP_CREDENTIAL_ID,
                OPENID_SCOPE,
                dspWallet);
        assertNotNull(oid4vcAccessToken, "OID4VP access token should not be null.");
        assertFalse(oid4vcAccessToken.isBlank(), "OID4VP access token should not be blank.");
        log.debug("Consumer obtained OID4VP access token for OID4VC endpoint.");
    }

    /**
     * Accesses the UptimeReport entity via the OID4VC transfer endpoint using the OID4VP access token.
     * Equivalent to the last part of step 7 in "Order through DSP" > "OID4VC" in DSP_INTEGRATION.md.
     */
    @Then("The consumer accesses the UptimeReport entity via the OID4VC transfer endpoint with OID4VP token.")
    public void theConsumerAccessesUptimeReportViaOid4vcWithOid4vpToken() throws Exception {
        assertNotNull(oid4vcDataAddress, "OID4VC data address must be available.");
        assertNotNull(oid4vcAccessToken, "OID4VP access token must be available.");
        String entityUrl = oid4vcDataAddress.getEndpoint()
                + "/ngsi-ld/v1/entities/" + UPTIME_REPORT_ENTITY_ID;
        Request request = new Request.Builder()
                .get()
                .url(entityUrl)
                .header("Accept", "application/json")
                .header("Authorization", "Bearer " + oid4vcAccessToken)
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    String.format("Should be able to access UptimeReport via OID4VC endpoint with OID4VP token. Got status %d",
                            response.code()));
            String body = response.body().string();
            JsonNode entity = OBJECT_MAPPER.readTree(body);
            assertEquals(UPTIME_REPORT_ENTITY_ID, entity.get("id").asText(),
                    "Returned entity ID should match the UptimeReport.");
            log.debug("Successfully accessed UptimeReport via OID4VC transfer endpoint at {}", entityUrl);
        }
    }

    @Given("An OID4VC Transfer Process is started.")
    public void theOid4VcTransferProcessIsStarted() throws Exception {
        setupConsumerIdentity();
        setupProviderIdentity();
        setupProviderCatalog();
        theProviderCreatesAnUptimeReportEntity();
        theConsumerHasAFinalizedOid4VcContractAgreement();
        theConsumerStartsTransferProcessViaOid4vc();
        theConsumerWaitsForOid4vcTransferStarted();
    }

    // ==================== DCP Helper Methods ====================

    /**
     * Extracts the first offer ID from a DCAT catalog response.
     * The offer ID is typically found in {@code dcat:dataset[].odrl:hasPolicy.@id}
     * or in the dataset's distribution offer reference.
     *
     * @param catalog the catalog JSON response
     * @return the first offer ID found, or null if none found
     */
    private String extractOfferIdFromCatalog(JsonNode catalog) {
        // Try dcat:dataset array
        JsonNode datasets = catalog.path("dcat:dataset");
        if (datasets.isMissingNode()) {
            datasets = catalog.path("dataset");
        }

        JsonNode firstDataset;
        if (datasets.isArray() && datasets.size() > 0) {
            firstDataset = datasets.get(0);
        } else if (!datasets.isMissingNode() && !datasets.isArray()) {
            firstDataset = datasets;
        } else {
            log.debug("No datasets found in catalog to extract offer ID.");
            return null;
        }

        // Look for the offer in odrl:hasPolicy
        JsonNode hasPolicy = firstDataset.path("odrl:hasPolicy");
        if (hasPolicy.isMissingNode()) {
            hasPolicy = firstDataset.path("hasPolicy");
        }

        JsonNode policyNode;
        if (hasPolicy.isArray() && hasPolicy.size() > 0) {
            policyNode = hasPolicy.get(0);
        } else if (!hasPolicy.isMissingNode() && !hasPolicy.isArray()) {
            policyNode = hasPolicy;
        } else {
            log.debug("No policy found in dataset to extract offer ID.");
            return null;
        }

        String offerId = policyNode.path("@id").asText(null);
        if (offerId != null) {
            log.debug("Extracted offer ID from DCP catalog: {}", offerId);
        }
        return offerId;
    }

    /**
     * Builds the contract policy for DCP negotiation as documented in DSP_INTEGRATION.md.
     * The policy includes a dayOfWeek constraint matching the provider's contract policy.
     *
     * @param offerId the offer ID extracted from the catalog
     * @return the policy as an ObjectNode
     */
    private ObjectNode buildDcpContractPolicy(String offerId) {
        ObjectNode policy = OBJECT_MAPPER.createObjectNode();
        policy.put("@context", "http://www.w3.org/ns/odrl.jsonld");
        policy.put("@type", "Offer");
        policy.put("@id", offerId);
        policy.put("assigner", PROVIDER_DID);
        policy.put("target", DSP_ASSET_ID);

        // Permission with dayOfWeek constraint as documented in DSP_INTEGRATION.md
        ObjectNode constraint = OBJECT_MAPPER.createObjectNode();
        constraint.put("leftOperand", "odrl:dayOfWeek");
        constraint.put("operator", "lt");
        constraint.put("rightOperand", 6);

        ObjectNode permission = OBJECT_MAPPER.createObjectNode();
        permission.put("action", "use");
        permission.set("constraint", constraint);

        ArrayNode permissions = OBJECT_MAPPER.createArrayNode();
        permissions.add(permission);
        policy.set("permission", permissions);

        return policy;
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
                .url(PROVIDER_PAP_ADDRESS + "/policy")
                .header("Content-Type", "application/json")
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    String.format("Policy '%s' should have been created.", policyName));
            String location = response.header("Location");
            if (location != null) {
                dspCreatedPolicies.add(location);
            }
            log.debug("Policy '{}' created at provider PAP.", policyName);
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
     * @param credentialId  the credential ID in the wallet (e.g., {@code user-credential})
     * @param scope         the OAuth scope to request
     * @param targetAddress the base URL of the service to authenticate against
     * @return the access token string
     * @throws Exception if the token exchange fails
     */
    private String getDspAccessToken(String credentialId, String scope, String targetAddress) throws Exception {
        OpenIdConfiguration oidcConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                targetAddress);
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
     * @param didDocUrl   the full URL to the DID document (e.g., {@code https://fancy-marketplace.biz/.well-known/did.json})
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
            log.debug("DID document verified at {} with id={}", didDocUrl, documentId);
        }
    }

    /**
     * Verifies that a credential has been stored in the IdentityHub for the given participant.
     * Queries the credentials endpoint of the IdentityHub management API.
     *
     * @param identityHubManagementAddress the base URL of the IdentityHub management API
     * @param participantDid               the DID of the participant whose credentials to check
     */
    private void verifyCredentialInIdentityHub(String identityHubManagementAddress, String participantDid) throws Exception {
        String base64ParticipantId = IdentityHubHelper.base64UrlEncode(participantDid);
        String credentialsUrl = identityHubManagementAddress
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
            log.debug("Credentials verified for participant {} at {}", participantDid, identityHubManagementAddress);
        }
    }

}
