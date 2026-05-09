package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.vm.opcode.FnExecResult;

/**
 * SHA256HASH
 * 0x02
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Sha256Hash extends PrecompiledContract {
    public Sha256Hash(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var costBase = price.lookForGasPrice("sha256");
        var costWord = price.lookForGasPrice("sha256Word");
        return (31L + input.length) / 32L * costWord + costBase;
    }

    @Override
    public FnExecResult run(byte[] input) {
        byte[] hash = Math256.padToWord(crypto.sha256sum(input));
        return success(hash);
    }
}
