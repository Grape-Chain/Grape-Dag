package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.utils.Bytes;

/**
 * DATACOPY
 * 0x04
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class DataCopy extends PrecompiledContract {
    public DataCopy(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var costBase = price.lookForGasPrice("identity");
        var costWord = price.lookForGasPrice("identityWord");

        return (31L + input.length) / 32L * costWord + costBase;
    }

    @Override
    public FnExecResult run(byte[] input) {
        return success(Bytes.copy(input));
    }
}
