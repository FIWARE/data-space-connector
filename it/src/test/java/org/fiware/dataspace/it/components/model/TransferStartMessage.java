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
public class TransferStartMessage {

	@JsonProperty("@context")
	private String context = "https://w3id.org/dspace/2024/1/context.json";
	@JsonProperty("@type")
	private String type = "dspace:TransferRequestMessage";

	@JsonProperty("dspace:consumerPid")
	private String consumerPid;
	@JsonProperty("dspace:agreementId")
	private String agreementId;
	@JsonProperty("dct:format")
	private String format = "http+pull";
	@JsonProperty("dspace:callbackAddress")
	private String callbackAddress;

}
