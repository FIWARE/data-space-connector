package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.cucumber.java.sl.In;
import org.bouncycastle.openssl.PEMKeyPair;
import org.bouncycastle.openssl.PEMParser;
import org.bouncycastle.openssl.jcajce.JcaPEMKeyConverter;
import org.checkerframework.checker.units.qual.C;
import org.keycloak.common.util.KeystoreUtil;
import org.keycloak.common.util.PemUtils;
import org.keycloak.crypto.KeyWrapper;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.StringReader;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.Key;
import java.security.KeyFactory;
import java.security.KeyPair;
import java.security.KeyStore;
import java.security.KeyStoreException;
import java.security.NoSuchAlgorithmException;
import java.security.NoSuchProviderException;
import java.security.PrivateKey;
import java.security.interfaces.ECPrivateKey;
import java.security.spec.ECPrivateKeySpec;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.KeySpec;
import java.util.Map;

/**
 * Helper methods for the test.
 */
public class TestUtils {

	private static final String CONSUMER_DID_HELPER = "http://did-consumer.127.0.0.1.nip.io:8080";
	private static final String PROVIDER_DID_HELPER = "http://did-provider.127.0.0.1.nip.io:8080";
	private static final String PRIVATE_KEY_PATH = "/did-material/private-key.pem";
	private static final String DID_ENV_PATH = "/did-material/did.env";

	public static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

	static {
		OBJECT_MAPPER.configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);
	}

	private static final HttpClient HTTP_CLIENT = HttpClient
			.newBuilder()
			// we don´t follow the redirect directly, since we are not a real wallet
			.followRedirects(HttpClient.Redirect.NEVER)
			.build();

	private TestUtils() {
		// prevent instantiation
	}

	public static String getFormDataAsString(Map<String, String> formData) {
		StringBuilder formBodyBuilder = new StringBuilder();
		for (Map.Entry<String, String> singleEntry : formData.entrySet()) {
			if (formBodyBuilder.length() > 0) {
				formBodyBuilder.append("&");
			}
			formBodyBuilder.append(URLEncoder.encode(singleEntry.getKey(), StandardCharsets.UTF_8));
			formBodyBuilder.append("=");
			formBodyBuilder.append(URLEncoder.encode(singleEntry.getValue(), StandardCharsets.UTF_8));
		}
		return formBodyBuilder.toString();
	}

	public static String getConsumerDid() throws Exception {
		return getDid(CONSUMER_DID_HELPER);
	}

	public static String getProviderDid() throws Exception {
		return getDid(PROVIDER_DID_HELPER);
	}

	private static String getDid(String host) throws Exception {
		HttpRequest didRequest = HttpRequest.newBuilder()
				.uri(URI.create(host + DID_ENV_PATH))
				.GET()
				.build();
		HttpResponse<InputStream> didResponse = HTTP_CLIENT.send(didRequest, HttpResponse.BodyHandlers.ofInputStream());
		String did = getDidFromEnv(didResponse.body());
		didResponse.body().close();
		return did;
	}

	private static String getDidFromEnv(InputStream didInputStream) throws Exception {
		String didEnvString = new String(didInputStream.readAllBytes(), StandardCharsets.UTF_8);
		return didEnvString.split("=")[1];
	}

	private static KeyWrapper getKey(String host) throws Exception {
		HttpRequest keyRequest = HttpRequest.newBuilder()
				.uri(URI.create(host + PRIVATE_KEY_PATH))
				.GET()
				.build();
		HttpResponse<InputStream> keyResponse = HTTP_CLIENT.send(keyRequest, HttpResponse.BodyHandlers.ofInputStream());
		KeyWrapper keyWrapper = getKeyWrapper(keyResponse.body());
		keyResponse.body().close();
		return keyWrapper;
	}

	public static KeyWrapper getConsumerKey() throws Exception {
		return getKey(CONSUMER_DID_HELPER);
	}

	public static KeyWrapper getProviderKey() throws Exception {
		return getKey(PROVIDER_DID_HELPER);
	}

	private static KeyWrapper getKeyWrapper(InputStream keyInputStream) throws Exception {
		String keyString = new String(keyInputStream.readAllBytes(), StandardCharsets.UTF_8);
		PEMParser pemParser = new PEMParser(new StringReader(keyString));
		JcaPEMKeyConverter converter = new JcaPEMKeyConverter().setProvider("BC");
		Object object = pemParser.readObject();
		KeyPair kp = converter.getKeyPair((PEMKeyPair) object);
		PrivateKey privateKey = kp.getPrivate();
		KeyWrapper keyWrapper = new KeyWrapper();
		keyWrapper.setPrivateKey(privateKey);
		return keyWrapper;
	}
}
