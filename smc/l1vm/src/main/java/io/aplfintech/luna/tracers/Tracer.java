package io.aplfintech.luna.tracers;

import io.aplfintech.luna.bcei.VmStateAccess;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.Message;
import io.aplfintech.luna.vm.opcode.OpCode;

import java.io.PrintWriter;
import java.math.BigInteger;

/**
 * Tracer implements "chain of responsibility" pattern to organize a chain of tracers.
 * Each tracer collects the information during the transaction execution which is printed out
 * by writeTrace method
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public abstract class Tracer implements VmLogger {
    private Tracer nextTracer;

    public static Tracer link(Tracer first, Tracer... chain) {
        var head = first;
        for (var next : chain) {
            head.nextTracer = next;
            head = next;
        }
        return first;
    }

    protected abstract void onWriteTrace(PrintWriter writer);

    public void writeTrace(PrintWriter writer) {
        onWriteTrace(writer);
        if (nextTracer != null) {
            nextTracer.writeTrace(writer);
        }
    }

    protected abstract void onMessageStart(VmStateAccess env, Message message);

    protected abstract void onMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration);

    protected abstract void onExecutionStart(VmStateAccess vm, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value);

    protected abstract void onExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status);

    protected abstract void onExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value);

    protected abstract void onExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status);

    protected abstract void onInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status);

    protected abstract void onFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status);

    @Override
    public void notifyMessageStart(VmStateAccess env, Message message) {
        onMessageStart(env, message);
        if (nextTracer != null) {
            nextTracer.notifyMessageStart(env, message);
        }
    }

    @Override
    public void notifyMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        onMessageEnd(message, startGas, gasUsed, remainingCoins, duration);
        if (nextTracer != null) {
            nextTracer.notifyMessageEnd(message, startGas, gasUsed, remainingCoins, duration);
        }
    }

    @Override
    public void notifyExecutionStart(VmStateAccess vm, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        onExecutionStart(vm, from, to, isCreate, input, gas, value);
        if (nextTracer != null) {
            nextTracer.notifyExecutionStart(vm, from, to, isCreate, input, gas, value);
        }
    }

    @Override
    public void notifyExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        onExecutionEnd(output, gasUsed, duration, status);
        if (nextTracer != null) {
            nextTracer.notifyExecutionEnd(output, gasUsed, duration, status);
        }
    }

    @Override
    public void notifyExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value) {
        onExecutionEnter(vm, from, to, input, gas, value);
        if (nextTracer != null) {
            nextTracer.notifyExecutionEnter(vm, from, to, input, gas, value);
        }
    }

    @Override
    public void notifyExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        onExecutionLeave(output, gasUsed, status);
        if (nextTracer != null) {
            nextTracer.notifyExecutionLeave(output, gasUsed, status);
        }
    }

    @Override
    public void notifyInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status) {
        onInstructionStart(pc, opCode, gas, cost, runState, returnData, depth, status);
        if (nextTracer != null) {
            nextTracer.notifyInstructionStart(pc, opCode, gas, cost, runState, returnData, depth, status);
        }
    }

    @Override
    public void notifyFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        onFault(pc, opCode, gas, cost, runState, depth, status);
        if (nextTracer != null) {
            nextTracer.notifyFault(pc, opCode, gas, cost, runState, depth, status);
        }
    }

}
