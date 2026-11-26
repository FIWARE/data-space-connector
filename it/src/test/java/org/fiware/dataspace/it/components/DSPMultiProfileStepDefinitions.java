package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import lombok.extern.slf4j.Slf4j;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.apache.http.HttpStatus;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

import static org.fiware.dataspace.it.components.DSPEnvironment.*;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Steps for dsp_multi_profile.feature: one DCP participant context attached to two dataspace profiles.
 * Identity and catalog setup steps come from {@link DSPStepDefinitions}, which also runs the {@code @dsp} cleanup.
 */
@Slf4j
public class DSPMultiProfileStepDefinitions extends StepDefintions {

    /** DSP protocol base of the provider's DCP context; a profile name follows it. */
    private static final String PROVIDER_CONTEXT_DSP_ADDRESS = DCP_PROVIDER_ADDRESS + "/api/dsp/" + PROVIDER_CONTEXT_ID;

    private static final String CATALOG_REQUEST_PATH = "/catalog/request";
    private static final String DSPACE_VERSION_PATH = "/.well-known/dspace-version";
    private static final String PROFILE_SEPARATOR = ",";
    private static final int LOGGED_BODY_LENGTH = 1000;

    private JsonNode catalogResponse;
    private int tokenlessCatalogStatus;
    private JsonNode versionMetadata;

    /** Requests the provider catalog through one profile; protocol and counterparty address both name it. */
    @When("The consumer requests the provider catalog through the DSP profile {string}.")
    public void theConsumerRequestsTheProviderCatalogThroughProfile(String profile) throws Exception {
        catalogResponse = DSPManagementHelper.requestCatalog(DCP_MANAGEMENT_API_ADDRESS, PROVIDER_CONTEXT_ID,
                PROVIDER_CONTEXT_DSP_ADDRESS + "/" + profile, profile);
        assertNotNull(catalogResponse, "Catalog response through profile " + profile + " should not be null.");
    }

    /** Checks the catalog read through a profile carries the offering set up by the provider. */
    @Then("The catalog read through the DSP profile contains at least one dataset.")
    public void theCatalogReadThroughTheProfileContainsADataset() {
        assertNotNull(catalogResponse, "The catalog must have been requested first.");
        assertTrue(DSPManagementHelper.containsDataset(catalogResponse),
                "The catalog should contain at least one dataset. Catalog: " + truncate(catalogResponse.toString()));
    }

    /** Sends a bodyless, tokenless catalog request straight to the provider's DSP endpoint, as the upstream docs do. */
    @When("A tokenless catalog request is sent to the provider through the DSP profile {string}.")
    public void aTokenlessCatalogRequestIsSent(String profile) throws Exception {
        Request request = new Request.Builder()
                .url(PROVIDER_CONTEXT_DSP_ADDRESS + "/" + profile + CATALOG_REQUEST_PATH)
                .post(RequestBody.create(new byte[0]))
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            tokenlessCatalogStatus = response.code();
            log.debug("Tokenless catalog request through {} answered {}.", profile, tokenlessCatalogStatus);
        }
    }

    /** 401 means the profile is attached and only the token is missing; 404 means it is not attached. */
    @Then("The provider answers the tokenless catalog request with status {int}.")
    public void theProviderAnswersWithStatus(int expectedStatus) {
        assertEquals(expectedStatus, tokenlessCatalogStatus,
                "Unexpected status for the tokenless catalog request.");
    }

    /** Fetches the per-context version metadata, which needs no token. */
    @When("The provider context's DSP version metadata is requested without a token.")
    public void theProviderVersionMetadataIsRequested() throws Exception {
        Request request = new Request.Builder()
                .url(PROVIDER_CONTEXT_DSP_ADDRESS + DSPACE_VERSION_PATH)
                .get()
                .build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            String body = response.body().string();
            assertEquals(HttpStatus.SC_OK, response.code(),
                    "The version metadata should be served without a token. Body: " + truncate(body));
            versionMetadata = OBJECT_MAPPER.readTree(body);
        }
    }

    /** Compares the advertised profiles with the expected comma-separated list, ignoring order. */
    @Then("The DSP version metadata lists exactly the profiles {string}.")
    public void theVersionMetadataListsExactlyTheProfiles(String expectedProfiles) {
        assertNotNull(versionMetadata, "The version metadata must have been requested first.");
        Set<String> expected = new HashSet<>(Arrays.asList(expectedProfiles.split(PROFILE_SEPARATOR)));
        Set<String> advertised = new HashSet<>();
        versionMetadata.path("protocolVersions").forEach(version -> advertised.add(version.path("profile").asText()));
        assertEquals(expected, advertised,
                "The context should advertise exactly its profiles. Metadata: " + truncate(versionMetadata.toString()));
    }

    private static String truncate(String text) {
        return text.substring(0, Math.min(text.length(), LOGGED_BODY_LENGTH));
    }
}
