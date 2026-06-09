package io.aplfintech.grape.interpreter;

import io.aplfintech.grape.exception.InterpreterExecutionException;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.contract.Contract;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Interpreter {
    InterpreterResult run(Contract contract, byte[] input, boolean readOnly) throws InterpreterExecutionException;

    Vm getVm();

    /**
     * Returns the last CALL's return data for subsequent reuse
     */
    byte[] getReturnData();

    /**
     * Returns true if interpreter starts in read-only mode
     */
    boolean isReadonly();

    /**
     * Sets the interpreter return data
     *
     * @param data return data
     */
    void setReturnData(byte[] data);

    /**
     * Clears the interpreter return data
     */
    void clearReturnData();

    String toFullString();
}
