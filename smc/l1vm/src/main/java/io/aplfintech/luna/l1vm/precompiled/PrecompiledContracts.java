package io.aplfintech.luna.l1vm.precompiled;

import io.aplfintech.luna.grap3.ether.precompile.BigModExp;
import io.aplfintech.luna.grap3.ether.precompile.Blake2F;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G1Add;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G1Mul;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G1MultiExp;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G2Add;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G2Mul;
import io.aplfintech.luna.grap3.ether.precompile.Bls12G2MultiExp;
import io.aplfintech.luna.grap3.ether.precompile.Bls12MapG1;
import io.aplfintech.luna.grap3.ether.precompile.Bls12MapG2;
import io.aplfintech.luna.grap3.ether.precompile.Bls12Pairing;
import io.aplfintech.luna.grap3.ether.precompile.DataCopy;
import io.aplfintech.luna.grap3.ether.precompile.EcAdd;
import io.aplfintech.luna.grap3.ether.precompile.EcMul;
import io.aplfintech.luna.grap3.ether.precompile.EcPairing;
import io.aplfintech.luna.grap3.ether.precompile.EcRecover;
import io.aplfintech.luna.grap3.ether.precompile.Ripemd160Hash;
import io.aplfintech.luna.grap3.ether.precompile.Sha256Hash;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.utils.Exceptions;
import io.aplfintech.luna.vm.PrecompiledFn;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.opcode.ExecFn;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.vm.opcode.FnResult;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

/**
 * Factory of precompiled contracts
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
@Slf4j
public class PrecompiledContracts {
    public static final FnResult EMPTY_SUCCESS_RESULT = new FnResult(VmStatus.VM_SUCCESS, new byte[0]);

    /**
     * Returns the precompiled contracts map.
     * That function executed during the code interpretation
     *
     * @return precompiled contracts map
     * @see ExecFn
     */
    public static PrecompiledFn[] createPrecompiledContracts(ChainConfig chainConfig, CryptoLib crypto) {
        var factory = new PrecompiledFactory(chainConfig, crypto);
        PrecompiledFn[] contracts;
        if (chainConfig.isFeatureEnabled("BLS12 381")) {
            contracts = factory.createBls12();
        } else {//Initial start opcodes
            contracts = factory.createInitial();
        }
        return contracts;
    }


    private static class PrecompiledFactory {

        private static final String ECDSA_NOT_IMPLEMENTED_YET = "ECDSA: not implemented yet.";
        private static final String BLS_12_NOT_IMPLEMENTED_YET = "BLS12: not implemented yet.";
        private static final String BLAKE_2_B_NOT_IMPLEMENTED_YET = "Blake2b: not implemented yet.";

        private final ChainConfig chainConfig;
        private final GasPrice price;
        private final CryptoLib crypto;

        public PrecompiledFactory(ChainConfig chainConfig, CryptoLib crypto) {
            this.chainConfig = chainConfig;
            this.price = chainConfig.gasPriceMap();
            this.crypto = crypto;
        }

        private PrecompiledFn[] createBls12() {
            var contracts = createInitial();
            contracts[0x06] = new EcAdd(price, crypto);
            contracts[0x07] = new EcMul(price, crypto);
            contracts[0x08] = new EcPairing(price, crypto);
            contracts[0x09] = new Blake2F(price, crypto);
            contracts[0x0a] = new Bls12G1Add(chainConfig, crypto);
            contracts[0x0b] = new Bls12G1Mul(chainConfig, crypto);
            contracts[0x0c] = new Bls12G1MultiExp(chainConfig, crypto);
            contracts[0x0d] = new Bls12G2Add(chainConfig, crypto);
            contracts[0x0e] = new Bls12G2Mul(chainConfig, crypto);
            contracts[0x0f] = new Bls12G2MultiExp(chainConfig, crypto);
            contracts[0x10] = new Bls12Pairing(chainConfig, crypto);
            contracts[0x11] = new Bls12MapG1(chainConfig, crypto);
            contracts[0x12] = new Bls12MapG2(chainConfig, crypto);

            return contracts;
        }

        private PrecompiledFn[] createInitial() {
            return new PrecompiledFn[]{
                /*0x00*/ null,
                /*0x01 ecrecover*/ new EcRecover(price, crypto),
                /*0x02*/ new Sha256Hash(price, crypto),
                /*0x03*/ new Ripemd160Hash(price, crypto),
                /*0x04 identity*/ new DataCopy(price, crypto),
                /*0x05*/ new BigModExp(price, crypto),
                /*0x06 ecadd*/ unsupported(price, ECDSA_NOT_IMPLEMENTED_YET),
                /*0x07 ecmul*/ unsupported(price, ECDSA_NOT_IMPLEMENTED_YET),
                /*0x08 ecpairing*/ unsupported(price, ECDSA_NOT_IMPLEMENTED_YET),
                /*0x09 blake2f*/ unsupported(price, BLAKE_2_B_NOT_IMPLEMENTED_YET),
                /*0x0a bls12-g1add*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x0b bls12-g1mul*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x0c bls12-g1multiexp*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x0d bls12-g2add*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x0e bls12-g2mul*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x0f bls12-g2multiexp*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x10 bls12-pairing*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x11 bls12-map-g1*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET),
                /*0x12 bls12-map-g2*/ unsupported(price, BLS_12_NOT_IMPLEMENTED_YET)
            };
        }

        private static PrecompiledFn unsupported(GasPrice unused, @NonNull String errorCause) {
            return new PrecompiledFn() {
                @Override
                public long requiredGas(byte[] input) {
                    return 0;
                }

                @Override
                public FnExecResult run(byte[] input) {
                    Exceptions.trap(VmStatus.VM_REJECTED, errorCause);
                    //unreachable code
                    return EMPTY_SUCCESS_RESULT;
                }
            };
        }
    }
}
