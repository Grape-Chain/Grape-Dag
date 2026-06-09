package io.aplfintech.grape.env;

import io.aplfintech.grape.model.Address;

import java.math.BigInteger;

/**
 * Transaction context
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface TxContext {
    /**
     * Returns the transaction Origin.
     * Used by the ORIGIN opCode
     *
     * @return the transaction Origin
     */
    Address getOrigin();

    /**
     * Returns the transaction gas price.
     * Used by the GASPRICE opCode
     *
     * @return the transaction gas price
     */
    BigInteger gasPrice();
}
