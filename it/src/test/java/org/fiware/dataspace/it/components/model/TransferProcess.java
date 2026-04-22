package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Response from transfer process queries on the EDC Management API.
 * Represents the state of a data transfer process.
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class TransferProcess {

    /** JSON-LD identifier of the transfer process. */
    @JsonProperty("@id")
    private String atId;

    /** Internal identifier of the transfer process. */
    private String id;

    /** Current state of the transfer (e.g., "requested", "started", "completed"). */
    private String state;
}
