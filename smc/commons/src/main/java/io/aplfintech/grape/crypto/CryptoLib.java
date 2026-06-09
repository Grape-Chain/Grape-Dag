package io.aplfintech.grape.crypto;

import java.math.BigInteger;

/**
 * General interface for all cryptography functions
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface CryptoLib {
    boolean validateSignatureValues(byte v, BigInteger r, BigInteger s);

    /**
     * Returns the public key of the signer
     * Recovers the public key from the given signature and hash of the message
     *
     * @param hash      the 32-byte hash of the message to be signed
     * @param signature the signature, 65-byte ECDSA signature containing the recovery id as the last element
     * @return the public key of the signer
     */
    byte[] ecRecover(byte[] hash, byte[] signature);

    byte[] keccak256(byte[] bytes);

    byte[] sha256sum(byte[] bytes);

    byte[] ripemd160sum(byte[] bytes);

    /**
     * Creates the new contract address by thw given sender address and sender nonce
     * Used inside VM to create new contract address
     *
     * @param address sender address
     * @param nonce   sender nonce
     * @return the new created address
     */
    byte[] createAddress(byte[] address, long nonce);

    /**
     * Creates the new contract address by thw given sender address, salt and contract code
     * Used inside VM to create new contract address
     *
     * @param address sender address
     * @param salt    sender nonce
     * @param code    contract code
     * @return the new created address
     */
    byte[] createAddress2(byte[] address, byte[] salt, byte[] code);

    /**
     * Recovers the address by the sender public key
     *
     * @param publicKey the sender public key
     * @return created address
     */
    byte[] recoverAddress(byte[] publicKey);
}
