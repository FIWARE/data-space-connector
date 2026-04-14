package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.ws.rs.core.MediaType;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.fiware.dataspace.it.components.model.IssuerCredential;
import org.fiware.dataspace.it.components.model.Policy;
import org.fiware.dataspace.it.components.model.TILResponse;
import org.fiware.dataspace.it.components.model.TrustedIssuer;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.fiware.dataspace.it.components.MPOperationsEnvironment.PROVIDER_PAP_ADDRESS;

@Slf4j
public abstract class StepDefintions {

    protected static final OkHttpClient HTTP_CLIENT = TestUtils.OK_HTTP_CLIENT;
    protected static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    protected void cleanUpTIL() throws Exception {
        Request getTilEntries = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer")
                .build();
        List<String> trustedIssuers = new ArrayList<>();
        try (Response tilEntriesResponse = HTTP_CLIENT.newCall(getTilEntries).execute()) {
            String tilResponseBody = tilEntriesResponse.body().string();
            TILResponse tilResponse = OBJECT_MAPPER.readValue(tilResponseBody, TILResponse.class);
            tilResponse.getItems()
                    .forEach(trustedIssuers::add);
        }
        trustedIssuers
                .forEach(id -> {
                    Request deletionRequest = new Request.Builder()
                            .delete()
                            .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer/" + id)
                            .build();
                    try (Response deletionResponse = HTTP_CLIENT.newCall(deletionRequest).execute()) {
                        log.debug("Deleted {} - code {}", id, deletionResponse.code());
                    } catch (IOException e) {
                        log.warn("Was not able to delete issuer {}", id);
                    }
                });
    }

    protected void prepareTil() throws Exception {

        IssuerCredential legalPersonCredential = new IssuerCredential("LegalPersonCredential", List.of());
        IssuerCredential userCredential = new IssuerCredential("UserCredential", List.of());
        IssuerCredential membershipCredential = new IssuerCredential("MembershipCredential", List.of());

        TrustedIssuer consumerTrustedIssuer = new TrustedIssuer(FancyMarketplaceEnvironment.CONSUMER_DID, List.of(legalPersonCredential, userCredential, membershipCredential));
        TrustedIssuer providerTrustedIssuer = new TrustedIssuer(MPOperationsEnvironment.PROVIDER_DID, List.of(legalPersonCredential));

        RequestBody consumerCreateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(consumerTrustedIssuer), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request consumerCreate = new Request.Builder()
                .post(consumerCreateBody)
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer")
                .build();
        try (Response consumerResponse = HTTP_CLIENT.newCall(consumerCreate).execute()) {
            log.info("Updated consumer - code {}", consumerResponse.code());
        }

        RequestBody providerCreateBody = RequestBody.create(OBJECT_MAPPER.writeValueAsString(providerTrustedIssuer), okhttp3.MediaType.parse(MediaType.APPLICATION_JSON));
        Request providerCreate = new Request.Builder()
                .post(providerCreateBody)
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer")
                .build();
        try (Response providerResponse = HTTP_CLIENT.newCall(providerCreate).execute()) {
            log.info("Updated provider - code {}", providerResponse.code());
        }

        Request getTilEntries = new Request.Builder()
                .get()
                .url(MPOperationsEnvironment.TIL_DIRECT_ADDRESS + "/issuer/" + FancyMarketplaceEnvironment.CONSUMER_DID)
                .build();
        try (Response tilEntriesResponse = HTTP_CLIENT.newCall(getTilEntries).execute()) {
            String tilResponseBody = tilEntriesResponse.body().string();
            log.warn("Consumer is registered: {}", tilResponseBody);
        }
    }

    /**
     * Generic helper to list and delete all TMForum resources at a given base URL and API path.
     *
     * @param baseUrl      the base URL of the TMForum API
     * @param apiPath      the TMForum API path
     * @param resourceName a human-readable name for logging purposes
     */
    protected void cleanUpTMForumResourceList(String baseUrl, String apiPath, String resourceName) {
        log.warn("Clean {}", resourceName);
        try {
            Request listRequest = new Request.Builder()
                    .get()
                    .url(baseUrl + apiPath)
                    .build();
            try (Response response = HTTP_CLIENT.newCall(listRequest).execute()) {
                okhttp3.ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    log.warn("No {} to clean up (status={})", resourceName, response.code());
                    return;
                }
                String bodyString = responseBody.string();
                List<java.util.Map<String, Object>> items;
                try {
                    items = OBJECT_MAPPER.readValue(bodyString,
                            new TypeReference<List<Map<String, Object>>>() {
                            });
                } catch (Exception e) {
                    log.warn("Could not parse {} list: {}", resourceName, e.getMessage());
                    return;
                }
                for (java.util.Map<String, Object> item : items) {
                    Object id = item.get("id");
                    if (id == null) continue;
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(baseUrl + apiPath + "/" + id)
                            .build();
                    try (Response deleteResp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.warn("Deleted {} {}: status={}", resourceName, id, deleteResp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up {}: {}", resourceName, e.getMessage());
        }
    }

    /**
     * Cleans up all policies from the provider's Policy Administration Point (PAP).
     */
    protected void cleanUpPolicies(String address) {
        try {
            Request getPolicies = new Request.Builder()
                    .get()
                    .url(address + "/policy")
                    .build();
            try (Response response = HTTP_CLIENT.newCall(getPolicies).execute()) {
                okhttp3.ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    return;
                }
                String bodyString = responseBody.string();
                List<Policy> policies;
                try {
                    policies = OBJECT_MAPPER.readValue(bodyString, new TypeReference<List<Policy>>() {
                    });
                } catch (Exception e) {
                    log.warn("Could not parse policies response: {}", e.getMessage());
                    return;
                }
                for (Policy policy : policies) {
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(address + "/policy/" + policy.getId())
                            .build();
                    try (Response deleteResp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.debug("Deleted policy {}: status={}", policy.getId(), deleteResp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up DSP policies: {}", e.getMessage());
        }
    }
}
