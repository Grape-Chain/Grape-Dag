package io.aplfintech.luna.env;

import io.aplfintech.luna.bcei.StateAccess;
import io.aplfintech.luna.model.Address;

import java.math.BigInteger;

/**
 * The VM context provide information about current transaction and current block
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Context extends TxContext, BlockContext {
    /**
     * Checks whether there are enough funds in the address' account to make a transfer.
     * This does not take the necessary gas in to account to make the transfer valid.
     *
     * @param stateAccess the state access object
     * @param address     checked address
     * @param value       transferred value
     * @return true iif enough funds in the address' account to make a transfer
     */
    boolean canTransfer(StateAccess stateAccess, Address address, BigInteger value);

    /**
     * Transfers coins from one account to another
     *
     * @param stateAccess the state access object
     * @param from        from account
     * @param to          to account
     * @param value       transferred amount
     */
    void transfer(StateAccess stateAccess, Address from, Address to, BigInteger value);

}
