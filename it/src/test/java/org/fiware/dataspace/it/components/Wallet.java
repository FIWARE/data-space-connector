package org.fiware.dataspace.it.components;


import io.ipfs.multibase.Multibase;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.*;
import org.apache.http.HttpStatus;
import org.bouncycastle.jce.interfaces.ECPublicKey;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.Credential;
import org.fiware.dataspace.it.components.model.CredentialOffer;
import org.fiware.dataspace.it.components.model.CredentialRequest;
import org.fiware.dataspace.it.components.model.Grant;
import org.fiware.dataspace.it.components.model.IssuerConfiguration;
import org.fiware.dataspace.it.components.model.OfferUri;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;
import org.fiware.dataspace.it.components.model.SupportedConfiguration;
import org.fiware.dataspace.it.components.model.TokenResponse;
import org.fiware.dataspace.it.components.model.VerifiablePresentation;
import org.keycloak.crypto.ECDSASignatureSignerContext;
import org.keycloak.crypto.KeyUse;
import org.keycloak.crypto.KeyWrapper;
import org.keycloak.jose.jws.JWSBuilder;
import org.keycloak.representations.JsonWebToken;

import java.math.BigInteger;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.NoSuchAlgorithmException;
import java.security.NoSuchProviderException;
import java.security.Security;
import java.security.spec.ECGenParameterSpec;
import java.time.Clock;
import java.util.Base64;
import java.util.HashMap;
import java.util.HexFormat;
import java.util.List;
import java.util.Map;
import java.util.Objects;

import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public class Wallet {

	private static final String OPENID_CREDENTIAL_ISSUER_PATH = "/realms/test-realm/.well-known/openid-credential-issuer";
	private static final String CREDENTIAL_OFFER_URI_PATH = "/realms/test-realm/protocol/oid4vc/credential-offer-uri";
	private static final String OID_WELL_KNOWN_PATH = "/.well-known/openid-configuration";
	private static final String PRE_AUTHORIZED_GRANT_TYPE = "urn:ietf:params:oauth:grant-type:pre-authorized_code";

	private final Map<String, String> credentialStorage = new HashMap<>();

	private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;

	private final KeyWrapper walletKey;
	private final String did;

	public Wallet() throws Exception {
		walletKey = getECKey();
		did = walletKey.getKid();
	}

	public String exchangeCredentialForToken(OpenIdConfiguration openIdConfiguration, String credentialId, String scope) throws Exception {
		String vpToken = Base64.getUrlEncoder()
				.withoutPadding()
				.encodeToString(createVPToken(did, walletKey, credentialStorage.get(credentialId)).getBytes());
		RequestBody requestBody = new FormBody.Builder()
				.add("grant_type", "vp_token")
				.add("vp_token", vpToken)
				.add("scope", scope)
				.build();
		Request tokenRequest = new Request.Builder()
				.post(requestBody)
				.url(openIdConfiguration.getTokenEndpoint())
				.build();
		Response tokenResponse = HTTP_CLIENT.newCall(tokenRequest).execute();
		try {
			assertEquals(HttpStatus.SC_OK, tokenResponse.code(), "A token should have been responded.");
			TokenResponse accessTokenResponse = OBJECT_MAPPER.readValue(tokenResponse.body().string(), TokenResponse.class);
			assertNotNull(accessTokenResponse.getAccessToken(), "The access token should have been returned.");
			return accessTokenResponse.getAccessToken();
		} finally {
			tokenResponse.body().close();
		}
	}

	private String createVPToken(String did, KeyWrapper key, String credential) {
		VerifiablePresentation verifiablePresentation = new VerifiablePresentation();
		verifiablePresentation.setVerifiableCredential(List.of(credential));
		verifiablePresentation.setHolder(did);
		key.setKid(did);
		key.setAlgorithm("ES256");
		key.setUse(KeyUse.SIG);

		ECDSASignatureSignerContext signerContext = new ECDSASignatureSignerContext(key);

		JsonWebToken jwt = new JsonWebToken()
				.issuer(did)
				.subject(did)
				.iat(Clock.systemUTC().millis());
		jwt.setOtherClaims("vp", verifiablePresentation);
		return new JWSBuilder()
				.type("JWT")
				.jsonContent(jwt)
				.sign(signerContext);
	}


	public void getCredentialFromIssuer(String userToken, String issuerHost, String credentialId) throws Exception {
		IssuerConfiguration issuerConfiguration = getIssuerConfiguration(issuerHost);
		OfferUri offerUri = getCredentialOfferUri(userToken, issuerHost, credentialId);
		CredentialOffer credentialOffer = getCredentialOffer(userToken, offerUri);

		var theCredential = getCredential(issuerConfiguration, credentialOffer);
		credentialStorage.put(credentialId, theCredential);
	}


	public IssuerConfiguration getIssuerConfiguration(String issuerHost) throws Exception {

		Request configRequest = new Request.Builder()
				.get()
				.url(issuerHost + OPENID_CREDENTIAL_ISSUER_PATH)
				.build();
		Response configResponse = HTTP_CLIENT.newCall(configRequest).execute();

		assertEquals(HttpStatus.SC_OK, configResponse.code(), "An issuer config should have been returned.");
		IssuerConfiguration issuerConfiguration = OBJECT_MAPPER.readValue(configResponse.body().string(), IssuerConfiguration.class);
		configResponse.body().close();
		return issuerConfiguration;
	}

	public OfferUri getCredentialOfferUri(String keycloakJwt, String issuerHost, String credentialConfigId) throws Exception {

		Request request = new Request.Builder()
				.url(issuerHost + CREDENTIAL_OFFER_URI_PATH + "?credential_configuration_id=" + credentialConfigId)
				.get()
				.header("Authorization", "Bearer " + keycloakJwt)
				.build();

		Response uriResponse = HTTP_CLIENT.newCall(request).execute();

		assertEquals(HttpStatus.SC_OK, uriResponse.code(), "An uri should have been returned.");
		OfferUri offerUri = OBJECT_MAPPER.readValue(uriResponse.body().string(), OfferUri.class);
		uriResponse.body().close();
		return offerUri;
	}

	public CredentialOffer getCredentialOffer(String keycloakJwt, OfferUri offerUri) throws Exception {

		Request uriRequest = new Request.Builder()
				.get()
				.url(offerUri.getIssuer() + offerUri.getNonce())
				.header("Authorization", "Bearer " + keycloakJwt)
				.build();

		Response offerResponse = HTTP_CLIENT.newCall(uriRequest).execute();

		assertEquals(HttpStatus.SC_OK, offerResponse.code(), "An offer should have been returned.");
		CredentialOffer credentialOffer = OBJECT_MAPPER.readValue(offerResponse.body().string(), CredentialOffer.class);
		offerResponse.body().close();
		return credentialOffer;
	}

	public String getTokenForOffer(IssuerConfiguration issuerConfiguration, CredentialOffer credentialOffer) throws Exception {
		String authorizationServer = issuerConfiguration.getAuthorizationServers().get(0);
		OpenIdConfiguration openIdConfiguration = getOpenIdConfiguration(authorizationServer);
		assertTrue(openIdConfiguration.getGrantTypesSupported().contains(PRE_AUTHORIZED_GRANT_TYPE), "The grant type should actually be supported by the authorization server.");

		Grant preAuthorizedGrant = credentialOffer.getGrants().get(PRE_AUTHORIZED_GRANT_TYPE);
		return getAccessToken(openIdConfiguration.getTokenEndpoint(), preAuthorizedGrant.getPreAuthorizedCode());
	}

	public String getCredential(IssuerConfiguration issuerConfiguration, CredentialOffer credentialOffer) throws Exception {
		String accessToken = getTokenForOffer(issuerConfiguration, credentialOffer);

		String credentialResponse = credentialOffer.getCredentialConfigurationIds()
				.stream()
				.map(offeredCredentialId -> issuerConfiguration.getCredentialConfigurationsSupported().get(offeredCredentialId))
				.map(supportedCredential -> {
					try {
						return requestOffer(accessToken, issuerConfiguration.getCredentialEndpoint(), supportedCredential);
					} catch (Exception e) {
						return null;
					}
				})
				.filter(Objects::nonNull)
				.findFirst()
				.get();
		return OBJECT_MAPPER.readValue(credentialResponse, Credential.class).getCredential();
	}

	private String requestOffer(String token, String credentialEndpoint, SupportedConfiguration offeredCredential) throws Exception {
		CredentialRequest credentialRequest = new CredentialRequest();
		credentialRequest.setCredentialIdentifier(offeredCredential.getId());
		credentialRequest.setFormat(offeredCredential.getFormat());

		RequestBody credentialRequestBody = RequestBody
				.create(OBJECT_MAPPER.writeValueAsString(credentialRequest), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
		Request credentialHttpRequest = new Request.Builder()
				.post(credentialRequestBody)
				.url(credentialEndpoint)
				.header("Authorization", "Bearer " + token)
				.header("Content-Type", MediaType.APPLICATION_JSON)
				.build();


		Response credentialResponse = HTTP_CLIENT.newCall(credentialHttpRequest).execute();
		assertEquals(HttpStatus.SC_OK, credentialResponse.code(), "A credential should have been returned.");
		String offer = credentialResponse.body().string();
		credentialResponse.body().close();
		return offer;
	}

	public String getAccessToken(String tokenEndpoint, String preAuthorizedCode) throws Exception {
		RequestBody requestBody = new FormBody.Builder()
				.add("grant_type", PRE_AUTHORIZED_GRANT_TYPE)
				.add("pre-authorized_code", preAuthorizedCode)
				.build();
		Request tokenRequest = new Request.Builder()
				.url(tokenEndpoint)
				.post(requestBody)
				.build();

		Response tokenResponse = HTTP_CLIENT.newCall(tokenRequest).execute();
		assertEquals(HttpStatus.SC_OK, tokenResponse.code(), "A valid token should have been returned.");

		String accessToken = OBJECT_MAPPER.readValue(tokenResponse.body().string(), TokenResponse.class).getAccessToken();
		tokenResponse.body().close();
		return accessToken;
	}

	public OpenIdConfiguration getOpenIdConfiguration(String authorizationServer) throws Exception {
		Request request = new Request.Builder()
				.get()
				.url(authorizationServer + OID_WELL_KNOWN_PATH)
				.build();
		Response openIdConfigResponse = HTTP_CLIENT.newCall(request).execute();

		assertEquals(HttpStatus.SC_OK, openIdConfigResponse.code(), "An openId config should have been returned.");
		OpenIdConfiguration openIdConfiguration = OBJECT_MAPPER.readValue(openIdConfigResponse.body().string(), OpenIdConfiguration.class);
		openIdConfigResponse.body().close();
		return openIdConfiguration;
	}


	private static KeyWrapper getECKey() throws Exception {
		try {
			Security.addProvider(new BouncyCastleProvider());

			KeyPairGenerator kpGen = KeyPairGenerator.getInstance("EC", "BC");
			ECGenParameterSpec spec = new ECGenParameterSpec("P-256");
			kpGen.initialize(spec);

			var keyPair = kpGen.generateKeyPair();
			KeyWrapper kw = new KeyWrapper();
			kw.setPrivateKey(keyPair.getPrivate());
			kw.setPublicKey(keyPair.getPublic());
			kw.setUse(KeyUse.SIG);
			kw.setKid(generateDid(keyPair));
			kw.setType("EC");
			kw.setAlgorithm("ES256");
			return kw;

		} catch (NoSuchAlgorithmException | NoSuchProviderException e) {
			throw new RuntimeException(e);
		}
	}

	private static String generateDid(KeyPair keyPair) throws Exception {
		if (keyPair.getPublic() instanceof ECPublicKey ecPublicKey) {
			byte[] encodedQ = ecPublicKey.getQ().getEncoded(true);
			byte[] codecBytes = new byte[2];
			codecBytes[0] = HexFormat.of().parseHex("80")[0];
			codecBytes[1] = HexFormat.of().parseHex("24")[0];

			byte[] prefixed = new byte[encodedQ.length + codecBytes.length];
			System.arraycopy(codecBytes, 0, prefixed, 0, codecBytes.length);
			System.arraycopy(encodedQ, 0, prefixed, codecBytes.length, encodedQ.length);

			String encodedKeyRaw = Multibase.encode(Multibase.Base.Base58BTC, prefixed);
			return "did:key:" + encodedKeyRaw;
		}
		throw new IllegalArgumentException("Key pair is not supported.");
	}

	private static byte[] marshalCompressed(BigInteger x, BigInteger y) {
		// we only support the P-256 curve here, thus a fixed length can be set
		byte[] compressed = new byte[33];
		compressed[0] = (byte) (y.testBit(0) ? 1 : 0);
		System.arraycopy(x.toByteArray(), 0, compressed, 1, x.toByteArray().length);
		return compressed;
	}

}
