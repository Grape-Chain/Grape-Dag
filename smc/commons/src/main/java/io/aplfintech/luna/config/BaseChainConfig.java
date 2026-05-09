package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.ToString;

/**
 * The chain config for hard forks
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"gasConfig", "vm", "gasPrice"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@ToString
@EqualsAndHashCode
public class BaseChainConfig {
    @JsonProperty("gasConfig")
    protected final GasConfig gasConfig;
    @JsonProperty("vm")
    protected final VmConfig vmConfig;
    @JsonProperty("gasPrice")
    protected final GasPriceMap gasPriceMap;

    @JsonCreator
    public BaseChainConfig(@JsonProperty("gasConfig") GasConfig gasConfig,
                           @JsonProperty("vm") VmConfig vmConfig,
                           @JsonProperty("gasPrice") GasPriceMap gasPriceMap
    ) {
        this.gasConfig = gasConfig;
        this.gasPriceMap = gasPriceMap;
        this.vmConfig = vmConfig;
    }

    @JsonIgnore
    public boolean isValid() {
        return gasConfig != null && gasConfig.isValid()
            && vmConfig != null && vmConfig.isValid()
            && gasPriceMap != null && gasPriceMap.isValid();
    }

}
