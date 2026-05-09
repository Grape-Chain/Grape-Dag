package io.aplfintech.luna.tracers;

import io.aplfintech.luna.math.Word256;
import io.aplfintech.luna.model.Key;
import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.Log;
import io.aplfintech.luna.vm.opcode.OpCode;
import lombok.Builder;

import java.util.List;
import java.util.Map;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Builder
public class StructLogData {
    long pc;
    OpCode opCode;
    long gas;
    long gasCost;
    byte[] memory;
    int memorySize;
    Word256[] stack;
    byte[] returnData;
    Map<Key, byte[]> storage;
    int depth;
    long refundCounter;
    ExecutionStatus status;
    List<Log> events;
    int skippedEvents;
}
