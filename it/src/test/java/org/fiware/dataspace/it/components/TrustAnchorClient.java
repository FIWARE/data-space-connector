package org.fiware.dataspace.it.components;

import lombok.extern.slf4j.Slf4j;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;

import java.io.IOException;
import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Data space membership at the trust anchor: writes go to its TIL, reads to its read-only TIR.
 * Registration is idempotent; removal waits until the publisher's TIR watcher has seen the DID (D6).
 */
@Slf4j
public class TrustAnchorClient {

    private static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
    private static final MediaType JSON = MediaType.parse("application/json");

    private final String tilAddress;
    private final String tirAddress;
    private final ContractManagementManagementClient publisher;

    public TrustAnchorClient(String tilAddress, String tirAddress, ContractManagementManagementClient publisher) {
        this.tilAddress = tilAddress;
        this.tirAddress = tirAddress;
        this.publisher = publisher;
    }

    /** Whether the TIR lists {@code did}, i.e. it is a data space participant. */
    public boolean isListed(String did) throws IOException {
        HttpUrl url = HttpUrl.get(tirAddress).newBuilder()
                .addPathSegments("v4/issuers").addPathSegment(did).build();
        try (Response response = HTTP_CLIENT.newCall(new Request.Builder().get().url(url).build()).execute()) {
            return response.code() == HttpStatus.SC_OK;
        }
    }

    /** Registers {@code did} with no credentials, as the participants' init containers do; a 409 is fine. */
    public void registerParticipant(String did) throws IOException {
        String body = TestUtils.OBJECT_MAPPER.writeValueAsString(Map.of("did", did, "credentials", List.of()));
        Request request = new Request.Builder()
                .post(RequestBody.create(body, JSON))
                .url(HttpUrl.get(tilAddress).newBuilder().addPathSegment("issuer").build())
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertTrue(response.code() == HttpStatus.SC_CREATED || response.code() == HttpStatus.SC_CONFLICT,
                    String.format("Registering %s at the trust anchor answered %d.", did, response.code()));
        }
        assertTrue(isRegistered(did), String.format("%s should be registered at the trust anchor TIL.", did));
    }

    /** Re-registers {@code did} if a scenario removed it. */
    public void restoreParticipant(String did) throws IOException {
        if (!isRegistered(did)) {
            registerParticipant(did);
        }
    }

    /**
     * Removes a listed participant once the TIR watcher has a snapshot holding it, so the removal is announced.
     * Returns the instant right before the {@code DELETE}: report entries older than that are not this removal's.
     */
    public Instant removeParticipant(String did) throws IOException {
        assertTrue(isListed(did), String.format("%s must be listed at the TIR before it can be removed.", did));
        publisher.awaitWatcherSaw(Instant.now());
        Instant since = Instant.now();
        delete(did, false);
        return since;
    }

    /**
     * Removal no event can announce: the publisher must have no pod, so there is no watcher to wait for. Returns the
     * instant right before the {@code DELETE}, as {@link #removeParticipant} does.
     */
    public Instant removeParticipantWhilePublisherDown(String did) throws Exception {
        assertTrue(isListed(did), String.format("%s must be listed at the TIR before it can be removed.", did));
        List<String> pods = publisher.pods();
        assertTrue(pods.isEmpty(), "The publisher should have no pod: " + pods);
        Instant since = Instant.now();
        delete(did, false);
        return since;
    }

    /** Cleanup form of the removal: no watcher wait, an absent DID is fine. */
    public void removeIfPresent(String did) throws IOException {
        delete(did, true);
    }

    private boolean isRegistered(String did) throws IOException {
        try (Response response = HTTP_CLIENT.newCall(new Request.Builder().get().url(issuerUrl(did)).build()).execute()) {
            return response.code() == HttpStatus.SC_OK;
        }
    }

    private void delete(String did, boolean absentIsFine) throws IOException {
        try (Response response = HTTP_CLIENT.newCall(new Request.Builder().delete().url(issuerUrl(did)).build()).execute()) {
            boolean deleted = response.code() == HttpStatus.SC_NO_CONTENT;
            boolean absent = absentIsFine && response.code() == HttpStatus.SC_NOT_FOUND;
            assertTrue(deleted || absent,
                    String.format("Removing %s from the trust anchor answered %d.", did, response.code()));
        }
        log.debug("Removed {} from the trust anchor.", did);
    }

    private HttpUrl issuerUrl(String did) {
        return HttpUrl.get(tilAddress).newBuilder().addPathSegment("issuer").addPathSegment(did).build();
    }
}
