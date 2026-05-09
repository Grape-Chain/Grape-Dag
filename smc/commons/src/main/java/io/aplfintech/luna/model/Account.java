package io.aplfintech.luna.model;

import java.math.BigInteger;

/**
 * General interface for account.
 * It's the representation of the account state
 * There are two types:
 * <li/>L1 address
 * <li/>Smart-contract
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Account extends Addressable {

    /**
     * Returns the account balance (amount of founds)
     *
     * @return the account balance
     */
    BigInteger balance();

    void addBalance(BigInteger value);

    void subBalance(BigInteger amount);

    /**
     * Returns the account nonce
     */
    long nonce();

    /**
     * Set the new account nonce
     */
    void setNonce(long nonce);

    /**
     * Returns the hash of the merkle root of the account storage trie
     *
     * @return the hash of the root account storage
     */
    byte[] storageRoot();

    /**
     * Returns the hash of the contract code corresponding the account address
     * or null if current account is non contract account
     *
     * @return the hash of the contract code corresponding the account address
     */
    byte[] codeHash();
}
