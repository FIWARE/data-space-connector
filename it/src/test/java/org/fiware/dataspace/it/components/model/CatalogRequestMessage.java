package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Collections;
import java.util.List;
import java.util.Map;

/**
 * Request body for DSP catalog requests sent to the EDC Management API.
 * Corresponds to the CatalogRequestMessage structure documented in DSP_INTEGRATION.md.
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class CatalogRequestMessage {

    /** JSON-LD context for the EDC management API. */
    @JsonProperty("@context")
    private List<String> context = Collections.singletonList("https://w3id.org/edc/connector/management/v0.0.1");

    /** JSON-LD type identifier for this message. */
    @JsonProperty("@type")
    private String type = "CatalogRequestMessage";

    /** The DSP protocol version to use. */
    private String protocol = "dataspace-protocol-http:2025-1";

    /** DID of the counter-party (provider). */
    private String counterPartyId;

    /** DSP endpoint address of the counter-party (provider). */
    private String counterPartyAddress;

    /** Query specification for filtering catalog results. Empty object means no filter. */
    private Map<String, Object> querySpec = Collections.emptyMap();
}
