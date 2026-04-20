package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.*;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.Policy;
import org.fiware.dataspace.tmf.model.*;
import org.keycloak.common.crypto.CryptoIntegration;
import org.opentest4j.AssertionFailedError;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.security.Security;
import java.time.Duration;
import java.util.*;

import static org.fiware.dataspace.it.components.CentralMarketplaceEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.*;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.*;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Cucumber step definitions for the Central Marketplace integration scenarios
 * documented in CENTRAL_MARKETPLACE.md.
 *
 * <p>These steps cover the complete flow of:
 * <ol>
 *     <li>Preparing the marketplace with TMForum access policies</li>
 *     <li>Provider registration at the central marketplace</li>
 *     <li>Product offering creation on the central marketplace</li>
 *     <li>Consumer buying access through the central marketplace</li>
 *     <li>Contract management notification activating provider service access</li>
 * </ol>
 *
 * @see <a href="../../../../../../doc/CENTRAL_MARKETPLACE.md">Central Marketplace Integration Guide</a>
 */
@Slf4j
public class CentralMarketplaceStepDefinitions extends StepDefintions {

    /**
     * Timeout in seconds for policy propagation at the central marketplace PAP.
     */
    private static final int POLICY_PROPAGATION_TIMEOUT_SECONDS = 20;

    /**
     * Timeout in seconds for contract management notification propagation after order completion.
     */
    private static final int CONTRACT_PROPAGATION_TIMEOUT_SECONDS = 60;

    /**
     * The grant type for VP token exchange.
     */
    private static final String GRANT_TYPE_VP_TOKEN = "vp_token";

    /**
     * The default scope used for user credential token exchange.
     */
    private static final String DEFAULT_SCOPE = "default";

    /**
     * The operator scope used for operator credential token exchange.
     */
    private static final String OPERATOR_SCOPE = "operator";

    /**
     * User credential configuration ID.
     */
    private static final String USER_CREDENTIAL = "user-credential";

    /**
     * Operator credential configuration ID.
     */
    private static final String OPERATOR_CREDENTIAL = "operator-credential";

    /**
     * Schema location for TMForum credential configuration characteristic.
     */
    private static final String CREDENTIALS_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json";

    /**
     * Schema location for TMForum ODRL policy configuration characteristic.
     */
    private static final String POLICY_CONFIG_SCHEMA =
            "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json";

    /**
     * Wallet for the provider (M&P Operations) for marketplace interactions.
     */
    private Wallet providerWallet;

    /**
     * Wallet for the consumer (Fancy Marketplace) for marketplace interactions.
     */
    private Wallet consumerWallet;

    /**
     * Stores the provider organization ID after registration at the central marketplace.
     */
    private String providerOrganizationId;

    /**
     * Stores the consumer organization ID after registration at the central marketplace.
     */
    private String consumerOrganizationId;

    /**
     * Stores the product specification ID created on the central marketplace.
     */
    private String centralProductSpecId;

    /**
     * Stores the product offering ID created on the central marketplace.
     */
    private String centralProductOfferingId;

    /**
     * Stores the listed offering ID when consumer browses the marketplace.
     */
    private String listedOfferingId;

    /**
     * Tracks policies created at consumer PAP for cleanup.
     */
    private final List<String> createdConsumerPolicies = new ArrayList<>();

    /**
     * Tracks policies created at provider PAP for cleanup.
     */
    private final List<String> createdProviderPolicies = new ArrayList<>();

    /**
     * Sets up the test environment before each central marketplace scenario.
     *
     * <p>Runs {@link #clean()} first to remove any leftover resources from previous scenarios,
     * ensuring test isolation even when multiple scenarios create the same types of objects
     * (e.g., policies, organizations, product offerings). This is critical because scenarios
     * are self-contained and repeat setup steps like policy registration and provider enrollment.
     */
    @Before("@central")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        providerWallet = new Wallet();
        consumerWallet = new Wallet();
        OBJECT_MAPPER.setSerializationInclusion(JsonInclude.Include.NON_EMPTY);
        log.debug("Central marketplace scenario setup: cleaning leftover resources from previous scenarios.");
        clean();
        prepareTil();
        // allow the verifier to fetch the new config
        Thread.sleep(3001);
    }

    /**
     * Cleans up all central marketplace resources to ensure test isolation between scenarios.
     *
     * <p>Since scenarios repeat steps that create the same types of objects (policies,
     * organizations, product specs/offerings, orders), this method performs exhaustive
     * cleanup by querying each service API and deleting all found resources rather than
     * relying on tracked resource IDs. This guarantees a clean state even if a previous
     * scenario failed mid-execution.</p>
     *
     * <p>Resources cleaned:
     * <ul>
     *     <li>Central marketplace TMForum entities (offerings, specs, orders, organizations)</li>
     *     <li>Consumer PAP policies (TMForum access policies)</li>
     *     <li>Provider PAP policies (contract management policy)</li>
     *     <li>Provider TIL (trust issuer list) entries</li>
     *     <li>Provider TMForum entities (direct API)</li>
     * </ul>
     */
    private void clean() throws Exception {
        cleanUpCentralMarketplaceTMForum();
        cleanUpConsumerPolicies();
        cleanUpProviderPolicies();
        cleanUpTIL();
        cleanUpProviderTMForum();
    }

    // --- Cleanup helpers ---

    /**
     * Removes all TMForum resources (offerings, specs, orders, organizations) from the central marketplace.
     */
    private void cleanUpCentralMarketplaceTMForum() {
        try {
            cleanUpTMForumResourcesAt(CONSUMER_TMF_DIRECT_ADDRESS);
        } catch (Exception e) {
            log.warn("Failed to clean up central marketplace TMForum resources: {}", e.getMessage());
        }
    }

    /**
     * Removes all TMForum resources from the provider's direct TMForum API.
     */
    private void cleanUpProviderTMForum() {
        try {
            cleanUpTMForumResourcesAt(TM_FORUM_API_ADDRESS);
        } catch (Exception e) {
            log.warn("Failed to clean up provider TMForum resources: {}", e.getMessage());
        }
    }

    /**
     * Cleans up TMForum resources (offerings, specs, orders, organizations) at the given base address.
     *
     * @param tmfBaseAddress the base address of the TMForum API (without /tmf-api prefix)
     */
    private void cleanUpTMForumResourcesAt(String tmfBaseAddress) throws Exception {
        // Clean offerings
        cleanUpTMForumResourceList(tmfBaseAddress,
                "/tmf-api/productCatalogManagement/v4/productOffering",
                "Central offerings");

        // Clean specs
        cleanUpTMForumResourceList(tmfBaseAddress,
                "/tmf-api/productCatalogManagement/v4/productSpecification",
                "Central specifications");

        // Clean orders
        cleanUpTMForumResourceList(tmfBaseAddress,
                "/tmf-api/productOrderingManagement/v4/productOrder",
                "Central orders");

        // Clean organizations
        cleanUpTMForumResourceList(tmfBaseAddress,
                "/tmf-api/party/v4/organization",
                "Central organizations");
    }

    /**
     * Removes all policies from the consumer PAP.
     */
    private void cleanUpConsumerPolicies() {
        cleanUpPoliciesAt(CONSUMER_PAP_ADDRESS);
    }

    /**
     * Removes all policies from the provider PAP.
     */
    private void cleanUpProviderPolicies() {
        cleanUpPoliciesAt(PROVIDER_PAP_ADDRESS);
    }

    /**
     * Removes all policies from the PAP at the given address.
     *
     * @param papAddress the base address of the PAP
     */
    private void cleanUpPoliciesAt(String papAddress) {
        try {
            Request getPolicies = new Request.Builder()
                    .url(papAddress + "/policy")
                    .get().build();
            try (Response policyResponse = HTTP_CLIENT.newCall(getPolicies).execute()) {
                if (!policyResponse.isSuccessful() || policyResponse.body() == null) return;
                List<Policy> policies = OBJECT_MAPPER.readValue(
                        policyResponse.body().string(), new TypeReference<List<Policy>>() {
                        });
                for (Policy policy : policies) {
                    Request deleteRequest = new Request.Builder()
                            .url(papAddress + "/policy/" + policy.getId())
                            .delete().build();
                    try (Response ignored = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up policies at {}: {}", papAddress, e.getMessage());
        }
    }

    // --- Step definitions: Marketplace Preparation ---

    /**
     * Verifies that the central marketplace consumer PAP endpoint is reachable.
     */
    @Given("The central marketplace PAP endpoint is available.")
    public void checkCentralMarketplacePapAvailable() throws Exception {
        Request papRequest = new Request.Builder()
                .get()
                .url(CONSUMER_PAP_ADDRESS + "/policy")
                .build();
        Response papResponse = HTTP_CLIENT.newCall(papRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, papResponse.code(),
                    "The central marketplace PAP endpoint should be available.");
        } finally {
            papResponse.body().close();
        }
    }

    /**
     * Registers the self-registration policy for legal persons at the central marketplace PAP.
     * This restricts organization creation to users with the REPRESENTATIVE role.
     */
    @When("The central marketplace registers self-registration policies for legal persons.")
    public void registerSelfRegistrationPolicies() throws Exception {
        createPolicyAtConsumerPap("allowSelfRegistrationLegalPerson");
    }

    /**
     * Registers product ordering policies at the central marketplace PAP.
     */
    @When("The central marketplace registers product ordering policies.")
    public void registerProductOrderingPolicies() throws Exception {
        createPolicyAtConsumerPap("allowProductOrder");
    }

    /**
     * Registers product offering creation policies at the central marketplace PAP.
     * Only users with the REPRESENTATIVE role can create offerings.
     */
    @When("The central marketplace registers product offering creation policies.")
    public void registerProductOfferingCreationPolicies() throws Exception {
        createPolicyAtConsumerPap("allowProductOfferingCreation");
    }

    /**
     * Registers product offering read policies at the central marketplace PAP.
     */
    @When("The central marketplace registers product offering read policies.")
    public void registerProductOfferingReadPolicies() throws Exception {
        createPolicyAtConsumerPap("allowProductOffering");
    }

    /**
     * Registers product specification policies at the central marketplace PAP.
     * Only users with the REPRESENTATIVE role can manage product specifications.
     */
    @When("The central marketplace registers product specification policies.")
    public void registerProductSpecPolicies() throws Exception {
        createPolicyAtConsumerPap("allowProductSpec");
    }

    /**
     * Verifies that the central marketplace PAP has at least 5 policies registered
     * (the minimum set required for marketplace operation).
     */
    @Then("The central marketplace has all required TMForum access policies.")
    public void centralMarketplaceHasRequiredPolicies() throws Exception {
        // Wait for policies to propagate
        Awaitility.await()
                .atMost(Duration.ofSeconds(POLICY_PROPAGATION_TIMEOUT_SECONDS))
                .untilAsserted(() -> {
                    Request getPolicies = new Request.Builder()
                            .url(CONSUMER_PAP_ADDRESS + "/policy")
                            .get().build();
                    Response policyResponse = HTTP_CLIENT.newCall(getPolicies).execute();
                    try {
                        assertEquals(HttpStatus.SC_OK, policyResponse.code(),
                                "The PAP should return policies.");
                        List<Policy> policies = OBJECT_MAPPER.readValue(
                                policyResponse.body().string(), new TypeReference<List<Policy>>() {
                                });
                        assertTrue(policies.size() >= 5,
                                "At least 5 TMForum access policies should be registered at the marketplace PAP.");
                    } finally {
                        policyResponse.body().close();
                    }
                });
    }

    // --- Step definitions: Provider Preparation ---

    /**
     * Creates the contract management access policy at the provider PAP.
     * This policy restricts access to the contract management endpoint to requests
     * authenticated with a MarketplaceCredential.
     */
    @When("The provider creates a contract management access policy.")
    public void createContractManagementPolicy() throws Exception {
        createPolicyAtProviderPap("allowContractManagement");
    }

    /**
     * Obtains a LegalPersonCredential (UserCredential) with REPRESENTATIVE role from the provider Keycloak.
     * This credential is used to authenticate the provider at the central marketplace.
     */
    @When("The provider obtains a LegalPersonCredential with REPRESENTATIVE role.")
    public void providerObtainsLegalPersonCredential() throws Exception {
        String accessToken = loginToProviderKeycloak(PROVIDER_EMPLOYEE_USER);
        providerWallet.getCredentialFromIssuer(
                accessToken, PROVIDER_KEYCLOAK_ADDRESS, PROVIDER_USER_CREDENTIAL);
    }

    /**
     * Registers the provider organization at the central marketplace with contract management configuration.
     * The registration includes the provider DID and contract management address, clientId, and scope.
     */
    @When("The provider registers at the central marketplace with contract management configuration.")
    public void providerRegistersAtCentralMarketplace() throws Exception {
        // Wait for policies to propagate before attempting registration
        Thread.sleep(10000);

        String accessToken = getProviderAccessTokenAtMarketplace();

        // Build organization with contract management configuration
        Map<String, Object> contractManagementConfig = Map.of(
                "address", CONTRACT_MANAGEMENT_ADDRESS,
                "clientId", "contract-management",
                "scope", List.of("external-marketplace"));

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(PROVIDER_DID);
        CharacteristicVO contractMgmtCharacteristic = new CharacteristicVO()
                .name("contractManagement")
                .value(contractManagementConfig);

        OrganizationCreateVO organizationCreateVO = new OrganizationCreateVO()
                .name("M&P Operations Org.")
                .partyCharacteristic(List.of(didCharacteristic, contractMgmtCharacteristic));

        RequestBody orgBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(organizationCreateVO),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request orgRequest = new Request.Builder()
                .post(orgBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/party/v4/organization")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response orgResponse = HTTP_CLIENT.newCall(orgRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, orgResponse.code(),
                    "The provider organization should have been created at the central marketplace.");
            OrganizationVO org = OBJECT_MAPPER.readValue(orgResponse.body().string(), OrganizationVO.class);
            providerOrganizationId = org.getId();
        } finally {
            orgResponse.body().close();
        }
    }

    /**
     * Verifies that the provider organization is registered at the central marketplace.
     */
    @Then("The provider organization is registered at the central marketplace.")
    public void providerOrganizationIsRegistered() {
        assertNotNull(providerOrganizationId,
                "The provider organization should have been registered at the central marketplace.");
    }

    // --- Step definitions: Product Offering Creation ---

    /**
     * Creates a product specification on the central marketplace with credentials and policy configuration.
     * The spec includes a credentialsConfig for OperatorCredential and a policyConfig
     * for K8SCluster creation with the full ODRL policy (as documented in CENTRAL_MARKETPLACE.md).
     */
    @When("The provider creates a product specification on the central marketplace.")
    public void providerCreatesProductSpecOnMarketplace() throws Exception {
        String accessToken = getProviderAccessTokenAtMarketplace();

        // Build credentials config
        Map<String, Object> credentialsValue = Map.of(
                "credentialsType", "OperatorCredential",
                "claims", List.of(
                        Map.of("name", "roles",
                                "path", "$.roles[?(@.target==\"" + PROVIDER_DID + "\")].names[*]",
                                "allowedValues", List.of("OPERATOR"))));

        // Build the full ODRL policy (as in CENTRAL_MARKETPLACE.md)
        Map<String, Object> odrlPolicy = buildFullK8sOdrlPolicy();

        ProductSpecificationCreateVO pscVo = new ProductSpecificationCreateVO()
                .brand("M&P Operations")
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S")
                .relatedParty(List.of(new RelatedPartyVO()
                        .id(providerOrganizationId)
                        .role("provider")))
                .productSpecCharacteristic(List.of(
                        new ProductSpecificationCharacteristicVO()
                                .id("credentialsConfig")
                                .name("Credentials Config")
                                .atSchemaLocation(URI.create(CREDENTIALS_CONFIG_SCHEMA))
                                .valueType("credentialsConfiguration")
                                .productSpecCharacteristicValue(List.of(
                                        new CharacteristicValueSpecificationVO()
                                                .isDefault(true)
                                                .value(credentialsValue))),
                        new ProductSpecificationCharacteristicVO()
                                .id("policyConfig")
                                .name("Policy for creation of K8S clusters.")
                                .atSchemaLocation(URI.create(POLICY_CONFIG_SCHEMA))
                                .valueType("authorizationPolicy")
                                .productSpecCharacteristicValue(List.of(
                                        new CharacteristicValueSpecificationVO()
                                                .isDefault(true)
                                                .value(odrlPolicy)))));

        RequestBody specBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(pscVo),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request specRequest = new Request.Builder()
                .post(specBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response specResponse = HTTP_CLIENT.newCall(specRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, specResponse.code(),
                    "The product specification should have been created on the central marketplace.");
            ProductSpecificationVO createdSpec = OBJECT_MAPPER.readValue(
                    specResponse.body().string(), ProductSpecificationVO.class);
            centralProductSpecId = createdSpec.getId();
        } finally {
            specResponse.body().close();
        }
    }

    /**
     * Creates a product offering on the central marketplace referencing the product specification.
     */
    @When("The provider creates a product offering on the central marketplace.")
    public void providerCreatesProductOfferingOnMarketplace() throws Exception {
        assertNotNull(centralProductSpecId,
                "The product specification must be created before the offering.");
        String accessToken = getProviderAccessTokenAtMarketplace();

        ProductOfferingCreateVO productOfferingCreate = new ProductOfferingCreateVO()
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S Offering")
                .productSpecification(new ProductSpecificationRefVO().id(centralProductSpecId));

        RequestBody offeringBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(productOfferingCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request offeringRequest = new Request.Builder()
                .post(offeringBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offeringResponse = HTTP_CLIENT.newCall(offeringRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, offeringResponse.code(),
                    "The product offering should have been created on the central marketplace.");
            ProductOfferingVO createdOffering = OBJECT_MAPPER.readValue(
                    offeringResponse.body().string(), ProductOfferingVO.class);
            centralProductOfferingId = createdOffering.getId();
        } finally {
            offeringResponse.body().close();
        }
    }

    /**
     * Verifies that exactly one product offering is available at the central marketplace.
     */
    @Then("One product offering is available at the central marketplace.")
    public void oneOfferingAvailableAtMarketplace() throws Exception {
        String accessToken = getProviderAccessTokenAtMarketplace();

        Request offerRequest = new Request.Builder()
                .get()
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offerings should be returned.");
            List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(
                    offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                    });
            assertEquals(1, offers.size(),
                    "There should be exactly one product offering at the central marketplace.");
        } finally {
            offerResponse.body().close();
        }
    }

    // --- Step definitions: Consumer Buying Flow ---

    /**
     * Obtains a UserCredential in SD-JWT format for the consumer employee.
     * Central marketplace flows require the SD-JWT format for UserCredentials.
     */
    @When("The consumer obtains a UserCredential in SD-JWT format.")
    public void consumerObtainsUserSdJwtCredential() throws Exception {
        String accessToken = loginToConsumerKeycloak(CONSUMER_SD_EMPLOYEE_USER);
        consumerWallet.getCredentialFromIssuer(
                accessToken, CONSUMER_KEYCLOAK_ADDRESS, USER_SD_CREDENTIAL, SD_JWT_FORMAT);
    }

    /**
     * Obtains an OperatorCredential for the consumer operator user.
     */
    @When("The consumer obtains an OperatorCredential.")
    public void consumerObtainsOperatorCredential() throws Exception {
        String accessToken = loginToConsumerKeycloak(OPERATOR_USER_NAME);
        consumerWallet.getCredentialFromIssuer(
                accessToken, CONSUMER_KEYCLOAK_ADDRESS, OPERATOR_CREDENTIAL);
    }

    /**
     * Verifies that the consumer cannot yet get an access token for the OperatorCredential
     * at the provider's data service (before the order is completed and contract management propagates).
     */
    @When("The consumer cannot yet get an access token for the OperatorCredential at the provider.")
    public void consumerCannotYetGetOperatorToken() throws Exception {
        assertThrows(AssertionFailedError.class, () -> {
            OpenIdConfiguration openIdConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                    PROVIDER_API_ADDRESS);
            consumerWallet.exchangeCredentialForToken(openIdConfig, OPERATOR_CREDENTIAL, OPERATOR_SCOPE);
        }, "The consumer should not yet be able to get an access token for the OperatorCredential.");
    }

    /**
     * Registers the consumer organization at the central marketplace.
     * Unlike the provider, the consumer does not need contract management configuration.
     */
    @When("The consumer registers at the central marketplace.")
    public void consumerRegistersAtMarketplace() throws Exception {
        String accessToken = getConsumerAccessTokenAtMarketplace();

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(CONSUMER_DID);

        OrganizationCreateVO organizationCreateVO = new OrganizationCreateVO()
                .name("Fancy Marketplace Inc.")
                .partyCharacteristic(List.of(didCharacteristic));

        RequestBody orgBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(organizationCreateVO),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request orgRequest = new Request.Builder()
                .post(orgBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/party/v4/organization")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response orgResponse = HTTP_CLIENT.newCall(orgRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, orgResponse.code(),
                    "The consumer organization should have been created at the central marketplace.");
            OrganizationVO org = OBJECT_MAPPER.readValue(orgResponse.body().string(), OrganizationVO.class);
            consumerOrganizationId = org.getId();
        } finally {
            orgResponse.body().close();
        }
    }

    /**
     * Lists offerings at the central marketplace and stores the first offering ID.
     */
    @When("The consumer lists offerings at the central marketplace.")
    public void consumerListsOfferings() throws Exception {
        String accessToken = getConsumerAccessTokenAtMarketplace();

        Request offerRequest = new Request.Builder()
                .get()
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offerings should be returned.");
            List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(
                    offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                    });
            assertFalse(offers.isEmpty(), "At least one offering should be available at the central marketplace.");
            listedOfferingId = offers.get(0).getId();
            log.debug("Listed offering ID: {}", listedOfferingId);
        } finally {
            offerResponse.body().close();
        }
    }

    /**
     * Creates and completes an order at the central marketplace for the listed offering.
     * The order is created referencing the consumer organization and then completed,
     * triggering the contract management notification flow.
     */
    @When("The consumer creates and completes an order at the central marketplace.")
    public void consumerCreatesAndCompletesOrder() throws Exception {
        assertNotNull(listedOfferingId, "An offering must be listed before creating an order.");
        assertNotNull(consumerOrganizationId, "The consumer must be registered before ordering.");

        String accessToken = getConsumerAccessTokenAtMarketplace();

        // Create order
        ProductOfferingRefVO offeringRef = new ProductOfferingRefVO().id(listedOfferingId);
        ProductOrderItemVO orderItem = new ProductOrderItemVO()
                .id("random-order-id")
                .action(OrderItemActionTypeVO.ADD)
                .productOffering(offeringRef);
        RelatedPartyVO relatedParty = new RelatedPartyVO().id(consumerOrganizationId);
        ProductOrderCreateVO orderCreate = new ProductOrderCreateVO()
                .productOrderItem(List.of(orderItem))
                .relatedParty(List.of(relatedParty));

        RequestBody orderBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(orderCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request orderRequest = new Request.Builder()
                .post(orderBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response orderResponse = HTTP_CLIENT.newCall(orderRequest).execute();
        String orderId;
        try {
            assertEquals(HttpStatus.SC_CREATED, orderResponse.code(),
                    "The product order should have been created.");
            ProductOrderVO orderVO = OBJECT_MAPPER.readValue(
                    orderResponse.body().string(), ProductOrderVO.class);
            orderId = orderVO.getId();
        } finally {
            orderResponse.body().close();
        }

        // Complete order - need fresh token
        accessToken = getConsumerAccessTokenAtMarketplace();
        ProductOrderUpdateVO orderUpdate = new ProductOrderUpdateVO()
                .state(ProductOrderStateTypeVO.COMPLETED);
        RequestBody updateBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(orderUpdate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request updateRequest = new Request.Builder()
                .patch(updateBody)
                .url(MARKETPLACE_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder/" + orderId)
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json;charset=utf-8")
                .addHeader("Content-Type", "application/json;charset=utf-8")
                .build();
        Response updateResponse = HTTP_CLIENT.newCall(updateRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, updateResponse.code(),
                    "The product order should have been completed.");
        } finally {
            updateResponse.body().close();
        }
        log.debug("Order {} completed at the central marketplace.", orderId);
    }

    /**
     * Verifies that after order completion and contract management notification propagation,
     * the consumer can now get an access token for the OperatorCredential at the provider's data service.
     * This confirms the end-to-end flow: marketplace order -> contract management notification ->
     * TIL entry + PAP policy creation at provider -> consumer can authenticate.
     */
    @Then("The consumer can get an access token for the OperatorCredential at the provider.")
    public void consumerCanGetOperatorToken() throws Exception {
        Awaitility.await()
                .atMost(Duration.ofSeconds(CONTRACT_PROPAGATION_TIMEOUT_SECONDS))
                .untilAsserted(() -> {
                    try {
                        OpenIdConfiguration openIdConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                                PROVIDER_API_ADDRESS);
                        String accessToken = consumerWallet.exchangeCredentialForToken(
                                openIdConfig, OPERATOR_CREDENTIAL, OPERATOR_SCOPE);
                        assertNotNull(accessToken,
                                "The consumer should now be able to get an access token for the OperatorCredential.");
                        assertFalse(accessToken.isEmpty(),
                                "The access token should not be empty.");
                        log.debug("Consumer successfully obtained OperatorCredential access token at provider.");
                    } catch (Throwable t) {
                        throw new AssertionFailedError(
                                "Consumer still cannot get OperatorCredential token: " + t.getMessage());
                    }
                });
    }

    // --- Helper methods ---

    /**
     * Creates a policy at the consumer PAP (central marketplace PAP).
     *
     * @param policyName the policy file name (without .json extension) from the policies resource directory
     */
    private void createPolicyAtConsumerPap(String policyName) throws IOException {
        String policyJson = getPolicy(policyName);
        RequestBody policyBody = RequestBody.create(policyJson,
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request policyRequest = new Request.Builder()
                .post(policyBody)
                .url(CONSUMER_PAP_ADDRESS + "/policy")
                .build();
        Response policyResponse = HTTP_CLIENT.newCall(policyRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, policyResponse.code(),
                    "The policy '" + policyName + "' should have been created at the consumer PAP.");
        } finally {
            policyResponse.body().close();
        }
        createdConsumerPolicies.add(policyResponse.header("Location"));
    }

    /**
     * Creates a policy at the provider PAP.
     *
     * @param policyName the policy file name (without .json extension) from the policies resource directory
     */
    private void createPolicyAtProviderPap(String policyName) throws IOException {
        String policyJson = getPolicy(policyName);
        RequestBody policyBody = RequestBody.create(policyJson,
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request policyRequest = new Request.Builder()
                .post(policyBody)
                .url(PROVIDER_PAP_ADDRESS + "/policy")
                .build();
        Response policyResponse = HTTP_CLIENT.newCall(policyRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, policyResponse.code(),
                    "The policy '" + policyName + "' should have been created at the provider PAP.");
        } finally {
            policyResponse.body().close();
        }
        createdProviderPolicies.add(policyResponse.header("Location"));
    }

    /**
     * Loads a policy JSON file from the classpath resources.
     *
     * @param policyName the policy file name (without .json extension)
     * @return the policy JSON string
     */
    private String getPolicy(String policyName) throws IOException {
        InputStream policyInputStream = this.getClass().getResourceAsStream(
                String.format("/policies/%s.json", policyName));
        assertNotNull(policyInputStream, "Policy file '" + policyName + ".json' should exist.");
        StringBuilder sb = new StringBuilder();
        for (int ch; (ch = policyInputStream.read()) != -1; ) {
            sb.append((char) ch);
        }
        return sb.toString();
    }

    /**
     * Gets an access token for the provider wallet at the central marketplace
     * by exchanging the provider's UserCredential via OID4VP.
     *
     * @return the access token for central marketplace API calls
     */
    private String getProviderAccessTokenAtMarketplace() throws Exception {
        OpenIdConfiguration openIdConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                MARKETPLACE_API_ADDRESS);
        return providerWallet.exchangeCredentialForToken(
                openIdConfig, PROVIDER_USER_CREDENTIAL, DEFAULT_SCOPE);
    }

    /**
     * Gets an access token for the consumer wallet at the central marketplace
     * by exchanging the consumer's SD-JWT UserCredential via OID4VP.
     *
     * @return the access token for central marketplace API calls
     */
    private String getConsumerAccessTokenAtMarketplace() throws Exception {
        OpenIdConfiguration openIdConfig = MPOperationsEnvironment.getOpenIDConfiguration(
                MARKETPLACE_API_ADDRESS);
        return consumerWallet.exchangeCredentialForToken(
                openIdConfig, USER_SD_CREDENTIAL, DEFAULT_SCOPE);
    }

    /**
     * Logs into the provider Keycloak and returns an access token.
     *
     * @param username the username to authenticate with
     * @return the Keycloak access token
     */
    private static String loginToProviderKeycloak(String username) throws Exception {
        KeycloakHelper providerKeycloak = new KeycloakHelper("test-realm", PROVIDER_KEYCLOAK_ADDRESS);
        return providerKeycloak.getUserToken(username, "test");
    }

    /**
     * Builds the full ODRL policy for K8SCluster creation as documented in CENTRAL_MARKETPLACE.md.
     * This policy allows users with the OPERATOR role and OperatorCredential to use K8SCluster entities.
     *
     * @return the ODRL policy as a Map structure for JSON serialization
     */
    private Map<String, Object> buildFullK8sOdrlPolicy() {
        Map<String, Object> odrlPolicy = new LinkedHashMap<>();
        odrlPolicy.put("@context", Map.of("odrl", "http://www.w3.org/ns/odrl/2/"));
        odrlPolicy.put("@id", "https://mp-operation.org/policy/common/k8s-full");
        odrlPolicy.put("odrl:uid", "https://mp-operation.org/policy/common/k8s-full");
        odrlPolicy.put("@type", "odrl:Policy");

        Map<String, Object> permission = new LinkedHashMap<>();
        permission.put("odrl:assigner", "https://www.mp-operation.org/");
        permission.put("odrl:target", Map.of(
                "@type", "odrl:AssetCollection",
                "odrl:source", "urn:asset",
                "odrl:refinement", List.of(
                        Map.of("@type", "odrl:Constraint",
                                "odrl:leftOperand", "ngsi-ld:entityType",
                                "odrl:operator", "odrl:eq",
                                "odrl:rightOperand", "K8SCluster"))));
        permission.put("odrl:assignee", Map.of(
                "@type", "odrl:PartyCollection",
                "odrl:source", "urn:user",
                "odrl:refinement", Map.of(
                        "@type", "odrl:LogicalConstraint",
                        "odrl:and", List.of(
                                Map.of("@type", "odrl:Constraint",
                                        "odrl:leftOperand", "vc:role",
                                        "odrl:operator", "odrl:hasPart",
                                        "odrl:rightOperand", Map.of("@value", "OPERATOR", "@type", "xsd:string")),
                                Map.of("@type", "odrl:Constraint",
                                        "odrl:leftOperand", "vc:type",
                                        "odrl:operator", "odrl:hasPart",
                                        "odrl:rightOperand", Map.of("@value", "OperatorCredential", "@type", "xsd:string"))))));
        permission.put("odrl:action", "odrl:use");
        odrlPolicy.put("odrl:permission", permission);

        return odrlPolicy;
    }
}
