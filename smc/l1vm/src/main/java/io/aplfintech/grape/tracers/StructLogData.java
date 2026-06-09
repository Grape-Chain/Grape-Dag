package io.aplfintech.grape.tracers;

import io.aplfintech.grape.math.Word256;
import io.aplfintech.grape.model.Key;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.vm.opcode.OpCode;
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
