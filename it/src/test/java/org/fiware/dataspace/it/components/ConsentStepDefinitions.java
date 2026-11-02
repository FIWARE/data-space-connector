package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.HttpUrl;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.keycloak.common.crypto.CryptoIntegration;

import java.io.IOException;
import java.security.Security;
import java.time.Duration;
import java.util.ArrayList;
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
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Cucumber step definitions for the consent-gated data access described in CONSENT_MANAGEMENT.md.
 *
 * <p>The flow under test is the one a data subject actually drives: the subject publishes personal
 * data at the provider, a consumer is denied access until the subject grants consent, and access
 * follows the consent from then on. Nothing is written into the consent-manager's database - every
 * call goes through its {@code /v1} API with a token of the authority verifier, as the real parties
 * would make it.
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
     * Scope for a participant's token at the same service; it asks for the MembershipCredential.
     */
    private static final String PARTICIPANT_SCOPE = "participant";

    /**
     * The consumer's MembershipCredential, issued by its own keycloak. Its issuer is the consumer DID,
     * which the verifier puts into the participant-identifier claim.
     */
    private static final String MEMBERSHIP_CREDENTIAL = "membership-credential";

    /**
     * The consumer keycloak user the MembershipCredential is issued to.
     */
    private static final String MEMBERSHIP_CREDENTIAL_USERNAME = "employee";

    /**
     * Id of the OPA policy that permits reading operator profiles. It carries no consent refinement
     * on purpose: OPA authorizes on the credential alone so that the consent-filter plugin is the
     * component deciding on consent.
     */
    private static final String OPERATOR_PROFILE_READ_POLICY_ID =
            "https://mp-operation.org/policy/common/operatorProfileRead";

    /**
     * The processing purpose declared on the product specification. Nothing can derive it - it is a
     * legal declaration by the provider, and the privacy notice is built from it.
     */
    private static final String PURPOSE_ID = "profile-service-provision";
    private static final String PURPOSE_URI = "https://w3id.org/dpv#ServiceProvision";

    /**
     * How long the policy needs to reach OPA, and the agreement the consent-manager's notice
     * generation.
     */
    private static final Duration PROPAGATION_TIMEOUT = Duration.ofSeconds(30);

    /**
     * How long the plugin's decision may take to follow a changed consent.
     */
    private static final Duration DECISION_TIMEOUT = Duration.ofSeconds(30);

    /**
     * How long a denial has to hold before it counts as one. A single 403 can be a decision
     * that has not caught up with a fresh grant, so the withdrawal scenarios assert that the
     * deny persists rather than accepting the first one observed.
     */
    private static final Duration DENY_HOLDS_FOR = Duration.ofSeconds(10);

    /**
     * The decision the subject takes on the notice, and the status it leaves the consent in.
     */
    private static final String STATUS_GRANTED = "GRANTED";

    /**
     * The status a withdrawal leaves the consent in.
     */
    private static final String STATUS_REVOKED = "REVOKED";

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
    private Response readOperatorProfile(String accessToken) throws IOException {
        Request request = new Request.Builder()
                .get()
                .url(CONSENT_ENFORCED_DATA_ADDRESS + "/ngsi-ld/v1/entities/" + OPERATOR_PROFILE_ENTITY_ID)
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

    /**
     * The consumer organization's wallet, holding its MembershipCredential.
     */
    private Wallet consumerWallet;

    private final List<KubernetesHelper.PortForward> portForwards = new ArrayList<>();

    private String subjectDid;
    private String providerOrganizationId;
    private String consumerOrganizationId;
    private String providerSelfDescription;
    private String consumerSelfDescription;
    private String subjectToken;
    private String agreementId;
    private String privacyNoticeId;
    private String consentId;

    @Before("@consent")
    public void setup() throws Exception {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        subjectWallet = new Wallet();
        subjectDid = subjectWallet.getDid();
        consumerWallet = new Wallet();
        log.info("The data subject of this scenario is {}.", subjectDid);
        portForwards.add(KubernetesHelper.portForward(
                PROVIDER_NAMESPACE, CONSENT_FACADE_SERVICE, CONSENT_FACADE_LOCAL_PORT, CONSENT_FACADE_PORT));
    }

    @After("@consent")
    public void tearDown() {
        portForwards.forEach(KubernetesHelper.PortForward::close);
        portForwards.clear();
    }

    // --- Given -------------------------------------------------------------------------------

    @Given("The provider allows reading operator profiles at OPA.")
    public void allowOperatorProfileRead() throws Exception {
        deletePolicyIfPresent(OPERATOR_PROFILE_READ_POLICY_ID);
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
                """.formatted(OPERATOR_PROFILE_READ_POLICY_ID, OPERATOR_PROFILE_ENTITY_TYPE);
        try (Response response = post(HTTP_CLIENT, PROVIDER_PAP_ADDRESS + "/policy", policy)) {
            assertTrue(response.isSuccessful(),
                    "The read policy should have been registered at the PAP, but was " + response.code());
        }
    }

    @Given("The data subject published an operator profile it owns.")
    public void publishOperatorProfile() throws Exception {
        deleteEntityIfPresent(OPERATOR_PROFILE_ENTITY_ID);
        // the dataOwner is what the OwnerResolver reads to decide whose data this is, so the consent
        // gate is bound to the subject and not to whoever requests it
        String entity = """
                {
                  "id": "%s",
                  "type": "%s",
                  "dataOwner": { "type": "Property", "value": "%s" },
                  "email": { "type": "Property", "value": "mipa@example.org" },
                  "educationalHistory": { "type": "Property", "value": "B.Sc. Industrial Engineering (2018), Certified Kubernetes Administrator (2022)" }
                }
                """.formatted(OPERATOR_PROFILE_ENTITY_ID, OPERATOR_PROFILE_ENTITY_TYPE, subjectDid);
        try (Response response = post(HTTP_CLIENT, SCORPIO_ADDRESS + "/ngsi-ld/v1/entities", entity)) {
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The operator profile should have been created.");
        }
    }

    @Given("Both participants are onboarded at the consent-manager.")
    public void onboardParticipants() throws Exception {
        providerOrganizationId = findOrCreateOrganization(PROVIDER_ORGANIZATION_NAME, PROVIDER_DID);
        consumerOrganizationId = findOrCreateOrganization(CONSUMER_ORGANIZATION_NAME, CONSUMER_DID);
        providerSelfDescription = CONSENT_FACADE_ADDRESS + "/participants/" + providerOrganizationId;
        consumerSelfDescription = CONSENT_FACADE_ADDRESS + "/participants/" + consumerOrganizationId;

        // each participant registers itself: the identifier is the DID in its token, never the body
        registerParticipant(fetchProviderToken(), "M&P Operations Inc.", "provider@mp-operation.org",
                providerSelfDescription);
        registerParticipant(consumerParticipantToken(), "Fancy Marketplace Co.", "consumer@fancy-marketplace.biz",
                consumerSelfDescription);

        // the two lookups the consent-filter plugin makes: the provider by its own token, the
        // consumer in the directory - it denies when either misses or has no self-description
        String providerToken = fetchProviderToken();
        try (Response response = get(HTTP_CLIENT, CONSENT_MANAGER_API + "/participants/me", providerToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(), "The provider should be registered: " + body);
            JsonNode provider = OBJECT_MAPPER.readTree(body);
            assertEquals(PROVIDER_DID, provider.path("identifier").asText(), "The provider is known by its DID.");
            assertEquals(providerSelfDescription, provider.path("selfDescriptionUri").asText(),
                    "The provider should carry its self-description.");
        }
        HttpUrl consumerLookup = HttpUrl.get(CONSENT_MANAGER_API + "/participants").newBuilder()
                .addQueryParameter("identifier", CONSUMER_DID)
                .build();
        try (Response response = get(HTTP_CLIENT, consumerLookup.toString(), providerToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(), "The directory should be readable: " + body);
            JsonNode consumers = OBJECT_MAPPER.readTree(body).path("content");
            assertEquals(1, consumers.size(), "The consumer should be in the directory: " + body);
            assertEquals(consumerSelfDescription, consumers.get(0).path("selfDescriptionUri").asText(),
                    "The consumer should carry its self-description.");
        }
    }

    @Given("A signed agreement between the participants covers the operator profile.")
    public void seedAgreement() throws Exception {
        // a real deployment gets this from the marketplace or an EDC negotiation; the facade only
        // projects it into the contract the notice is generated from. provider-id/consumer-id are
        // passed through verbatim, and the consent-manager resolves the parties by participant
        // identifier, so they carry the participant DIDs and not self-description URLs
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
                OPERATOR_PROFILE_ENTITY_ID, PROVIDER_DID, CONSUMER_DID,
                System.currentTimeMillis() / 1000);
        try (Response response = post(HTTP_CLIENT, agreementApi(), agreement)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_CREATED, response.code(), "The agreement should have been created: " + body);
            // the id names the contract this scenario's notice is generated from
            agreementId = OBJECT_MAPPER.readTree(body).path("id").asText();
        }
        assertFalse(agreementId.isEmpty(), "The created agreement should have returned its id.");
    }

    @Given("The data subject is registered at the consent-manager and linked to the provider.")
    public void registerSubject() throws Exception {
        // the subject registers itself; the user identifier is the holder DID its token carries.
        // The operation takes no body, and answers 201 the first time and 200 after.
        subjectToken = subjectToken();
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/users/register", "", subjectToken)) {
            assertTrue(response.isSuccessful(),
                    "The subject should have been able to register, but got " + response.body().string());
        }

        // the provider links the subject to itself, as a provider registers its data subjects
        String link = """
                { "identifier": "%s" }
                """.formatted(subjectDid);
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/participants/me/users", link,
                fetchProviderToken())) {
            assertTrue(response.isSuccessful(),
                    "The provider should have linked the subject, but got " + response.body().string());
        }
    }

    @Given("The data subject granted consent for its own data.")
    public void subjectGrantedConsent() throws Exception {
        grantConsent();
    }

    // --- When --------------------------------------------------------------------------------

    @When("The consumer requests the operator profile.")
    public void requestOperatorProfile() throws Exception {
        try (Response response = readOperatorProfile(dataAccessToken())) {
            log.info("Reading {} through the consent-enforced host answered {}.",
                    OPERATOR_PROFILE_ENTITY_ID, response.code());
        }
    }

    @When("The data subject grants consent for its own data.")
    public void grantConsent() throws Exception {
        subjectToken = subjectToken();
        generatePrivacyNotice();

        // no purposes/data election: the consent covers the whole notice
        String grant = """
                { "privacyNoticeId": "%s", "decision": "%s" }
                """.formatted(privacyNoticeId, STATUS_GRANTED);
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/consents", grant, subjectToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_CREATED, response.code(),
                    "The subject should have been able to grant the consent, but got " + body);
            JsonNode consent = OBJECT_MAPPER.readTree(body);
            consentId = consent.path("id").asText();
            assertEquals(STATUS_GRANTED, consent.path("status").asText(), "The consent should be granted: " + body);
            // the plugin allows only a consent naming exactly this provider and consumer
            assertEquals(PROVIDER_DID, consent.path("provider").asText(), "The consent should name the provider.");
            assertEquals(CONSUMER_DID, consent.path("consumer").asText(), "The consent should name the consumer.");
        }
        assertFalse(consentId.isEmpty(), "The grant should have returned a consent id.");
        log.info("The subject granted the consent {}.", consentId);

        assertConsentInForce(true);
    }

    @When("The data subject withdraws its consent.")
    public void withdrawConsent() throws Exception {
        subjectToken = subjectToken();
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/consents/" + consentId + "/revoke", "",
                subjectToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(), "The subject should have been able to revoke: " + body);
            assertEquals(STATUS_REVOKED, OBJECT_MAPPER.readTree(body).path("status").asText(),
                    "The consent should be revoked: " + body);
        }

        assertConsentInForce(false);
    }

    // --- Then --------------------------------------------------------------------------------

    @Then("The consumer stays denied access to the operator profile.")
    public void accessStaysDenied() {
        // `during` rather than `until`: a single 403 could just be a decision that has not
        // caught up yet, which would let a broken withdrawal pass. The deny has to hold.
        Awaitility.await("The consent-filter plugin should keep denying the read.")
                .atMost(DENY_HOLDS_FOR.plus(DECISION_TIMEOUT))
                .during(DENY_HOLDS_FOR)
                .pollInterval(Duration.ofSeconds(2))
                .untilAsserted(() -> {
                    try (Response response = readOperatorProfile(dataAccessToken())) {
                        assertEquals(HttpStatus.SC_FORBIDDEN, response.code(),
                                "Without a granted consent the consent-filter plugin should deny the read.");
                    }
                });
    }

    @Then("The consumer can read the operator profile.")
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
                    try (Response response = readOperatorProfile(dataAccessToken())) {
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
     * Has the provider generate the privacy notice of <em>this scenario's</em> agreement - the offer
     * the subject consents to.
     *
     * <p>The notice is asked for by contract, so it can only be the one this scenario's agreement
     * stands for; an older agreement's notice is never picked up. Generation reads the agreement
     * through the consent-facade, which may not see it yet, so it is retried until the notice
     * declares data and a purpose.
     */
    private void generatePrivacyNotice() {
        String request = """
                { "contractUri": "%s" }
                """.formatted(String.format(CONTRACT_URI_TEMPLATE, agreementId));
        Awaitility.await("The privacy notice should have been generated from the agreement.")
                .atMost(PROPAGATION_TIMEOUT)
                .pollInterval(Duration.ofSeconds(2))
                .untilAsserted(() -> {
                    try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/privacy-notices/by-contract",
                            request, fetchProviderToken())) {
                        String body = response.body().string();
                        assertTrue(response.code() == HttpStatus.SC_OK || response.code() == HttpStatus.SC_CREATED,
                                "The notice should be generated from " + agreementId + ": " + body);
                        JsonNode notice = OBJECT_MAPPER.readTree(body);
                        assertFalse(notice.path("data").isEmpty(), "The notice needs a data resource: " + body);
                        assertFalse(notice.path("purposes").isEmpty(), "The notice needs a purpose: " + body);
                        privacyNoticeId = notice.path("id").asText();
                    }
                });
        // `asText()` yields "" rather than null for a missing field, so a null-check would
        // never fire and the grant would go out with an empty privacyNoticeId
        assertFalse(privacyNoticeId == null || privacyNoticeId.isEmpty(),
                "The notice id is needed to grant the consent.");
    }

    /**
     * Asserts what the provider reads at the consent-manager: the consent among the subject's
     * granted ones, and its validity check against the notice.
     */
    private void assertConsentInForce(boolean expected) throws Exception {
        String providerToken = fetchProviderToken();
        HttpUrl listUrl = HttpUrl.get(CONSENT_MANAGER_API + "/consents").newBuilder()
                .addQueryParameter("userIdentifier", subjectDid)
                .build();
        try (Response response = get(HTTP_CLIENT, listUrl.toString(), providerToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(), "The consents should be listable: " + body);
            // the listing returns only GRANTED consents unless asked otherwise
            boolean listed = OBJECT_MAPPER.readTree(body).path("content").findValuesAsText("id").contains(consentId);
            assertEquals(expected, listed, "The granted consents of the subject were " + body);
        }
        HttpUrl checkUrl = HttpUrl.get(CONSENT_MANAGER_API + "/consents/check").newBuilder()
                .addQueryParameter("userIdentifier", subjectDid)
                .addQueryParameter("privacyNoticeId", privacyNoticeId)
                .build();
        try (Response response = get(HTTP_CLIENT, checkUrl.toString(), providerToken)) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(), "The consent should be checkable: " + body);
            assertEquals(expected, OBJECT_MAPPER.readTree(body).path("valid").asBoolean(),
                    "The validity check answered " + body);
        }
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
     * The consumer's participant token: its MembershipCredential, presented to the authority verifier.
     */
    private String consumerParticipantToken() throws Exception {
        if (consumerWallet.getStoredCredential(MEMBERSHIP_CREDENTIAL) == null) {
            String keycloakToken = FancyMarketplaceEnvironment.loginToConsumerKeycloak(MEMBERSHIP_CREDENTIAL_USERNAME);
            consumerWallet.getCredentialFromIssuer(
                    keycloakToken, FancyMarketplaceEnvironment.CONSUMER_KEYCLOAK_ADDRESS, MEMBERSHIP_CREDENTIAL);
        }
        return ScriptHelper.getAccessTokenViaOid4vp(
                CONSENT_MANAGER_VERIFIER_ADDRESS, MEMBERSHIP_CREDENTIAL, PARTICIPANT_SCOPE, consumerWallet);
    }

    /**
     * Registers the participant the token names. A registration left by an earlier run answers 409
     * and is updated instead, so the self-description always is the one this run's agreement uses.
     */
    private void registerParticipant(String participantToken, String legalName, String email,
                                     String selfDescription) throws Exception {
        String participant = """
                { "legalName": "%s", "email": "%s", "selfDescriptionUri": "%s" }
                """.formatted(legalName, email, selfDescription);
        try (Response response = post(HTTP_CLIENT, CONSENT_MANAGER_API + "/participants", participant,
                participantToken)) {
            if (response.code() == HttpStatus.SC_CREATED) {
                return;
            }
            assertEquals(HttpStatus.SC_CONFLICT, response.code(),
                    "The participant should be onboarded, but got " + response.body().string());
        }
        try (Response response = put(HTTP_CLIENT, CONSENT_MANAGER_API + "/participants/me", participant,
                participantToken)) {
            assertEquals(HttpStatus.SC_OK, response.code(),
                    "The registration should have been updated, but got " + response.body().string());
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
     * <p>Find-or-create keeps the self-description URLs stable across runs, so a registration left
     * by an earlier run keeps pointing at a self-description that exists.
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
                            && PROVIDER_DID.equals(characteristic.path("value").asText())) {
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

    private Response put(OkHttpClient client, String url, String body, String token) throws IOException {
        Request request = new Request.Builder()
                .put(RequestBody.create(body, JSON))
                .url(url)
                .header("Authorization", "Bearer " + token)
                .build();
        return client.newCall(request).execute();
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
