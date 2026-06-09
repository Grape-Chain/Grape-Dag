package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.vm.opcode.FnExecResult;

/**
 * RIPEMD160HASH
 * 0x03
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Ripemd160Hash extends PrecompiledContract {
    public Ripemd160Hash(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var costBase = price.lookForGasPrice("ripemd160");
        var costWord = price.lookForGasPrice("ripemd160Word");

        return (31L + input.length) / 32L * costWord + costBase;
    }

    @Override
    public FnExecResult run(byte[] input) {
        byte[] hash = Math256.padToWord(crypto.ripemd160sum(input));
        return success(hash);
    }
}
