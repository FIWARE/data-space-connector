package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import io.cucumber.java.After;
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
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.Security;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;

import static org.fiware.dataspace.it.components.ConsentEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.CONSUMER_DID;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.TEST_USER_NAME;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_API_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_DID;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_PAP_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.SCORPIO_ADDRESS;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.TMF_DIRECT_ADDRESS;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Cucumber step definitions for the consent-gated data access described in CONSENT_MANAGEMENT.md.
 *
 * <p>The flow under test is the one a data subject actually drives: the subject publishes personal
 * data at the provider, a consumer is denied access until the subject grants consent, and access
 * follows the consent from then on. Nothing is written into the consent-manager's database - the
 * consent is recorded through the real give-consent API, as a subject would.
 *
 * <p>The subject and the requesting consumer share one {@link Wallet}, mirroring the walkthrough:
 * its {@code did:key} is both the {@code dataOwner} of the published entity and the identity the
 * access tokens are issued to. Because that key is generated per run, every run works on a subject
 * the consent-manager has never seen, which is what keeps the scenarios independent of each other and
 * of earlier runs.
 *
 * @see ConsentEnvironment
 */
@Slf4j
public class ConsentStepDefinitions extends StepDefintions {

    /**
     * The credential the subject presents - both to the provider (to read data) and to the authority
     * verifier (to grant consent).
     */
    private static final String USER_CREDENTIAL = "user-credential";

    /**
     * Scope for the data access token at the provider.
     */
    private static final String DEFAULT_SCOPE = "default";

    /**
     * Scope for the subject's token at the authority verifier's consent-manager service.
     */
    private static final String OPENID_SCOPE = "openid";

    /**
     * Id of the OPA policy that permits reading personal profiles. It carries no consent refinement
     * on purpose: OPA authorizes on the credential alone so that the consent-filter plugin is the
     * component deciding on consent.
     */
    private static final String PERSONAL_PROFILE_READ_POLICY_ID =
            "https://mp-operation.org/policy/common/personalProfileRead";

    /**
     * The processing purpose declared on the product specification. Nothing can derive it - it is a
     * legal declaration by the provider, and the privacy notice is built from it.
     */
    private static final String PURPOSE_ID = "profile-service-provision";
    private static final String PURPOSE_URI = "https://w3id.org/dpv#ServiceProvision";

    /**
     * How long the policy needs to reach OPA, and a projected notice the consent-manager.
     */
    private static final Duration PROPAGATION_TIMEOUT = Duration.ofSeconds(30);

    /**
     * How long the plugin's decision may take to follow a changed consent.
     */
    private static final Duration DECISION_TIMEOUT = Duration.ofSeconds(30);

    /**
     * The event a withdrawal adds to the consent's lifecycle log in the receipt.
     */
    private static final String EVENT_STATE_REVOKED = "consent revoked";

    private static final okhttp3.MediaType JSON = okhttp3.MediaType.parse(MediaType.APPLICATION_JSON);

    /**
     * Reads the entity through the consent-enforced host.
     *
     * <p>The {@code Accept} header is explicit on purpose. The OwnerResolver reads the data owner at
     * {@code /dataOwner/value}, which only exists in the concise NGSI-LD representation - asking
     * without an {@code Accept} lets the broker answer in expanded JSON-LD, where the attribute is a
     * full URI and the resolver rightly reports no owner. Every request the gate sees must be one
     * whose shape the deployment's owner pointer describes.
     */
    private Response readPersonalProfile(String accessToken) throws IOException {
        Request request = new Request.Builder()
                .get()
                .url(CONSENT_ENFORCED_DATA_ADDRESS + "/ngsi-ld/v1/entities/" + PERSONAL_PROFILE_ENTITY_ID)
                .header("Authorization", "Bearer " + accessToken)
                .header("Accept", MediaType.APPLICATION_JSON)
                .build();
        return HTTP_CLIENT.newCall(request).execute();
    }

    private static final OkHttpClient DIRECT_CLIENT = TestUtils.DIRECT_HTTP_CLIENT;

    /**
     * The wallet acting as the data subject and as the requesting consumer.
     */
    private Wallet subjectWallet;

    private final List<KubernetesHelper.PortForward> portForwards = new ArrayList<>();

    private String subjectDid;
    private String providerOrganizationId;
    private String consumerOrganizationId;
    private String providerSelfDescription;
    private String consumerSelfDescription;
    private String providerToken;
    private String subjectToken;
    private String userIdentifier;
    private String privacyNoticeId;
    private ArrayNode noticeData;
    private String consentId;

    @Before("@consent")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        subjectWallet = new Wallet();
        subjectDid = subjectWallet.getDid();
        log.info("The data subject of this scenario is {}.", subjectDid);
        portForwards.add(KubernetesHelper.portForward(
                TRUST_ANCHOR_NAMESPACE, CONSENT_MANAGER_SERVICE, CONSENT_MANAGER_LOCAL_PORT, CONSENT_MANAGER_PORT));
        portForwards.add(KubernetesHelper.portForward(
                PROVIDER_NAMESPACE, CONSENT_FACADE_SERVICE, CONSENT_FACADE_LOCAL_PORT, CONSENT_FACADE_PORT));
    }

    @After("@consent")
    public void tearDown() {
        portForwards.forEach(KubernetesHelper.PortForward::close);
        portForwards.clear();
    }

    // --- Given -------------------------------------------------------------------------------

    @Given("The provider allows reading personal profiles at OPA.")
    public void allowPersonalProfileRead() throws Exception {
        deletePolicyIfPresent(PERSONAL_PROFILE_READ_POLICY_ID);
        String policy = """
                {
                  "@context": { "odrl": "http://www.w3.org/ns/odrl/2/" },
                  "@id": "%1$s",
                  "odrl:uid": "%1$s",
                  "@type": "odrl:Policy",
                  "odrl:permission": {
                    "odrl:assigner": { "@id": "https://www.mp-operation.org/" },
                    "odrl:target": {
                      "@type": "odrl:AssetCollection",
                      "odrl:source": "urn:asset",
                      "odrl:refinement": [
                        { "@type": "odrl:Constraint", "odrl:leftOperand": "ngsi-ld:entityType",
                          "odrl:operator": { "@id": "odrl:eq" }, "odrl:rightOperand": "%2$s" }
                      ]
                    },
                    "odrl:assignee": { "@id": "vc:any" },
                    "odrl:action": { "@id": "odrl:read" }
                  }
                }
                """.formatted(PERSONAL_PROFILE_READ_POLICY_ID, PERSONAL_PROFILE_ENTITY_TYPE);
        try (Response response = post(HTTP_CLIENT, PROVIDER_PAP_ADDRESS + "/policy", policy)) {
            assertTrue(response.isSuccessful(),
                    "The read policy should have been registered at the PAP, but was " + response.code());
        }
    }

    @Given("The data subject published a personal profile it owns.")
    public void publishPersonalProfile() throws Exception {
        deleteEntityIfPresent(PERSONAL_PROFILE_ENTITY_ID);
        // the dataOwner is what the OwnerResolver reads to decide whose data this is, so the consent
        // gate is bound to the subject and not to whoever requests it
        String entity = """
                {
                  "id": "%s",
                  "type": "%s",
                  "dataOwner": { "type": "Property", "value": "%s" },
                  "email": { "type": "Property", "value": "alice@example.org" },
                  "loyaltyPoints": { "type": "Property", "value": 4200 }
                }
                """.formatted(PERSONAL_PROFILE_ENTITY_ID, PERSONAL_PROFILE_ENTITY_TYPE, subjectDid);
        try (Response response = post(HTTP_CLIENT, SCORPIO_ADDRESS + "/ngsi-ld/v1/entities", entity)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The personal profile should have been created.");
        }
    }

    @Given("Both participants are onboarded at the consent-manager.")
    public void onboardParticipants() throws Exception {
        providerOrganizationId = findOrCreateOrganization(PROVIDER_ORGANIZATION_NAME, PROVIDER_DID);
        consumerOrganizationId = findOrCreateOrganization(CONSUMER_ORGANIZATION_NAME, CONSUMER_DID);
        providerSelfDescription = CONSENT_FACADE_ADDRESS + "/participants/" + providerOrganizationId;
        consumerSelfDescription = CONSENT_FACADE_ADDRESS + "/participants/" + consumerOrganizationId;

        // onboarding is an authority action on an unauthenticated endpoint, so it goes directly to the
        // consent-manager instead of through the participant-authenticated facade
        registerParticipant("M&P Operations Inc.", "provider@mp-operation.org", PROVIDER_DID,
                "consent-it-provider", providerSelfDescription);
        registerParticipant("Fancy Marketplace Co.", "consumer@fancy-marketplace.biz", CONSUMER_DID,
                "consent-it-consumer", consumerSelfDescription);

        providerToken = fetchProviderToken();
        try (Response response = get(HTTP_CLIENT, CONSENT_MANAGER_FACADE_ADDRESS + "/participants/me", providerToken)) {
            assertEquals(HttpStatus.SC_OK, response.code(), "The provider token should be accepted by the facade.");
            String storedSelfDescription = OBJECT_MAPPER.readTree(response.body().string())
                    .path("selfDescriptionURL").asText();
            assertEquals(providerSelfDescription, storedSelfDescription,
                    "The stored self description must match the one the agreement is written with - "
                            + "the consent-manager matches participants on that exact string.");
        }
    }

    @Given("A signed agreement between the participants covers the personal profile.")
    public void seedAgreement() throws Exception {
        // a real deployment gets this from the marketplace or an EDC negotiation; the facade only
        // projects it into the privacy notice the subject consents to
        cleanUpAgreementsOfPair();
        String specificationId = createProductSpecification();
        String offeringId = createProductOffering(specificationId);
        String agreement = """
                {
                  "name": "Consent IT profile sharing agreement",
                  "status": "approved",
                  "agreementItem": [ { "productOffering": [ { "id": "%s" } ] } ],
                  "engagedParty": [ { "id": "%s", "role": "Provider" }, { "id": "%s", "role": "Consumer" } ],
                  "characteristic": [
                    { "name": "policy", "value": { "@type": "Set", "uid": "urn:policy:consent-it",
                        "permission": [ { "target": "%s", "action": "use" } ] } },
                    { "name": "provider-id", "value": "%s" },
                    { "name": "consumer-id", "value": "%s" },
                    { "name": "signing-date", "value": %d } ]
                }
                """.formatted(offeringId, providerOrganizationId, consumerOrganizationId,
                PERSONAL_PROFILE_ENTITY_ID, providerSelfDescription, consumerSelfDescription,
                System.currentTimeMillis() / 1000);
        try (Response response = post(HTTP_CLIENT, agreementApi(), agreement)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The agreement should have been created.");
        }
    }

    @Given("The data subject is registered at the provider and has a PDI account.")
    public void registerSubject() throws Exception {
        // the provider registers its data subject - a repeated registration is a no-op
        String registration = """
                { "email": "%s", "identifier": "%s" }
                """.formatted(subjectDid, subjectDid);
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_FACADE_ADDRESS + "/users/register",
                registration, providerToken)) {
            assertTrue(response.isSuccessful(),
                    "The subject should have been registered at the provider, but was " + response.code());
        }

        userIdentifier = searchUserIdentifier();

        // the subject creates its own PDI account; the account e-mail is the holder DID, the same
        // value the UserIdentifier carries
        String signup = """
                { "firstName": "Alice", "lastName": "Subject", "email": "%s", "password": "consent-it-password" }
                """.formatted(subjectDid);
        String subjectUserId;
        try (Response response = post(HTTP_CLIENT, CONSENT_USER_ADDRESS + "/users/signup", signup)) {
            assertTrue(response.isSuccessful(),
                    "The subject should have been able to sign up, but was " + response.code());
            subjectUserId = OBJECT_MAPPER.readTree(response.body().string()).path("user").path("_id").asText();
        }
        assertFalse(subjectUserId.isEmpty(), "The signup should have returned the subject's user id.");

        // attaching the provider-side identifier is what makes the subject's own token resolve to
        // that account; signup alone does not link identifiers
        String attach = """
                { "userId": "%s", "userIdentifiers": ["%s"] }
                """.formatted(subjectUserId, userIdentifier);
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_FACADE_ADDRESS + "/users/identifier",
                attach, providerToken)) {
            assertTrue(response.isSuccessful(),
                    "The identifier should have been attached to the account, but was " + response.code());
        }
    }

    @Given("The data subject granted consent for its own data.")
    public void subjectGrantedConsent() throws Exception {
        grantConsent();
    }

    // --- When --------------------------------------------------------------------------------

    @When("The consumer requests the personal profile.")
    public void requestPersonalProfile() throws Exception {
        try (Response response = readPersonalProfile(dataAccessToken())) {
            log.info("Reading {} through the consent-enforced host answered {}.",
                    PERSONAL_PROFILE_ENTITY_ID, response.code());
        }
    }

    @When("The data subject grants consent for its own data.")
    public void grantConsent() throws Exception {
        subjectToken = subjectToken();
        fetchPrivacyNotice();

        String grant = """
                { "privacyNoticeId": "%s", "event": "given", "data": %s }
                """.formatted(privacyNoticeId, OBJECT_MAPPER.writeValueAsString(noticeData));
        try (Response response = post(HTTP_CLIENT, CONSENT_USER_ADDRESS + "/consents/user", grant, subjectToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_CREATED, response.code(),
                    "The subject should have been able to grant the consent, but got " + body);
            consentId = OBJECT_MAPPER.readTree(body).path("record").path("recordId").asText();
        }
        assertFalse(consentId.isEmpty(), "The grant should have returned a consent record id.");
        log.info("The subject granted the consent {}.", consentId);
    }

    @When("The data subject withdraws its consent.")
    public void withdrawConsent() throws Exception {
        try (Response response = delete(HTTP_CLIENT, CONSENT_USER_ADDRESS + "/consents/" + consentId, subjectToken)) {
            String body = response.body().string();
            assertTrue(response.isSuccessful(), "The subject should have been able to revoke, but got " + body);
            // the answer is the ISO 27560 receipt, where a withdrawal is an event in the consent's
            // lifecycle log rather than a status field
            assertTrue(OBJECT_MAPPER.readTree(body).findValuesAsText("eventState").contains(EVENT_STATE_REVOKED),
                    "The receipt should log the withdrawal: " + body);
        }
    }

    // --- Then --------------------------------------------------------------------------------

    @Then("The consumer is denied access to the personal profile.")
    public void accessIsDenied() {
        awaitDataAccess(HttpStatus.SC_FORBIDDEN,
                "Without a granted consent the consent-filter plugin should deny the read.");
    }

    @Then("The consumer can read the personal profile.")
    public void accessIsAllowed() {
        awaitDataAccess(HttpStatus.SC_OK,
                "With a granted consent the plugin should let the identical request through.");
    }

    // --- the flow's building blocks -----------------------------------------------------------

    /**
     * Reads the entity through the consent-enforced host until the expected decision is observed.
     *
     * <p>Both gates are asynchronous - the OPA policy has to reach the enforcement point and the
     * plugin's view of the consent has to catch up - so the decision is awaited rather than sampled
     * once. The token is re-issued on every attempt because it is short-lived.
     */
    private void awaitDataAccess(int expectedStatus, String message) {
        Awaitility.await(message)
                .atMost(DECISION_TIMEOUT)
                .pollInterval(Duration.ofSeconds(2))
                .untilAsserted(() -> {
                    try (Response response = readPersonalProfile(dataAccessToken())) {
                        assertEquals(expectedStatus, response.code(), message);
                    }
                });
    }

    /**
     * Issues the subject's credential into its wallet and exchanges it for an access token at the
     * authority verifier's consent-manager service. Its {@code sub} is the holder DID, which the
     * consent-manager maps to the subject's account.
     */
    private String subjectToken() throws Exception {
        ensureSubjectCredential();
        return ScriptHelper.getAccessTokenViaOid4vp(
                CONSENT_MANAGER_VERIFIER_ADDRESS, USER_CREDENTIAL, OPENID_SCOPE, subjectWallet);
    }

    /**
     * Issues the subject's credential into its wallet, once per scenario.
     *
     * <p>The same credential serves both roles the wallet plays: it is presented to the provider to
     * read the data and to the authority verifier to grant the consent.
     */
    private void ensureSubjectCredential() throws Exception {
        if (subjectWallet.getStoredCredential(USER_CREDENTIAL) != null) {
            return;
        }
        String keycloakToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(TEST_USER_NAME);
        subjectWallet.getCredentialFromIssuer(
                keycloakToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, USER_CREDENTIAL);
    }

    /**
     * An access token for the provider's data service, presenting the subject's credential. It is
     * short-lived, so it is fetched per attempt rather than kept.
     */
    private String dataAccessToken() throws Exception {
        ensureSubjectCredential();
        return ScriptHelper.getAccessTokenViaOid4vp(
                PROVIDER_API_ADDRESS, USER_CREDENTIAL, DEFAULT_SCOPE, subjectWallet);
    }

    /**
     * Fetches the notice the facade projected from the agreement - the offer the subject consents to.
     *
     * <p>Projection has to read the agreement, resolve both participants and the offering's
     * specification, so it is awaited: an empty {@code data} or {@code purposes} means the projection
     * has not caught up (or the agreement is not consent-ready).
     */
    private void fetchPrivacyNotice() {
        String providerKey = base64(providerSelfDescription);
        String consumerKey = base64(consumerSelfDescription);
        String noticeUrl = String.format("%s/consents/%s/%s/%s",
                CONSENT_MANAGER_FACADE_ADDRESS, subjectDid, providerKey, consumerKey);

        Awaitility.await("The privacy notice should have been projected from the agreement.")
                .atMost(PROPAGATION_TIMEOUT)
                .pollInterval(Duration.ofSeconds(2))
                .untilAsserted(() -> {
                    Request request = new Request.Builder()
                            .get()
                            .url(noticeUrl)
                            .header("Authorization", "Bearer " + providerToken)
                            .header("x-user-key", userIdentifier)
                            .build();
                    try (Response response = HTTP_CLIENT.newCall(request).execute()) {
                        String body = response.body().string();
                        assertEquals(HttpStatus.SC_OK, response.code(), "The notice should be readable: " + body);
                        JsonNode notices = OBJECT_MAPPER.readTree(body);
                        assertTrue(notices.isArray() && !notices.isEmpty(),
                                "A notice should have been projected: " + body);
                        JsonNode notice = notices.get(0);
                        assertFalse(notice.path("data").isEmpty(), "The notice needs a data resource: " + body);
                        assertFalse(notice.path("purposes").isEmpty(), "The notice needs a purpose: " + body);
                        privacyNoticeId = notice.path("_id").asText();
                        ArrayNode resources = OBJECT_MAPPER.createArrayNode();
                        notice.path("data").forEach(data -> resources.add(data.path("resource").asText()));
                        noticeData = resources;
                    }
                });
        assertNotNull(privacyNoticeId, "The notice id is needed to grant the consent.");
    }

    /**
     * Mints the provider's participant token at its own consent-facade - asked for exactly the way
     * the consent-filter plugin asks for it.
     */
    private String fetchProviderToken() throws Exception {
        String tokenUrl = "http://localhost:" + CONSENT_FACADE_LOCAL_PORT + INTERNAL_TOKENS_PATH;
        String body = """
                { "audience": "%s" }
                """.formatted(CONSENT_MANAGER_AUDIENCE);
        try (Response response = post(DIRECT_CLIENT, tokenUrl, body)) {
            String responseBody = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(),
                    "The facade should have minted a participant token: " + responseBody);
            String token = OBJECT_MAPPER.readTree(responseBody).path("access_token").asText();
            assertFalse(token.isEmpty(), "The token service should have returned an access token.");
            return token;
        }
    }

    /**
     * The lookup the consent-filter plugin performs on every request: it selects the subject at the
     * provider. An empty result means the registration has not propagated yet.
     */
    private String searchUserIdentifier() {
        String searchBody = """
                { "selfDescription": "%s", "email": "%s" }
                """.formatted(providerSelfDescription, subjectDid);
        List<String> resolved = new ArrayList<>();
        Awaitility.await("The subject should be resolvable at the provider.")
                .atMost(PROPAGATION_TIMEOUT)
                .pollInterval(Duration.ofSeconds(2))
                .untilAsserted(() -> {
                    try (Response response = post(HTTP_CLIENT,
                            CONSENT_MANAGER_FACADE_ADDRESS + "/users/identifier/search", searchBody, providerToken)) {
                        String body = response.body().string();
                        assertEquals(HttpStatus.SC_OK, response.code(), "The search should succeed: " + body);
                        JsonNode result = OBJECT_MAPPER.readTree(body);
                        assertTrue(result.path("userIdentifierExists").asBoolean(),
                                "The subject's identifier should exist: " + body);
                        String identifier = result.path("userIdentifier").asText();
                        assertFalse(identifier.isEmpty(), "The identifier should not be empty: " + body);
                        resolved.clear();
                        resolved.add(identifier);
                    }
                });
        return resolved.get(0);
    }

    private void registerParticipant(String legalName, String email, String did, String clientId,
                                     String selfDescription) throws Exception {
        // the clientID/clientSecret are required by the participant schema; nothing in this flow uses them
        String participant = """
                { "legalName": "%s", "email": "%s", "did": "%s",
                  "clientID": "%s", "clientSecret": "consent-it",
                  "selfDescriptionURL": "%s" }
                """.formatted(legalName, email, did, clientId, selfDescription);
        String url = "http://localhost:" + CONSENT_MANAGER_LOCAL_PORT + "/v1/participants";
        try (Response response = post(DIRECT_CLIENT, url, participant)) {
            // 201 = onboarded now, 409 = onboarded by an earlier run - both are fine
            assertTrue(response.isSuccessful() || response.code() == HttpStatus.SC_CONFLICT,
                    "The participant should be onboarded, but was " + response.code() + ": " + response.body().string());
        }
    }

    private String createProductSpecification() throws Exception {
        // the purpose characteristic is the provider's declaration of what the data is processed for;
        // the notice's purpose is built from it and a missing one silently becomes the product's name
        String specification = """
                {
                  "name": "Personal Profile",
                  "description": "The subject's profile",
                  "productSpecCharacteristic": [
                    { "name": "purpose", "valueType": "object",
                      "productSpecCharacteristicValue": [ { "value": {
                        "id": "%s",
                        "name": "Personal profile for service provision",
                        "description": "Deliver the requested service.",
                        "purpose": "%s" } } ] } ]
                }
                """.formatted(PURPOSE_ID, PURPOSE_URI);
        try (Response response = post(HTTP_CLIENT,
                TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productSpecification", specification)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The specification should have been created.");
            return OBJECT_MAPPER.readTree(response.body().string()).path("id").asText();
        }
    }

    private String createProductOffering(String specificationId) throws Exception {
        String offering = """
                { "name": "Personal Profile Offering", "productSpecification": { "id": "%s" } }
                """.formatted(specificationId);
        try (Response response = post(HTTP_CLIENT,
                TMF_DIRECT_ADDRESS + "/tmf-api/productCatalogManagement/v4/productOffering", offering)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The offering should have been created.");
            return OBJECT_MAPPER.readTree(response.body().string()).path("id").asText();
        }
    }

    /**
     * Finds the TM Forum organization backing a participant, or creates it.
     *
     * <p>Find-or-create keeps the self-description URLs stable across runs. That matters: a
     * participant stays pinned to the self-description it was onboarded with, so a new organization
     * would mismatch the agreement and the notice projection would find nothing.
     */
    private String findOrCreateOrganization(String name, String did) throws Exception {
        String organizationApi = TMF_DIRECT_ADDRESS + "/tmf-api/party/v4/organization";
        try (Response response = get(HTTP_CLIENT, organizationApi + "?limit=1000", null)) {
            assertTrue(response.isSuccessful(), "The organizations should be listable.");
            for (JsonNode organization : OBJECT_MAPPER.readTree(response.body().string())) {
                if (name.equals(organization.path("name").asText())) {
                    return organization.path("id").asText();
                }
            }
        }
        String organization = """
                { "name": "%1$s", "tradingName": "%1$s", "isLegalEntity": true,
                  "organizationType": "company",
                  "contactMedium": [ { "characteristic": { "country": "DE" } } ],
                  "partyCharacteristic": [ { "name": "did", "value": "%2$s" } ] }
                """.formatted(name, did);
        try (Response response = post(HTTP_CLIENT, organizationApi, organization)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The organization should have been created.");
            return OBJECT_MAPPER.readTree(response.body().string()).path("id").asText();
        }
    }

    /**
     * Removes the agreements of this scenario's participant pair.
     *
     * <p>The flow assumes a single agreement between the pair: the OwnerResolver takes the first
     * contract that permits the requested object, so a stale one from an earlier run can shadow the
     * one seeded here.
     */
    private void cleanUpAgreementsOfPair() throws Exception {
        try (Response response = get(HTTP_CLIENT, agreementApi() + "?limit=1000", null)) {
            if (!response.isSuccessful()) {
                return;
            }
            for (JsonNode agreement : OBJECT_MAPPER.readTree(response.body().string())) {
                boolean isOfPair = false;
                for (JsonNode characteristic : agreement.path("characteristic")) {
                    if ("provider-id".equals(characteristic.path("name").asText())
                            && providerSelfDescription.equals(characteristic.path("value").asText())) {
                        isOfPair = true;
                    }
                }
                if (isOfPair) {
                    String id = agreement.path("id").asText();
                    try (Response ignored = delete(HTTP_CLIENT, agreementApi() + "/" + id, null)) {
                        log.debug("Removed the stale agreement {}.", id);
                    }
                }
            }
        }
    }

    /**
     * Removes only this scenario's read policy, if an earlier run left it.
     *
     * <p>Deliberately targeted: the provider's PAP also holds the policy that authorizes the
     * consent-facade's cross-org TM Forum reads, and the notice projection depends on it - wiping the
     * PAP would break the flow this test is about.
     */
    private void deletePolicyIfPresent(String policyUid) throws Exception {
        try (Response response = get(HTTP_CLIENT, PROVIDER_PAP_ADDRESS + "/policy", null)) {
            if (!response.isSuccessful()) {
                return;
            }
            for (JsonNode policy : OBJECT_MAPPER.readTree(response.body().string())) {
                if (policyUid.equals(policy.path("odrl:uid").asText())) {
                    try (Response ignored = delete(HTTP_CLIENT,
                            PROVIDER_PAP_ADDRESS + "/policy/" + policy.path("id").asText(), null)) {
                        log.debug("Removed the read policy of a previous run.");
                    }
                }
            }
        }
    }

    private void deleteEntityIfPresent(String entityId) throws Exception {
        try (Response ignored = delete(HTTP_CLIENT, SCORPIO_ADDRESS + "/ngsi-ld/v1/entities/" + entityId, null)) {
            log.debug("Removed a previous {}.", entityId);
        }
    }

    private String agreementApi() {
        return TMF_DIRECT_ADDRESS + "/tmf-api/agreementManagement/v4/agreement";
    }

    private static String base64(String value) {
        return Base64.getEncoder().encodeToString(value.getBytes(StandardCharsets.UTF_8));
    }

    // --- plain http ---------------------------------------------------------------------------

    private Response post(OkHttpClient client, String url, String body) throws IOException {
        return post(client, url, body, null);
    }

    private Response post(OkHttpClient client, String url, String body, String token) throws IOException {
        Request.Builder request = new Request.Builder()
                .post(RequestBody.create(body, JSON))
                .url(url);
        if (token != null) {
            request.header("Authorization", "Bearer " + token);
        }
        return client.newCall(request.build()).execute();
    }

    private Response get(OkHttpClient client, String url, String token) throws IOException {
        Request.Builder request = new Request.Builder().get().url(url);
        if (token != null) {
            request.header("Authorization", "Bearer " + token);
        }
        return client.newCall(request.build()).execute();
    }

    private Response delete(OkHttpClient client, String url, String token) throws IOException {
        Request.Builder request = new Request.Builder().delete().url(url);
        if (token != null) {
            request.header("Authorization", "Bearer " + token);
        }
        return client.newCall(request.build()).execute();
    }
}
