package io.aplfintech.grape.bcei;

import io.aplfintech.grape.model.Account;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Log;

import java.math.BigInteger;

/**
 * General interface to give the access to the blockchain State
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface StateAccess {
    /**
     * Returns the current chain id
     *
     * @return the chain id
     */
    BigInteger chainId();

    /**
     * Returns <code>true<code/> if account corresponding to the given address is non-existent
     *
     * @param address address to check
     * @return true iif account corresponding to then given address is non-existent
     */
    boolean isAccountExists(Address address);

    /**
     * Explicitly creates an account for given address.
     *
     * @param address address for account creation
     */
    void createAccount(Address address);

    /**
     * Returns <code>true<code/> if account corresponding to the given address is empty or non-existent
     *
     * @param address address to check
     * @return true iif account corresponding to the given address is empty or non-existent
     */
    boolean accountIsEmpty(Address address);

    /**
     * Returns the account corresponding to the given address
     *
     * @param address given address
     * @return the account corresponding to the given address
     */
    Account getAccount(Address address);

    void putAccount(Address address, Account account);

    void deleteAccount(Address address);

    // Account fields modifiers
    BigInteger getBalance(Address address);

    void addBalance(Address address, BigInteger amount);

    void subBalance(Address address, BigInteger amount);

    long getNonce(Address address);

    void setNonce(Address address, long nonce);

    void putContractCode(Address address, byte[] data);

    byte[] getContractCode(Address address);

    long getContractCodeSize(Address contractAddress);

    /**
     * Returns the code hash of a specified account
     * If account empty or non-existent returns emptyHash = KECCAK256_NULL
     * If account is a precompiled account returns emptyHash
     *
     * @param extContractAddress account address
     * @return the code hash of a specified account
     */
    byte[] getContractCodeHash(Address extContractAddress);

    /**
     * Returns the value from the contract storage by the given address and key
     * Value is retrieved from the current (uncommitted) state
     *
     * @param address contract address
     * @param key     the key
     * @return value from the storage
     */
    byte[] getContractStorage(Address address, byte[] key);

    /**
     * Returns the value from the contract storage by the given address and key
     * Value is retrieved from the latest committed state
     *
     * @param address contract address
     * @param key     the key
     * @return value from the storage
     */
    byte[] getCommittedContractStorage(Address address, byte[] key);

    void putContractStorage(Address address, byte[] key, byte[] data);

    /**
     * Save the events emitted in the current state
     *
     * @param eventLogs the emitted events
     */
    void saveLog(Log[] eventLogs);

    /**
     * Returns the num'th block hash in the blockchain
     * It's used by the BLOCKHASH VM op code.
     *
     * @param num block number
     * @return the num'th block hash
     */
    byte[] getBlockHash(BigInteger num);

    void clearContractStorage(Address address);

    void checkpoint();

    void commit();

    void revert();

    /**
     * Returns true if the state for the given address deleted, or state is already marked as suicided
     *
     * @param address checked address
     * @return true if the state for the given address doesn't exist
     */
    boolean hasSuicided(Address address);

    /**
     * Marks the state object by the given address as suicided and delete during the next 'update state' phase
     *
     * @param address checked address
     */
    void suicide(Address address);

    String dumpState();
}
