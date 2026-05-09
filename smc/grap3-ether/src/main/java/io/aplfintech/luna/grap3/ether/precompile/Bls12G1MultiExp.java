package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12G1MultiExp extends AbstractBls12 {
    private static final int PARAMETER_LENGTH = 160;

    public Bls12G1MultiExp(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_G1MULTIEXP",
                LibEthPairings.BLS12_G1MULTIEXP_OPERATION_RAW_VALUE,
                Integer.MAX_VALUE / PARAMETER_LENGTH * PARAMETER_LENGTH,
                chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var k = input.length / PARAMETER_LENGTH;
        return 12L * k * getDiscount(k);
    }

}
