package io.aplfintech.grape.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.ToString;

@JsonPropertyOrder({"minGasLimit", "gasLimitBoundDivisor", "maxRefundQuotient"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@ToString
@EqualsAndHashCode
public class GasConfig {
    //Minimum the gas limit may ever be
    private Integer minGasLimit;
    //The bound divisor of the gas limit, used in update calculations
    private Integer gasLimitBoundDivisor;
    //Maximum refund quotient; max tx refund is min(tx.gasUsed/maxRefundQuotient, tx.gasRefund)
    private Integer maxRefundQuotient;

    @JsonCreator
    public GasConfig(@JsonProperty("minGasLimit") Integer minGasLimit,
                     @JsonProperty("gasLimitBoundDivisor") Integer gasLimitBoundDivisor,
                     @JsonProperty("maxRefundQuotient") Integer maxRefundQuotient) {
        this.minGasLimit = minGasLimit;
        this.gasLimitBoundDivisor = gasLimitBoundDivisor;
        this.maxRefundQuotient = maxRefundQuotient;
    }

    @JsonIgnore
    public boolean isValid() {
        return minGasLimit != null && minGasLimit > 0
            && gasLimitBoundDivisor != null && gasLimitBoundDivisor > 0
            && maxRefundQuotient != null && maxRefundQuotient > 0;
    }

    public void merge(GasConfig mergedConfig) {
        if (mergedConfig != null) {
            if (mergedConfig.minGasLimit != null) {
                this.minGasLimit = mergedConfig.minGasLimit;
            }
            if (mergedConfig.gasLimitBoundDivisor != null) {
                this.gasLimitBoundDivisor = mergedConfig.gasLimitBoundDivisor;
            }
            if (mergedConfig.maxRefundQuotient != null) {
                this.maxRefundQuotient = mergedConfig.maxRefundQuotient;
            }
        }
    }

    /**
     * Creates copy of the given gas config
     */
    public static GasConfig from(@NonNull GasConfig gasConfig) {
        return new GasConfig(gasConfig.minGasLimit, gasConfig.gasLimitBoundDivisor, gasConfig.maxRefundQuotient);
    }
}
