package io.aplfintech.luna.vm.opcode;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface OpCode {
    String fullName();

    byte getCode();

    /**
     * Returns the op code name
     * This name is used as the key for looking for the operation fee in the price map
     */
    String getName();

    boolean isDynamicGas();

    /**
     * Returns opCode's execution function
     */
    ExecFn getFn();

    /**
     * Returns opCode's base fee
     *
     * @return opCode's base fee
     */
    int getFee();

    /**
     * Sets opCode's base Fee
     *
     * @param fee base fee
     */
    void setFee(int fee);

    /**
     * Check that opcode state is correct
     * example, the base fee is set
     *
     * @return true iif the opcode state is correct
     */
    boolean validate();
}
