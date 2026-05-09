package io.aplfintech.luna.grap3.crypto;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class CryptoLibException extends RuntimeException {
    public CryptoLibException() {
    }

    public CryptoLibException(String message) {
        super(message);
    }

    public CryptoLibException(String message, Throwable cause) {
        super(message, cause);
    }
}
