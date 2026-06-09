package io.aplfintech.grape.vm;

import io.aplfintech.grape.vm.opcode.FnExecResult;

/**
 * Precompiled contracts interface
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface PrecompiledFn {
    /**
     * Returns the required gas for contract execution
     *
     * @param input given input
     * @return the required gas for contract performing
     */
    long requiredGas(byte[] input);

    /**
     * Runs the precompiled contract and returns the result
     *
     * @param input contract input
     * @return result of contract execution
     */
    FnExecResult run(byte[] input);

}
