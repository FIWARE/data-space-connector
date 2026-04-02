package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Collections;
import java.util.List;

/**
 * Request body for DSP contract negotiation sent to the EDC Management API.
 * Corresponds to the ContractRequest structure documented in DSP_INTEGRATION.md.
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class ContractRequest {

    /** JSON-LD context for the EDC management API. */
    @JsonProperty("@context")
    private List<String> context = Collections.singletonList("https://w3id.org/edc/connector/management/v0.0.1");

    /** JSON-LD type identifier for this message. */
    @JsonProperty("@type")
    private String type = "ContractRequest";

    /** DSP endpoint address of the counter-party (provider). */
    private String counterPartyAddress;

    /** DID of the counter-party (provider). */
    private String counterPartyId;

    /** The DSP protocol version to use. */
    private String protocol = "dataspace-protocol-http:2025-1";

    /** ODRL policy for the contract negotiation, including offer ID, assigner, permissions, and target. */
    private Object policy;
}
