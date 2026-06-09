package io.aplfintech.grape.vm.opcode;

import io.aplfintech.grape.vm.ExecutionStatus;

/**
 * Result of function executing,
 * i.e. result of the opCode performance
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface FnExecResult {
    /**
     * Description of the exception, if any occurred
     */
    ExecutionStatus executionStatus();

    default boolean hasError() {
        return executionStatus() != null && executionStatus().isFailure();
    }

    default boolean isSuccess() {
        return !hasError();
    }

    /**
     * Return value from the contract
     */
    byte[] output();

}
