package io.aplfintech.grape.exception;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class NonceModificationException extends ExecutionException {
    public NonceModificationException(String message) {
        super(message);
    }
}
