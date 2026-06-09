package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;

import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.PrecompiledFn;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.vm.opcode.FnResult;
import io.aplfintech.grape.utils.Bytes;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;
import org.hyperledger.besu.crypto.altbn128.AltBn128Point;

import java.math.BigInteger;

import static io.aplfintech.grape.math.Math256.WORD_SIZE;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public abstract class PrecompiledContract implements PrecompiledFn {
    public static final FnExecResult EMPTY_SUCCESS_RESULT = new FnResult(VmStatus.VM_SUCCESS, new byte[0]);

    final GasPrice price;
    final CryptoLib crypto;

    protected PrecompiledContract(@NonNull GasPrice price, @NonNull CryptoLib crypto) {
        this.price = price;
        this.crypto = crypto;
    }

    static FnExecResult success(byte[] result) {
        return new FnResult(VmStatus.VM_SUCCESS, result);
    }

    static FnExecResult error(ExecutionStatus status, String description) {
        log.error(description + ' ' + status.fullName());
        return new FnResult(status, null);
    }

    static BigInteger extractParam(byte @NonNull [] input, int offset, int length) {
        if (offset > input.length || length == 0) {
            return BigInteger.ZERO;
        }
        final byte[] raw = Bytes.slicePadded(input, offset, length);
        return new BigInteger(1, raw);
    }

    //AltBn128
    static final String ALT_BN_128_POINT_OUT_OF_CURVE = "AltBn128Point out of curve";

    static byte[] marshal(@NonNull AltBn128Point point) {
        var x = Bytes.leftPadBytes(point.getX().toBytes().toArray(), WORD_SIZE);
        var y = Bytes.leftPadBytes(point.getY().toBytes().toArray(), WORD_SIZE);
        return Bytes.concat(x, y);//64 bytes
    }

}
