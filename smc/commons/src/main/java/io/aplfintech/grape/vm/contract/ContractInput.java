package io.aplfintech.grape.vm.contract;

import java.math.BigInteger;

/**
 * A contract input data
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContractInput {
    byte[] data();

    BigInteger gasLimit();
}
