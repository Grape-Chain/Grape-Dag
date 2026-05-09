package io.aplfintech.luna.vm.opcode;

import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.utils.HexUtils;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public record FnResult(ExecutionStatus executionStatus, byte[] output) implements FnExecResult {

    @Override
    public String toString() {
        return "FnResult{" +
            "executionStatus=" + executionStatus.fullName() +
            ", output=" + HexUtils.toHex(output, true) +
            '}';
    }
}
