package io.aplfintech.grape.tracers;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.opcode.OpCode;

import java.math.BigInteger;

/**
 * General logger interface is used to collect execution traces from a transaction execution.
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface VmLogger {
    void notifyMessageStart(VmStateAccess env, Message message);

    void notifyMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration);

    void notifyExecutionStart(VmStateAccess env, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value);

    void notifyExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status);

    void notifyExecutionEnter(VmStateAccess env, Address from, Address to, byte[] input, long gas, BigInteger value);

    void notifyExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status);

    void notifyInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status);

    void notifyFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status);

}
