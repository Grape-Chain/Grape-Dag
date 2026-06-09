package io.aplfintech.grape.grap3.crypto.utils;

import io.aplfintech.grape.grap3.crypto.KeyPair;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import org.bouncycastle.crypto.AsymmetricCipherKeyPair;
import org.bouncycastle.crypto.KeyGenerationParameters;
import org.bouncycastle.crypto.generators.Ed25519KeyPairGenerator;
import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters;
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters;

import java.security.SecureRandom;
import java.util.HexFormat;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class KeyUtils {
    public static String toHex(byte[] key) {
        return HexFormat.of().formatHex(key);
    }

    public static byte[] fromHex(String key) {
        return HexFormat.of().parseHex(key);
    }

    public static RandomKeyGenerator getEd25519Generator() {
        return () -> {
            Ed25519KeyPairGenerator generator = new Ed25519KeyPairGenerator();
            generator.init(new KeyGenerationParameters(new SecureRandom(), 1));
            AsymmetricCipherKeyPair keyPair = generator.generateKeyPair();
            return new KeyPair(((Ed25519PrivateKeyParameters) keyPair.getPrivate()).getEncoded(),
                ((Ed25519PublicKeyParameters) keyPair.getPublic()).getEncoded());
        };
    }

    public interface RandomKeyGenerator {
        KeyPair generateRandom();
    }
}
