package org.fiware.dataspace.it.components;

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
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.io.InputStream;
import java.security.Security;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class StepDefinitions {

    private static final OkHttpClient HTTP_CLIENT = new OkHttpClient();
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();
    private static final String USER_CREDENTIAL = "user-credential";
    private static final String GRANT_TYPE_VP_TOKEN = "vp_token";
    private static final String RESPONSE_TYPE_DIRECT_POST = "direct_post";

    private Wallet fancyMarketplaceEmployeeWallet;

    private List<String> createdPolicies = new ArrayList<>();
    private List<String> createdEntities = new ArrayList<>();

    @Before
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        fancyMarketplaceEmployeeWallet = new Wallet();
    }

    @After
    public void cleanUp() {
        cleanUpPolicies();
        cleanUpEntities();
    }

    private void cleanUpPolicies() {
        createdPolicies.forEach(policyId -> {
            Request deletionRequest = new Request.Builder()
                    .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy/" + policyId)
                    .delete()
                    .build();
            try {
                HTTP_CLIENT.newCall(deletionRequest).execute();
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
        assertEquals(HttpStatus.SC_OK, HTTP_CLIENT.newCall(didCheckRequest).execute().code(), "The did should be registered at the trust-anchor.");
    }


    @Given("Fancy Marketplace is registered as a participant in the data space.")
    public void checkFMRegistered() throws Exception {
        Request didCheckRequest = new Request.Builder()
                .url(TrustAnchorEnvironment.TIR_ADDRESS + "/v4/issuers/" + getDid(FancyMarketplaceEnvironment.DID_CONSUMER_ADDRESS))
                .build();
        assertEquals(HttpStatus.SC_OK, HTTP_CLIENT.newCall(didCheckRequest).execute().code(), "The did should be registered at the trust-anchor.");
    }

    @When("M&P Operations registers a policy to allow every participant access to its energy reports.")
    public void mpRegisterEnergyReportPolicy() throws Exception {
        RequestBody policyBody = RequestBody.create(MediaType.parse("application/json"), getPolicy("energyReport"));
        Request policyCreationRequest = new Request.Builder()
                .post(policyBody)
                .url(MPOperationsEnvironment.PROVIDER_PAP_ADDRESS + "/policy")
                .build();
        Response policyCreationResponse = HTTP_CLIENT.newCall(policyCreationRequest).execute();
        assertEquals(HttpStatus.SC_OK, policyCreationResponse.code(), "The policy should have been created.");
        createdPolicies.add(policyCreationResponse.header("Location"));
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

    @When("Fancy Marketplace issues a credential to its employee.")
    public void issueCredentialToEmployee() throws Exception {
        String accessToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak();
        fancyMarketplaceEmployeeWallet.getCredentialFromIssuer(accessToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
    }

    @Then("Fancy Marketplace' employee can access the EnergyReport.")
    public void accessTheEnergyReport() throws Exception {
        OpenIdConfiguration openIdConfiguration = MPOperationsEnvironment.getOpenIDConfiguration();
        assertTrue(openIdConfiguration.getGrantTypesSupported().contains(GRANT_TYPE_VP_TOKEN), "The M&P environment should support vp_tokens");
        assertTrue(openIdConfiguration.getResponseModeSupported().contains(RESPONSE_TYPE_DIRECT_POST), "The M&P environment should support direct_post");
        assertNotNull(openIdConfiguration.getTokenEndpoint(), "The M&P environment should provide a token endpoint.");

        String accessToken = fancyMarketplaceEmployeeWallet.exchangeCredentialForToken(openIdConfiguration, USER_CREDENTIAL);
        Request authenticatedEntityRequest = new Request.Builder().get()
                .url(MPOperationsEnvironment.PROVIDER_API_ADDRESS + "/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1")
                .addHeader("Authorization", "Bearer " + accessToken)
                .addHeader("Accept", "application/json")
                .build();

        Awaitility.await()
                .atMost(Duration.ofSeconds(60))
                .until(() -> {
                    return HttpStatus.SC_OK == HTTP_CLIENT.newCall(authenticatedEntityRequest).execute().code();
                });
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
