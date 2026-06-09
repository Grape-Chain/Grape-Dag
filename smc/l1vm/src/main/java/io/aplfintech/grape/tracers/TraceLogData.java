package io.aplfintech.grape.tracers;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import lombok.Builder;

import java.util.List;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Builder
public class TraceLogData {
    Address caller;
    Address contract;
    byte[] input;
    byte[] output;
    long duration;
    List<StructLogData> opLogs;
    ExecutionStatus status;
}
