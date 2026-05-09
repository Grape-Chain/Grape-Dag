package io.aplfintech.luna.grap3.crypto;

import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import org.bouncycastle.jce.provider.BouncyCastleProvider;

import java.security.Provider;
import java.security.Security;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Crypto {
    private static final Provider provider;

    static {
        Security.setProperty("crypto.policy", "unlimited");
        provider = new BouncyCastleProvider();
        Security.addProvider(provider);
    }

    public static Provider getProvider() {
        return provider;
    }

    public static DSA currentDSA() {
        return new Ed25519DSA();
    }

    public static Hasher newHasher() {
        return Hashers.sha256();
    }

}
