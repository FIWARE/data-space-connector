package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Response from contract negotiation queries on the EDC Management API.
 * Represents the state and result of a contract negotiation process.
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class ContractNegotiation {

    /** JSON-LD identifier of the negotiation. */
    @JsonProperty("@id")
    private String atId;

    /** Internal identifier of the negotiation. */
    private String id;

    /** Current state of the negotiation (e.g., "requested", "finalized"). */
    private String state;

    /** The agreement ID assigned once the negotiation is finalized. */
    private String contractAgreementId;
}
