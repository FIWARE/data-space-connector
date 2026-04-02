package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import lombok.extern.slf4j.Slf4j;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.bouncycastle.jce.interfaces.ECPrivateKey;
import org.bouncycastle.jce.interfaces.ECPublicKey;
import org.bouncycastle.openssl.PEMKeyPair;
import org.bouncycastle.openssl.PEMParser;
import org.bouncycastle.openssl.jcajce.JcaPEMKeyConverter;

import java.io.StringReader;
import java.math.BigInteger;
import java.security.KeyPair;
import java.util.Base64;

import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.fiware.dataspace.it.components.TestUtils.OK_HTTP_CLIENT;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Helper class for interacting with the Tractus-X IdentityHub and HashiCorp Vault
 * in the DSP deployment profile.
 * <p>
 * Encapsulates the key insertion, participant registration, and credential management
 * operations documented in DSP_INTEGRATION.md "Setup the consumer" and "Setup the provider" sections.
 *
 * @see DSPEnvironment
 */
@Slf4j
public class IdentityHubHelper {

    /** The IdentityHub management API path for participant operations. */
    private static final String PARTICIPANTS_API_PATH = "/api/identity/v1alpha/participants";

    /** The Vault KV secret engine path prefix for key storage. */
    private static final String VAULT_SECRET_PATH = "/v1/secret/data/";

    /** API key for authenticating with the IdentityHub management API. */
    public static final String IDENTITY_HUB_API_KEY = "c3VwZXItdXNlcg==.random";

    /** Authentication token for the Vault instance. */
    public static final String VAULT_TOKEN = "root";

    /** Default key alias used for participant key registration. */
    public static final String DEFAULT_KEY_ALIAS = "key-1";

    /** JSON media type constant for HTTP requests. */
    private static final MediaType JSON = MediaType.parse("application/json");

    /** The HTTP port used by IdentityHub and Vault services in the local deployment. */
    private static final int SERVICE_PORT = 8080;

    private static final OkHttpClient HTTP_CLIENT = OK_HTTP_CLIENT;

    private IdentityHubHelper() {
        // prevent instantiation
    }

    /**
     * Inserts a private key (as JWK) into a Vault KV secret engine.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST 'http://vault-{instance}.127.0.0.1.nip.io:8080/v1/secret/data/key-1' \
     *   --header 'X-Vault-Token: root' \
     *   --data '{"data":{"content":"...jwk..."}}'
     * </pre>
     *
     * @param vaultAddress the base URL of the Vault instance (e.g., {@code http://vault-fancy-marketplace.127.0.0.1.nip.io})
     * @param keyAlias     the key alias to use as the path suffix (e.g., {@code key-1})
     * @param jwk          the JWK JSON string representing the private key
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static void insertKeyIntoVault(String vaultAddress, String keyAlias, String jwk) throws Exception {
        ObjectNode payload = OBJECT_MAPPER.createObjectNode();
        ObjectNode data = OBJECT_MAPPER.createObjectNode();
        data.put("content", jwk);
        payload.set("data", data);

        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(payload), JSON);
        Request request = new Request.Builder()
                .url(vaultAddress + ":" + SERVICE_PORT + VAULT_SECRET_PATH + keyAlias)
                .post(body)
                .header("X-Vault-Token", VAULT_TOKEN)
                .build();

        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("Vault key insertion should succeed. Got status %d: %s",
                            response.code(), response.body() != null ? response.body().string() : "no body"));
            log.info("Successfully inserted key '{}' into Vault at {}", keyAlias, vaultAddress);
        }
    }

    /**
     * Registers a participant in the IdentityHub management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST 'http://identityhub-management-{instance}.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants' \
     *   --header 'x-api-key: c3VwZXItdXNlcg==.random' \
     *   --header 'Content-Type: application/json' \
     *   --data '...'
     * </pre>
     *
     * @param identityHubManagementAddress the base URL of the IdentityHub management API
     * @param participantJson              the JSON payload for participant registration (from {@link #buildParticipantPayload})
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static void registerParticipant(String identityHubManagementAddress, String participantJson) throws Exception {
        RequestBody body = RequestBody.create(participantJson, JSON);
        Request request = new Request.Builder()
                .url(identityHubManagementAddress + ":" + SERVICE_PORT + PARTICIPANTS_API_PATH)
                .post(body)
                .header("Accept", "*/*")
                .header("x-api-key", IDENTITY_HUB_API_KEY)
                .header("Content-Type", "application/json")
                .build();

        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("Participant registration should succeed. Got status %d: %s",
                            response.code(), response.body() != null ? response.body().string() : "no body"));
            log.info("Successfully registered participant at {}", identityHubManagementAddress);
        }
    }

    /**
     * Inserts a verifiable credential into the IdentityHub for a given participant.
     * <p>
     * The participant ID is Base64url-encoded and used as a path segment.
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST 'http://identityhub-management-{instance}.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants/{base64url-pid}/credentials' \
     *   --header 'x-api-key: c3VwZXItdXNlcg==.random' \
     *   --data '{"id":"...","participantContextId":"...","verifiableCredentialContainer":{...}}'
     * </pre>
     *
     * @param identityHubManagementAddress the base URL of the IdentityHub management API
     * @param participantId               the DID of the participant (e.g., {@code did:web:fancy-marketplace.biz})
     * @param credentialId                the credential identifier (e.g., {@code membership-credential})
     * @param rawVc                       the raw JWT credential string
     * @param credentialContent           the decoded VC content as a JSON string (the {@code .vc} claim from the JWT payload)
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static void insertCredential(String identityHubManagementAddress, String participantId,
                                        String credentialId, String rawVc, String credentialContent) throws Exception {
        String base64ParticipantId = base64UrlEncode(participantId);

        ObjectNode container = OBJECT_MAPPER.createObjectNode();
        container.put("rawVc", rawVc);
        container.put("format", "VC1_0_JWT");
        container.set("credential", OBJECT_MAPPER.readTree(credentialContent));

        ObjectNode payload = OBJECT_MAPPER.createObjectNode();
        payload.put("id", credentialId);
        payload.put("participantContextId", participantId);
        payload.set("verifiableCredentialContainer", container);

        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(payload), JSON);
        String url = identityHubManagementAddress + ":" + SERVICE_PORT
                + PARTICIPANTS_API_PATH + "/" + base64ParticipantId + "/credentials";
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("x-api-key", IDENTITY_HUB_API_KEY)
                .header("Content-Type", "application/json")
                .build();

        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.isSuccessful(),
                    String.format("Credential insertion should succeed. Got status %d: %s",
                            response.code(), response.body() != null ? response.body().string() : "no body"));
            log.info("Successfully inserted credential '{}' for participant '{}' at {}",
                    credentialId, participantId, identityHubManagementAddress);
        }
    }

    /**
     * Converts a PEM-encoded EC private key (PKCS#8 format) to a JWK JSON string.
     * <p>
     * This is the Java equivalent of {@code doc/scripts/get-private-jwk-p-256.sh}.
     * Extracts the P-256 curve parameters (x, y, d) and encodes them as Base64url
     * values in a standard EC JWK.
     *
     * @param pemKeyContent the PEM key content as a string
     * @return the JWK JSON string with fields: kty, crv, x, y, d
     * @throws Exception if the key cannot be parsed or is not a P-256 EC key
     */
    public static String getPrivateKeyAsJwk(String pemKeyContent) throws Exception {
        PEMParser pemParser = new PEMParser(new StringReader(pemKeyContent));
        Object object = pemParser.readObject();
        pemParser.close();

        KeyPair keyPair;
        JcaPEMKeyConverter converter = new JcaPEMKeyConverter().setProvider("BC");
        if (object instanceof PEMKeyPair) {
            keyPair = converter.getKeyPair((PEMKeyPair) object);
        } else {
            throw new IllegalArgumentException("Unsupported PEM format. Expected PEMKeyPair.");
        }

        ECPublicKey publicKey = (ECPublicKey) keyPair.getPublic();
        ECPrivateKey privateKey = (ECPrivateKey) keyPair.getPrivate();

        BigInteger x = publicKey.getQ().getAffineXCoord().toBigInteger();
        BigInteger y = publicKey.getQ().getAffineYCoord().toBigInteger();
        BigInteger d = privateKey.getD();

        ObjectNode jwk = OBJECT_MAPPER.createObjectNode();
        jwk.put("kty", "EC");
        jwk.put("crv", "P-256");
        jwk.put("x", bigIntToBase64Url(x, 32));
        jwk.put("y", bigIntToBase64Url(y, 32));
        jwk.put("d", bigIntToBase64Url(d, 32));

        return OBJECT_MAPPER.writeValueAsString(jwk);
    }

    /**
     * Builds the participant creation JSON payload for the IdentityHub management API.
     * <p>
     * This is the Java equivalent of {@code doc/scripts/get-participant-create.sh}.
     * Creates a JSON structure with the participant's identity, public key, and service endpoints.
     *
     * @param jwk                    the full JWK (including private key {@code d}) as a JSON string
     * @param did                    the participant's DID (e.g., {@code did:web:fancy-marketplace.biz})
     * @param identityHubAddress     the base URL of the participant's public IdentityHub (for credential service endpoint)
     * @param keyAlias               the key alias (e.g., {@code key-1})
     * @return the participant creation JSON payload as a string
     * @throws Exception if JSON processing fails
     */
    public static String buildParticipantPayload(String jwk, String did, String identityHubAddress, String keyAlias) throws Exception {
        // Create public JWK by removing the private key 'd' and adding kid
        JsonNode jwkNode = OBJECT_MAPPER.readTree(jwk);
        ObjectNode publicJwk = ((ObjectNode) jwkNode).deepCopy();
        publicJwk.remove("d");
        publicJwk.put("kid", did + "#" + keyAlias);

        String base64ParticipantId = base64UrlEncode(did);

        // Build the credential service endpoint URL
        String credentialServiceUrl = identityHubAddress + "/api/credentials/v1/participants/" + base64ParticipantId;

        // Build the participant payload
        ObjectNode payload = OBJECT_MAPPER.createObjectNode();

        ArrayNode roles = OBJECT_MAPPER.createArrayNode();
        roles.add("admin");
        payload.set("role", roles);

        ArrayNode serviceEndpoints = OBJECT_MAPPER.createArrayNode();
        ObjectNode credentialService = OBJECT_MAPPER.createObjectNode();
        credentialService.put("type", "CredentialService");
        credentialService.put("serviceEndpoint", credentialServiceUrl);
        credentialService.put("id", "credential-service");
        serviceEndpoints.add(credentialService);
        payload.set("serviceEndpoints", serviceEndpoints);

        payload.put("active", true);
        payload.put("participantId", did);
        payload.put("did", did);

        ObjectNode key = OBJECT_MAPPER.createObjectNode();
        key.put("keyId", did + "#" + keyAlias);
        key.put("privateKeyAlias", keyAlias);
        key.set("publicKeyJwk", publicJwk);
        payload.set("key", key);

        return OBJECT_MAPPER.writeValueAsString(payload);
    }

    /**
     * Base64url-encodes a string without padding, as used in IdentityHub API paths.
     *
     * @param input the string to encode
     * @return the Base64url-encoded string without padding
     */
    public static String base64UrlEncode(String input) {
        return Base64.getUrlEncoder().withoutPadding().encodeToString(input.getBytes());
    }

    /**
     * Converts a BigInteger to a fixed-length Base64url-encoded string.
     * <p>
     * Ensures the byte array is exactly {@code length} bytes by padding with leading zeros
     * or trimming a leading sign byte as needed.
     *
     * @param value  the BigInteger value to encode
     * @param length the desired byte length before Base64url encoding
     * @return the Base64url-encoded string without padding
     */
    private static String bigIntToBase64Url(BigInteger value, int length) {
        byte[] bytes = value.toByteArray();
        byte[] result = new byte[length];

        if (bytes.length == length) {
            result = bytes;
        } else if (bytes.length > length) {
            // Strip leading zero byte (sign byte)
            System.arraycopy(bytes, bytes.length - length, result, 0, length);
        } else {
            // Pad with leading zeros
            System.arraycopy(bytes, 0, result, length - bytes.length, bytes.length);
        }

        return Base64.getUrlEncoder().withoutPadding().encodeToString(result);
    }
}
