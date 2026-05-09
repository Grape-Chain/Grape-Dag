package io.aplfintech.luna.config;

import io.aplfintech.luna.grap3.ether.CryptoLibProvider;
import io.aplfintech.luna.crypto.CryptoLib;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class CryptoConfig {
    private static final CryptoLib INSTANCE;

    static {
        INSTANCE = CryptoLibProvider.crypto();
    }

    public static CryptoLib crypto() {
        return INSTANCE;

    }
}
