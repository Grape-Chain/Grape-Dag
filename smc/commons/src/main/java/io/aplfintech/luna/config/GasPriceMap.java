package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonIgnore;
import lombok.NonNull;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

import java.util.HashMap;
import java.util.Map;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class GasPriceMap implements GasPrice, Map<String, PriceItem> {
    @Delegate
    private Map<String, PriceItem> gasPriceMap;

    public GasPriceMap() {
        this.gasPriceMap = new HashMap<>();
    }

    public GasPriceMap(@NonNull Map<String, PriceItem> gasPriceMap) {
        this.gasPriceMap = new HashMap<>(gasPriceMap);
    }

    /**
     * Looks for gas price by item name
     *
     * @param item given item name
     * @return gas price value
     * @throws IllegalStateException in case the requested item not found
     */
    @Override
    public int lookForGasPrice(String item) {
        var price = gasPriceMap.get(item);
        if (price == null) {
            var msg = "Can't locate the gas price for item=" + item;
            log.error(msg);
            throw new IllegalStateException(msg);
        }
        return price.getValue();
    }

    @JsonIgnore
    public boolean isValid() {
        return gasPriceMap != null && !gasPriceMap.isEmpty();
    }

    public void merge(GasPriceMap mergedGsPrice) {
        if (mergedGsPrice != null) {
            this.gasPriceMap.putAll(mergedGsPrice);
        }
    }

    @Override
    public String toString() {
        var json = ConfigHelper.writeValueAsString(this);
        return "gasPriceMap=" + json;
    }

    /**
     * Creates copy of the given gas price map
     */
    public static GasPriceMap from(GasPriceMap gasPriceMap) {
        return new GasPriceMap(gasPriceMap);
    }

}
