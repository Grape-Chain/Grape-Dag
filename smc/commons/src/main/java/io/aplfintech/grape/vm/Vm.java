package io.aplfintech.grape.vm;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.env.Context;
import io.aplfintech.grape.interpreter.Interpreter;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.contract.Code;
import io.aplfintech.grape.vm.contract.ContractRef;
import io.aplfintech.grape.vm.contract.ContractResult;
import lombok.NonNull;

import java.math.BigInteger;

/**
 * The generic interface for Grape1 based VMs
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Vm {

    /**
     * Executes the contract associated with the addr with the given input as parameters.
     * It also handles any necessary value transfer required and takes the necessary steps to create accounts
     * and reverses the state in case of an execution error or failed value transfer
     *
     * @param caller   contract caller
     * @param addr     contract address
     * @param input    input parameters for the given contract
     * @param gasLimit gas limit for the contract execution
     * @param value    value to transfer
     * @return the contract execution result
     */
    ContractResult runCall(ContractRef caller, Address addr, byte[] input, long gasLimit, BigInteger value);

    /**
     * Executes the contract associated with the addr with the given input as parameters.
     * It also handles any necessary value transfer required and reverses the state in case of an execution error.
     * The contract addr must be existed. The caller address uses as context for the contract execution.
     * <code>runCallCode</code> differs from <code>runCall</code> in the sense that it executes the given address code
     * with the caller as context.
     * Used only from opCode functions
     *
     * @param caller   contract caller
     * @param addr     contract address
     * @param input    input parameters for the given contract
     * @param gasLimit gas limit for the contract execution
     * @param value    value to transfer
     * @return the contract execution result
     */
    ContractResult runCallCode(ContractRef caller, Address addr, byte[] input, long gasLimit, BigInteger value);

    /**
     * Executes the contract associated with the addr with the given input as parameters.
     * It reverses the state in case of an execution error.
     * <code>runDelegateCall</code> differs from <code>runCallCode</code> in the sense that it executes
     * the given address code with the caller as context and the caller is set to the caller of the caller.
     * Used only from opCode functions
     *
     * @param caller   contract caller
     * @param addr     contract address
     * @param input    input parameters for the given contract
     * @param gasLimit gas limit for the contract execution
     * @return the contract execution result
     */
    ContractResult runDelegateCall(ContractRef caller, Address addr, byte[] input, long gasLimit);

    /**
     * Executes the contract associated with the addr with the given input as parameters
     * while disallowing any modifications to the state during the call.
     * Opcodes that attempt to perform such modifications will result in exceptions instead of performing the modifications
     * It reverses the state in case of an execution error.
     *
     * @param caller   contract caller
     * @param addr     contract address
     * @param input    input parameters for the given contract
     * @param gasLimit gas limit for the contract execution
     * @return the contract execution result
     */
    ContractResult runStaticCall(ContractRef caller, Address addr, byte[] input, long gasLimit);

    /**
     * Creates a new contract using code as deployment code
     *
     * @param caller   contract caller
     * @param code     contract deployment code
     * @param gasLimit gas limit for the contract execution
     * @param value    value to transfer
     * @return the contract execution result
     */
    ContractResult create(ContractRef caller, Code code, long gasLimit, BigInteger value);

    /**
     * Creates a new contract using code as deployment code
     * Used different routine of the contract address creation
     *
     * @param caller   contract caller
     * @param code     contract deployment code
     * @param gasLimit gas limit for the contract execution
     * @param value    value to transfer
     * @param salt     salt value to create contract address
     * @return the contract execution result
     */
    ContractResult create2(ContractRef caller, Code code, long gasLimit, BigInteger value, byte[] salt);

    /**
     * Increases the call depth
     */
    void enter();

    /**
     * Decreases the call depth
     */
    void leave();

    /**
     * Returns the call depth
     *
     * @return the call depth
     */
    int getDepth();

    /**
     * Returns the current VM context
     *
     * @return the current VM context
     */
    Context context();

    VmStateAccess stateAccess();

    Interpreter interpreter();

    /**
     * Returns the gas available for the current call
     *
     * @return the gas available for the current call
     */
    long getCallGas();

    /**
     * Set the value of the available gas for the current call
     *
     * @param value the value of the available gas for the current call
     */
    void setCallGas(long value);

    /**
     * Add a contract log event
     *
     * @param event a contract log event
     */
    void addLog(@NonNull Log event);

    String toFullString();

    enum CreateKind {
        CREATE,
        CREATE2
    }

    enum CallKind {
        CALL,
        CALL_CODE,
        DELEGATE_CALL,
        STATIC_CALL
    }
}
