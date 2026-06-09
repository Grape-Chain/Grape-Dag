package io.aplfintech.grape.grap3.crypto;

import java.math.BigInteger;

public interface DSARecoverable extends DSA {
    boolean validateSignatureValues(byte v, BigInteger r, BigInteger s);

    /**
     * Returns the public key of the signer
     * Recovers the public key from the given signature and hash of the message
     *
     * @param hash      the 32-byte hash of the message to be signed
     * @param signature the signature, 65-byte ECDSA signature containing the recovery id as the last element
     * @return the public key of the signer
     * @throws CryptoLibException
     */
    byte[] recover(byte[] hash, byte[] signature) throws CryptoLibException;
}
