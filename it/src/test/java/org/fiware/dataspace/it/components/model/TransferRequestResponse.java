package org.fiware.dataspace.it.components.model;


import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class TransferRequestResponse {

	@JsonProperty("dspace:consumerPid")
	private String consumerPid;
	@JsonProperty("dspace:providerPid")
	private String providerPid;
}
