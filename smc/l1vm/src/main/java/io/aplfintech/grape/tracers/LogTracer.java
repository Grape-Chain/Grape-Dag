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
 * Wrapper of VmLogger
 * It allows to use logger as a tracer
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class LogTracer extends Tracer {
    private final VmLogger logger;

    public LogTracer(VmLogger logger) {
        this.logger = logger;
    }

    @Override
    protected void onWriteTrace(PrintWriter writer) {
        //nothing to do, all info was already printed
    }

    @Override
    protected void onMessageStart(VmStateAccess env, Message message) {
        logger.notifyMessageStart(env, message);
    }

    @Override
    protected void onMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        logger.notifyMessageEnd(message, startGas, gasUsed, remainingCoins, duration);
    }

    @Override
    protected void onExecutionStart(VmStateAccess vm, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        logger.notifyExecutionStart(vm, from, to, isCreate, input, gas, value);
    }

    @Override
    protected void onExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        logger.notifyExecutionEnd(output, gasUsed, duration, status);
    }

    @Override
    protected void onExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value) {
        logger.notifyExecutionEnter(vm, from, to, input, gas, value);
    }

    @Override
    protected void onExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        logger.notifyExecutionLeave(output, gasUsed, status);
    }

    @Override
    protected void onInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status) {
        logger.notifyInstructionStart(pc, opCode, gas, cost, runState, returnData, depth, status);
    }

    @Override
    protected void onFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        logger.notifyFault(pc, opCode, gas, cost, runState, depth, status);
    }
}
