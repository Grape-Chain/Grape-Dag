package io.aplfintech.luna.grap3.crypto;

public interface DSA {
    byte[] sign(byte[] privateKey, byte[] message);

    boolean verify(byte[] pubKey, byte[] signature, byte[] message);

    byte[] generatePublicKey(byte[] privateKey);
}
