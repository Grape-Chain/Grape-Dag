package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12G2Add extends AbstractBls12 {
    public Bls12G2Add(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_G2ADD", LibEthPairings.BLS12_G2ADD_OPERATION_RAW_VALUE, 512, chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("Bls12381G2AddGas");
    }

}
