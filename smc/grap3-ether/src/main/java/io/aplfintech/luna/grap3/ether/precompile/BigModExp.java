package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.math.BigNum;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.vm.opcode.FnExecResult;

import java.math.BigInteger;

import static io.aplfintech.luna.math.Math256.WORD_SIZE;
import static io.aplfintech.luna.utils.Bytes.*;

/**
 * MODEXP
 * <p/>Uses BigInteger to calculate result
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class BigModExp extends PrecompiledContract {
    public BigModExp(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        BigInteger gQuadDivisor = BigInteger.valueOf(price.lookForGasPrice("gQuadDivisor"));
        BigInteger big8 = BigInteger.valueOf(8);
        BigInteger big7 = BigInteger.valueOf(7);
        BigInteger big32 = BigInteger.valueOf(32);

        BigInteger baseLen = new BigInteger(1, slicePadded(input, 0, 32));
        BigInteger expLen = new BigInteger(1, slicePadded(input, 32, 32));
        BigInteger modLen = new BigInteger(1, slicePadded(input, 64, 32));
        var in = input.length > 96 ? slice(input, 96) : new byte[0];
        BigInteger inputLen = BigInteger.valueOf(in.length);
        BigInteger expHead;
        if (inputLen.compareTo(baseLen) <= 0) {
            expHead = BigInteger.ZERO;
        } else {
            if (expLen.compareTo(big32) > 0) {
                expHead = new BigInteger(1, slicePadded(in, baseLen.intValue(), 32));
            } else {
                expHead = new BigInteger(1, slicePadded(in, baseLen.intValue(), expLen.intValue()));
            }
        }
        BigInteger msb = expHead.bitLength() > 0 ? BigInteger.valueOf(expHead.bitLength() - 1L) : BigInteger.ZERO;
        BigInteger adjExpLen;
        if (expLen.compareTo(big32) > 0) {
            adjExpLen = expLen.subtract(big32).multiply(big8).add(msb);
        } else {
            adjExpLen = msb;
        }
        var gas = modLen.max(baseLen).add(big7).divide(big8).pow(2)//ceiling(x/8)^2
            .multiply(BigInteger.ONE.max(adjExpLen))
            .divide(gQuadDivisor)// div(3)
            .longValue();
        if (gas < 200) {
            return 200;
        }
        return gas;
    }

    @Override
    public FnExecResult run(byte[] input) {
        BigInteger baseLen = extractParam(input, 0, WORD_SIZE);
        BigInteger expLen = extractParam(input, 32, WORD_SIZE);
        BigInteger modLen = extractParam(input, 64, WORD_SIZE);
        if (baseLen.signum() == 0 && modLen.signum() == 0) {
            return EMPTY_SUCCESS_RESULT;
        }
        var in = input.length > 96 ? slice(input, 96) : new byte[0];
        // Retrieve the operands
        BigInteger base = extractParam(in, 0, baseLen.intValue());
        BigInteger exp = extractParam(in, baseLen.intValue(), expLen.intValue());
        BigInteger mod = extractParam(in, baseLen.add(expLen).intValue(), modLen.intValue());

        if (mod.signum() == 0) {
            return success(leftPadBytes(new byte[0], modLen.intValue()));
        }
        BigNum v;
        if (BigInteger.ONE.compareTo(base) == 0) {// base == 1
            //return base % mod
            v = Math256.uint256(base.mod(mod));
        } else {
            //return base ** exp % mod
            v = Math256.uint256(base.modPow(exp, mod));
        }
        return success(leftPadBytes(v.asWord().bytes(), modLen.intValue()));
    }
}
