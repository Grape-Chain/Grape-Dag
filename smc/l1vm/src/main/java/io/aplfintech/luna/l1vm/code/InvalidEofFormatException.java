package io.aplfintech.luna.l1vm.code;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class InvalidEofFormatException extends RuntimeException {
    public InvalidEofFormatException() {
    }

    public InvalidEofFormatException(String message) {
        super(message);
    }

    public InvalidEofFormatException(String message, Throwable cause) {
        super(message, cause);
    }
}
