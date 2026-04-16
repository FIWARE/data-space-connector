package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Slf4j
public abstract class MPOperationsEnvironment {

    public static final String PROVIDER_DID = "did:web:mp-operations.org";
    /**
     * Base URL of the provider's Keycloak instance for credential issuance.
     */
    public static final String PROVIDER_KEYCLOAK_ADDRESS = "https://keycloak-provider.127.0.0.1.nip.io";
    public static final String PROVIDER_PAP_ADDRESS = "http://pap-provider.127.0.0.1.nip.io";
    public static final String PROVIDER_API_ADDRESS = "https://mp-data-service.127.0.0.1.nip.io";
    public static final String TM_FORUM_API_ADDRESS = "http://mp-tmf-api.127.0.0.1.nip.io";
    // dataservice address with transfer process protocol enforcement
    public static final String PROVIDER_TPP_DATA_API_ADDRESS = "http://tpp-data-service.127.0.0.1.nip.io";
    public static final String PROVIDER_TPP_API_ADDRESS = "http://tpp-service.127.0.0.1.nip.io";

    // direct access endpoints
    public static final String SCORPIO_ADDRESS = "http://scorpio-provider.127.0.0.1.nip.io";
    public static final String TMF_DIRECT_ADDRESS = "http://tm-forum-api.127.0.0.1.nip.io";
    public static final String CONSUMER_TMF_DIRECT_ADDRESS = "http://consumer-tmf.127.0.0.1.nip.io";
    public static final String TIL_DIRECT_ADDRESS = "http://til-provider.127.0.0.1.nip.io";
    public static final String CCS_DIRECT_ADDRESS = "http://provider-ccs.127.0.0.1.nip.io";
    public static final String RAINBOW_DIRECT_ADDRESS = "http://rainbow-provider.127.0.0.1.nip.io";
    public static final String OIDC_WELL_KNOWN_PATH = "/.well-known/openid-configuration";
    private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    public static OpenIdConfiguration getOpenIDConfiguration(String targetHost) throws Exception {
        Request wellKnownRequest = new Request.Builder().get()
                .url(targetHost + OIDC_WELL_KNOWN_PATH)
                .build();
        Response wellKnownResponse = HTTP_CLIENT.newCall(wellKnownRequest).execute();
        try {
            log.warn("The request: " + wellKnownRequest.toString());
            assertEquals(HttpStatus.SC_OK, wellKnownResponse.code(), "The oidc config should have been returned.");
            return OBJECT_MAPPER.readValue(wellKnownResponse.body().string(), OpenIdConfiguration.class);
        } finally {
            wellKnownResponse.body().close();
        }
    }

}
