package io.aplfintech.luna.config;

/**
 * Gas price manager interface. Looks for gas price by item name.
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface GasPrice {
    /**
     * Looks for gas price by item name
     *
     * @param item given item name
     * @return gas price value
     */
    int lookForGasPrice(String item);
}
