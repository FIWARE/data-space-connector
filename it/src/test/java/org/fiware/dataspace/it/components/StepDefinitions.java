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
import org.fiware.dataspace.it.components.model.*;
import org.fiware.dataspace.tmf.model.*;
import org.keycloak.common.crypto.CryptoIntegration;
import org.opentest4j.AssertionFailedError;

import java.io.IOException;
import java.io.InputStream;
import java.security.Security;
import java.time.Duration;
import java.util.*;

import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.OPERATOR_USER_NAME;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.TEST_USER_NAME;
import static org.junit.jupiter.api.Assertions.*;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class StepDefinitions {

	private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
	private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();
	private static final String USER_CREDENTIAL = "user-credential";
	private static final String OPERATOR_CREDENTIAL = "operator-credential";
	private static final String DEFAULT_SCOPE = "default";
	private static final String OPERATOR_SCOPE = "operator";
	private static final String GRANT_TYPE_VP_TOKEN = "vp_token";
	private static final String RESPONSE_TYPE_DIRECT_POST = "direct_post";

	private Wallet fancyMarketplaceEmployeeWallet;
	private OrganizationVO fancyMarketplaceRegistration;
	private String transferProcessId;
	private List<String> createdPolicies = new ArrayList<>();
	private List<String> createdEntities = new ArrayList<>();

	@Before
	public void setup() throws Exception {
		CryptoIntegration.init(this.getClass().getClassLoader());
		Security.addProvider(new BouncyCastleProvider());
		fancyMarketplaceEmployeeWallet = new Wallet();
		OBJECT_MAPPER.setSerializationInclusion(JsonInclude.Include.NON_EMPTY);

		clean();
	}

	@After
	public void cleanUp() throws Exception {
		clean();
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
		Response catalogsResponse = HTTP_CLIENT.newCall(catalogsRequest).execute();
		List<DcatCatalog> catalogs = OBJECT_MAPPER.readValue(catalogsResponse.body().string(), new TypeReference<List<DcatCatalog>>() {
		});

		for (DcatCatalog dcat : catalogs) {
			Request deleteRequest = new Request.Builder()
					.delete()
					.url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/catalogs/" + dcat.getId())
					.build();
			HTTP_CLIENT.newCall(deleteRequest).execute();
		}
	}

	private void cleanUpAgreements() throws Exception {
		Request agreementsRequest = new Request.Builder()
				.get()
				.url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements")
				.build();
		Response catalogsResponse = HTTP_CLIENT.newCall(agreementsRequest).execute();
		List<Agreement> agreements = OBJECT_MAPPER.readValue(catalogsResponse.body().string(), new TypeReference<List<Agreement>>() {
		});

		for (Agreement agreement : agreements) {
			Request deleteRequest = new Request.Builder()
					.delete()
					.url(MPOperationsEnvironment.RAINBOW_DIRECT_ADDRESS + "/api/v1/agreements/" + agreement.getAgreementId())
					.build();
			HTTP_CLIENT.newCall(deleteRequest).execute();
		}
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
		RequestBody tilUpdateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(tilConfig), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
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

		Request orderRequest = new Request.Builder()
				.get()
				.url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder")
				.build();
		Response orderResponse = HTTP_CLIENT.newCall(orderRequest).execute();
		assertEquals(HttpStatus.SC_OK, orderResponse.code(), "The spec should have been returend");
		List<ProductOrderVO> orders = OBJECT_MAPPER.readValue(orderResponse.body().string(), new TypeReference<List<ProductOrderVO>>() {
		});
		orders.stream()
				.map(ProductOrderVO::getId)
				.forEach(id -> {
					Request deletionRequest = new Request.Builder()
							.delete()
							.url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productOrderingManagement/v4/productOrder/" + id)
							.build();
					try {
						HTTP_CLIENT.newCall(deletionRequest).execute();
					} catch (IOException e) {
						// ignore
					}
				});
		orderResponse.body().close();

		Request agreementsRequest = new Request.Builder()
				.get()
				.url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification")
				.build();
		Response agreementsResponse = HTTP_CLIENT.newCall(agreementsRequest).execute();
		assertEquals(HttpStatus.SC_OK, agreementsResponse.code(), "The spec should have been returend");
		List<TMFAgreement> agreements = OBJECT_MAPPER.readValue(agreementsResponse.body().string(), new TypeReference<List<TMFAgreement>>() {
		});
		agreements.stream()
				.map(TMFAgreement::getId)
				.forEach(id -> {
					Request deletionRequest = new Request.Builder()
							.delete()
							.url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + "/tmf-api/agreementManagement/v4/agreement/" + id)
							.build();
					try {
						HTTP_CLIENT.newCall(deletionRequest).execute();
					} catch (IOException e) {
						// ignore
					}
				});
		orderResponse.body().close();

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
				String message = r.body().string();
				r.body().close();
				log.warn(message);
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
														.value("Endpoint of the K8S service.")))
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
														.value("Endpoint of the reporting service.")))
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
				.value(getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS));

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
		assertEquals(HttpStatus.SC_CREATED, organizationCreateResponse.code(), "The organization should have been created.");
		fancyMarketplaceRegistration = OBJECT_MAPPER.readValue(organizationCreateResponse.body().string(), OrganizationVO.class);
		organizationCreateResponse.body().close();
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
				assertEquals(HttpStatus.SC_CREATED, HTTP_CLIENT.newCall(creationRequest).execute().code(), "The cluster should now have been created.");
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
				.until(() -> HttpStatus.SC_OK == HTTP_CLIENT.newCall(authenticatedEntityRequest).execute().code());
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

}
