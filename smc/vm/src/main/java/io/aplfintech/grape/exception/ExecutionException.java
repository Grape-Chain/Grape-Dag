package io.aplfintech.grape.exception;

/**
 * The general exception for all error cases until contract execution started
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public abstract class ExecutionException extends Exception {

    public ExecutionException(String message) {
        super(message);
    }

    public ExecutionException(String message, Throwable cause) {
        super(message, cause);
    }
}
