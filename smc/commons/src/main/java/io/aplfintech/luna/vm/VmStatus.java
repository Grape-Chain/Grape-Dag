package io.aplfintech.luna.vm;

/**
 * The execution status code, successful execution is represented by VM_SUCCESS having code value 0.
 * <p>
 * Positive values represent failures defined by VM specifications with generic
 * VM_FAILURE code of value 1.
 * <p>
 * Status codes with negative values represent VM internal errors
 * not provided by VM specifications.
 * <p>
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public enum VmStatus implements ExecutionStatus {

    /**
     * Execution finished with success.
     */
    VM_SUCCESS(0),

    /**
     * Execution finished with STOP token.
     * It's an internal status, never returned to outside
     */
    VM_STOP_TOKEN(0),

    /**
     * Generic execution failure.
     */
    VM_FAILURE(1),

    /**
     * The execution has run out of gas.
     */
    VM_OUT_OF_GAS(2),

    /**
     * Contract creation code storage out of gas
     */
    VM_CODESTORE_OUT_OF_GAS(3),

    /**
     * Call depth has exceeded the limit (if any)
     */
    VM_CALL_DEPTH_EXCEEDED(4),

    /**
     * The caller does not have enough funds for value transfer.
     */
    VM_INSUFFICIENT_BALANCE(5),

    /**
     * The contract address collision
     */
    VM_CONTRACT_ADDRESS_COLLISION(6),

    /**
     * Execution terminated with REVERT opcode.
     */
    VM_REVERT(7),

    /**
     * The Maximum length of contract code exceeded
     */
    VM_CODESIZE_EXCEEDS_MAXIMUM(8),

    /**
     * Execution has violated the jump destination restrictions.
     */
    VM_BAD_JUMP_DESTINATION(9),

    /**
     * Execution has violated the 'begin sub' restrictions.
     */
    VM_INVALID_BEGINSUB(10),

    /**
     * Execution has violated the jump sub restrictions.
     */
    VM_INVALID_JUMPSUB(11),

    /**
     * Tried to execute an operation which is restricted in static mode.
     */
    VM_STATIC_MODE_VIOLATION(12),

    /**
     * Return data out of bounds
     */
    VM_RETURN_DATA_OUT_OF_BOUNDS(13),

    /**
     * The Gas unit overflow
     */
    VM_GAS_UNIT_OVERFLOW(14),

    /**
     * The nonce unit overflow
     */
    VM_NONCE_UNIT_OVERFLOW(15),

    /**
     * Contract code must not begin with 0xef
     * (Runtime code after publishing)
     */
    VM_INVALID_CONTRACT_CODE(16),

    /**
     * The designated INVALID instruction has been hit during execution.
     */
    VM_INVALID_INSTRUCTION(17),

    /**
     * The execution has attempted to put more items on the VM stack
     * than the specified limit.
     */
    VM_STACK_OVERFLOW(18),

    /**
     * Execution of an opcode has required more items on the VM stack.
     */
    VM_STACK_UNDERFLOW(19),

    /**
     * An argument to a state (memory or stack) accessing method has a value outside the
     * accepted range of values.
     */
    VM_ARGUMENT_OUT_OF_RANGE(20),

    /**
     * An argument to a precompiled contract has a value of wrong format.
     */
    VM_INVALID_ARGUMENT(21),

    /**
     * Any internal error of precompiled contract execution,
     * for example: any ecliptic curve arithmetic error
     */
    VM_PRECOMPILE_ERROR(22),

    /**
     * VM implementation generic internal error.
     */
    VM_INTERNAL_ERROR(-1),

    /**
     * The execution of the given code and/or message has been rejected
     * by the VM implementation.
     * <p>
     * This error SHOULD be used to signal that the VM is not able to or
     * willing to execute the given code type or message.
     */
    VM_REJECTED(-2),

    /**
     * The VM failed to allocate the amount of memory needed for execution.
     */
    VM_OUT_OF_MEMORY(-3),

    /**
     * Value length exceeds memory capacity
     */
    VM_MEMORY_CAPACITY_EXCEEDS(-4),

    /**
     * Unexpected behavior of any routine
     */
    VM_UNEXPECTED_BEHAVIOR(-5),

    /**
     * Inconsistent state
     * The special status for all ExecutionException
     */
    VM_INCONSISTENT_SATE(-6),

    ;
    private final int code;

    VmStatus(int code) {
        this.code = code;
    }

    /**
     * Returns true if status code represents Successful execution
     *
     * @return true if status code represents Successful execution
     */
    @Override
    public boolean isSuccess() {
        return code == 0;
    }

    /**
     * Returns true if status code represents Failure execution
     *
     * @return true if status code represents Failure execution
     */
    @Override
    public boolean isFailure() {
        return code != 0;
    }

    @Override
    public String getName() {
        return this.name();
    }

    @Override
    public int getErrorCode() {
        return code;
    }

    @Override
    public String toString() {
        return this.name() + '(' + this.code + ')';
    }
}
