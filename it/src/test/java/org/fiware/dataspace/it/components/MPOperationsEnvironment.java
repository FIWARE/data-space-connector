package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.squareup.okhttp.OkHttpClient;
import com.squareup.okhttp.Request;
import com.squareup.okhttp.Response;
import org.apache.http.HttpStatus;
import org.fiware.dataspace.it.components.model.OpenIdConfiguration;

import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
public abstract class MPOperationsEnvironment {

    public static final String DID_PROVIDER_ADDRESS = "http://did-provider.127.0.0.1.nip.io:8080";
    public static final String PROVIDER_PAP_ADDRESS = "http://pap-provider.127.0.0.1.nip.io:8080";
    public static final String PROVIDER_API_ADDRESS = "http://mp-data-service.127.0.0.1.nip.io:8080";
    public static final String SCORPIO_ADDRESS = "http://scorpio-provider.127.0.0.1.nip.io:8080";

    public static final String OIDC_WELL_KNOWN_PATH = "/.well-known/openid-configuration";
    private static final OkHttpClient HTTP_CLIENT = new OkHttpClient();
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    public static OpenIdConfiguration getOpenIDConfiguration() throws Exception {
        Request wellKnownRequest = new Request.Builder().get()
                .url(PROVIDER_API_ADDRESS + OIDC_WELL_KNOWN_PATH)
                .build();
        Response wellKnownResponse = HTTP_CLIENT.newCall(wellKnownRequest).execute();
        assertEquals(HttpStatus.SC_OK, wellKnownResponse.code(), "The oidc config should have been returned.");
        return OBJECT_MAPPER.readValue(wellKnownResponse.body().string(), OpenIdConfiguration.class);
    }

}
