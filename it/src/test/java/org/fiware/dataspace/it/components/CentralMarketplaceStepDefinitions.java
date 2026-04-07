package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import lombok.extern.slf4j.Slf4j;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.fiware.dataspace.it.components.model.Policy;
import org.keycloak.common.crypto.CryptoIntegration;

import java.security.Security;
import java.util.List;

import static org.fiware.dataspace.it.components.CentralMarketplaceEnvironment.*;
import static org.fiware.dataspace.it.components.FancyMarketplaceEnvironment.CONSUMER_DID;
import static org.fiware.dataspace.it.components.MPOperationsEnvironment.*;
import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.fiware.dataspace.it.components.TestUtils.OK_HTTP_CLIENT;

/**
 * Cucumber step definitions and lifecycle hooks for the Central Marketplace scenarios
 * tagged with {@code @central}.
 * <p>
 * Provides {@link Before} and {@link After} hooks that clean up central marketplace-specific
 * resources (TMForum entities at the central marketplace, contract management policies, and
 * TIL entries) to ensure test isolation.
 *
 * @see CentralMarketplaceEnvironment
 */
@Slf4j
public class CentralMarketplaceStepDefinitions {

    /** HTTP port used by local services in the k3s deployment. */
    private static final int SERVICE_PORT = 8080;

    /** TMForum API path for product order management. */
    private static final String TMF_PRODUCT_ORDER_PATH = "/tmf-api/productOrderingManagement/v4/productOrder";

    /** TMForum API path for product offering catalog management. */
    private static final String TMF_PRODUCT_OFFERING_PATH = "/tmf-api/productCatalogManagement/v4/productOffering";

    /** TMForum API path for product specification catalog management. */
    private static final String TMF_PRODUCT_SPEC_PATH = "/tmf-api/productCatalogManagement/v4/productSpecification";

    /** TMForum API path for party (organization) management. */
    private static final String TMF_ORGANIZATION_PATH = "/tmf-api/party/v4/organization";

    private static final OkHttpClient HTTP_CLIENT = OK_HTTP_CLIENT;

    /**
     * Setup hook executed before each {@code @central} scenario.
     * Initializes cryptographic providers and cleans up stale resources.
     */
    @Before("@central")
    public void setupCentral() {
        CryptoIntegration.init(this.getClass().getClassLoader());
        Security.addProvider(new BouncyCastleProvider());
        log.info("Central marketplace test setup: cleaning up stale resources.");
        try {
            cleanCentralResources();
        } catch (Exception e) {
            log.warn("Error during central marketplace pre-test cleanup: {}", e.getMessage());
        }
    }

    /**
     * Cleanup hook executed after each {@code @central} scenario.
     * Removes all central marketplace-specific resources to ensure test isolation.
     */
    @After("@central")
    public void cleanUpCentral() {
        log.info("Central marketplace test cleanup: removing test resources.");
        try {
            cleanCentralResources();
        } catch (Exception e) {
            log.warn("Error during central marketplace post-test cleanup: {}", e.getMessage());
        }
    }

    /**
     * Cleans up all resources created during central marketplace tests.
     * Handles partial failures gracefully by catching and logging exceptions for each resource type.
     */
    private void cleanCentralResources() {
        cleanUpCentralMarketplaceTMForum();
        cleanUpProviderTMForum();
        cleanUpProviderTIL();
        cleanUpProviderPolicies();
    }

    /**
     * Cleans up TMForum resources at the central marketplace endpoint.
     * Removes product orders, product offerings, product specifications, and organizations.
     */
    private void cleanUpCentralMarketplaceTMForum() {
        String marketplaceBase = MARKETPLACE_API_ADDRESS + ":" + SERVICE_PORT;
        cleanUpTMForumResourceList(marketplaceBase,
                TMF_PRODUCT_ORDER_PATH, "central marketplace orders");
        cleanUpTMForumResourceList(marketplaceBase,
                TMF_PRODUCT_OFFERING_PATH, "central marketplace offerings");
        cleanUpTMForumResourceList(marketplaceBase,
                TMF_PRODUCT_SPEC_PATH, "central marketplace specs");
        cleanUpTMForumResourceList(marketplaceBase,
                TMF_ORGANIZATION_PATH, "central marketplace organizations");
    }

    /**
     * Cleans up TMForum resources at the provider's direct TMForum API.
     * Removes orders, offerings, specifications, agreements, and organizations
     * that may have been created via contract management notifications.
     */
    private void cleanUpProviderTMForum() {
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                TMF_PRODUCT_ORDER_PATH, "provider orders");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                TMF_PRODUCT_OFFERING_PATH, "provider offerings");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                TMF_PRODUCT_SPEC_PATH, "provider specs");
        cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
                TMF_ORGANIZATION_PATH, "provider organizations");
    }

    /**
     * Cleans up the consumer's entry from the provider's Trusted Issuers List (TIL).
     * After cleanup, re-registers the consumer with the default UserCredential trust entry
     * to restore the baseline state.
     */
    private void cleanUpProviderTIL() {
        try {
            Request deleteRequest = new Request.Builder()
                    .delete()
                    .url(TIL_DIRECT_ADDRESS + "/issuer/" + CONSUMER_DID)
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                log.debug("TIL cleanup for consumer DID: status={}", resp.code());
            }

            // Re-register baseline TIL entry for consumer
            String tilPayload = OBJECT_MAPPER.writeValueAsString(
                    java.util.Map.of(
                            "did", CONSUMER_DID,
                            "credentials", java.util.List.of(
                                    java.util.Map.of(
                                            "credentialsType", "UserCredential",
                                            "claims", java.util.List.of()))));
            RequestBody body = RequestBody.create(tilPayload,
                    okhttp3.MediaType.parse("application/json"));
            Request postRequest = new Request.Builder()
                    .post(body)
                    .url(TIL_DIRECT_ADDRESS + "/issuer")
                    .build();
            try (Response resp = HTTP_CLIENT.newCall(postRequest).execute()) {
                log.debug("TIL baseline restore for consumer DID: status={}", resp.code());
            }
        } catch (Exception e) {
            log.warn("Failed to clean up provider TIL entries: {}", e.getMessage());
        }
    }

    /**
     * Cleans up all policies from the provider's Policy Administration Point (PAP).
     */
    private void cleanUpProviderPolicies() {
        try {
            Request getPolicies = new Request.Builder()
                    .get()
                    .url(PROVIDER_PAP_ADDRESS + "/policy")
                    .build();
            try (Response response = HTTP_CLIENT.newCall(getPolicies).execute()) {
                ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    log.warn("Could not retrieve policies for cleanup: status={}", response.code());
                    return;
                }
                String bodyString = responseBody.string();
                List<Policy> policies;
                try {
                    policies = OBJECT_MAPPER.readValue(bodyString, new TypeReference<List<Policy>>() {});
                } catch (Exception e) {
                    log.warn("Could not parse policies response: {}", e.getMessage());
                    return;
                }
                for (Policy policy : policies) {
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(PROVIDER_PAP_ADDRESS + "/policy/" + policy.getId())
                            .build();
                    try (Response deleteResp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.debug("Deleted policy {}: status={}", policy.getId(), deleteResp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up provider policies: {}", e.getMessage());
        }
    }

    /**
     * Generic helper to list and delete all TMForum resources at a given base URL and API path.
     *
     * @param baseUrl      the base URL of the TMForum API (e.g., central marketplace or provider direct)
     * @param apiPath      the TMForum API path (e.g., {@code /tmf-api/productCatalogManagement/v4/productOffering})
     * @param resourceName a human-readable name for logging purposes
     */
    private void cleanUpTMForumResourceList(String baseUrl, String apiPath, String resourceName) {
        try {
            Request listRequest = new Request.Builder()
                    .get()
                    .url(baseUrl + apiPath)
                    .build();
            try (Response response = HTTP_CLIENT.newCall(listRequest).execute()) {
                ResponseBody responseBody = response.body();
                if (responseBody == null || !response.isSuccessful()) {
                    log.debug("No {} to clean up (status={})", resourceName, response.code());
                    return;
                }
                String bodyString = responseBody.string();
                List<java.util.Map<String, Object>> items;
                try {
                    items = OBJECT_MAPPER.readValue(bodyString,
                            new TypeReference<List<java.util.Map<String, Object>>>() {});
                } catch (Exception e) {
                    log.warn("Could not parse {} list: {}", resourceName, e.getMessage());
                    return;
                }
                for (java.util.Map<String, Object> item : items) {
                    Object id = item.get("id");
                    if (id == null) {
                        continue;
                    }
                    Request deleteRequest = new Request.Builder()
                            .delete()
                            .url(baseUrl + apiPath + "/" + id)
                            .build();
                    try (Response deleteResp = HTTP_CLIENT.newCall(deleteRequest).execute()) {
                        log.debug("Deleted {} {}: status={}", resourceName, id, deleteResp.code());
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Failed to clean up {}: {}", resourceName, e.getMessage());
        }
    }
}
