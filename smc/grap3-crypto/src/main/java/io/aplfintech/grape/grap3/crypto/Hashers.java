package io.aplfintech.grape.grap3.crypto;

import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import org.bouncycastle.jcajce.provider.digest.Keccak;
import org.bouncycastle.jcajce.provider.digest.RIPEMD160;
import org.bouncycastle.jcajce.provider.digest.SHA256;

import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;


@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Hashers {

    public static Hasher sha256() {
        var digest = new SHA256.Digest();
        return new HasherImpl(digest);
    }

    public static Hasher keccak256() {
        var digest = new Keccak.Digest256();
        return new HasherImpl(digest);
    }

    public static Hasher ripemd160() {
        var digest = new RIPEMD160.Digest();
        return new HasherImpl(digest);
    }

    private static class HasherImpl implements Hasher {
        private final MessageDigest digest;

        public HasherImpl(@NonNull MessageDigest digest) {
            this.digest = digest;
        }

        @Override
        public void update(byte[] message) {
            digest.update(message);
        }

        @Override
        public byte[] digest(byte[] message) {
            return digest.digest(message);
        }

        @Override
        public byte[] digest() {
            return digest.digest();
        }
    }

    private static MessageDigest getDigest(@NonNull String name) {
        try {
            return MessageDigest.getInstance(name);
        } catch (NoSuchAlgorithmException e) {
            throw new CryptoLibException("No algorithm", e);
        }
    }

}
