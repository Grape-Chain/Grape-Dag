package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;

import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import org.hyperledger.besu.crypto.altbn128.AltBn128Point;
import org.hyperledger.besu.crypto.altbn128.Fq;

import static io.aplfintech.grape.math.Math256.WORD_SIZE;

/**
 * Bn128ADD
 * 0x06
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class EcAdd extends PrecompiledContract {
    public EcAdd(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("ecAdd");
    }

    @Override
    public FnExecResult run(byte[] input) {
        var x1 = extractParam(input, 0, WORD_SIZE);
        var y1 = extractParam(input, 32, WORD_SIZE);
        var x2 = extractParam(input, 64, WORD_SIZE);
        var y2 = extractParam(input, 96, WORD_SIZE);

        var p1 = new AltBn128Point(Fq.create(x1), Fq.create(y1));
        var p2 = new AltBn128Point(Fq.create(x2), Fq.create(y2));
        if (!p1.isOnCurve() || !p2.isOnCurve()) {
            return error(VmStatus.VM_PRECOMPILE_ERROR, ALT_BN_128_POINT_OUT_OF_CURVE);
        }
        var result = marshal(p1.add(p2));
        return success(result);
    }

}
