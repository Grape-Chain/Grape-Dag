package io.aplfintech.grape.tracers;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.l1vm.SimpleStorage;
import io.aplfintech.grape.l1vm.Storage;
import io.aplfintech.grape.math.Word256;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Key;
import io.aplfintech.grape.utils.TracerUtils;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.opcode.OpCode;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.io.IOException;
import java.io.PrintWriter;
import java.math.BigInteger;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class StructTracer extends Tracer {

    private final LoggerConfig cfg;
    private VmStateAccess env;
    private byte[] input;
    private long duration;
    private Address contract;
    private Address caller;
    private final Storage storage;
    private List<StructLogData> opLogs;
    private final List<TraceLogData> msgOpLogs;
    private byte[] output;
    private int latestLogSize = 0;

    private long gasLimit;//TODO going to use to capture Tx execution
    private long gasUsed;//TODO going to use to capture Tx execution
    private ExecutionStatus status;


    public StructTracer(@NonNull LoggerConfig cfg) {
        this.cfg = cfg;
        this.storage = new SimpleStorage();
        this.msgOpLogs = new ArrayList<>();
        this.opLogs = new ArrayList<>();
    }

    @Override
    protected void onMessageStart(VmStateAccess env, Message message) {
        //empty body
    }

    @Override
    protected void onMessageEnd(Message message, long startGas, long gasUsed, long remainingCoins, long duration) {
        //empty body
    }

    @Override
    protected void onExecutionStart(VmStateAccess env, Address from, Address to, boolean isCreate, byte[] input, long gas, BigInteger value) {
        this.env = env;
        this.caller = from;
        this.contract = to;
        this.input = input;
    }

    @Override
    protected void onExecutionEnd(byte[] output, long gasUsed, long duration, ExecutionStatus status) {
        this.output = output;
        this.status = status;
        this.duration = duration;
        //shift op logs
        var trLogData = TraceLogData.builder()
            .caller(caller)
            .contract(contract)
            .input(input)
            .output(output)
            .duration(duration)
            .opLogs(opLogs)
            .status(status)
            .build();
        this.msgOpLogs.add(trLogData);
        this.opLogs = new ArrayList<>();
    }

    @Override
    protected void onExecutionEnter(VmStateAccess vm, Address from, Address to, byte[] input, long gas, BigInteger value) {
        //empty body
    }

    @Override
    protected void onExecutionLeave(byte[] output, long gasUsed, ExecutionStatus status) {
        //empty body
    }

    @Override
    protected void onInstructionStart(long pc, OpCode opCode, long gas, long cost, RunContext runState, byte[] returnData, int depth, ExecutionStatus status) {
        // check if already accumulated the specified number of logs
        if (cfg.outputLength() != 0 && cfg.outputLength() <= opLogs.size()) {
            return;
        }
        var stack = runState.getStack();
        var stateContract = runState.getContract();

        //make snapshot of the current memory state
        byte[] memSnapshot = null;
        if (cfg.memoryEnabled()) {
            memSnapshot = runState.getMemory().read(0, runState.getMemory().size());
        }

        //make snapshot of the current stack state
        Word256[] stackSnapshot = null;
        if (!cfg.stackDisabled() && stack.size() > 0) {
            stackSnapshot = stack.peek(stack.size()).toArray(new Word256[0]);
        }

        //make snapshot of the current storage
        Map<Key, byte[]> storageSnapshot = null;
        byte op = opCode.getCode();
        //SLOAD=0x54, SSTORE=0x55
        if (!cfg.storageDisabled() && (op == 0x54 || op == 0x55)) {
            if (op == 0x54 && stack.size() >= 1) {//SLOAD requires 1 arg
                var key = stack.peek(1).get(0).bytes32();
                var value = env.getContractStorage(stateContract.address(), key);
                storage.put(stateContract.address(), key, value);
                storageSnapshot = new HashMap<>(storage.getMapping(stateContract.address()));
            } else if (op == 0x55 && stack.size() >= 2) {//SSTORE requires 2 args
                var args = stack.peek(2);
                var key = args.get(0).bytes32();
                var value = args.get(1).bytes32();
                storage.put(stateContract.address(), key, value);
                storageSnapshot = new HashMap<>(storage.getMapping(stateContract.address()));
            }
        }
        byte[] rData = null;
        if (cfg.returnDataEnabled() && returnData != null) {
            //TODO does it really need to make a copy?
            rData = Bytes.copy(returnData);
        }

        //make snapshot of the fired events
        var stateLog = runState.getInterpreter().getVm().stateAccess().getLog();
        var stateLogSize = stateLog.size();
        List<Log> stateLogDiff = null;
        int skippedEvents = 0;
        if (stateLogSize > latestLogSize) {
            stateLogDiff = stateLog.stream().skip(latestLogSize).toList();
            skippedEvents = latestLogSize;
            latestLogSize = stateLogSize;
        }

        var strLog = StructLogData.builder()
            .pc(pc)
            .opCode(opCode)
            .gas(gas)
            .gasCost(cost)
            .memory(memSnapshot)
            .memorySize(runState.getMemory().size())
            .stack(stackSnapshot)
            .returnData(rData)
            .storage(storageSnapshot)
            .events(stateLogDiff)
            .skippedEvents(skippedEvents)
            .depth(depth)
            .refundCounter(env.getRefundGas())
            .status(status)
            .build();

        opLogs.add(strLog);
    }

    @Override
    protected void onFault(long pc, byte opCode, long gas, long cost, RunContext runState, int depth, ExecutionStatus status) {
        //empty body
    }

    @Override
    protected void onWriteTrace(final PrintWriter writer) {
        if (writer != null) {
            try {
                writer.printf("%n%n####### next trace ######%n");
                if (msgOpLogs.isEmpty()) {
                    if (!opLogs.isEmpty()) {
                        writeMsgTrace(writer, opLogs, input, caller, contract, status, duration);
                    }
                } else {
                    msgOpLogs.forEach(tr -> writeMsgTrace(writer, tr.opLogs, tr.input, tr.caller, tr.contract, tr.status, tr.duration));
                }
            } finally {
                writer.flush();
            }
        } else {
            log.error("Can't write trace data, writer is NULL.");
        }
    }

    private void writeMsgTrace(PrintWriter writer, List<StructLogData> opLogs, byte[] input, Address caller, Address contract, ExecutionStatus status, long duration) {
        writer.printf("###### Incoming message=%s%n", HexUtils.toHex(input, true));
        writer.printf("###### from=%s -> to=%s ######%n%n", caller.hexAddress(), contract.hexAddress());
        try {

            writeTrace(writer, opLogs);

        } catch (IOException e) {
            log.error("Tracer IO error", e);
        }
        writer.printf("###### Execution status=%s duration %s nanos ######%n%n", status, duration);
    }

    private static void writeTrace(PrintWriter writer, List<StructLogData> opLogs) throws IOException {
        for (var strLog : opLogs) {
            writer.printf("%-16s pc=%08d gas=%d cost=%d", strLog.opCode.fullName(), strLog.pc, strLog.gas, strLog.gasCost);
            if (strLog.status != null && strLog.status.isFailure()) {
                writer.printf(" Status: %s", strLog.status);
            }
            writer.println();
            writer.println("Stack:");
            if (strLog.stack != null && strLog.stack.length > 0) {
                for (int i = 0; i < strLog.stack.length; i++) {
                    writer.printf("%08d  %s%n", i, strLog.stack[i].hex());
                }
            } else {
                writer.println("=== empty ===");
            }
            if (strLog.memory != null && strLog.memory.length > 0) {
                writer.println("Memory (dump):");
                TracerUtils.hexDump(writer, strLog.memory);
            }
            if (strLog.storage != null && strLog.storage.size() > 0) {
                writer.println("Storage (mapping):");
                for (var e : strLog.storage.entrySet()) {
                    writer.printf("%s: %s%n", e.getKey().hex(), HexUtils.toHex(e.getValue(), true));
                }
            }
            if (strLog.returnData != null && strLog.returnData.length > 0) {
                writer.println("ReturnData (dump):");
                TracerUtils.hexDump(writer, strLog.returnData);
            }
            if (strLog.events != null && !strLog.events.isEmpty()) {
                writer.println("Events log:");
                eventDump(writer, strLog.skippedEvents, strLog.events);
            }

            writer.println();
        }
    }

    private static void eventDump(PrintWriter writer, int skippedEvent, List<Log> events) {
        int i = skippedEvent;
        for (var event : events) {
            writer.printf("%2d: %s%n", ++i, event);
        }
    }

}
