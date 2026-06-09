package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;

import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.utils.Bytes;
import org.hyperledger.besu.crypto.altbn128.*;

import java.util.ArrayList;
import java.util.List;

import static io.aplfintech.grape.math.Math256.*;

/**
 * Bn128Pairing
 * 0x08
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class EcPairing extends PrecompiledContract {

    private static final int INPUT_LENGTH = 192;
    private static final byte[] TRUE = UINT_256_ONE.bytes32();
    private static final byte[] FALSE = UINT_256_ZERO.bytes32();

    public EcPairing(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var p = price.lookForGasPrice("ecPairing");
        var pw = price.lookForGasPrice("ecPairingWord");
        var wordCount = Bytes.length(input) / INPUT_LENGTH;
        return Math.multiplyExact((long) pw, wordCount) + p;
    }

    @Override
    public FnExecResult run(byte[] input) {
        if (Bytes.isEmpty(input)) {
            return success(TRUE);
        }
        if (Bytes.length(input) % INPUT_LENGTH != 0) {
            return error(VmStatus.VM_INVALID_ARGUMENT, "Bad elliptic curve pairing length");
        }
        var parameters = input.length / INPUT_LENGTH;
        final List<AltBn128Point> a = new ArrayList<>();
        final List<AltBn128Fq2Point> b = new ArrayList<>();
        for (int i = 0; i < parameters; ++i) {
            var p1_x = extractParam(input, i * INPUT_LENGTH, WORD_SIZE);
            var p1_y = extractParam(input, i * INPUT_LENGTH + 32, WORD_SIZE);
            var p1 = new AltBn128Point(Fq.create(p1_x), Fq.create(p1_y));
            if (!p1.isOnCurve()) {
                return error(VmStatus.VM_PRECOMPILE_ERROR, ALT_BN_128_POINT_OUT_OF_CURVE);
            }
            a.add(p1);

            var p2_xImag = extractParam(input, i * INPUT_LENGTH + 64, WORD_SIZE);
            var p2_xReal = extractParam(input, i * INPUT_LENGTH + 96, WORD_SIZE);
            var p2_yImag = extractParam(input, i * INPUT_LENGTH + 128, WORD_SIZE);
            var p2_yReal = extractParam(input, i * INPUT_LENGTH + 160, WORD_SIZE);
            final Fq2 p2_x = Fq2.create(p2_xReal, p2_xImag);
            final Fq2 p2_y = Fq2.create(p2_yReal, p2_yImag);
            var p2 = new AltBn128Fq2Point(p2_x, p2_y);
            if (!p2.isOnCurve() || !p2.isInGroup()) {
                return error(VmStatus.VM_PRECOMPILE_ERROR, ALT_BN_128_POINT_OUT_OF_CURVE);
            }
            b.add(p2);
        }

        var exp = Fq12.one();
        for (int i = 0; i < parameters; ++i) {
            exp = exp.multiply(AltBn128Fq12Pairer.pair(a.get(i), b.get(i)));
        }

        if (AltBn128Fq12Pairer.finalize(exp).equals(Fq12.one())) {
            return success(TRUE);
        } else {
            return success(FALSE);
        }
    }

}
