package io.aplfintech.grape.vm;

import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.opcode.FnExecResult;

/**
 * Result of executing the message by the VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface MessageResult extends FnExecResult {
    ContractResult contractResult();

    /**
     * The initial amount of the gas
     */
    long initGas();

    /**
     * The amount of gas to be refund
     */
    long refundGas();

    /**
     * The amount of the used gas
     */
    long usedGas();

    /**
     * The amount of the available gas,
     * i.e. remaining after execution
     */
    long gas();
}
