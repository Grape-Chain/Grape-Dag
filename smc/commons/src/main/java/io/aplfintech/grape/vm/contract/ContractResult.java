package io.aplfintech.grape.vm.contract;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.vm.opcode.FnExecResult;

/**
 * Result of executing the smart contract
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContractResult extends FnExecResult {
    /**
     * Returns the created contract address
     * Used by opcodes CREATE and CREATE2
     *
     * @return the created contract address
     */
    Address contract();

    /**
     * Returns the Amount of the remaining gas
     *
     * @return the Amount of the remaining gas
     */
    long gas();

    /**
     * Returns the amount of the used gas
     *
     * @return the amount of the used gas
     */
    long gasUsed();

    /**
     * Reset all unused gas
     */
    void resetGas();

    /**
     * Returns the list of events emitted by the contract
     */
    Log[] getEventLog();

    /**
     * Returns error description for the failed execution status
     */
    String errorDescription();
}
