package io.aplfintech.grape.exception;

import io.aplfintech.grape.vm.ExecutionStatus;
import lombok.Getter;

/**
 * General VM Error
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmException extends RuntimeException {
    @Getter
    private final ExecutionStatus status;

    public VmException(ExecutionStatus status) {
        super(status.fullName());
        this.status = status;
    }

    public VmException(ExecutionStatus status, String message) {
        super(message);
        this.status = status;
    }

}
