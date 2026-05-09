package io.aplfintech.luna.vm.contract;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;

import java.util.Map;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonIgnoreProperties(ignoreUnknown = true)
@Data
public class CompiledContractInfo {
    private String name;
    @JsonProperty("bytecode")
    private ByteCode byteCode;
    @JsonProperty("functionhashes")
    private Map<String, String> functionHashes;
}
