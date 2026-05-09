package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12G2Mul extends AbstractBls12 {
    public Bls12G2Mul(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_G2MUL", LibEthPairings.BLS12_G2MUL_OPERATION_RAW_VALUE, 288, chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("Bls12381G2MulGas");
    }

}
