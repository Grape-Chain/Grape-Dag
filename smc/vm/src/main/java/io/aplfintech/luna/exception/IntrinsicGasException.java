package io.aplfintech.luna.exception;

import lombok.Getter;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class IntrinsicGasException extends ExecutionException {
    @Getter
    private final long intrinsicGas;

    public IntrinsicGasException(long intrinsicGas, String message) {
        super(message);
        this.intrinsicGas = intrinsicGas;
    }
}
