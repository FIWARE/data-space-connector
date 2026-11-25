package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.security.Security;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.fiware.dataspace.it.components.ReferenceEnvironment.*;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Steps of the reference profiles scenario: a trust anchor, a provider with its marketplace and a
 * consumer, deployed from charts/*&#47;examples. What the profiles keep internal (the trust
 * anchor's list API, the PAP, the broker) is reached the way their README tells a user to: with a
 * port-forward.
 */
@Slf4j
public class ReferenceStepDefinitions {

    private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
    private static final OkHttpClient DIRECT_HTTP_CLIENT = TestUtils.DIRECT_HTTP_CLIENT;
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();
    private static final okhttp3.MediaType JSON = okhttp3.MediaType.parse("application/json");

    private static final String USER_CREDENTIAL = "user-credential";
    private static final String DEFAULT_SCOPE = "default";
    private static final String ENERGY_REPORT_ID = "urn:ngsi-ld:EnergyReport:reference-1";
    /** Time for a new policy to reach OPA, and for the verifier to see a new issuer. */
    private static final Duration PROPAGATION_TIMEOUT = Duration.ofSeconds(60);

    private Wallet employeeWallet;
    private final List<String> createdPolicies = new ArrayList<>();

    @Before("@reference")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        employeeWallet = new Wallet();
    }

    @After("@reference")
    public void cleanUp() throws Exception {
        if (createdPolicies.isEmpty()) {
            return;
        }
        try (KubernetesHelper.PortForward pap = KubernetesHelper.portForward(
                PROVIDER_NAMESPACE, PAP_SERVICE, PAP_LOCAL_PORT, PAP_PORT)) {
            for (String policy : createdPolicies) {
                Request deletion = new Request.Builder().delete()
                        .url(localUrl(PAP_LOCAL_PORT, policy.startsWith("/") ? policy : "/policy/" + policy))
                        .build();
                try (Response response = DIRECT_HTTP_CLIENT.newCall(deletion).execute()) {
                    log.debug("Deleted policy {}: {}", policy, response.code());
                }
            }
        }
        createdPolicies.clear();
    }

    @Given("The trust anchor registers the provider and the consumer of the reference deployment.")
    public void registerParticipants() throws Exception {
        try (KubernetesHelper.PortForward til = KubernetesHelper.portForward(
                TRUST_ANCHOR_NAMESPACE, TRUST_ANCHOR_TIL_SERVICE, TRUST_ANCHOR_TIL_LOCAL_PORT, TRUST_ANCHOR_TIL_PORT)) {
            for (String did : List.of(PROVIDER_DID, CONSUMER_DID)) {
                RequestBody body = RequestBody.create(
                        OBJECT_MAPPER.writeValueAsString(Map.of("did", did, "credentials", List.of())), JSON);
                Request registration = new Request.Builder().post(body)
                        .url(localUrl(TRUST_ANCHOR_TIL_LOCAL_PORT, "/issuer"))
                        .build();
                try (Response response = DIRECT_HTTP_CLIENT.newCall(registration).execute()) {
                    // 409: already registered by an earlier run
                    assertTrue(response.code() == HttpStatus.SC_CREATED || response.code() == HttpStatus.SC_CONFLICT,
                            "The participant " + did + " should have been registered, got " + response.code());
                }
            }
        }
    }

    @Then("The trust anchor's registry lists the provider and the consumer.")
    public void participantsAreListed() {
        for (String did : List.of(PROVIDER_DID, CONSUMER_DID)) {
            Request request = new Request.Builder().get().url(TIR_ADDRESS + "/v4/issuers/" + did).build();
            Awaitility.await().atMost(PROPAGATION_TIMEOUT).ignoreExceptions().untilAsserted(() -> {
                try (Response response = HTTP_CLIENT.newCall(request).execute()) {
                    assertEquals(HttpStatus.SC_OK, response.code(), "The registry should list " + did);
                }
            });
        }
    }

    @When("The provider allows every participant to read its energy reports.")
    public void createEnergyReportPolicy() throws Exception {
        String policy;
        try (InputStream in = getClass().getResourceAsStream("/policies/energyReport.json")) {
            policy = new String(in.readAllBytes(), StandardCharsets.UTF_8);
        }
        try (KubernetesHelper.PortForward pap = KubernetesHelper.portForward(
                PROVIDER_NAMESPACE, PAP_SERVICE, PAP_LOCAL_PORT, PAP_PORT)) {
            Request creation = new Request.Builder().post(RequestBody.create(policy, JSON))
                    .url(localUrl(PAP_LOCAL_PORT, "/policy"))
                    .build();
            try (Response response = DIRECT_HTTP_CLIENT.newCall(creation).execute()) {
                assertEquals(HttpStatus.SC_OK, response.code(), "The policy should have been created.");
                String location = response.header("Location");
                if (location != null) {
                    createdPolicies.add(location);
                }
            }
        }
    }

    @When("The provider stores an energy report.")
    public void createEnergyReport() throws Exception {
        Map<String, Object> entity = Map.of(
                "id", ENERGY_REPORT_ID,
                "type", "EnergyReport",
                "consumption", Map.of("type", "Property", "value", "94"));
        try (KubernetesHelper.PortForward broker = KubernetesHelper.portForward(
                PROVIDER_NAMESPACE, SCORPIO_SERVICE, SCORPIO_LOCAL_PORT, SCORPIO_PORT)) {
            Request creation = new Request.Builder()
                    .post(RequestBody.create(OBJECT_MAPPER.writeValueAsString(entity), JSON))
                    .url(localUrl(SCORPIO_LOCAL_PORT, "/ngsi-ld/v1/entities"))
                    .build();
            try (Response response = DIRECT_HTTP_CLIENT.newCall(creation).execute()) {
                // 409: stored by an earlier run
                assertTrue(response.code() == HttpStatus.SC_CREATED || response.code() == HttpStatus.SC_CONFLICT,
                        "The entity should have been created, got " + response.code());
            }
        }
    }

    @When("The consumer issues a user credential to its employee.")
    public void issueUserCredential() throws Exception {
        String userToken = new KeycloakHelper(REALM, CONSUMER_KEYCLOAK_ADDRESS)
                .getUserToken(CONSUMER_USER, CONSUMER_USER_PASSWORD);
        employeeWallet.getCredentialFromIssuer(userToken, CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
        assertNotNull(employeeWallet.getStoredCredential(USER_CREDENTIAL), "The credential should have been issued.");
    }

    @Then("The employee reads the energy report through the provider's data service.")
    public void readEnergyReport() {
        Awaitility.await().atMost(PROPAGATION_TIMEOUT).ignoreExceptions().untilAsserted(() -> {
            OpenIdConfiguration configuration = MPOperationsEnvironment.getOpenIDConfiguration(DATA_SERVICE_ADDRESS);
            String accessToken = employeeWallet.exchangeCredentialForToken(configuration, USER_CREDENTIAL, DEFAULT_SCOPE);
            Request request = new Request.Builder().get()
                    .url(DATA_SERVICE_ADDRESS + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ID)
                    .addHeader("Authorization", "Bearer " + accessToken)
                    .addHeader("Accept", "application/json")
                    .build();
            try (Response response = HTTP_CLIENT.newCall(request).execute()) {
                assertEquals(HttpStatus.SC_OK, response.code(), "The energy report should be readable with the token.");
            }
        });
    }

    @Then("The provider's data service rejects a request without a token.")
    public void dataServiceRejectsAnonymous() throws Exception {
        assertUnauthorized(DATA_SERVICE_ADDRESS + "/ngsi-ld/v1/entities/" + ENERGY_REPORT_ID);
    }

    @Then("The provider's TM Forum API offers token-based access and rejects a request without a token.")
    public void tmForumApiIsProtected() throws Exception {
        OpenIdConfiguration configuration = MPOperationsEnvironment.getOpenIDConfiguration(TMF_ADDRESS);
        assertNotNull(configuration.getTokenEndpoint(), "The TM Forum API should name the verifier's token endpoint.");
        assertUnauthorized(TMF_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering");
    }

    @Then("The provider's marketplace portal is served.")
    public void marketplaceIsServed() {
        Request request = new Request.Builder().get().url(MARKETPLACE_ADDRESS + "/").build();
        Awaitility.await().atMost(PROPAGATION_TIMEOUT).ignoreExceptions().untilAsserted(() -> {
            try (Response response = HTTP_CLIENT.newCall(request).execute()) {
                assertEquals(HttpStatus.SC_OK, response.code(), "The marketplace portal should answer.");
            }
        });
    }

    private void assertUnauthorized(String url) throws Exception {
        Request request = new Request.Builder().get().url(url).addHeader("Accept", "application/json").build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_UNAUTHORIZED, response.code(), "A request without a token should be rejected: " + url);
        }
    }

    private static String localUrl(int port, String path) {
        return "http://127.0.0.1:" + port + path;
    }
}
