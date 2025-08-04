package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import org.bouncycastle.openssl.PEMKeyPair;
import org.bouncycastle.openssl.PEMParser;
import org.bouncycastle.openssl.jcajce.JcaPEMKeyConverter;
import org.keycloak.crypto.KeyWrapper;

import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;
import java.io.InputStream;
import java.io.StringReader;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.KeyManagementException;
import java.security.KeyPair;
import java.security.NoSuchAlgorithmException;
import java.security.PrivateKey;
import java.security.cert.X509Certificate;
import java.util.Map;

/**
 * Helper methods for the test.
 */
@Slf4j
public class TestUtils {

	private static final String CONSUMER_DID_HELPER = "http://did-consumer.127.0.0.1.nip.io:8080";
	private static final String PROVIDER_DID_HELPER = "http://did-provider.127.0.0.1.nip.io:8080";
	private static final String PRIVATE_KEY_PATH = "/did-material/private-key.pem";
	private static final String DID_ENV_PATH = "/did-material/did.env";
	private static final Integer SQUID_PORT = 8888;

	public static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

	static {
		OBJECT_MAPPER.configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);
	}

	public static final OkHttpClient OK_HTTP_CLIENT;

	static {
		try {
			OK_HTTP_CLIENT = new OkHttpClient.Builder()
					.proxy(getHttpProxy())
					.followRedirects(false)
					.sslSocketFactory(getTrustAllContext().getSocketFactory(), getTrustAllManager())
					.build();
		} catch (KeyManagementException e) {
			throw new RuntimeException(e);
		} catch (NoSuchAlgorithmException e) {
			throw new RuntimeException(e);
		}
	}

	private TestUtils() {
		// prevent instantiation
	}


	private static Proxy getHttpProxy() {
		return new Proxy(Proxy.Type.HTTP, new InetSocketAddress("127.0.0.1", SQUID_PORT));
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

	public static X509TrustManager getTrustAllManager() {
		return new X509TrustManager() {
			public void checkClientTrusted(X509Certificate[] chain, String authType) {
			}

			public void checkServerTrusted(X509Certificate[] chain, String authType) {
			}

			public X509Certificate[] getAcceptedIssuers() {
				return new X509Certificate[0];
			}
		};
	}

	public static SSLContext getTrustAllContext() throws KeyManagementException, NoSuchAlgorithmException {

		SSLContext trustAllContext = SSLContext.getInstance("TLS");
		trustAllContext.init(null, new TrustManager[]{getTrustAllManager()}, new java.security.SecureRandom());
		return trustAllContext;
	}

	public static String getConsumerDid() throws Exception {
		return getDid(CONSUMER_DID_HELPER);
	}

	public static String getProviderDid() throws Exception {
		return getDid(PROVIDER_DID_HELPER);
	}

	private static String getDid(String host) throws Exception {
		Request keyRequest = new Request.Builder()
				.get()
				.url(host + DID_ENV_PATH)
				.build();
		Response didResponse = OK_HTTP_CLIENT.newCall(keyRequest).execute();
		String did = getDidFromEnv(didResponse.body().byteStream());
		didResponse.body().close();
		return did;
	}

	private static String getDidFromEnv(InputStream didInputStream) throws Exception {
		String didEnvString = new String(didInputStream.readAllBytes(), StandardCharsets.UTF_8);
		return didEnvString.split("=")[1];
	}

	private static KeyWrapper getKey(String host) throws Exception {
		Request keyRequest = new Request.Builder()
				.get()
				.url(host + PRIVATE_KEY_PATH)
				.build();
		Response keyResponse = OK_HTTP_CLIENT.newCall(keyRequest).execute();
		KeyWrapper keyWrapper = getKeyWrapper(keyResponse.body().byteStream());
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
