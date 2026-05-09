package io.aplfintech.luna.exception;

import io.aplfintech.luna.vm.ExecutionStatus;
import lombok.Getter;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class InterpreterExecutionException extends RuntimeException {
    @Getter
    private final ExecutionStatus status;

    public InterpreterExecutionException(ExecutionStatus status) {
        this.status = status;
    }

    public InterpreterExecutionException(ExecutionStatus status, String message) {
        super(message);
        this.status = status;
    }

    public InterpreterExecutionException(ExecutionStatus status, String message, Throwable cause) {
        super(message, cause);
        this.status = status;
    }
}
