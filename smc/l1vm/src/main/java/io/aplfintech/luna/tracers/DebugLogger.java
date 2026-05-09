package io.aplfintech.luna.tracers;

import io.aplfintech.luna.bcei.VmStateAccess;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.Message;
import io.aplfintech.luna.vm.opcode.OpCode;
import io.aplfintech.luna.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class DebugLogger implements VmLogger {

    public DebugLogger(@NonNull LoggerConfig config) {
        //ignores config
    }

    @Override
    public void notifyMessageStart(VmStateAccess env, Message message) {
        log.debug("Message Start: State={} message={}", env, message);
    }

    @Override
    public void notifyMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        //duration in nanoseconds
        long ms = duration / 1000;
        log.debug("Message END: startGas={} gas={} remaining={} duration={}ms", startGas, gasUsed, remainingCoins, ms);
    }

    @Override
    public void notifyExecutionStart(VmStateAccess env, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        log.debug("Execution Start: State={} from=0x{} to=0x{} input=0x{} gas={} value={}",
            env, from.hexAddress(), to.hexAddress(),
            HexUtils.toHex(input),
            gas, value);
    }

    @Override
    public void notifyExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        //duration in nanoseconds
        long ms = duration / 1000;
        log.debug("Execution END: used gas={} output={} status={} duration={}ms", gasUsed,
            HexUtils.toHex(output),
            statusToString(status),
            ms);
    }

    @Override
    public void notifyExecutionEnter(VmStateAccess env, Address from, Address to, byte[] input, long gas, BigInteger value) {
        log.debug("Execution ENTER: State={} from=0x{} to=0x{} input=0x{} gas={} value={}",
            env, from.hexAddress(), to.hexAddress(),
            HexUtils.toHex(input),
            gas, value);
    }

    @Override
    public void notifyExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        log.debug("Execution LEAVE: used gas={} output={} status={}", gasUsed,
            HexUtils.toHex(output),
            statusToString(status));
    }

    @Override
    public void notifyInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] output, int depth, ExecutionStatus status) {
        log.debug("###Trace: pc={} opCode={} cost={} gas={} depth={} output={} status={}",
            pc, opCode.fullName(), cost, gas, depth,
            HexUtils.toHex(output),
            statusToString(status));
    }

    @Override
    public void notifyFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        log.debug("status={}", statusToString(status));
    }

    private static String statusToString(ExecutionStatus status) {
        return status == null ? "unknown" : status.toString();
    }
}
