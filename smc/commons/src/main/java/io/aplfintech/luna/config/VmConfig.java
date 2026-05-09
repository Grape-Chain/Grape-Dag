package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.ToString;

/**
 * The VM config
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"callCreateDepth", "maxCodeSize"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@EqualsAndHashCode
@ToString
public class VmConfig {
    //Maximum depth of call/create stack
    private Integer callCreateDepth;
    //Maximum length of contract code
    private Integer maxCodeSize;

    @JsonCreator
    public VmConfig(@JsonProperty("callCreateDepth") Integer callCreateDepth,
                    @JsonProperty("maxCodeSize") Integer maxCodeSize) {
        this.callCreateDepth = callCreateDepth;
        this.maxCodeSize = maxCodeSize;
    }

    @JsonIgnore
    public boolean isValid() {
        return callCreateDepth != null && callCreateDepth > 0
            && maxCodeSize != null && maxCodeSize > 0;
    }

    public void merge(VmConfig mergedConfig) {
        if (mergedConfig != null) {
            if (mergedConfig.callCreateDepth != null) {
                this.callCreateDepth = mergedConfig.callCreateDepth;
            }
            if (mergedConfig.maxCodeSize != null) {
                this.maxCodeSize = mergedConfig.maxCodeSize;
            }
        }
    }

    /**
     * Creates copy of the given VM config
     */
    public static VmConfig from(@NonNull VmConfig vmConfig) {
        return new VmConfig(vmConfig.callCreateDepth, vmConfig.maxCodeSize);
    }
}
