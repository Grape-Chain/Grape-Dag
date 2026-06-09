package io.aplfintech.grape.vm.impl;

import io.aplfintech.grape.l1vm.VmResult;
import io.aplfintech.grape.grapech.utils.Abi;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.MessageResult;
import io.aplfintech.grape.vm.Receipt;
import io.aplfintech.grape.vm.VmStatus;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Receipts {
    public static Receipt from(@NonNull MessageResult result) {
        String errorMessage = null;
        if (result.hasError() && VmStatus.VM_REVERT == result.executionStatus()) {
            //try to decode the revert output
            var output = result.output();
            if (Abi.isRevertedOutput(output)) {
                errorMessage = Abi.unpackRevert(output);
                log.error("REVERT output={}", errorMessage);
            }
        }
        return new SimpleReceipt(result, true, errorMessage);
    }

    public static Receipt error(@NonNull String errorMessage) {
        return new SimpleReceipt(null, false, errorMessage);
    }

    public static Receipt error(@NonNull Message message, @NonNull String errorDescription) {
        var contractResult = VmResult.error(VmStatus.VM_INCONSISTENT_SATE, 0, message.to(), errorDescription);
        var result = new VmMessageResult(contractResult, message.gasLimit().longValue(), 0, 0, 0);
        return new SimpleReceipt(result, false, errorDescription);
    }

    private record SimpleReceipt(MessageResult result, boolean success, String errorMessage) implements Receipt {
        @Override
        public String humanString() {
            return "Receipt{" +
                (success ? result.toString() : "error=" + errorMessage) +
                "}";
        }

        @Override
        public String toString() {
            return humanString();
        }
    }
}
