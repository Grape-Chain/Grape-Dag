package io.aplfintech.grape.vm.contract;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Addressable;

import java.math.BigInteger;

/**
 * The reference to the caller contract instance
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContractRef extends Addressable {

    /**
     * Returns the contract address,
     * i.e. self address
     */
    Address address();

    /**
     * Returns the contract call value (transaction amount)
     */
    BigInteger value();

}
