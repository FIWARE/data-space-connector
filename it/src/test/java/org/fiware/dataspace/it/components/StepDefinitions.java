package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.squareup.okhttp.MediaType;
import com.squareup.okhttp.OkHttpClient;
import com.squareup.okhttp.Request;
import com.squareup.okhttp.RequestBody;
import com.squareup.okhttp.Response;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import lombok.extern.slf4j.Slf4j;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.Policy;
import org.fiware.dataspace.tmf.model.CharacteristicVO;
import org.fiware.dataspace.tmf.model.OrderItemActionTypeVO;
import org.fiware.dataspace.tmf.model.OrganizationCreateVO;
import org.fiware.dataspace.tmf.model.OrganizationVO;
import org.fiware.dataspace.tmf.model.ProductOfferingCreateVO;
import org.fiware.dataspace.tmf.model.ProductOfferingRefVO;
import org.fiware.dataspace.tmf.model.ProductOfferingVO;
import org.fiware.dataspace.tmf.model.ProductOrderCreateVO;
import org.fiware.dataspace.tmf.model.ProductOrderItemVO;
import org.fiware.dataspace.tmf.model.ProductSpecificationCreateVO;
import org.fiware.dataspace.tmf.model.ProductSpecificationRefVO;
import org.fiware.dataspace.tmf.model.ProductSpecificationVO;
import org.fiware.dataspace.tmf.model.RelatedPartyVO;
import org.keycloak.common.crypto.CryptoIntegration;
import org.opentest4j.AssertionFailedError;

import java.io.IOException;
import java.io.InputStream;
import java.security.Security;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class StepDefinitions {

    private static final OkHttpClient HTTP_CLIENT = new OkHttpClient();
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();
    private static final String USER_CREDENTIAL = "user-credential";
    private static final String OPERATOR_CREDENTIAL = "operator-credential";
    private static final String DEFAULT_SCOPE = "default";
    private static final String OPERATOR_SCOPE = "operator";
    private static final String GRANT_TYPE_VP_TOKEN = "vp_token";
    private static final String RESPONSE_TYPE_DIRECT_POST = "direct_post";

    private Wallet fancyMarketplaceEmployeeWallet;
    private OrganizationVO fancyMarketplaceRegistration;
    private List<String> createdPolicies = new ArrayList<>();
    private List<String> createdEntities = new ArrayList<>();

    @Before
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        fancyMarketplaceEmployeeWallet = new Wallet();
    }

    @After
    public void cleanUp() throws Exception {
        cleanUpPolicies();
        cleanUpEntities();
        cleanUpTMForum();
        cleanUpTIL();
    }

    private void cleanUpTIL() throws Exception {
        String consumerDid = getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS);
        Request tilCleanRequest = new Request.Builder()
                .delete()
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer/" + consumerDid)
                .build();
        HTTP_CLIENT.newCall(tilCleanRequest).execute();
        Map tilConfig = Map.of(
                "did", getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS),
                "credentials", List.of(Map.of("credentialsType", "UserCredential", "claims", List.of())));
        RequestBody tilUpdateBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(tilConfig));
        Request tilUpdateRequest = new Request.Builder()
                .post(tilUpdateBody)
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer")
                .build();
        HTTP_CLIENT.newCall(tilUpdateRequest).execute();
    }

    private void cleanUpTMForum() throws Exception {
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offer should have been returend");
        List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
        });
        offers.stream()
                .map(ProductOfferingVO::getId)
                .forEach(id -> {
                    Request deletionRequest = new Request.Builder()
                            .delete()
                            .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering/" + id)
                            .build();
                    try {
                        HTTP_CLIENT.newCall(deletionRequest).execute();
                    } catch (IOException e) {
                        // ignore
                    }
                });
        offerResponse.body().close();

        Request specRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .build();
        Response specResponse = HTTP_CLIENT.newCall(specRequest).execute();
        assertEquals(HttpStatus.SC_OK, specResponse.code(), "The spec should have been returend");
        List<ProductSpecificationVO> specs = OBJECT_MAPPER.readValue(specResponse.body().string(), new TypeReference<List<ProductSpecificationVO>>() {
        });
        specs.stream()
                .map(ProductSpecificationVO::getId)
                .forEach(id -> {
                    Request deletionRequest = new Request.Builder()
                            .delete()
                            .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification/" + id)
                            .build();
                    try {
                        HTTP_CLIENT.newCall(deletionRequest).execute();
                    } catch (IOException e) {
                        // ignore
                    }
                });
        specResponse.body().close();

        Request organizationRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/party/v4/organization")
                .build();
        Response organizationResponse = HTTP_CLIENT.newCall(organizationRequest).execute();
        assertEquals(HttpStatus.SC_OK, organizationResponse.code(), "The spec should have been returend");
        List<OrganizationVO> organizations = OBJECT_MAPPER.readValue(organizationResponse.body().string(), new TypeReference<List<OrganizationVO>>() {
        });
        organizations.stream()
                .map(OrganizationVO::getId)
                .forEach(id -> {
                    Request deletionRequest = new Request.Builder()
                            .delete()
                            .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/party/v4/organization/" + id)
                            .build();
                    try {
                        HTTP_CLIENT.newCall(deletionRequest).execute();
                    } catch (IOException e) {
                        // ignore
                    }
                });
        organizationResponse.body().close();
    }

    private void cleanUpPolicies() throws Exception {
        Request getPolicies = new Request.Builder()
                .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy")
                .get().build();
        Response policyResponse = HTTP_CLIENT.newCall(getPolicies).execute();

        List<Policy> policies = OBJECT_MAPPER.readValue(policyResponse.body().string(), new TypeReference<List<Policy>>() {
        });

        policies.forEach(policyId -> {
            Request deletionRequest = new Request.Builder()
                    .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy/" + policyId.getId())
                    .delete()
                    .build();
            try {
                Response r = HTTP_CLIENT.newCall(deletionRequest).execute();
                log.warn(r.body().string());
            } catch (IOException e) {
                // just log
                log.warn("Was not able to clean up policy {}.", policyId);
            }
        });
    }

    private void cleanUpEntities() {
        createdEntities.forEach(entityId -> {
            Request deletionRequest = new Request.Builder()
                    .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + entityId)
                    .delete()
                    .build();
            try {
                HTTP_CLIENT.newCall(deletionRequest).execute();
            } catch (IOException e) {
                // just log
                log.warn("Was not able to clean up entitiy {}.", entityId);
            }
        });
    }

    @Given("M&P Operations is registered as a participant in the data space.")
    public void checkMPRegistered() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + getDid(MPOperationsEnvironment.DID_PROVIDER_ADDRESS))
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        assertEquals(HttpStatus.SC_OK, tirResponse.code(), "The did should be registered at the trust-anchor.");
        tirResponse.body().close();
    }


    @Given("Fancy Marketplace is registered as a participant in the data space.")
    public void checkFMRegistered() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS))
                .build();
        Response tirResponse = HTTP_CLIENT.newCall(didCheckRequest).execute();
        assertEquals(HttpStatus.SC_OK, tirResponse.code(), "The did should be registered at the trust-anchor.");
        tirResponse.body().close();
    }

    @Given("Fancy Marketplace is not allowed to create a cluster at M&P Operations.")
    public void fmNotAllowedToCreateCluster() throws Exception {
        assertThrows(AssertionFailedError.class, () ->
                getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE), "Fancy Marketplace is not allowed to use the operator credential.");
        String userToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE);
        Request createClusterRequest = createK8SClusterRequest(userToken);
        Response creationResponse = HTTP_CLIENT.newCall(createClusterRequest).execute();
        assertEquals(HttpStatus.SC_FORBIDDEN, creationResponse.code(), "The creation should not be allowed.");
    }

    private static Request createK8SClusterRequest(String accessToken) throws IOException {
        Map clusterEntity = Map.of("type", "K8SCluster",
                "id", "urn:ngsi-ld:K8SCluster:fancy-marketplace",
                "name", Map.of("type", "Property", "value", "Fancy Marketplace Cluster"),
                "numNodes", Map.of("type", "Property", "value", "3"),
                "k8sVersion", Map.of("type", "Property", "value", "1.26.0"));
        RequestBody clusterCreationBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(clusterEntity));
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
        RequestBody policyBody = RequestBody.create(MediaType.parse("application/json"), getPolicy(policy));
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
        ProductSpecificationCreateVO pscVo = new ProductSpecificationCreateVO()
                .brand("M&P Operations")
                .version("1.0.0")
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S");
        RequestBody specificationRequestBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(pscVo));
        Request specificationRequest = new Request.Builder()
                .post(specificationRequestBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
                .build();
        Response specificationResponse = HTTP_CLIENT.newCall(specificationRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, specificationResponse.code(), "The specification should have been created.");
        ProductSpecificationVO createdSpec = OBJECT_MAPPER.readValue(specificationResponse.body().string(), ProductSpecificationVO.class);

        ProductOfferingCreateVO productOfferingCreate = new ProductOfferingCreateVO()
                .lifecycleStatus("ACTIVE")
                .name("M&P K8S Offering")
                .version("1.0.0")
                .productSpecification(new ProductSpecificationRefVO().id(createdSpec.getId()));
        RequestBody productOfferingBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(productOfferingCreate));
        Request productOfferingRequest = new Request.Builder()
                .post(productOfferingBody)
                .url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .build();
        Response productOfferingResponse = HTTP_CLIENT.newCall(productOfferingRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, productOfferingResponse.code(), "The offering should have been created.");
    }

    @When("Fancy Marketplace registers itself at M&P Operations.")
    public void registerAtMP() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE);

        CharacteristicVO didCharacteristic = new CharacteristicVO()
                .name("did")
                .value(getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS));

        OrganizationCreateVO organizationCreateVO = new OrganizationCreateVO()
                .organizationType("Consumer")
                .name("Fancy Marketplace Inc.")
                .partyCharacteristic(List.of(didCharacteristic));

        RequestBody organizationCreateBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(organizationCreateVO));
        Request organizationCreateRequest = new Request.Builder()
                .post(organizationCreateBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/party/v4/organization")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response organizationCreateResponse = HTTP_CLIENT.newCall(organizationCreateRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, organizationCreateResponse.code(), "The organization should have been created.");
        fancyMarketplaceRegistration = OBJECT_MAPPER.readValue(organizationCreateResponse.body().string(), OrganizationVO.class);
    }

    @When("Fancy Marketplace buys access to M&P's k8s services.")
    public void buyAccess() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE);
        Request offerRequest = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute();
        assertEquals(HttpStatus.SC_OK, offerResponse.code(), "The offer should have been returend");
        List<ProductOfferingVO> offers = OBJECT_MAPPER.readValue(offerResponse.body().string(), new TypeReference<List<ProductOfferingVO>>() {
        });
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
        RequestBody pocBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(poc));
        Request pocRequest = new Request.Builder()
                .post(pocBody)
                .url(MPOperationsEnvironment.TM_FORUM_API_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
                .addHeader("Authorization", "Bearer " + accessToken)
                .build();
        Response pocResponse = HTTP_CLIENT.newCall(pocRequest).execute();
        assertEquals(HttpStatus.SC_CREATED, pocResponse.code(), "The product ordering should have been created.");
    }

    @When("M&P Operations registers a policy to allow every participant access to its energy reports.")
    public void mpRegisterEnergyReportPolicy() throws Exception {
        createPolicyAtMP("energyReport");
    }

    @When("M&P Operations allows operators to create clusters.")
    public void mpRegisterClusterCreatePolicy() throws Exception {
        createPolicyAtMP("clusterCreate");
    }

    @When("M&P Operations creates an energy report.")
    public void createEnergyReport() throws Exception {
        Map offerEntity = Map.of("type", "EnergyReport",
                "id", "urn:ngsi-ld:EnergyReport:fms-1",
                "name", Map.of("type", "Property", "value", "Standard Server"),
                "consumption", Map.of("type", "Property", "value", "94"));
        RequestBody requestBody = RequestBody.create(MediaType.parse("application/json"), OBJECT_MAPPER.writeValueAsString(offerEntity));

        Request creationRequest = new Request.Builder()
                .url(MPOperationsEnvironment.SCORPIO_ADDRESS + "/ngsi-ld/v1/entities")
                .post(requestBody)
                .build();
        assertEquals(HttpStatus.SC_CREATED, HTTP_CLIENT.newCall(creationRequest).execute().code(), "The entity should have been created.");
        createdEntities.add("urn:ngsi-ld:EnergyReport:fms-1");
    }

    @When("Fancy Marketplace issues a user credential to its employee.")
    public void issueUserCredentialToEmployee() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak();
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
    }


    @When("Fancy Marketplace issues an operator credential to its employee.")
    public void issueOperatorCredentialToEmployee() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak();
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, OPERATOR_CREDENTIAL);
    }

    @Then("Fancy Marketplace operators can create clusters.")
    public void createK8SCluster() throws Exception {
        Awaitility.await().atMost(Duration.ofSeconds(60)).until(() -> {
            try {
                String accessToken = getAccessTokenForFancyMarketplace(OPERATOR_CREDENTIAL, OPERATOR_SCOPE);
                Request creationRequest = createK8SClusterRequest(accessToken);
                assertEquals(HttpStatus.SC_CREATED, HTTP_CLIENT.newCall(creationRequest).execute().code(), "The cluster should now have been created.");
                return true;
            } catch (Throwable t) {
                log.info("No token: {}", t);
                return false;
            }
        });

        createdEntities.add("urn:ngsi-ld:K8SCluster:fancy-marketplace");
    }

    @Then("Fancy Marketplace' employee can access the EnergyReport.")
    public void accessTheEnergyReport() throws Exception {
        String accessToken = getAccessTokenForFancyMarketplace(USER_CREDENTIAL, DEFAULT_SCOPE);
        Request authenticatedEntityRequest = new Request.Builder().get()
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .build();

        Awaitility.await()
                .atMost(Duration.ofSeconds(60))
                .until(() -> HttpStatus.SC_OK == HTTP_CLIENT.newCall(authenticatedEntityRequest).execute().code());
    }

    private String getAccessTokenForFancyMarketplace(String credentialId, String scope) throws Exception {
        OpenIdConfiguration openIdConfiguration = MPOperationsEnvironment.getOpenIDConfiguration(MPOperationsEnvironment.PROVIDER_API_ADDRESS);
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

}
