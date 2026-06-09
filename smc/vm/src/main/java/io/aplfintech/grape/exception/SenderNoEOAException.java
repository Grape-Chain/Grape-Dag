package io.aplfintech.grape.exception;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class SenderNoEOAException extends ExecutionException {
    public SenderNoEOAException(String message) {
        super(message);
    }
}
