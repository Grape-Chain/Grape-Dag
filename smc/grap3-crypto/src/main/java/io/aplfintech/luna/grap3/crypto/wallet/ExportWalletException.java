package io.aplfintech.luna.grap3.crypto.wallet;

import io.aplfintech.luna.grap3.crypto.CryptoLibException;

/**
 * @author andrew.zinchenko@gmail.com
 */
public class ExportWalletException extends CryptoLibException {
    public ExportWalletException(String message) {
        super(message);
    }

    public ExportWalletException(String message, Throwable cause) {
        super(message, cause);
    }
}
