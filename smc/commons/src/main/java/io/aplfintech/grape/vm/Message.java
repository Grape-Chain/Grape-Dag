package io.aplfintech.grape.vm;

import io.aplfintech.grape.model.Address;

import java.math.BigInteger;

/**
 * General interface for VM messages
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Message {
    long nonce();

    Address from();

    Address to();

    BigInteger amount();

    BigInteger gasPrice();

    BigInteger gasLimit();

    byte[] data();

    /**
     * Returns true for gas estimation and read-method calling to prevent preCheck routine in the state.
     * PreCheck verifies sender account nonce consistence and condition that sender account is EOA
     */
    boolean isFake();
}
