package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;

import java.io.IOException;
import java.util.List;
import java.util.Optional;

/**
 * Grants of one TIL's management API. A {@code null} scopeType addresses an untyped grant (e.g. an order) and
 * sends no {@code scopeType} parameter; a missing issuer reads as no grants, since a removal may delete it.
 */
public class ProviderTilClient {

    private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
    private static final ObjectMapper OBJECT_MAPPER = TestUtils.OBJECT_MAPPER;
    private static final MediaType JSON = MediaType.parse("application/json");
    private static final String SCOPE_PARAM = "scope";
    private static final String SCOPE_TYPE_PARAM = "scopeType";

    private final String tilAddress;

    public ProviderTilClient(String tilAddress) {
        this.tilAddress = tilAddress;
    }

    /** One grant an issuer holds credentials under. */
    public record Grant(String scope, String scopeType, int credentialCount) {
    }

    /** A credential entry of an issuer, sent back as read so a re-written grant is identical. */
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record Credential(String credentialsType, JsonNode validFor, JsonNode claims) {
    }

    private record Issuer(String did, List<Credential> credentials) {
    }

    /** {@code GET /issuer/{did}/scopes}. */
    public List<Grant> scopes(String did) throws IOException {
        return get(issuerUrl(did).addPathSegment("scopes").build())
                .map(body -> readValue(body, new TypeReference<List<Grant>>() {
                }))
                .orElse(List.of());
    }

    /** The credentials of {@code GET /issuer/{did}}, merged over every grant and the unscoped ones. */
    public List<Credential> credentials(String did) throws IOException {
        return get(issuerUrl(did).build())
                .map(body -> readValue(body, new TypeReference<Issuer>() {
                }))
                .map(Issuer::credentials)
                .orElse(List.of());
    }

    /** Sets a grant's credentials to exactly {@code credentials}; creates the issuer if needed. */
    public void putGrant(String did, String scope, String scopeType, List<Credential> credentials) throws IOException {
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(credentials), JSON);
        execute(new Request.Builder().put(body).url(grantUrl(did, scope, scopeType)).build(), HttpStatus.SC_OK);
    }

    /** Deletes one grant; the issuer stays. A grant already gone (e.g. revoked by CM with its order) is fine. */
    public void removeGrant(String did, String scope, String scopeType) throws IOException {
        Request request = new Request.Builder().delete().url(grantUrl(did, scope, scopeType)).build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            if (response.code() != HttpStatus.SC_NOT_FOUND) {
                successBody(response, HttpStatus.SC_NO_CONTENT);
            }
        }
    }

    private HttpUrl.Builder issuerUrl(String did) {
        return HttpUrl.get(tilAddress).newBuilder().addPathSegment("issuer").addPathSegment(did);
    }

    private HttpUrl grantUrl(String did, String scope, String scopeType) {
        HttpUrl.Builder url = issuerUrl(did).addPathSegment("credential").addQueryParameter(SCOPE_PARAM, scope);
        if (scopeType != null) {
            url.addQueryParameter(SCOPE_TYPE_PARAM, scopeType);
        }
        return url.build();
    }

    private Optional<String> get(HttpUrl url) throws IOException {
        try (Response response = HTTP_CLIENT.newCall(new Request.Builder().get().url(url).build()).execute()) {
            if (response.code() == HttpStatus.SC_NOT_FOUND) {
                return Optional.empty();
            }
            return Optional.of(successBody(response, HttpStatus.SC_OK));
        }
    }

    private void execute(Request request, int expectedStatus) throws IOException {
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            successBody(response, expectedStatus);
        }
    }

    private static String successBody(Response response, int expectedStatus) throws IOException {
        String body = response.body() == null ? "" : response.body().string();
        if (response.code() != expectedStatus) {
            throw new IOException(String.format("%s %s answered %d: %s",
                    response.request().method(), response.request().url(), response.code(), body));
        }
        return body;
    }

    private static <T> T readValue(String body, TypeReference<T> type) {
        try {
            return OBJECT_MAPPER.readValue(body, type);
        } catch (IOException e) {
            throw new IllegalStateException("Unreadable TIL response: " + body, e);
        }
    }
}
