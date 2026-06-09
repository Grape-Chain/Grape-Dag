package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12G2MultiExp extends AbstractBls12 {
    private static final int PARAMETER_LENGTH = 288;

    public Bls12G2MultiExp(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_G2MULTIEXP",
                LibEthPairings.BLS12_G2MULTIEXP_OPERATION_RAW_VALUE,
                Integer.MAX_VALUE / PARAMETER_LENGTH * PARAMETER_LENGTH,
                chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        var k = input.length / PARAMETER_LENGTH;
        return 55L * k * getDiscount(k);
    }

}
