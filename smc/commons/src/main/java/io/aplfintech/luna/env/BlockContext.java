package io.aplfintech.luna.env;

import io.aplfintech.luna.model.Address;

import java.math.BigInteger;

/**
 * Simple entity that simulates the 'block' behavior
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface BlockContext {
    /**
     * Returns the block number.
     * Used by NUMBER and BLOCKHASH opCodes
     *
     * @return the block number
     */
    BigInteger blockNumber();

    /**
     * Returns the block's beneficiary address.
     * The miner of the block. Used by the COINBASE opCode
     *
     * @return the block's beneficiary address
     */
    Address coinbase();

    /**
     * Returns the block timestamp in seconds.
     * Used by the TIMESTAMP opCode
     *
     * @return the block timestamp in seconds
     */
    long timestamp();

    /**
     * Returns the block difficulty.
     * The latest version of EVM uses PREVRANDAO instead of DIFFICULTY opCode
     *
     * @return the block difficulty
     */
    default byte[] difficulty() {
        return prevRandao();
    }

    /**
     * Returns the previous RANDAO.
     * The RANDAO acts as an infrastructure in the Ethereum system. It is called by other contracts.
     * Contracts for different purposes require different random numbers.
     * The precompiled RANDAO contract generates those numbers (EIP-4399).
     *
     * @return the previous RANDAO
     */
    byte[] prevRandao();

    /**
     * Returns the block gas limit.
     * Used by the GASLIMIT opCode
     *
     * @return the block gas limit
     */
    BigInteger gasLimit();

    /**
     * Returns the block base fee per gas (EIP-1559, EIP-3198)
     * Used by the BASEFEE opCode
     *
     * @return the block base fee per gas
     */
    BigInteger baseFeePerGas();

}
