package io.aplfintech.grape.utils;

import io.aplfintech.grape.exception.VmException;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.VmStatus;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Exceptions {
    public static VmException internal(String internalErrorDescription) {
        return from(VmStatus.VM_INTERNAL_ERROR, internalErrorDescription);
    }

    public static VmException from(ExecutionStatus status, String description) {
        return new VmException(status, description);
    }

    public static VmException from(ExecutionStatus status) {
        return new VmException(status);
    }

    public static void trap(boolean predicate, ExecutionStatus status, String description) {
        if (predicate) {
            trap(status, description);
        }
    }

    public static void trap(ExecutionStatus status, String description) {
        log.error(description + ' ' + status.fullName());
        throw new VmException(status, description);
    }


}
