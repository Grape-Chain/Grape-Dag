package io.aplfintech.luna.vm.contract;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.model.Account;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Addressable;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface StateSpec {
    /**
     * Set the block context for the current state
     *
     * @param block block
     */
    StateSpec block(BlockContext block);

    /**
     * Add account in the current state as known account
     *
     * @param accounts the account
     */
    StateSpec account(Account... accounts);

    /**
     * Adds the contract runtime byte code (without code for deploying the smart-contract) to the current state
     *
     * @param address  contract address
     * @param contract compiled contract with ABI specification
     */
    StateSpec contract(Address address, CompiledContract contract);

    StateSpec balanceIsEqual(Address address, BigInteger balance);

    default StateSpec balanceIsEqual(Address address, long balance) {
        return balanceIsEqual(address, BigInteger.valueOf(balance));
    }

    default StateSpec balanceIsEqual(Addressable object, BigInteger balance) {
        return balanceIsEqual(object.address(), balance);
    }

    default StateSpec balanceIsEqual(Addressable object, long balance) {
        return balanceIsEqual(object.address(), balance);
    }

    MessageSpec newMessage();

    MessageSpec nextMessage();
}
