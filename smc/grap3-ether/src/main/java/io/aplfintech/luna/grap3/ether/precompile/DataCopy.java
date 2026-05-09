package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.utils.Bytes;

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
