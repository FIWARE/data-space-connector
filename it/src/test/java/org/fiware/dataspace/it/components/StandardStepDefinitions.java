package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
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
import org.fiware.dataspace.it.components.model.*;
import org.fiware.dataspace.tmf.model.*;
import org.keycloak.common.crypto.CryptoIntegration;
import org.opentest4j.AssertionFailedError;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.security.Security;
import java.time.Duration;
import java.util.*;

import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.OPERATOR_USER_NAME;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.TEST_USER_NAME;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_PAP_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.TMF_DIRECT_ADDRESS;
import static org.junit.jupiter.api.Assertions.*;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class StandardStepDefinitions extends StepDefintions {

    private static final String USER_CREDENTIAL = "user-credential";
    private static final String OPERATOR_CREDENTIAL = "operator-credential";
    private static final String DEFAULT_SCOPE = "default";
    private static final String OPERATOR_SCOPE = "operator";
    private static final String GRANT_TYPE_VP_TOKEN = "vp_token";
    private static final String RESPONSE_TYPE_DIRECT_POST = "direct_post";
    private static final String ENERGY_REPORT_ENTITY_ID = "urn:ngsi-ld:EnergyReport:fms-1";
    /**
     * Entity ID for the small K8S cluster created during marketplace tests.
     */
    private static final String K8S_CLUSTER_ENTITY_ID = "urn:ngsi-ld:K8SCluster:fancy-marketplace";
    /**
     * Entity ID for a larger K8S cluster used to test size restrictions.
     */
    private static final String K8S_CLUSTER_BIG_ENTITY_ID = "urn:ngsi-ld:K8SCluster:fancy-marketplace-big";
    /**
     * Schema location for the TMForum credential configuration characteristic.
     */
    private static final String CREDENTIALS_CONFIG_SCHEMA = "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json";
    /**
     * Schema location for the TMForum ODRL policy configuration characteristic.
     */
    private static final String POLICY_CONFIG_SCHEMA = "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json";
    /**
     * Timeout in seconds for policy propagation to OPA via the PAP.
     */
    private static final int POLICY_PROPAGATION_TIMEOUT_SECONDS = 20;
    /**
     * Timeout in seconds for data access assertions that require async policy updates.
     */
    private static final int DATA_ACCESS_TIMEOUT_SECONDS = 20;
    /**
     * Timeout in seconds for order completion and contract management propagation.
     */
    private static final int ORDER_PROPAGATION_TIMEOUT_SECONDS = 60;

    private Wallet fancyMarketplaceEmployeeWallet;
    private OrganizationVO fancyMarketplaceRegistration;
    private String transferProcessId;
    /**
     * Stores the product specification ID for the Small K8S spec.
     */
    private String productSpecSmallId;
    /**
     * Stores the product specification ID for the Full K8S spec.
     */
    private String productSpecFullId;
    private List<String> createdPolicies = new ArrayList<>();
    private List<String> createdEntities = new ArrayList<>();

    @Before
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        fancyMarketplaceEmployeeWallet = new Wallet();
        OBJECT_MAPPER.setSerializationInclusion(JsonInclude.Include.NON_EMPTY);
        clean();
        prepareTil();
        // allow the verifier to fetch the new config
        Thread.sleep(3001);
    }


    public void clean() throws Exception {
        cleanUpPolicies();
        cleanUpEntities();
        cleanUpDcatCatalog();
        cleanUpTMForum();
        cleanUpTIL();
        cleanUpAgreements();
    }


    private void cleanUpDcatCatalog() throws Exception {
        Request catalogsRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/catalogs")
                .build();
        try (Response catalogsResponse = HTTP_CLIENT.newCall(catalogsRequest).execute()) {
            ResponseBody responseBody = catalogsResponse.body();
            if (responseBody == null || !catalogsResponse.isSuccessful()) {
                return;
            }
            String bodyString = responseBody.string();
            List<DcatCatalog> catalogs;
            try {
                catalogs = OBJECT_MAPPER.readValue(bodyString, new TypeReference<List<DcatCatalog>>() {
                });
            } catch (Exception e) {
                log.warn("Could not parse catalogs response (status={}): {}", catalogsResponse.code(), bodyString);
                return;
            }
            for (DcatCatalog dcat : catalogs) {
                Request deleteRequest = new Request.Builder()
                        .delete()
                        .url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/catalogs/" + dcat.getId())
                        .build();
                try (Response ignored = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                }
            }
        }
    }

    private void cleanUpAgreements() throws Exception {
        Request agreementsRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements")
                .build();
        try (Response agreementsResponse = HTTP_CLIENT.newCall(agreementsRequest).execute()) {
            ResponseBody responseBody = agreementsResponse.body();
            if (responseBody == null || !agreementsResponse.isSuccessful()) {
                return;
            }
            String bodyString = responseBody.string();
            List<Agreement> agreements;
            try {
                agreements = OBJECT_MAPPER.readValue(bodyString, new TypeReference<List<Agreement>>() {
                });
            } catch (Exception e) {
                log.warn("Could not parse agreements response (status={}): {}", agreementsResponse.code(), bodyString);
                return;
            }
            for (Agreement agreement : agreements) {
                Request deleteRequest = new Request.Builder()
                        .delete()
                        .url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements/" + agreement.getAgreementId())
                        .build();
                try (Response ignored = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                }
            }
        }
    }

    private void cleanUpCCS() throws Exception {

        Request getServicesRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.CCS_DIRECT_ADDRESS + "/service")
                .build();
        List<String> serviceIds = new ArrayList<>();
        try (Response getServiceResponse = HTTP_CLIENT.newCall(getServicesRequest).execute()) {
            String servicesResponseBody = getServiceResponse.body().string();
            ServicesResponse servicesResponse = OBJECT_MAPPER.readValue(servicesResponseBody, ServicesResponse.class);
            servicesResponse.getServices()
                    .stream()
                    .map(ServiceConfig::getId)
                    .forEach(serviceIds::add);
        }
        serviceIds
                .forEach(id -> {
                    Request deletionRequest = new Request.Builder()
                            .delete()
                            .url(MPOperationsEnvironment.CCS_DIRECT_ADDRESS + "/service/" + id)
                            .build();
                    try (Response deletionResponse = HTTP_CLIENT.newCall(deletionRequest).execute()) {
                        log.debug("Deleted {} - code {}", id, deletionResponse.code());
                    } catch (IOException e) {
                        log.warn("Was not able to delete service {}", id);
                    }
                });
    }

    private void cleanUpTMForum() throws Exception {

        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/productOffering", "Standard offeringss");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productCatalogManagement/v4/productSpecification", "Standard specifications");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/productOrderingManagement/v4/productOrder", "Standard orders");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/agreementManagement/v4/agreement", "Standard agreements");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                "/tmf-api/party/v4/organization", "Standard organizations");

    }

    private void cleanUpPolicies() throws Exception {
        super.cleanUpPolicies(PROVIDER_PAP_ADDRESS);
    }

    private static final List<String> WELL_KNOWN_ENTITY_IDS = List.of(
            K8S_CLUSTER_ENTITY_ID,
            K8S_CLUSTER_BIG_ENTITY_ID,
            ENERGY_REPORT_ENTITY_ID
    );

    private void cleanUpEntities() {
        Set<String> toDelete = new LinkedHashSet<>(WELL_KNOWN_ENTITY_IDS);
        toDelete.addAll(createdEntities);
        toDelete.forEach(this::deleteEntity);
    }

    /**
     * Deletes an entity from Scorpio by ID, ignoring errors (e.g., if it does not exist).
     *
     * @param entityId the NGSI-LD entity ID to delete
     */
    private void deleteEntity(String entityId) {
        Request deletionRequest = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + entityId)
                .delete()
                .build();
        try (Response response = HTTP_CLIENT.newCall(deletionRequest).execute()) {
            // ignore result — entity may not exist
        } catch (IOException e) {
            log.warn("Was not able to delete entity {}.", entityId);
        }
    }

    @Given("M&P Operations is registered as a participant in the data space.")
    public void checkMPRegistered() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + MPOperationsEnvironment.PROVIDER_DID)
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        assertEquals(HttpStatus.SC_OK, tirResponse.code(), "The did should be registered at the trust-anchor.");
        tirResponse.body().close();
    }


    @Given("Fancy Marketplace is registered as a participant in the data space.")
    public void checkFMRegistered() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + FancyMarketplaceEnvironment.CONSUMER_DID)
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        assertEquals(HttpStatus.SC_OK, tirResponse.code(), "The did should be registered at the trust-anchor.");
        tirResponse.body().close();
    }

    @Given("Fancy Marketplace is not allowed to create a cluster at M&P Operations.")
    public void fmNotAllowedToCreateCluster() throws Exception {
        assertThrows(AssertionFailedError.class, () ->
                getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS), "Fancy Marketplace is not allowed to use the operator credential.");
        String userToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        Request createClusterRequest = createK8SClusterRequest(userToken);
        Response creationResponse = HTTP_CLIENT.newCall(createClusterRequest).execute();
        creationResponse.body().close();
        assertEquals(HttpStatus.SC_FORBIDDEN, creationResponse.code(), "The creation should not be allowed.");
    }

    private static Request getTheDetailedReport(String accessToken, String transferProcessId) throws IOException {
        return new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.PROVIDER_TPP_DATA_API_ADDRESS + "/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .addHeader("transferId", transferProcessId)
                .build();
    }

    private static Request createK8SClusterRequest(String accessToken) throws IOException {
        Map clusterEntity = Map.of("type", "K8SCluster",
                "id", "urn:ngsi-ld:K8SCluster:fancy-marketplace",
                "name", Map.of("type", "Property", "value", "Fancy Marketplace Cluster"),
                "numNodes", Map.of("type", "Property", "value", "3"),
                "k8sVersion", Map.of("type", "Property", "value", "1.26.0"));
        RequestBody clusterCreationBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(clusterEntity), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        return new Request.Builder()
                .post(clusterCreationBody)
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .build();
    }

    @Given("M&P Operations allows self-registration of organizations.")
    public void allowSelfRegistration() throws Exception {
        createPolicyAtMP("allowProductOffering");
        createPolicyAtMP("allowSelfRegistration");
        // we need to wait a little for the policies to be updated
        Thread.sleep(10000);
    }

    @Given("M&P Operations allows to buy its offerings.")
    public void allowProductOrder() throws Exception {
        createPolicyAtMP("allowProductOrder");
        // we need to wait a little for the policies to be updated
        Thread.sleep(10000);

    }

    private void createPolicyAtMP(String policy) throws IOException {
        RequestBody policyBody = RequestBody.create(getPolicy(policy), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request policyCreationRequest = new Request.Builder()
                .post(policyBody)
                .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy")
                .build();
        Response policyCreationResponse = HTTP_CLIENT.newCall(policyCreationRequest).execute();
        assertEquals(HttpStatus.SC_OK, policyCreationResponse.code(), "The policy should have been created.");
        policyCreationResponse.body().close();
        createdPolicies.add(policyCreationResponse.header("Location"));
    }

    @Given("M&P Operations offers a managed kubernetes.")
    public void createManagedKubernetesOffering() throws Exception {
        CategoryCreateVO categoryCreateVO = new CategoryCreateVO()
                .description("Reports")
                .name("Reports Category");
        RequestBody categoryRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(categoryCreateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request categoryRequest = new Request.Builder()
                .post(categoryRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/category")
                .build();
        Response categoryResponse = HTTP_CLIENT.newCall(categoryRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, categoryResponse.code(), "The category should have been created.");

        CategoryVO categoryVO = OBJECT_MAPPER.readValue(categoryResponse.body().string(), CategoryVO.class);
        CatalogCreateVO catalogCreateVO = new CatalogCreateVO()
                .description("M&P Operations Catalog")
                .name("M&P Operations Data")
                .category(List.of(new CategoryRefVO()
                        .id(categoryVO.getId())));
        categoryResponse.body().close();

        RequestBody catalogRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(catalogCreateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request catalogRequest = new Request.Builder()
                .post(catalogRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/catalog")
                .build();
        Response catalogResponse = HTTP_CLIENT.newCall(catalogRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, catalogResponse.code(), "The catalog should have been created.");
        catalogResponse.body().close();

        ProductSpecificationCreateVO pscVo = new ProductSpecificationCreateVO()
                .brand("M&P Operations")
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S").productSpecCharacteristic(List.of(
                        new ProductSpecificationCharacteristicVO()
                                .id("endpointUrl")
                                .name("K8S Service Endpoint")
                                .valueType("endpointUrl")
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value(MPOperationsEnvironment.PROVIDER_TPP_DATA_API_ADDRESS)
                                                        .isDefault(true))),
                        new ProductSpecificationCharacteristicVO()
                                .id("endpointDescription")
                                .name("K8S Service Endpoint Description")
                                .valueType("endpointDescription")
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value("Endpoint of the K8S service."))),
                        new ProductSpecificationCharacteristicVO()
                                .id("credentialsConfig")
                                .name("Credentials Config for the Target Service")
                                .valueType("credentialsConfiguration")
                                .atSchemaLocation(URI.create("https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json"))
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value(Map.of(
                                                                "credentialsType", "OperatorCredential",
                                                                "claims", List.of(
                                                                        Map.of("name", "roles",
                                                                                "path", "$.roles[?(@.target==\\\"" + MPOperationsEnvironment.PROVIDER_DID + "\\\")].names[*]",
                                                                                "allowedValues", List.of("OPERATOR"))))
                                                        )
                                        ))));

        RequestBody specificationRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(pscVo), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request specificationRequest = new Request.Builder()
                .post(specificationRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .build();
        Response specificationResponse = HTTP_CLIENT.newCall(specificationRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, specificationResponse.code(), "The specification should have been created.");
        ProductSpecificationVO createdSpec = OBJECT_MAPPER.readValue(specificationResponse.body().string(), ProductSpecificationVO.class);
        specificationResponse.body().close();

        ProductOfferingCreateVO productOfferingCreate = new ProductOfferingCreateVO()
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S Offering")
                .category(List.of(new CategoryRefVO().id(categoryVO.getId())))
                .version("1.0.0")
                .productSpecification(new ProductSpecificationRefVO().id(createdSpec.getId()));
        RequestBody productOfferingBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(productOfferingCreate), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request productOfferingRequest = new Request.Builder()
                .post(productOfferingBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response productOfferingResponse = HTTP_CLIENT.newCall(productOfferingRequest).execute();
        productOfferingResponse.body().close();
        assertEquals(HttpStatus.SC_CREATED, productOfferingResponse.code(), "The offering should have been created.");
    }


    @Given("M&P Operations offers detailed reports.")
    public void createReportsOffering() throws Exception {
        CategoryCreateVO categoryCreateVO = new CategoryCreateVO()
                .description("Reports about operations data.")
                .name("Reports Category");
        RequestBody categoryRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(categoryCreateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request categoryRequest = new Request.Builder()
                .post(categoryRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/category")
                .build();
        Response categoryResponse = HTTP_CLIENT.newCall(categoryRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, categoryResponse.code(), "The category should have been created.");

        CategoryVO categoryVO = OBJECT_MAPPER.readValue(categoryResponse.body().string(), CategoryVO.class);
        CatalogCreateVO catalogCreateVO = new CatalogCreateVO()
                .description("M&P Operations Catalog")
                .name("M&P Operations Data")
                .category(List.of(new CategoryRefVO()
                        .id(categoryVO.getId())));
        categoryResponse.body().close();

        RequestBody catalogRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(catalogCreateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request catalogRequest = new Request.Builder()
                .post(catalogRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/catalog")
                .build();
        Response catalogResponse = HTTP_CLIENT.newCall(catalogRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, catalogResponse.code(), "The catalog should have been created.");
        catalogResponse.body().close();
        ProductSpecificationCreateVO pscVo = new ProductSpecificationCreateVO()
                .brand("M&P Operations")
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name("M&P Uptime Reports")
                .productSpecCharacteristic(List.of(
                        new ProductSpecificationCharacteristicVO()
                                .id("endpointUrl")
                                .name("Reporting Service Endpoint")
                                .valueType("endpointUrl")
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value(MPOperationsEnvironment.PROVIDER_TPP_DATA_API_ADDRESS)
                                                        .isDefault(true))),
                        new ProductSpecificationCharacteristicVO()
                                .id("endpointDescription")
                                .name("Reporting Service Endpoint Description")
                                .valueType("endpointDescription")
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value("Endpoint of the reporting service."))),
                        new ProductSpecificationCharacteristicVO()
                                .id("credentialsConfig")
                                .name("Credentials Config for the Target Service")
                                .valueType("credentialsConfiguration")
                                .atSchemaLocation(URI.create("https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json"))
                                .productSpecCharacteristicValue(
                                        List.of(
                                                new CharacteristicValueSpecificationVO()
                                                        .value(Map.of(
                                                                "credentialsType", "OperatorCredential",
                                                                "claims", List.of(
                                                                        Map.of("name", "roles",
                                                                                "path", "$.roles[?(@.target==\\\"" + MPOperationsEnvironment.PROVIDER_DID + "\\\")].names[*]",
                                                                                "allowedValues", List.of("OPERATOR"))))
                                                        )
                                        ))
                ));
        RequestBody specificationRequestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(pscVo), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request specificationRequest = new Request.Builder()
                .post(specificationRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .build();
        Response specificationResponse = HTTP_CLIENT.newCall(specificationRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, specificationResponse.code(), "The specification should have been created.");
        ProductSpecificationVO createdSpec = OBJECT_MAPPER.readValue(specificationResponse.body().string(), ProductSpecificationVO.class);
        specificationResponse.body().close();

        ProductOfferingCreateVO productOfferingCreate = new ProductOfferingCreateVO()
                .lifecycleStatus("ACTIVE")
                .name("M&P Uptime Reports Offering")
                .version("1.0.0")
                .category(List.of(new CategoryRefVO().id(categoryVO.getId())))
                .productSpecification(new ProductSpecificationRefVO().id(createdSpec.getId()));
        RequestBody productOfferingBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(productOfferingCreate), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request productOfferingRequest = new Request.Builder()
                .post(productOfferingBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response productOfferingResponse = HTTP_CLIENT.newCall(productOfferingRequest).execute();
        productOfferingResponse.body().close();
        assertEquals(HttpStatus.SC_CREATED, productOfferingResponse.code(), "The offering should have been created.");
    }

    @When("Fancy Marketplace registers itself at M&P Operations.")
    public void registerAtMP() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(FancyMarketplaceEnvironment.CONSUMER_DID);

        OrganizationCreateVO organizationCreateVO = new OrganizationCreateVO()
                .organizationType("Consumer")
                .name("Fancy Marketplace Inc.")
                .partyCharacteristic(List.of(didCharacteristic));

        RequestBody organizationCreateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(organizationCreateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request organizationCreateRequest = new Request.Builder()
                .post(organizationCreateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/party/v4/organization")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response organizationCreateResponse = HTTP_CLIENT.newCall(organizationCreateRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, organizationCreateResponse.code(), "The organization should have been created.");
            fancyMarketplaceRegistration = OBJECT_MAPPER.readValue(organizationCreateResponse.body().string(), OrganizationVO.class);
        } finally {
            organizationCreateResponse.body().close();
        }
    }

    @When("Fancy Marketplace buys access to M&P's k8s services.")
    public void buyAccess() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offer should have been returend");
        List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
        });
        offerResponse.body().close();
        Optional<String> optionalId = offers.stream()
                .map(ProductOfferingVO::getId)
                .findFirst();
        assertTrue(optionalId.isPresent(), "The id should be present.");


        ProductOfferingRefVO productOfferingRefVO = new ProductOfferingRefVO()
                .id(optionalId.get());
        ProductOrderItemVO pod = new ProductOrderItemVO()
                .id("my-item")
                .action(OrderItemActionTypeVO.ADD)
                .productOffering(productOfferingRefVO);
        RelatedPartyVO relatedPartyVO = new RelatedPartyVO()
                .id(fancyMarketplaceRegistration.getId());
        ProductOrderCreateVO poc = new ProductOrderCreateVO()
                .productOrderItem(List.of(pod))
                .relatedParty(List.of(relatedPartyVO));
        RequestBody pocBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(poc), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request pocRequest = new Request.Builder()
                .post(pocBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response pocResponse = HTTP_CLIENT.newCall(pocRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, pocResponse.code(), "The product ordering should have been created.");
        ProductOrderVO productOrderVO = OBJECT_MAPPER.readValue(pocResponse.body().string(), ProductOrderVO.class);
        pocResponse.body().close();


        ProductOrderUpdateVO productOrderUpdateVO = new ProductOrderUpdateVO()
                .state(ProductOrderStateTypeVO.COMPLETED);
        RequestBody updateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(productOrderUpdateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request updateRequest = new Request.Builder()
                .patch(updateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder/" + productOrderVO.getId())
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response updateResponse = HTTP_CLIENT.newCall(updateRequest).execute();
        updateResponse.body().close();
        assertEquals(HttpStatus.SC_OK, updateResponse.code(), "The product ordering should have been updated.");

    }

    @When("Fancy Marketplace buys access to M&P's uptime reports.")
    public void buyAccessToReports() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offer should have been returend");
        List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
        });
        offerResponse.body().close();

        Optional<String> optionalId = offers.stream()
                .map(ProductOfferingVO::getId)
                .findFirst();
        assertTrue(optionalId.isPresent(), "The id should be present.");

        ProductOfferingRefVO productOfferingRefVO = new ProductOfferingRefVO()
                .id(optionalId.get());
        ProductOrderItemVO pod = new ProductOrderItemVO()
                .id("my-item")
                .action(OrderItemActionTypeVO.ADD)
                .productOffering(productOfferingRefVO);
        RelatedPartyVO relatedPartyVO = new RelatedPartyVO()
                .id(fancyMarketplaceRegistration.getId());
        ProductOrderCreateVO poc = new ProductOrderCreateVO()
                .productOrderItem(List.of(pod))
                .relatedParty(List.of(relatedPartyVO));
        RequestBody pocBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(poc), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request pocRequest = new Request.Builder()
                .post(pocBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response pocResponse = HTTP_CLIENT.newCall(pocRequest).execute();
        ProductOrderVO productOrderVO = OBJECT_MAPPER.readValue(pocResponse.body().string(), ProductOrderVO.class);
        pocResponse.body().close();
        assertEquals(HttpStatus.SC_CREATED, pocResponse.code(), "The product ordering should have been created.");

        ProductOrderUpdateVO productOrderUpdateVO = new ProductOrderUpdateVO()
                .state(ProductOrderStateTypeVO.COMPLETED);
        RequestBody updateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(productOrderUpdateVO), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request updateRequest = new Request.Builder()
                .patch(updateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder/" + productOrderVO.getId())
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response updateResponse = HTTP_CLIENT.newCall(updateRequest).execute();
        updateResponse.body().close();
        assertEquals(HttpStatus.SC_OK, updateResponse.code(), "The product ordering should have been created.");
    }

    @When("M&P Operations registers a policy to allow every participant access to its energy reports.")
    public void mpRegisterEnergyReportPolicy() throws Exception {
        createPolicyAtMP("energyReport");
    }

    @When("M&P Operations allows operators to create clusters.")
    public void mpRegisterClusterCreatePolicy() throws Exception {
        createPolicyAtMP("clusterCreate");
    }

    @When("M&P Operations allows to read its dcat-catalog.")
    public void mpRegisterCatalogReadPolicy() throws Exception {
        createPolicyAtMP("allowCatalogRead");
    }

    @When("M&P Operations allows to read its agreements.")
    public void mpRegisterAgreementPolicy() throws Exception {
        createPolicyAtMP("allowTMFAgreementRead");
    }

    @When("M&P Operations allows operators to read uptime reports.")
    public void mpRegisterUptimeReportPolicy() throws Exception {
        createPolicyAtMP("uptimeReport");
    }

    @When("M&P Operations allows operators to request data transfers.")
    public void mpRegisterDataTransferPolicy() throws Exception {
        createPolicyAtMP("transferRequest");
    }

    @When("M&P Operations creates an energy report.")
    public void createEnergyReport() throws Exception {
        Map offerEntity = Map.of("type", "EnergyReport",
                "id", "urn:ngsi-ld:EnergyReport:fms-1",
                "name", Map.of("type", "Property", "value", "Standard Server"),
                "consumption", Map.of("type", "Property", "value", "94"));
        RequestBody requestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(offerEntity), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));

        Request creationRequest = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities")
                .post(requestBody)
                .build();
        Response creationResponse = HTTP_CLIENT.newCall(creationRequest).execute();
        //assertEquals(HttpStatus.SC_CREATED, creationResponse.code(), "The entity should have been created.");
        creationResponse.body().close();
        createdEntities.add("urn:ngsi-ld:EnergyReport:fms-1");
    }

    @When("M&P Operations creates an uptime report.")
    public void createUptimeReport() throws Exception {
        Map offerEntity = Map.of("type", "UptimeReport",
                "id", "urn:ngsi-ld:UptimeReport:fms-1",
                "name", Map.of("type", "Property", "value", "Standard Server"),
                "uptime", Map.of("type", "Property", "value", "99.9"));
        RequestBody requestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(offerEntity), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));

        Request creationRequest = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities")
                .post(requestBody)
                .build();
        Response creationResponse = HTTP_CLIENT.newCall(creationRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, creationResponse.code(), "The entity should have been created.");
        creationResponse.body().close();
        createdEntities.add("urn:ngsi-ld:UptimeReport:fms-1");
    }

    @When("Fancy Marketplace issues a user credential to its employee.")
    public void issueUserCredentialToEmployee() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(TEST_USER_NAME);
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
    }

    @When("Fancy Marketplace issues an operator credential to its employee.")
    public void issueOperatorCredentialToEmployee() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(OPERATOR_USER_NAME);
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, OPERATOR_CREDENTIAL);
    }

    @When("Fancy Marketplace requests and starts the Data Transfer.")
    public void requestDataTransfer() throws Exception {
        Awaitility.await().atMost(Duration.ofSeconds(60)).untilAsserted(() -> {
            try {
                String accessToken = getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_TPP_API_ADDRESS);
                log.info("The token {}", accessToken);
                String agreementId = getAgreementId(accessToken);
                String consumerPid = "urn:uuid:" + UUID.randomUUID();
                transferProcessId = requestTransfer(accessToken, consumerPid, agreementId);
                startTransfer(accessToken, consumerPid, transferProcessId);
            } catch (Throwable t) {
                throw new AssertionFailedError(String.format("Was not able to start the data transfer: %s", t));
            }
        });
    }

    @Then("Fancy Marketplace operators can get the report.")
    public void getTheReports() throws Exception {
        Awaitility.await().atMost(Duration.ofSeconds(60)).untilAsserted(() -> {
            String accessToken = getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
            Request reportRequest = getTheDetailedReport(accessToken, transferProcessId);
            Response reportResponse = HTTP_CLIENT.newCall(reportRequest).execute();
            try {
                assertEquals(HttpStatus.SC_OK, reportResponse.code(), "The report should now have been returned.");
            } finally {
                reportResponse.body().close();
            }
        });
    }

    @Then("Fancy Marketplace operators can create clusters.")
    public void createK8SCluster() throws Exception {
        Awaitility.await().atMost(Duration.ofSeconds(60)).untilAsserted(() -> {
            try {
                String accessToken = getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
                Request creationRequest = createK8SClusterRequest(accessToken);
                Response creationResponse = HTTP_CLIENT.newCall(creationRequest).execute();
                try {
                    assertEquals(HttpStatus.SC_CREATED, creationResponse.code(), "The cluster should now have been created.");
                } finally {
                    creationResponse.body().close();
                }
            } catch (Throwable t) {
                throw new AssertionFailedError(String.format("Error: %s", t));
            }
        });

        createdEntities.add("urn:ngsi-ld:K8SCluster:fancy-marketplace");
    }

    @Then("Fancy Marketplace' employee can access the EnergyReport.")
    public void accessTheEnergyReport() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        Request authenticatedEntityRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .build();

        Awaitility.await()
                .atMost(Duration.ofSeconds(20))
                .until(() -> {
                    Response r = HTTP_CLIENT.newCall(authenticatedEntityRequest).execute();
                    try {
                        return HttpStatus.SC_OK == r.code();
                    } finally {
                        r.body().close();
                    }
                });
    }

    @Then("M&P Operations uptime report service is offered at the IDSA Catalog Endpoint.")
    public void accessCatalogEndpoint() throws Exception {

        Awaitility.await()
                .atMost(Duration.ofSeconds(60))
                .untilAsserted(() -> {
                    String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
                    Request authenticatedCatalogRequest = new Request.Builder().get()
                            .url(MPOperationsEnvironment.PROVIDER_TPP_API_ADDRESS + "/api/v1/catalogs")
                            .addHeader("Authorization", "Bearer " + accessToken)
                            .addHeader("Accept", "application/json")
                            .build();
                    Response catalogResponse = HTTP_CLIENT.newCall(authenticatedCatalogRequest).execute();
                    assertEquals(HttpStatus.SC_OK, catalogResponse.code(), "The catalog should have been returned.");
                    List<DcatCatalog> dcatCatalog = OBJECT_MAPPER.readValue(catalogResponse.body().string(), new TypeReference<List<DcatCatalog>>() {
                    });
                    catalogResponse.body().close();
                    assertEquals(1, dcatCatalog.size(), "The catalog entry should have been returned.");

                    DcatCatalog theCatalog = dcatCatalog.get(0);
                    assertEquals("dcat:Catalog", theCatalog.getType(), "The catalog should be of the correct type.");
                    assertEquals(1, theCatalog.getService().size(), "The offered service should be included.");

                    DcatService theService = theCatalog.getService().get(0);
                    assertEquals("dcat:DataService", theService.getType(), "The service should be of correct type.");
                    assertEquals(MPOperationsEnvironment.PROVIDER_TPP_DATA_API_ADDRESS, theService.getEndpointUrl(), "The correct endpoint should be returned for the service.");
                });


    }


    private static void startTransfer(String accessToken, String consumerPid, String providerPid) throws Exception {
        TransferStartMessage transferStartMessage = new TransferStartMessage();
        transferStartMessage.setConsumerPid(consumerPid);
        transferStartMessage.setProviderPid(providerPid);
        RequestBody requestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(transferStartMessage), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));

        Request transferRequest = new Request.Builder()
                .post(requestBody)
                .url(MPOperationsEnvironment.PROVIDER_TPP_API_ADDRESS + "/transfers/" + providerPid + "/start")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response transferResponse = HTTP_CLIENT.newCall(transferRequest).execute();
        transferResponse.body().close();
        assertEquals(HttpStatus.SC_OK, transferResponse.code(), "The transfer should have been started.");
    }

    private static String requestTransfer(String accessToken, String consumerPid, String agreementId) throws Exception {
        TransferRequestMessage transferRequestMessage = new TransferRequestMessage();
        transferRequestMessage.setAgreementId(agreementId);
        transferRequestMessage.setConsumerPid(consumerPid);
        transferRequestMessage.setCallbackAddress(FancyMarketplaceEnvironment.CONSUMER_TPP_ADDRESS);
        RequestBody requestBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(transferRequestMessage), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));

        Request transferRequest = new Request.Builder()
                .post(requestBody)
                .url(MPOperationsEnvironment.PROVIDER_TPP_API_ADDRESS + "/transfers/request")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response transferResponse = HTTP_CLIENT.newCall(transferRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, transferResponse.code(), "The transfer should have been requested.");
        String providerPid = OBJECT_MAPPER.readValue(transferResponse.body().string(), TransferRequestResponse.class).getProviderPid();
        transferResponse.body().close();
        return providerPid;
    }

    private static String getAgreementId(String accessToken) throws Exception {
        Request orderRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response orderResponse = HTTP_CLIENT.newCall(orderRequest).execute();
        assertEquals(HttpStatus.SC_OK, orderResponse.code(), "The orders should have been returned.");
        List<ProductOrder> orders = OBJECT_MAPPER.readValue(orderResponse.body().string(), new TypeReference<List<ProductOrder>>() {
        });
        orderResponse.body().close();
        assertEquals(1, orders.size(), "The agreement should have been returned.");

        String tmfAgreementId = orders.get(0).getAgreement().get(0).getId();
        Request agreementRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/agreementManagement/v4/agreement/" + tmfAgreementId)
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();

        Response agreementResponse = HTTP_CLIENT.newCall(agreementRequest).execute();
        assertEquals(HttpStatus.SC_OK, agreementResponse.code(), "The agreement should have been returned.");
        TMFAgreement tmfAgreement = OBJECT_MAPPER.readValue(agreementResponse.body().string(), TMFAgreement.class);
        Optional<String> agreementId = tmfAgreement.getCharacteristic()
                .stream()
                .filter(characteristic -> characteristic.getName().equals("Data-Space-Protocol-Agreement-Id"))
                .map(Characteristic::getValue)
                .filter(Objects::nonNull)
                .filter(String.class::isInstance)
                .map(String.class::cast)
                .findAny();
        agreementResponse.body().close();
        assertTrue(agreementId.isPresent(), "The agreement id should be present.");

        return agreementId.get();
    }

    private String getAccessTokenForFancyMarketplace(String credentialId, String scope, String targetAddress) throws Exception {
        OpenIdConfiguration openIdConfiguration = MPOperationsEnvironment.getOpenIDConfiguration(targetAddress);
        assertTrue(openIdConfiguration.getGrantTypesSupported().contains(GRANT_TYPE_VP_TOKEN), "The M&P environment should support vp_tokens");
        assertTrue(openIdConfiguration.getResponseModeSupported().contains(RESPONSE_TYPE_DIRECT_POST), "The M&P environment should support direct_post");
        assertNotNull(openIdConfiguration.getTokenEndpoint(), "The M&P environment should provide a token endpoint.");

        String accessToken = fancyMarketplaceEmployeeWallet.exchangeCredentialForToken(openIdConfiguration, credentialId, scope);
        return accessToken;
    }

    private String getDid(String didHelperAddress) throws Exception {
        Request didRequest = new Request.Builder().url(didHelperAddress + "/did-material/did.env").build();
        Response didResponse = HTTP_CLIENT.newCall(didRequest).execute();
        String didEnv = didResponse.body().string();
        didResponse.body().close();
        return didEnv.split("=")[1];
    }

    private String getPolicy(String policyName) throws IOException {
        InputStream policyInputStream = this.getClass().getResourceAsStream(String.format("/policies/%s.json", policyName));
        StringBuilder sb = new StringBuilder();
        for (int ch; (ch = policyInputStream.read()) != -1; ) {
            sb.append((char) ch);
        }
        return sb.toString();
    }

    // --- LOCAL.MD: Trust Anchor Verification Steps ---

    /**
     * Verifies that the trust anchor's TIR endpoint is reachable and responds successfully.
     */
    @Given("The trust anchor TIR endpoint is available.")
    public void checkTirEndpointAvailable() throws Exception {
        Request tirRequest = new Request.Builder()
                .get()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers")
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(tirRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, tirResponse.code(), "The TIR endpoint should be available.");
        } finally {
            tirResponse.body().close();
        }
    }

    /**
     * Verifies that the M&P Operations DID is registered at the trust anchor's TIR.
     */
    @Then("M&P Operations DID is registered at the trust anchor.")
    public void checkMPDIDAtTrustAnchor() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + MPOperationsEnvironment.PROVIDER_DID)
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, tirResponse.code(), "M&P Operations DID should be registered at the trust anchor.");
        } finally {
            tirResponse.body().close();
        }
    }

    /**
     * Verifies that the Fancy Marketplace DID is registered at the trust anchor's TIR.
     */
    @Then("Fancy Marketplace DID is registered at the trust anchor.")
    public void checkFMDIDAtTrustAnchor() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + FancyMarketplaceEnvironment.CONSUMER_DID)
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, tirResponse.code(), "Fancy Marketplace DID should be registered at the trust anchor.");
        } finally {
            tirResponse.body().close();
        }
    }

    /**
     * Verifies that the TIR lists all registered issuers and the list is non-empty.
     */
    @Then("The trust anchor lists all registered issuers.")
    public void trustAnchorListsIssuers() throws Exception {
        Request issuersRequest = new Request.Builder()
                .get()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers")
                .build();
        Response issuersResponse = HTTP_CLIENT.newCall(issuersRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, issuersResponse.code(), "The issuers list should be returned.");
            String body = issuersResponse.body().string();
            assertNotNull(body, "The issuers list response body should not be null.");
            assertFalse(body.isEmpty(), "The issuers list should not be empty.");
        } finally {
            issuersResponse.body().close();
        }
    }

    // --- LOCAL.MD: Credential Issuance Steps ---

    /**
     * Verifies that the consumer Keycloak instance exposes the openid-credential-issuer well-known endpoint.
     */
    @Given("The consumer Keycloak credential issuer is configured.")
    public void checkConsumerKeycloakIssuerConfigured() throws Exception {
        IssuerConfiguration issuerConfig = fancyMarketplaceEmployeeWallet
                .getIssuerConfiguration(FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS);
        assertNotNull(issuerConfig, "The issuer configuration should be returned.");
        assertNotNull(issuerConfig.getCredentialEndpoint(), "The credential endpoint should be configured.");
        assertFalse(issuerConfig.getCredentialConfigurationsSupported().isEmpty(),
                "At least one credential configuration should be supported.");
    }

    /**
     * Issues a user credential to the consumer employee via the OID4VCI Same-Device flow.
     * This exercises the complete flow: Keycloak login -> offer URI -> pre-authorized code -> credential.
     */
    @When("The consumer employee logs into Keycloak and receives a user credential.")
    public void consumerEmployeeGetsUserCredential() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(TEST_USER_NAME);
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(
                accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
    }

    /**
     * Verifies that the user credential was successfully stored in the wallet after issuance.
     */
    @Then("The user credential is stored in the wallet.")
    public void userCredentialIsStored() {
        String credential = fancyMarketplaceEmployeeWallet.getStoredCredential(USER_CREDENTIAL);
        assertNotNull(credential, "The user credential should be stored in the wallet.");
        assertFalse(credential.isEmpty(), "The stored credential should not be empty.");
    }

    /**
     * Verifies that the credential issuer metadata contains supported credential configurations
     * including the user-credential type.
     */
    @Then("The credential issuer metadata contains supported credential configurations.")
    public void credentialIssuerMetadataContainsConfigurations() throws Exception {
        IssuerConfiguration issuerConfig = fancyMarketplaceEmployeeWallet
                .getIssuerConfiguration(FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS);
        assertTrue(issuerConfig.getCredentialConfigurationsSupported().containsKey(USER_CREDENTIAL),
                "The issuer should support the user-credential configuration.");
    }

    // --- LOCAL.MD: Policy and Entity Creation Steps ---

    /**
     * Creates the energy report access policy at the provider's PAP endpoint.
     */
    @When("The provider creates an energy report access policy at the PAP.")
    public void providerCreatesEnergyReportPolicy() throws Exception {
        createPolicyAtMP("energyReport");
    }

    /**
     * Creates an EnergyReport entity directly at the provider's Scorpio broker.
     */
    @When("The provider creates an EnergyReport entity at Scorpio.")
    public void providerCreatesEnergyReportEntity() throws Exception {
        Map<String, Object> offerEntity = Map.of("type", "EnergyReport",
                "id", ENERGY_REPORT_ENTITY_ID,
                "name", Map.of("type", "Property", "value", "Standard Server"),
                "consumption", Map.of("type", "Property", "value", "94"));
        RequestBody requestBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(offerEntity),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request creationRequest = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities")
                .post(requestBody)
                .build();
        Response creationResponse = HTTP_CLIENT.newCall(creationRequest).execute();
        creationResponse.body().close();
        createdEntities.add(ENERGY_REPORT_ENTITY_ID);
    }

    /**
     * Verifies that the energy report policy exists and is active at the provider PAP.
     */
    @Then("The energy report policy is active at the provider PAP.")
    public void energyReportPolicyIsActive() throws Exception {
        Awaitility.await()
                .atMost(Duration.ofSeconds(POLICY_PROPAGATION_TIMEOUT_SECONDS))
                .untilAsserted(() -> {
                    Request getPolicies = new Request.Builder()
                            .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy")
                            .get().build();
                    Response policyResponse = HTTP_CLIENT.newCall(getPolicies).execute();
                    try {
                        assertEquals(HttpStatus.SC_OK, policyResponse.code(), "The PAP should return policies.");
                        String body = policyResponse.body().string();
                        assertTrue(body.contains("energyReport"),
                                "The energy report policy should be listed in the active policies.");
                    } finally {
                        policyResponse.body().close();
                    }
                });
    }

    // --- LOCAL.MD: OpenID Configuration and Data Access Steps ---

    /**
     * Verifies that the provider data service exposes a well-known openid-configuration endpoint
     * with the expected grant types and token endpoint.
     */
    @Then("The provider data service exposes an openid-configuration endpoint.")
    public void providerDataServiceExposesOpenIdConfig() throws Exception {
        OpenIdConfiguration config = MPOperationsEnvironment.getOpenIDConfiguration(
                MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        assertNotNull(config, "The openid-configuration should be returned.");
        assertNotNull(config.getTokenEndpoint(), "A token endpoint should be provided.");
        assertTrue(config.getGrantTypesSupported().contains(GRANT_TYPE_VP_TOKEN),
                "The data service should support vp_token grant type.");
    }

    /**
     * Verifies that the Fancy Marketplace employee can exchange a user credential for an access token
     * via the OID4VP flow at the provider's data service.
     */
    @Then("Fancy Marketplace employee can exchange the credential for an access token.")
    public void fmEmployeeCanExchangeCredentialForToken() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE,
                MPOperationsEnvironment.PROVIDER_API_ADDRESS);
        assertNotNull(accessToken, "The access token should be returned.");
        assertFalse(accessToken.isEmpty(), "The access token should not be empty.");
    }

    /**
     * Verifies that the Fancy Marketplace employee can access the EnergyReport entity
     * using the access token obtained via OID4VP.
     */
    @Then("Fancy Marketplace employee can access the EnergyReport with the access token.")
    public void fmEmployeeCanAccessEnergyReport() throws Exception {
        Awaitility.await()
                .atMost(Duration.ofSeconds(DATA_ACCESS_TIMEOUT_SECONDS))
                .untilAsserted(() -> {
                    String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE,
                            MPOperationsEnvironment.PROVIDER_API_ADDRESS);
                    Request entityRequest = new Request.Builder()
                            .get()
                            .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ENTITY_ID)
                            .addHeader("Authorization", "Bearer " + accessToken)
                            .addHeader("Accept", "application/json")
                            .build();
                    Response entityResponse = HTTP_CLIENT.newCall(entityRequest).execute();
                    try {
                        assertEquals(HttpStatus.SC_OK, entityResponse.code(),
                                "The EnergyReport entity should be accessible with a valid token.");
                    } finally {
                        entityResponse.body().close();
                    }
                });
    }

    // --- LOCAL.MD: Unauthenticated Access Rejection Step ---

    /**
     * Verifies that an unauthenticated GET request to the provider data service is rejected with HTTP 401.
     */
    @Then("An unauthenticated request to the provider data service returns 401.")
    public void unauthenticatedRequestReturns401() throws Exception {
        Request unauthenticatedRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ENTITY_ID)
                .addHeader("Accept", "application/json")
                .build();
        Response response = HTTP_CLIENT.newCall(unauthenticatedRequest).execute();
        try {
            assertEquals(HttpStatus.SC_UNAUTHORIZED, response.code(),
                    "An unauthenticated request should be rejected with 401.");
        } finally {
            response.body().close();
        }
    }

    // --- LOCAL.MD: Marketplace and Service Buying Flow Steps ---

    /**
     * Creates the "M&P K8S Small" product specification at the provider's TMForum API.
     * Includes both credentialsConfig (OperatorCredential with OPERATOR role) and
     * policyConfig (ODRL policy restricting numNodes to 3) characteristics as documented in LOCAL.MD.
     */
    @When("The provider creates a K8S Small product specification with credentials and policy config.")
    public void createSmallProductSpec() throws Exception {
        productSpecSmallId = createProductSpecWithPolicy("M&P K8S Small",
                "https://mp-operation.org/policy/common/k8s-small", buildSmallPolicyRefinements());
    }

    /**
     * Creates the "M&P K8S" (full) product specification at the provider's TMForum API.
     * Includes both credentialsConfig (OperatorCredential with OPERATOR role) and
     * policyConfig (ODRL policy allowing unrestricted K8SCluster creation) characteristics.
     */
    @When("The provider creates a K8S Full product specification with credentials and policy config.")
    public void createFullProductSpec() throws Exception {
        productSpecFullId = createProductSpecWithPolicy("M&P K8S",
                "https://mp-operation.org/policy/common/k8s-full", buildFullPolicyRefinements());
    }

    /**
     * Creates a Small product offering referencing the Small product specification.
     */
    @When("The provider creates a Small product offering referencing the Small specification.")
    public void createSmallProductOffering() throws Exception {
        assertNotNull(productSpecSmallId, "The Small product spec must be created first.");
        createProductOffering("M&P K8S Offering Small", productSpecSmallId);
    }

    /**
     * Creates a Full product offering referencing the Full product specification.
     */
    @When("The provider creates a Full product offering referencing the Full specification.")
    public void createFullProductOffering() throws Exception {
        assertNotNull(productSpecFullId, "The Full product spec must be created first.");
        createProductOffering("M&P K8S Offering", productSpecFullId);
    }

    /**
     * Verifies that exactly two product offerings are available at the provider's TMForum API.
     */
    @Then("Two product offerings are available at the provider TMForum API.")
    public void twoOfferingsAvailable() throws Exception {
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        try {
            assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offerings should be returned.");
            List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(
                    offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                    });
            assertEquals(2, offers.size(), "There should be exactly two product offerings (Small and Full).");
        } finally {
            offerResponse.body().close();
        }
    }

    /**
     * Registers Fancy Marketplace as an organization at M&P Operations using the representative's
     * user credential via the OID4VP-authenticated TMForum API endpoint.
     */
    @When("Fancy Marketplace representative registers at M&P Operations.")
    public void representativeRegistersAtMP() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE,
                MPOperationsEnvironment.PROVIDER_API_ADDRESS);

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(FancyMarketplaceEnvironment.CONSUMER_DID);

        OrganizationCreateVO organizationCreateVO = new OrganizationCreateVO()
                .organizationType("Consumer")
                .name("Fancy Marketplace Inc.")
                .partyCharacteristic(List.of(didCharacteristic));

        RequestBody organizationCreateBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(organizationCreateVO),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request organizationCreateRequest = new Request.Builder()
                .post(organizationCreateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/party/v4/organization")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response organizationCreateResponse = HTTP_CLIENT.newCall(organizationCreateRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, organizationCreateResponse.code(),
                    "The organization should have been created.");
            fancyMarketplaceRegistration = OBJECT_MAPPER.readValue(
                    organizationCreateResponse.body().string(), OrganizationVO.class);
        } finally {
            organizationCreateResponse.body().close();
        }
    }

    /**
     * Verifies that Fancy Marketplace is registered as an organization at M&P Operations.
     */
    @Then("Fancy Marketplace is registered as an organization at M&P Operations.")
    public void fmIsRegisteredOrganization() {
        assertNotNull(fancyMarketplaceRegistration, "The Fancy Marketplace registration should exist.");
        assertNotNull(fancyMarketplaceRegistration.getId(),
                "The registration should have an ID.");
    }

    /**
     * Buys the first available product offering at M&P Operations on behalf of the
     * Fancy Marketplace representative: lists offerings, creates and completes a product order.
     */
    @When("Fancy Marketplace representative buys the first available offering.")
    public void representativeBuysFirstOffering() throws Exception {
        assertNotNull(fancyMarketplaceRegistration,
                "Fancy Marketplace must be registered before buying.");

        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE,
                MPOperationsEnvironment.PROVIDER_API_ADDRESS);

        // List offerings
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offerings should be returned.");
        List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(
                offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
                });
        offerResponse.body().close();
        assertFalse(offers.isEmpty(), "At least one offering should be available.");

        String offerId = offers.get(0).getId();

        // Create product order
        ProductOfferingRefVO productOfferingRefVO = new ProductOfferingRefVO().id(offerId);
        ProductOrderItemVO pod = new ProductOrderItemVO()
                .id("marketplace-order-item")
                .action(OrderItemActionTypeVO.ADD)
                .productOffering(productOfferingRefVO);
        RelatedPartyVO relatedPartyVO = new RelatedPartyVO()
                .id(fancyMarketplaceRegistration.getId());
        ProductOrderCreateVO poc = new ProductOrderCreateVO()
                .productOrderItem(List.of(pod))
                .relatedParty(List.of(relatedPartyVO));
        RequestBody pocBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(poc),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request pocRequest = new Request.Builder()
                .post(pocBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS
                        + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response pocResponse = HTTP_CLIENT.newCall(pocRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, pocResponse.code(),
                "The product order should have been created.");
        ProductOrderVO productOrderVO = OBJECT_MAPPER.readValue(
                pocResponse.body().string(), ProductOrderVO.class);
        pocResponse.body().close();

        // Complete the order
        ProductOrderUpdateVO productOrderUpdateVO = new ProductOrderUpdateVO()
                .state(ProductOrderStateTypeVO.COMPLETED);
        RequestBody updateBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(productOrderUpdateVO),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request updateRequest = new Request.Builder()
                .patch(updateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS
                        + "/tmf-api/productOrderingManagement/v4/productOrder/"
                        + productOrderVO.getId())
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response updateResponse = HTTP_CLIENT.newCall(updateRequest).execute();
        updateResponse.body().close();
        assertEquals(HttpStatus.SC_OK, updateResponse.code(),
                "The product order should have been completed.");
    }

    /**
     * Verifies that the Fancy Marketplace operator can create a K8S cluster with the specified
     * number of nodes. Waits for contract management propagation to complete.
     *
     * @see #createK8SClusterWithNodes(String, String, int)
     */
    @Then("Fancy Marketplace operator can create a K8S cluster with {int} nodes.")
    public void operatorCanCreateClusterWithNodes(int numNodes) throws Exception {
        String entityId = numNodes <= 3 ? K8S_CLUSTER_ENTITY_ID : K8S_CLUSTER_BIG_ENTITY_ID;
        Awaitility.await().atMost(Duration.ofSeconds(ORDER_PROPAGATION_TIMEOUT_SECONDS)).untilAsserted(() -> {
            try {
                String accessToken = getAccessTokenForFancyMarketplace(
                        OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
                Response creationResponse = createK8SClusterWithNodes(accessToken, entityId, numNodes);
                try {
                    assertEquals(HttpStatus.SC_CREATED, creationResponse.code(),
                            String.format("The cluster with %d and id %s nodes should have been created.", numNodes, entityId));
                } finally {
                    creationResponse.body().close();
                }
            } catch (Throwable t) {
                throw new AssertionFailedError(String.format(
                        "Cluster creation with %d nodes failed: %s", numNodes, t.getMessage()));
            }
        });
        createdEntities.add(entityId);
    }

    /**
     * Verifies that the Fancy Marketplace operator cannot create a K8S cluster with the specified
     * number of nodes (e.g., when restricted by the small offering policy).
     */
    @Then("Fancy Marketplace operator cannot create a K8S cluster with {int} nodes.")
    public void operatorCannotCreateClusterWithNodes(int numNodes) throws Exception {
        String entityId = numNodes <= 3 ? K8S_CLUSTER_ENTITY_ID : K8S_CLUSTER_BIG_ENTITY_ID;
        // Allow a brief pause for policy propagation, then verify rejection
        Awaitility.await().atMost(Duration.ofSeconds(ORDER_PROPAGATION_TIMEOUT_SECONDS)).untilAsserted(() -> {
            try {
                String accessToken = getAccessTokenForFancyMarketplace(
                        OPERATOR_CREDENTIAL, OPERATOR_SCOPE, MPOperationsEnvironment.PROVIDER_API_ADDRESS);
                Response creationResponse = createK8SClusterWithNodes(accessToken, entityId, numNodes);
                try {
                    assertTrue(creationResponse.code() == HttpStatus.SC_FORBIDDEN
                                    || creationResponse.code() == HttpStatus.SC_UNAUTHORIZED,
                            String.format("Cluster creation with %d nodes should be forbidden, got %d.",
                                    numNodes, creationResponse.code()));
                } finally {
                    creationResponse.body().close();
                }
            } catch (Throwable t) {
                throw new AssertionFailedError(String.format(
                        "Unexpected error checking cluster restriction with %d nodes for entity %s: %s",
                        numNodes, entityId, t.getMessage()));
            }
        });
    }

    // --- Helper methods for marketplace flow ---

    /**
     * Creates a product specification at the provider's TMForum API with both
     * credentialsConfig and policyConfig characteristics.
     *
     * @param specName          the name of the product specification
     * @param policyId          the ODRL policy ID (e.g., "https://mp-operation.org/policy/common/k8s-small")
     * @param targetRefinements the ODRL target refinement constraints for the policy
     * @return the created product specification ID
     */
    private String createProductSpecWithPolicy(String specName, String policyId,
                                               List<Map<String, Object>> targetRefinements) throws Exception {
        // Build credentials config characteristic
        Map<String, Object> credentialsValue = Map.of(
                "credentialsType", "OperatorCredential",
                "claims", List.of(
                        Map.of("name", "roles",
                                "path", "$.roles[?(@.target==\"" + MPOperationsEnvironment.PROVIDER_DID + "\")].names[*]",
                                "allowedValues", List.of("OPERATOR"))));

        // Build ODRL policy for policyConfig
        Map<String, Object> odrlPolicy = new LinkedHashMap<>();
        odrlPolicy.put("@context", Map.of("odrl", "http://www.w3.org/ns/odrl/2/"));
        odrlPolicy.put("@id", policyId);
        odrlPolicy.put("odrl:uid", policyId);
        odrlPolicy.put("@type", "odrl:Policy");

        Map<String, Object> permission = new LinkedHashMap<>();
        permission.put("odrl:assigner", "https://www.mp-operation.org/");
        permission.put("odrl:target", Map.of(
                "@type", "odrl:AssetCollection",
                "odrl:source", "urn:asset",
                "odrl:refinement", targetRefinements));
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

        ProductSpecificationCreateVO pscVo = new ProductSpecificationCreateVO()
                .brand("M&P Operations")
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name(specName)
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
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .build();
        Response specResponse = HTTP_CLIENT.newCall(specRequest).execute();
        try {
            assertEquals(HttpStatus.SC_CREATED, specResponse.code(),
                    "The product specification should have been created.");
            ProductSpecificationVO createdSpec = OBJECT_MAPPER.readValue(
                    specResponse.body().string(), ProductSpecificationVO.class);
            return createdSpec.getId();
        } finally {
            specResponse.body().close();
        }
    }

    /**
     * Builds the ODRL target refinement list for the "small" K8S policy,
     * which restricts entity type to K8SCluster and numNodes to 3.
     */
    private List<Map<String, Object>> buildSmallPolicyRefinements() {
        return List.of(
                Map.of("@type", "odrl:Constraint",
                        "odrl:leftOperand", "ngsi-ld:entityType",
                        "odrl:operator", "odrl:eq",
                        "odrl:rightOperand", "K8SCluster"),
                Map.of("@type", "odrl:Constraint",
                        "http:bodyValue", "$.numNodes.value",
                        "odrl:operator", "odrl:eq",
                        "odrl:rightOperand", "3"));
    }

    /**
     * Builds the ODRL target refinement list for the "full" K8S policy,
     * which only restricts entity type to K8SCluster (no numNodes restriction).
     */
    private List<Map<String, Object>> buildFullPolicyRefinements() {
        return List.of(
                Map.of("@type", "odrl:Constraint",
                        "odrl:leftOperand", "ngsi-ld:entityType",
                        "odrl:operator", "odrl:eq",
                        "odrl:rightOperand", "K8SCluster"));
    }

    /**
     * Creates a product offering at the provider's TMForum API referencing the given specification.
     *
     * @param offeringName the name of the product offering
     * @param specId       the product specification ID to reference
     */
    private void createProductOffering(String offeringName, String specId) throws Exception {
        ProductOfferingCreateVO productOfferingCreate = new ProductOfferingCreateVO()
                .lifecycleStatus("ACTIVE")
                .name(offeringName)
                .version("1.0.0")
                .productSpecification(new ProductSpecificationRefVO().id(specId));
        RequestBody offeringBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(productOfferingCreate),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request offeringRequest = new Request.Builder()
                .post(offeringBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS
                        + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response offeringResponse = HTTP_CLIENT.newCall(offeringRequest).execute();
        offeringResponse.body().close();
        assertEquals(HttpStatus.SC_CREATED, offeringResponse.code(),
                "The product offering should have been created.");
    }

    /**
     * Creates a K8S cluster entity with the specified number of nodes.
     *
     * @param accessToken the bearer token for authentication
     * @param entityId    the NGSI-LD entity ID
     * @param numNodes    the number of cluster nodes
     * @return the HTTP response from the creation request
     */
    private Response createK8SClusterWithNodes(String accessToken, String entityId,
                                               int numNodes) throws Exception {
        Map<String, Object> clusterEntity = Map.of(
                "type", "K8SCluster",
                "id", entityId,
                "name", Map.of("type", "Property", "value", "Fancy Marketplace Cluster"),
                "numNodes", Map.of("type", "Property", "value", String.valueOf(numNodes)),
                "k8sVersion", Map.of("type", "Property", "value", "1.26.0"));
        RequestBody clusterBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(clusterEntity),
                okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request creationRequest = new Request.Builder()
                .post(clusterBody)
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .build();
        return HTTP_CLIENT.newCall(creationRequest).execute();
    }

}
