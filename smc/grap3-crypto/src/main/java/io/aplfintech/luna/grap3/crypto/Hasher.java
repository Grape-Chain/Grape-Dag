package io.aplfintech.luna.grap3.crypto;

public interface Hasher {
    void update(byte[] message);

    byte[] digest(byte[] message);
    byte[] digest();
}
