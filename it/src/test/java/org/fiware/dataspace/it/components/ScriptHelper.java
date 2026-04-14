package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import lombok.extern.slf4j.Slf4j;
import okhttp3.FormBody;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.TokenResponse;

import java.util.Base64;

import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.fiware.dataspace.it.components.TestUtils.OK_HTTP_CLIENT;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * Java equivalents of the shell scripts in {@code doc/scripts/} used by DSP and central marketplace tests.
 * <p>
 * This avoids shell script dependencies in Java integration tests by providing pure Java
 * implementations of credential retrieval, token exchange, and JWT payload decoding.
 *
 * @see <a href="../../../../../../doc/scripts/get_credential.sh">get_credential.sh</a>
 * @see <a href="../../../../../../doc/scripts/get_access_token_oid4vp.sh">get_access_token_oid4vp.sh</a>
 * @see <a href="../../../../../../doc/scripts/get-payload-from-jwt.sh">get-payload-from-jwt.sh</a>
 */
@Slf4j
public class ScriptHelper {

    /** Grant type for pre-authorized code exchange in OID4VC credential issuance. */
    private static final String PRE_AUTHORIZED_GRANT_TYPE = "urn:ietf:params:oauth:grant-type:pre-authorized_code";

    /** Grant type for VP token exchange in OID4VP flows. */
    private static final String VP_TOKEN_GRANT_TYPE = "vp_token";

    /** Path to the OpenID Credential Issuer well-known configuration. */
    private static final String OPENID_CREDENTIAL_ISSUER_PATH = "/realms/test-realm/.well-known/openid-credential-issuer";

    /** Path to request a credential offer URI from Keycloak. */
    private static final String CREDENTIAL_OFFER_URI_PATH = "/realms/test-realm/protocol/oid4vc/credential-offer-uri";

    /** Path to the OpenID Connect well-known configuration. */
    private static final String OIDC_WELL_KNOWN_PATH = "/.well-known/openid-configuration";

    /** Default password for test users in the local deployment. */
    private static final String TEST_USER_PASSWORD = "test";

    /** Default Keycloak realm used in the local deployment. */
    private static final String TEST_REALM = "test-realm";

    /** Default Keycloak client ID used for user authentication. */
    private static final String KEYCLOAK_CLIENT_ID = "account-console";

    private static final OkHttpClient HTTP_CLIENT = OK_HTTP_CLIENT;

    private ScriptHelper() {
        // prevent instantiation
    }

    /**
     * Retrieves a credential from a Keycloak OID4VC issuer.
     * <p>
     * Java equivalent of {@code doc/scripts/get_credential.sh}.
     * Performs the full OID4VC pre-authorized code flow:
     * <ol>
     *   <li>Get Keycloak user token via password grant</li>
     *   <li>Request credential offer URI</li>
     *   <li>Retrieve credential offer (contains pre-authorized code)</li>
     *   <li>Exchange pre-authorized code for credential access token</li>
     *   <li>Request credential with the access token</li>
     * </ol>
     *
     * @param keycloakBaseUrl           the base URL of the Keycloak instance (e.g., {@code https://keycloak-consumer.127.0.0.1.nip.io})
     * @param credentialConfigurationId the credential configuration ID to request (e.g., {@code membership-credential})
     * @param username                  the Keycloak username (e.g., {@code employee})
     * @return the raw credential string (JWT)
     * @throws Exception if any step in the credential issuance flow fails
     */
    public static String getCredential(String keycloakBaseUrl, String credentialConfigurationId, String username) throws Exception {
        return getCredential(keycloakBaseUrl, credentialConfigurationId, username, null);
    }

    /**
     * Retrieves a credential from a Keycloak OID4VC issuer with an optional format override.
     *
     * @param keycloakBaseUrl           the base URL of the Keycloak instance
     * @param credentialConfigurationId the credential configuration ID to request
     * @param username                  the Keycloak username
     * @param format                    the credential format (e.g., {@code jwt_vc}, {@code vc+sd-jwt}), or {@code null} for default
     * @return the raw credential string (JWT or SD-JWT)
     * @throws Exception if any step in the credential issuance flow fails
     */
    public static String getCredential(String keycloakBaseUrl, String credentialConfigurationId,
                                       String username, String format) throws Exception {
        // Step 1: Get Keycloak user token
        String userToken = getKeycloakToken(keycloakBaseUrl, username, TEST_USER_PASSWORD);

        // Step 2: Request credential offer URI
        String offerUriUrl = keycloakBaseUrl + CREDENTIAL_OFFER_URI_PATH
                + "?credential_configuration_id=" + credentialConfigurationId;
        Request offerUriRequest = new Request.Builder()
                .get()
                .url(offerUriUrl)
                .header("Authorization", "Bearer " + userToken)
                .build();
        try (Response offerUriResponse = HTTP_CLIENT.newCall(offerUriRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, offerUriResponse.code(), "Credential offer URI request should succeed.");
            JsonNode offerUri = OBJECT_MAPPER.readTree(offerUriResponse.body().string());
            String issuer = offerUri.get("issuer").asText();
            String nonce = offerUri.get("nonce").asText();

            // Step 3: Get credential offer
            Request offerRequest = new Request.Builder()
                    .get()
                    .url(issuer + nonce)
                    .header("Authorization", "Bearer " + userToken)
                    .build();
            try (Response offerResponse = HTTP_CLIENT.newCall(offerRequest).execute()) {
                assertEquals(HttpStatus.SC_OK, offerResponse.code(), "Credential offer request should succeed.");
                JsonNode credentialOffer = OBJECT_MAPPER.readTree(offerResponse.body().string());

                // Extract pre-authorized code
                String preAuthCode = credentialOffer
                        .get("grants")
                        .get(PRE_AUTHORIZED_GRANT_TYPE)
                        .get("pre-authorized_code")
                        .asText();

                // Get issuer configuration for credential endpoint
                String credentialIssuer = credentialOffer.get("credential_issuer").asText();
                Request issuerConfigRequest = new Request.Builder()
                        .get()
                        .url(keycloakBaseUrl + OPENID_CREDENTIAL_ISSUER_PATH)
                        .build();
                try (Response issuerConfigResponse = HTTP_CLIENT.newCall(issuerConfigRequest).execute()) {
                    assertEquals(HttpStatus.SC_OK, issuerConfigResponse.code(), "Issuer config request should succeed.");
                    JsonNode issuerConfig = OBJECT_MAPPER.readTree(issuerConfigResponse.body().string());

                    // Step 4: Exchange pre-authorized code for access token
                    String authServer = issuerConfig.get("authorization_servers").get(0).asText();
                    Request oidcConfigRequest = new Request.Builder()
                            .get()
                            .url(authServer + OIDC_WELL_KNOWN_PATH)
                            .build();
                    try (Response oidcConfigResponse = HTTP_CLIENT.newCall(oidcConfigRequest).execute()) {
                        assertEquals(HttpStatus.SC_OK, oidcConfigResponse.code(), "OIDC config request should succeed.");
                        JsonNode oidcConfig = OBJECT_MAPPER.readTree(oidcConfigResponse.body().string());
                        String tokenEndpoint = oidcConfig.get("token_endpoint").asText();

                        RequestBody tokenBody = new FormBody.Builder()
                                .add("grant_type", PRE_AUTHORIZED_GRANT_TYPE)
                                .add("pre-authorized_code", preAuthCode)
                                .build();
                        Request tokenRequest = new Request.Builder()
                                .post(tokenBody)
                                .url(tokenEndpoint)
                                .build();
                        try (Response tokenResponse = HTTP_CLIENT.newCall(tokenRequest).execute()) {
                            assertEquals(HttpStatus.SC_OK, tokenResponse.code(), "Token exchange should succeed.");
                            TokenResponse token = OBJECT_MAPPER.readValue(
                                    tokenResponse.body().string(), TokenResponse.class);
                            String accessToken = token.getAccessToken();

                            // Step 5: Request credential
                            String credentialEndpoint = issuerConfig.get("credential_endpoint").asText();
                            String credentialIdentifier = credentialOffer.get("credential_configuration_ids")
                                    .get(0).asText();

                            // Determine format
                            String credFormat = format;
                            if (credFormat == null) {
                                JsonNode supportedConfig = issuerConfig
                                        .get("credential_configurations_supported")
                                        .get(credentialIdentifier);
                                if (supportedConfig != null && supportedConfig.has("format")) {
                                    credFormat = supportedConfig.get("format").asText();
                                }
                            }

                            // Build credential request
                            com.fasterxml.jackson.databind.node.ObjectNode credReq = OBJECT_MAPPER.createObjectNode();
                            credReq.put("credential_identifier", credentialIdentifier);
                            if (credFormat != null) {
                                credReq.put("format", credFormat);
                            }

                            RequestBody credBody = RequestBody.create(
                                    OBJECT_MAPPER.writeValueAsString(credReq),
                                    okhttp3.MediaType.parse("application/json"));
                            Request credRequest = new Request.Builder()
                                    .post(credBody)
                                    .url(credentialEndpoint)
                                    .header("Authorization", "Bearer " + accessToken)
                                    .header("Content-Type", "application/json")
                                    .build();
                            try (Response credResponse = HTTP_CLIENT.newCall(credRequest).execute()) {
                                assertEquals(HttpStatus.SC_OK, credResponse.code(), "Credential request should succeed.");
                                JsonNode credResult = OBJECT_MAPPER.readTree(credResponse.body().string());
                                String credential = credResult.get("credential").asText();
                                assertNotNull(credential, "Credential should not be null.");
                                log.debug("Successfully retrieved credential '{}' from {}", credentialConfigurationId, keycloakBaseUrl);
                                return credential;
                            }
                        }
                    }
                }
            }
        }
    }

    /**
     * Exchanges a verifiable credential for an access token via the OID4VP flow.
     * <p>
     * Java equivalent of {@code doc/scripts/get_access_token_oid4vp.sh}.
     * Gets the OpenID configuration from the target endpoint, then uses the
     * provided Wallet to create a VP token and exchange it for an access token.
     * <p>
     * The credential must already be stored in the Wallet's credential storage
     * (via a previous call to {@link Wallet#getCredentialFromIssuer}).
     *
     * @param baseUrl      the base URL of the service to authenticate against (must expose {@code /.well-known/openid-configuration})
     * @param credentialId the credential ID as stored in the wallet (e.g., {@code membership-credential})
     * @param scope        the OAuth scope to request (e.g., {@code openid})
     * @param wallet       the Wallet instance used to create and sign the VP token
     * @return the access token string
     * @throws Exception if the token exchange fails
     */
    public static String getAccessTokenViaOid4vp(String baseUrl, String credentialId, String scope,
                                                  Wallet wallet) throws Exception {
        // Get OpenID configuration
        Request oidcRequest = new Request.Builder()
                .get()
                .url(baseUrl + OIDC_WELL_KNOWN_PATH)
                .build();
        try (Response oidcResponse = HTTP_CLIENT.newCall(oidcRequest).execute()) {
            assertEquals(HttpStatus.SC_OK, oidcResponse.code(), "OpenID configuration should be available.");
            OpenIdConfiguration oidcConfig = OBJECT_MAPPER.readValue(
                    oidcResponse.body().string(), OpenIdConfiguration.class);

            // Exchange credential for token via the wallet
            return wallet.exchangeCredentialForToken(oidcConfig, credentialId, scope);
        }
    }

    /**
     * Decodes the payload of a JWT token and returns it as a JSON string.
     * <p>
     * Java equivalent of {@code doc/scripts/get-payload-from-jwt.sh}.
     * Extracts the second part (payload) of the JWT, Base64url-decodes it,
     * and returns the resulting JSON string.
     *
     * @param jwt the JWT string (header.payload.signature)
     * @return the decoded payload as a JSON string
     * @throws Exception if the JWT format is invalid or decoding fails
     */
    public static String getPayloadFromJwt(String jwt) throws Exception {
        String[] parts = jwt.split("\\.");
        if (parts.length < 2) {
            throw new IllegalArgumentException("Invalid JWT format. Expected at least 2 parts separated by '.'");
        }
        String payload = parts[1];
        byte[] decoded = Base64.getUrlDecoder().decode(payload);
        return new String(decoded);
    }

    /**
     * Extracts the {@code vc} claim from a JWT credential's payload.
     * <p>
     * This is equivalent to running:
     * <pre>
     * ./doc/scripts/get-payload-from-jwt.sh "${CREDENTIAL}" | jq -r '.vc'
     * </pre>
     *
     * @param jwt the JWT credential string
     * @return the VC content as a JSON string
     * @throws Exception if the JWT format is invalid or the {@code vc} claim is missing
     */
    public static String getVcContentFromJwt(String jwt) throws Exception {
        String payload = getPayloadFromJwt(jwt);
        JsonNode payloadNode = OBJECT_MAPPER.readTree(payload);
        JsonNode vcNode = payloadNode.get("vc");
        assertNotNull(vcNode, "JWT payload should contain a 'vc' claim.");
        return OBJECT_MAPPER.writeValueAsString(vcNode);
    }

    /**
     * Gets a Keycloak access token for a user via password grant.
     *
     * @param keycloakBaseUrl the base URL of the Keycloak instance
     * @param username        the username to authenticate
     * @param password        the password to authenticate
     * @return the access token string
     * @throws Exception if authentication fails
     */
    private static String getKeycloakToken(String keycloakBaseUrl, String username, String password) throws Exception {
        RequestBody body = new FormBody.Builder()
                .add("grant_type", "password")
                .add("client_id", KEYCLOAK_CLIENT_ID)
                .add("username", username)
                .add("password", password)
                .build();
        Request request = new Request.Builder()
                .post(body)
                .url(keycloakBaseUrl + "/realms/" + TEST_REALM + "/protocol/openid-connect/token")
                .build();

        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "Keycloak token request should succeed.");
            TokenResponse tokenResponse = OBJECT_MAPPER.readValue(response.body().string(), TokenResponse.class);
            return tokenResponse.getAccessToken();
        }
    }
}
