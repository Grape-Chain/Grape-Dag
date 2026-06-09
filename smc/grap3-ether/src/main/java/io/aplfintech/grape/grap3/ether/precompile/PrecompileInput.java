package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.vm.contract.ContractInput;

import java.math.BigInteger;

/**
 * Input for precompiled contracts
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */

public record PrecompileInput(byte[] data, BigInteger gasLimit) implements ContractInput {

}
