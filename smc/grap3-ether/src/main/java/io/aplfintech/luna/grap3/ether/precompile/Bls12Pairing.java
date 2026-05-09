package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;


/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Bls12Pairing extends AbstractBls12 {
    private static final int PARAMETER_LENGTH = 384;

    public Bls12Pairing(ChainConfig chainConfig, CryptoLib crypto) {
        super("BLS12_PAIRING",
                LibEthPairings.BLS12_PAIR_OPERATION_RAW_VALUE,
                Integer.MAX_VALUE / PARAMETER_LENGTH * PARAMETER_LENGTH,
                chainConfig, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        long k = input.length / PARAMETER_LENGTH;
        return price.lookForGasPrice("Bls12381PairingBaseGas") + price.lookForGasPrice("Bls12381PairingPerPairGas") * k;
    }

}
