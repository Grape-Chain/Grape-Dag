package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;

import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import org.hyperledger.besu.crypto.altbn128.AltBn128Point;
import org.hyperledger.besu.crypto.altbn128.Fq;

import static io.aplfintech.luna.math.Math256.WORD_SIZE;

/**
 * Bn128MULL
 * 0x07
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class EcMul extends PrecompiledContract {

    public EcMul(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("ecMul");
    }

    @Override
    public FnExecResult run(byte[] input) {
        var x = extractParam(input, 0, WORD_SIZE);
        var y = extractParam(input, 32, WORD_SIZE);
        var n = extractParam(input, 64, WORD_SIZE);

        var p = new AltBn128Point(Fq.create(x), Fq.create(y));
        if (!p.isOnCurve()) {
            return error(VmStatus.VM_PRECOMPILE_ERROR, ALT_BN_128_POINT_OUT_OF_CURVE);
        }
        var result = marshal(p.multiply(n));
        return success(result);
    }

}
