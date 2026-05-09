package io.aplfintech.luna.vm.contract;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;

/**
 * Contract details from Remix IDE
 * the Bytecode item
 */
@JsonIgnoreProperties(ignoreUnknown = true)
@Data
public class ByteCode {

    @JsonProperty("object")
    private String object;

    @JsonProperty("runtimeCode")
    private String runtimeCode;
}
