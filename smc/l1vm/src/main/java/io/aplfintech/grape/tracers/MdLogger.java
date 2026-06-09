package io.aplfintech.grape.tracers;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.opcode.OpCode;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.io.PrintWriter;
import java.math.BigInteger;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.stream.Collectors;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class MdLogger implements VmLogger {
    static final byte[] ZERO_BYTES = {0};
    private final LoggerConfig cfg;
    private final PrintWriter writer;
    private VmStateAccess env;

    private BigInteger gasPrice;

    public MdLogger(@NonNull PrintWriter writer, @NonNull LoggerConfig config) {
        this.writer = writer;
        this.cfg = config;
    }

    @Override
    public void notifyMessageStart(VmStateAccess env, Message message) {
        this.env = env;
        this.gasPrice = message.gasPrice();
        writer.printf("%n%s%n<=== Incoming Message: gas=%d data=%s%n",
            new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSSZ").format(new Date()),
            message.gasLimit(),
            HexUtils.toHex(message.data(), true)
        );
    }

    @Override
    public void notifyMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        writer.printf("%n%s%n===> Processed Message: gas=%d used gas=%d price=%d remaining=%d duration=%d%n",
            new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSSZ").format(new Date()),
            startGas, gasUsed, gasPrice, remainingCoins, duration);
        this.gasPrice = BigInteger.ZERO;
    }

    @Override
    public void notifyExecutionStart(VmStateAccess env, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        this.env = env;
        if (isCreate) {
            writer.printf("From: %s%nCreate at: %s%nData: %s%nGas: %d%nValue: %d neutrino%n",
                from.hexAddress(), to.hexAddress(), toHex(input), gas, value);
        } else {
            writer.printf("From: %s%nTo: %s%nData: %s%nGas: %d%nValue: %d neutrino%n",
                from.hexAddress(), to.hexAddress(), toHex(input), gas, value);

        }
        String line = "|------|-------------------|--------------------------|----------|----------|----------|";
        writer.println(line);
        writer.printf("|%-6s|%-19s|%-26s|%-10s|%-10s|%-10s|%n", "Pc", "Op", "Stack", "Gas", "Cost", "Refund");
        writer.println(line);
        writer.flush();
    }

    @Override
    public void notifyExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        if (cfg.returnDataEnabled()) {
            String trimmedOutput;
            if (output != null) {
                var limit = Math.min(cfg.outputLength() + 1, output.length);
                trimmedOutput = toHex(Bytes.slice(output, 0, limit));
            } else {
                trimmedOutput = "NULL";
            }
            writer.printf("%nOutput: length=%s data=%s%nConsumed gas: %d%nStatus: %s%n", lengthToStr(output), trimmedOutput, gasUsed, status);
        } else {
            writer.printf("%nOutput: length=%s%nConsumed gas: %d%nStatus: %s%n", lengthToStr(output), gasUsed, status);
        }
        writer.flush();
    }

    @Override
    public void notifyExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value) {
        //nothing to do
    }

    @Override
    public void notifyExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        //nothing to do
    }

    @Override
    public void notifyInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status) {
        var stack = runState.getStack();
        writer.printf("| %4d | %-17s |", pc, opCode.fullName());
        String strStack;
        if (!cfg.stackDisabled() && stack.size() > 0) {
            strStack = stack.peek(stack.size()).stream()
                .map(word256 -> {
                        byte[] value = Bytes.trimLeftZeros(word256.bytes());
                        return HexUtils.toHex(value.length == 0 ? ZERO_BYTES : value, true);
                    }
                )
                .collect(Collectors.joining(","));
            var limit = Math.min(strStack.length(), cfg.outputLength() + 1);
            strStack = strStack.substring(0, limit);
        } else {
            strStack = "";
        }
        //print stack elements
        writer.printf("%-25s |", strStack);
        //print gas
        writer.printf(" %8d | %8d |", gas, cost);
        //print refund gas
        writer.printf("%10d|", env.getRefundGas());
        writer.println();
        if (status != null && status.isFailure()) {
            writer.printf("Execution Status: %s%n", status);
        }
        writer.flush();
    }

    @Override
    public void notifyFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        writer.printf("%nError: at pc=%d, op=0x%x: %s%n", pc, opCode, status);
        writer.flush();
    }

    private static String toHex(byte[] bytes) {
        if (bytes == null) return "NULL";
        return bytes.length == 0 ? "[]" : HexUtils.toHex(bytes, true);
    }

    private static String lengthToStr(byte[] output) {
        return output == null ? "undefined" : String.valueOf(output.length);
    }

}

