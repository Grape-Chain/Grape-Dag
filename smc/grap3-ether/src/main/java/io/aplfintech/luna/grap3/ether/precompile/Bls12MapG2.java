package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12MapG2 extends AbstractBls12 {
    public Bls12MapG2(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_MAP_G2", LibEthPairings.BLS12_MAP_FP2_TO_G2_OPERATION_RAW_VALUE, 128, chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("Bls12381MapG2Gas");
    }

}
