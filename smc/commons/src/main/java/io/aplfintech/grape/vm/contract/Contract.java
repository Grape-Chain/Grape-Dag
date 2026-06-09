package io.aplfintech.grape.vm.contract;

import io.aplfintech.grape.model.Address;

/**
 * The contract representation in the state
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Contract extends GasValve, ContractRef {

    /**
     * Returns the caller address
     */
    Address caller();

    /**
     * Returns the contract code
     *
     * @return the contract code
     */
    Code code();

    /**
     * Returns the contract input
     * i.e. contract parameters
     *
     * @return the contract input
     */
    byte[] getInput();

    void setInput(byte[] data);

    /**
     * Returns true if the provided pc location doesn't go beyond code
     *
     * @param dest given pc location
     * @param kind the kind of JUMP operation, 1==JUMP or JUMPI and 2==JUMPSUB
     * @return true iif the provided pc location doesn't go beyond code
     */
    boolean isValidJumpDest(long dest, int kind);

    /**
     * Returns opCode corresponded the given program counter
     *
     * @param pc given pc location
     * @return opCode corresponded the given program counter
     */
    byte getOPCode(long pc);


    String toFullString();
}
