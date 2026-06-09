package io.aplfintech.grape.tracers;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.opcode.OpCode;

import java.io.PrintWriter;
import java.math.BigInteger;

/**
 * Null tracer, it does nothing
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class NullTracer extends Tracer {
    @Override
    protected void onWriteTrace(PrintWriter writer) {
        //nothing to do, all info was already printed
    }

    @Override
    protected void onMessageStart(VmStateAccess env, Message message) {
        //nothing to do
    }

    @Override
    protected void onMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        //nothing to do
    }

    @Override
    protected void onExecutionStart(VmStateAccess vm, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        //nothing to do
    }

    @Override
    protected void onExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        //nothing to do
    }

    @Override
    protected void onExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value) {
        //nothing to do
    }

    @Override
    protected void onExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        //nothing to do
    }

    @Override
    protected void onInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status) {
        //nothing to do
    }

    @Override
    protected void onFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        //nothing to do
    }
}
