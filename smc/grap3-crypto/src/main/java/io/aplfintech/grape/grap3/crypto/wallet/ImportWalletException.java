package io.aplfintech.grape.grap3.crypto.wallet;

import io.aplfintech.grape.grap3.crypto.CryptoLibException;

/**
 * @author andrew.zinchenko@gmail.com
 */
public class ImportWalletException extends CryptoLibException {
    public ImportWalletException(String message) {
        super(message);
    }

    public ImportWalletException(String message, Throwable cause) {
        super(message, cause);
    }
}
