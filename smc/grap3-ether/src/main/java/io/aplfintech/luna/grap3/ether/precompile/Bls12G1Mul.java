package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12G1Mul extends AbstractBls12 {
    public Bls12G1Mul(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_G1MUL", LibEthPairings.BLS12_G1MUL_OPERATION_RAW_VALUE, 160, chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("Bls12381G1MulGas");
    }

}
