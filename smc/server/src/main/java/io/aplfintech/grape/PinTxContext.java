package io.aplfintech.grape;

import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.model.Address;
import lombok.AllArgsConstructor;

import java.math.BigInteger;

/**
 * Block context based on pinning tx from grape1 node
 */
@AllArgsConstructor
public class PinTxContext implements BlockContext {
    private final int pinTxNumber;
    private final long pinTxTimestamp;
    private final Address coinbaseAccount;

    @Override
    public BigInteger blockNumber() {
        return BigInteger.valueOf(pinTxNumber);
    }

    @Override
    public Address coinbase() {
        return coinbaseAccount; // account supplied with transaction for execution (usually - statically defined on grape1 peer node)
    }

    @Override
    public long timestamp() {
        return pinTxTimestamp; // timestamp of pinning tx where this transaction is getting executed in seconds since epoch
    }

    @Override
    public byte[] prevRandao() { // source of randomness (the same 0x44 opcode as for difficulty), must return ramness from consensus layer
        return new byte[32];
    }

    @Override
    public BigInteger gasLimit() {
        return BigInteger.valueOf(10_000_000L); // max amount of gas to be used for tx execution
    }

    @Override
    public BigInteger baseFeePerGas() { // minimal fee per gas for transaction to be included into block
        return BigInteger.valueOf(10);
    }
}
