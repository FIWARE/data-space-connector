package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import lombok.extern.slf4j.Slf4j;
import okhttp3.MediaType;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.awaitility.Awaitility;
import org.fiware.dataspace.it.components.model.ContractNegotiation;
import org.fiware.dataspace.it.components.model.DataAddress;
import org.fiware.dataspace.it.components.model.IdResponse;
import org.fiware.dataspace.it.components.model.TransferProcess;

import java.time.Duration;
import java.util.List;

import static org.fiware.dataspace.it.components.TestUtils.OBJECT_MAPPER;
import static org.fiware.dataspace.it.components.TestUtils.OK_HTTP_CLIENT;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Helper class for interacting with the FDSC-EDC Management API in the DSP deployment profile.
 * <p>
 * Provides methods for catalog requests, contract negotiations, transfer processes, and
 * EDR (Endpoint Data Reference) retrieval. All API calls match the curl commands documented
 * in DSP_INTEGRATION.md "Order through DSP" section.
 * <p>
 * Supports both DCP and OID4VC management API endpoints by parameterizing the management
 * API base address.
 *
 * @see DSPEnvironment
 */
@Slf4j
public class DSPManagementHelper {

    /**
     * JSON media type for HTTP request bodies.
     */
    private static final MediaType JSON = MediaType.parse("application/json");

    /**
     * EDC Management API context URI.
     */
    private static final String EDC_MANAGEMENT_CONTEXT = "https://w3id.org/edc/connector/management/v0.0.1";

    /**
     * DSP protocol version used in all requests.
     */
    private static final String DSP_PROTOCOL = "dataspace-protocol-http:2025-1";

    /**
     * ODRL JSON-LD context URI used in policy objects.
     */
    private static final String ODRL_CONTEXT = "http://www.w3.org/ns/odrl.jsonld";

    /**
     * API path for catalog requests.
     */
    private static final String CATALOG_REQUEST_PATH = "/api/v1/management/v3/catalog/request";

    /**
     * API path for contract negotiation operations.
     */
    private static final String CONTRACT_NEGOTIATIONS_PATH = "/api/v1/management/v3/contractnegotiations";

    /**
     * API path for querying contract negotiations.
     */
    private static final String CONTRACT_NEGOTIATIONS_REQUEST_PATH = "/api/v1/management/v3/contractnegotiations/request";

    /**
     * API path for transfer process operations.
     */
    private static final String TRANSFER_PROCESSES_PATH = "/api/v1/management/v3/transferprocesses";

    /**
     * API path for querying transfer processes.
     */
    private static final String TRANSFER_PROCESSES_REQUEST_PATH = "/api/v1/management/v3/transferprocesses/request";

    /**
     * API path prefix for EDR data address retrieval; append /{transferId}/dataaddress.
     */
    private static final String EDRS_PATH_PREFIX = "/api/v1/management/v3/edrs/";

    /**
     * The HTTP port used by management API services in the local deployment.
     */
    private static final int SERVICE_PORT = 8080;

    /**
     * Negotiation state indicating a finalized contract agreement.
     */
    private static final String STATE_FINALIZED = "FINALIZED";

    /**
     * Transfer process state indicating the transfer has started.
     */
    private static final String STATE_STARTED = "STARTED";

    /**
     * Default timeout in seconds for polling negotiation and transfer states.
     */
    private static final long DEFAULT_POLL_TIMEOUT_SECONDS = 120;

    /**
     * Default poll interval in seconds between state checks.
     */
    private static final long DEFAULT_POLL_INTERVAL_SECONDS = 3;

    /**
     * Transfer type for HTTP data pull transfers.
     */
    public static final String TRANSFER_TYPE_HTTP_DATA_PULL = "HttpData-PULL";

    private DSPManagementHelper() {
        // prevent instantiation
    }

    /**
     * Requests the catalog from the provider via the DSP Management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST '{managementApiAddress}:8080/api/v1/management/v3/catalog/request' \
     *   --header 'Content-Type: application/json' \
     *   --data '{"@context":[...],"@type":"CatalogRequestMessage","protocol":"...","counterPartyId":"...","counterPartyAddress":"...","querySpec":{}}'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API (e.g., DCP or OID4VC)
     * @param counterPartyId       DID of the provider (e.g., {@code did:web:mp-operations.org})
     * @param counterPartyAddress  DSP endpoint of the provider (e.g., {@code http://dcp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1})
     * @return the catalog response as a parsed JSON tree
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static JsonNode requestCatalog(String managementApiAddress, String counterPartyId,
                                          String counterPartyAddress) throws Exception {
        ObjectNode requestBody = OBJECT_MAPPER.createObjectNode();
        ArrayNode context = OBJECT_MAPPER.createArrayNode();
        context.add(EDC_MANAGEMENT_CONTEXT);
        requestBody.set("@context", context);
        requestBody.put("@type", "CatalogRequestMessage");
        requestBody.put("protocol", DSP_PROTOCOL);
        requestBody.put("counterPartyId", counterPartyId);
        requestBody.put("counterPartyAddress", counterPartyAddress);
        requestBody.set("querySpec", OBJECT_MAPPER.createObjectNode());

        String url = managementApiAddress + ":" + SERVICE_PORT + CATALOG_REQUEST_PATH;
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(requestBody), JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "";
            log.info("Catalog request to {} returned status {}", url, response.code());
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Catalog request failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readTree(responseBody);
        }
    }

    /**
     * Starts a contract negotiation with the provider via the DSP Management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST '{managementApiAddress}:8080/api/v1/management/v3/contractnegotiations' \
     *   --header 'Content-Type: application/json' \
     *   --data '{"@context":[...],"@type":"ContractRequest","counterPartyAddress":"...","counterPartyId":"...","protocol":"...","policy":{...}}'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API
     * @param counterPartyAddress  DSP endpoint of the provider
     * @param counterPartyId       DID of the provider
     * @param offerId              the offer ID from the catalog (e.g., {@code OFFER-1:ASSET-1:123})
     * @param assetId              the target asset ID (e.g., {@code ASSET-1})
     * @param policy               the ODRL policy object (permissions, constraints) for the negotiation
     * @return the negotiation response as a parsed JSON tree
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static IdResponse startNegotiation(String managementApiAddress, String counterPartyAddress,
                                              String counterPartyId, String offerId, String assetId,
                                              Object policy) throws Exception {
        ObjectNode requestBody = OBJECT_MAPPER.createObjectNode();
        ArrayNode context = OBJECT_MAPPER.createArrayNode();
        context.add(EDC_MANAGEMENT_CONTEXT);
        requestBody.set("@context", context);
        requestBody.put("@type", "ContractRequest");
        requestBody.put("counterPartyAddress", counterPartyAddress);
        requestBody.put("counterPartyId", counterPartyId);
        requestBody.put("protocol", DSP_PROTOCOL);

        // Build the policy node
        JsonNode policyNode;
        if (policy instanceof JsonNode) {
            policyNode = (JsonNode) policy;
        } else {
            policyNode = OBJECT_MAPPER.valueToTree(policy);
        }

        // Ensure the policy has the required fields
        ObjectNode policyObj = policyNode.isObject() ? (ObjectNode) policyNode.deepCopy() : OBJECT_MAPPER.createObjectNode();
        if (!policyObj.has("@context")) {
            policyObj.put("@context", ODRL_CONTEXT);
        }
        if (!policyObj.has("@type")) {
            policyObj.put("@type", "Offer");
        }
        if (!policyObj.has("@id")) {
            policyObj.put("@id", offerId);
        }
        if (!policyObj.has("assigner")) {
            policyObj.put("assigner", counterPartyId);
        }
        if (!policyObj.has("target")) {
            policyObj.put("target", assetId);
        }

        requestBody.set("policy", policyObj);

        String url = managementApiAddress + ":" + SERVICE_PORT + CONTRACT_NEGOTIATIONS_PATH;
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(requestBody), JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "";
            log.info("Contract negotiation start at {} returned status {}", url, response.code());
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Contract negotiation start failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readValue(responseBody, IdResponse.class);
        }
    }

    /**
     * Queries the current contract negotiations from the DSP Management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST '{managementApiAddress}:8080/api/v1/management/v3/contractnegotiations/request' \
     *   --header 'Content-Type: application/json'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API
     * @return a list of {@link ContractNegotiation} objects representing current negotiations
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static List<ContractNegotiation> getNegotiations(String managementApiAddress) throws Exception {
        String url = managementApiAddress + ":" + SERVICE_PORT + CONTRACT_NEGOTIATIONS_REQUEST_PATH;
        RequestBody body = RequestBody.create("", JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "[]";
            log.debug("Get negotiations at {} returned status {}", url, response.code());
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Get negotiations failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readValue(responseBody,
                    OBJECT_MAPPER.getTypeFactory().constructCollectionType(List.class, ContractNegotiation.class));
        }
    }

    /**
     * Polls the contract negotiation state until it reaches "FINALIZED" and returns the agreement ID.
     * <p>
     * Uses Awaitility to poll the negotiations endpoint with a configurable timeout.
     * Once the negotiation is finalized, extracts and returns the {@code contractAgreementId}.
     *
     * @param managementApiAddress the base URL of the management API
     * @return the contract agreement ID from the finalized negotiation
     * @throws Exception if polling times out or no finalized negotiation is found
     */
    public static String waitForNegotiationFinalized(String managementApiAddress, String negotiationId) throws Exception {
        final String[] agreementId = new String[1];

        Awaitility.await()
                .atMost(Duration.ofSeconds(DEFAULT_POLL_TIMEOUT_SECONDS))
                .pollInterval(Duration.ofSeconds(DEFAULT_POLL_INTERVAL_SECONDS))
                .untilAsserted(() -> {
                    List<ContractNegotiation> negotiations = getNegotiations(managementApiAddress);
                    assertFalse(negotiations.isEmpty(), "Expected at least one negotiation");
                    log.debug("Get negotiation {}", negotiationId);
                    ContractNegotiation finalized = negotiations.stream()
                            .filter(n -> n.getAtId().equals(negotiationId))
                            .findFirst()
                            .orElse(null);

                    assertNotNull(finalized,
                            String.format("Expected a negotiation in state '%s', but found states: %s",
                                    STATE_FINALIZED,
                                    negotiations.stream().map(ContractNegotiation::getState)
                                            .reduce((a, b) -> a + ", " + b).orElse("none")));
                    assertTrue(finalized.getState().equalsIgnoreCase(STATE_FINALIZED), "The negotiation should be finalized.");

                    agreementId[0] = finalized.getContractAgreementId();
                    assertNotNull(agreementId[0], "Agreement ID should not be null when negotiation is finalized");
                    log.info("Negotiation finalized with agreement ID: {}", agreementId[0]);
                });

        return agreementId[0];
    }

    /**
     * Starts a transfer process via the DSP Management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST '{managementApiAddress}:8080/api/v1/management/v3/transferprocesses' \
     *   --header 'Content-Type: application/json' \
     *   --data '{"@context":[...],"assetId":"...","counterPartyId":"...","counterPartyAddress":"...","connectorId":"...","contractId":"...","protocol":"...","transferType":"HttpData-PULL"}'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API
     * @param assetId              the asset ID to transfer (e.g., {@code ASSET-1})
     * @param counterPartyId       DID of the provider
     * @param counterPartyAddress  DSP endpoint of the provider
     * @param contractId           the agreement ID from a finalized negotiation
     * @param transferType         the transfer type (e.g., {@code HttpData-PULL})
     * @return the transfer process response as a parsed JSON tree
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static IdResponse startTransferProcess(String managementApiAddress, String assetId,
                                                  String counterPartyId, String counterPartyAddress,
                                                  String contractId, String transferType) throws Exception {
        ObjectNode requestBody = OBJECT_MAPPER.createObjectNode();
        ArrayNode context = OBJECT_MAPPER.createArrayNode();
        context.add(EDC_MANAGEMENT_CONTEXT);
        requestBody.set("@context", context);
        requestBody.put("assetId", assetId);
        requestBody.put("counterPartyId", counterPartyId);
        requestBody.put("counterPartyAddress", counterPartyAddress);
        requestBody.put("connectorId", counterPartyId);
        requestBody.put("contractId", contractId);
        requestBody.put("protocol", DSP_PROTOCOL);
        requestBody.put("transferType", transferType);

        String url = managementApiAddress + ":" + SERVICE_PORT + TRANSFER_PROCESSES_PATH;
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(requestBody), JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "";
            log.info("Transfer process start at {} returned status {} - body: {}", url, response.code(), responseBody);
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Transfer process start failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readValue(responseBody, IdResponse.class);
        }
    }

    /**
     * Starts a transfer process via the OID4VC Management API, including a data destination.
     * <p>
     * This variant includes a {@code dataDestination} field with type {@code HttpProxy},
     * as documented in the OID4VC section of DSP_INTEGRATION.md.
     *
     * @param managementApiAddress the base URL of the OID4VC management API
     * @param assetId              the asset ID to transfer
     * @param counterPartyId       DID of the provider
     * @param counterPartyAddress  DSP endpoint of the provider (OID4VC variant)
     * @param contractId           the agreement ID from a finalized negotiation
     * @param transferType         the transfer type (e.g., {@code HttpData-PULL})
     * @return the transfer process response as a parsed JSON tree
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static IdResponse startTransferProcessWithDataDestination(String managementApiAddress, String assetId,
                                                                     String counterPartyId, String counterPartyAddress,
                                                                     String contractId, String transferType) throws Exception {
        ObjectNode requestBody = OBJECT_MAPPER.createObjectNode();
        ArrayNode context = OBJECT_MAPPER.createArrayNode();
        context.add(EDC_MANAGEMENT_CONTEXT);
        requestBody.set("@context", context);
        requestBody.put("assetId", assetId);
        requestBody.put("counterPartyId", counterPartyId);
        requestBody.put("counterPartyAddress", counterPartyAddress);
        requestBody.put("connectorId", counterPartyId);
        requestBody.put("contractId", contractId);

        ObjectNode dataDestination = OBJECT_MAPPER.createObjectNode();
        dataDestination.put("type", "HttpProxy");
        requestBody.set("dataDestination", dataDestination);

        requestBody.put("protocol", DSP_PROTOCOL);
        requestBody.put("transferType", transferType);

        String url = managementApiAddress + ":" + SERVICE_PORT + TRANSFER_PROCESSES_PATH;
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(requestBody), JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "";
            log.info("Transfer process (OID4VC) start at {} returned status {}", url, response.code());
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Transfer process start (OID4VC) failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readValue(responseBody, IdResponse.class);
        }
    }

    /**
     * Queries the current transfer processes from the DSP Management API.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X POST '{managementApiAddress}:8080/api/v1/management/v3/transferprocesses/request' \
     *   --header 'Content-Type: application/json' \
     *   --data '{"@context":[...],"@type":"QuerySpec"}'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API
     * @return a list of {@link TransferProcess} objects representing current transfers
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static List<TransferProcess> getTransferProcesses(String managementApiAddress) throws Exception {
        ObjectNode requestBody = OBJECT_MAPPER.createObjectNode();
        ArrayNode context = OBJECT_MAPPER.createArrayNode();
        context.add(EDC_MANAGEMENT_CONTEXT);
        requestBody.set("@context", context);
        requestBody.put("@type", "QuerySpec");

        String url = managementApiAddress + ":" + SERVICE_PORT + TRANSFER_PROCESSES_REQUEST_PATH;
        RequestBody body = RequestBody.create(OBJECT_MAPPER.writeValueAsString(requestBody), JSON);
        Request request = new Request.Builder()
                .url(url)
                .post(body)
                .header("Accept", "*/*")
                .header("Content-Type", "application/json")
                .build();

        try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
            String responseBody = response.body() != null ? response.body().string() : "[]";
            log.info("Get transfer processes at {} returned status {}", url, response.code());
            if (!response.isSuccessful()) {
                throw new RuntimeException(String.format(
                        "Get transfer processes failed with status %d: %s", response.code(), responseBody));
            }
            return OBJECT_MAPPER.readValue(responseBody,
                    OBJECT_MAPPER.getTypeFactory().constructCollectionType(List.class, TransferProcess.class));
        }
    }

    /**
     * Polls the transfer process state until it reaches "STARTED" and returns the transfer ID.
     * <p>
     * Uses Awaitility to poll the transfer processes endpoint with a configurable timeout.
     * Once a transfer is in the "STARTED" state, extracts and returns the transfer ID
     * (from the {@code @id} field).
     *
     * @param managementApiAddress the base URL of the management API
     * @return the transfer process ID from the started transfer
     * @throws Exception if polling times out or no started transfer is found
     */
    public static String waitForTransferStarted(String managementApiAddress, String transferId) throws Exception {
        final String[] transferIds = new String[1];

        Awaitility.await()
                .atMost(Duration.ofSeconds(DEFAULT_POLL_TIMEOUT_SECONDS))
                .pollInterval(Duration.ofSeconds(DEFAULT_POLL_INTERVAL_SECONDS))
                .untilAsserted(() -> {
                    List<TransferProcess> transfers = getTransferProcesses(managementApiAddress);
                    assertFalse(transfers.isEmpty(), "Expected at least one transfer process");

                    TransferProcess started = transfers.stream()
                            .filter(t -> t.getAtId().equals(transferId))
                            .findFirst()
                            .orElse(null);

                    assertNotNull(started,
                            String.format("Expected a transfer in state '%s', but found states: %s",
                                    STATE_STARTED,
                                    transfers.stream().map(TransferProcess::getState)
                                            .reduce((a, b) -> a + ", " + b).orElse("none")));
                    assertTrue(STATE_STARTED.equalsIgnoreCase(started.getState()), "The transfer is not started.");
                    transferIds[0] = started.getAtId();
                    assertNotNull(transferIds[0], "Transfer ID (@id) should not be null when transfer is started");
                    log.info("Transfer process started with ID: {}", transferIds[0]);
                });

        return transferIds[0];
    }

    /**
     * Retrieves the EDR (Endpoint Data Reference) data address for a completed transfer.
     * <p>
     * Equivalent to the curl command:
     * <pre>
     * curl -X GET '{managementApiAddress}:8080/api/v1/management/v3/edrs/{transferId}/dataaddress'
     * </pre>
     *
     * @param managementApiAddress the base URL of the management API
     * @param transferId           the transfer process ID
     * @return a {@link DataAddress} containing the endpoint URL and access token
     * @throws Exception if the HTTP request fails or returns a non-success status
     */
    public static DataAddress getDataAddress(String managementApiAddress, String transferId) throws Exception {
        String url = managementApiAddress + ":" + SERVICE_PORT + EDRS_PATH_PREFIX + transferId + "/dataaddress";
        final DataAddress[] dataAddress = new DataAddress[1];
        Awaitility.await()
                .atMost(Duration.ofSeconds(DEFAULT_POLL_TIMEOUT_SECONDS))
                .pollInterval(Duration.ofSeconds(DEFAULT_POLL_INTERVAL_SECONDS))
                .untilAsserted(() -> {
                    Request request = new Request.Builder()
                            .url(url)
                            .get()
                            .header("Accept", "*/*")
                            .build();

                    try (Response response = OK_HTTP_CLIENT.newCall(request).execute()) {
                        String responseBody = response.body() != null ? response.body().string() : "";
                        log.info("Get data address for transfer {} at {} returned status {}", transferId, url, response.code());
                        assertTrue(response.isSuccessful(), String.format(
                                "Get data address failed with status %d: %s", response.code(), responseBody));
                        dataAddress[0] = OBJECT_MAPPER.readValue(responseBody, DataAddress.class);
                        assertNotNull(dataAddress[0].getEndpoint(), "Data address endpoint should not be null");
                        log.info("Retrieved data address: endpoint={}", dataAddress[0].getEndpoint());
                    }
                });
        return dataAddress[0];
    }
}
