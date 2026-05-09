package io.aplfintech.luna.grap3.ether.precompile;

import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.crypto.CryptoLib;

import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.utils.Bytes;
import org.hyperledger.besu.crypto.Hash;

/**
 * BLAKE2F
 * 0x09
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class Blake2F extends PrecompiledContract {

    private static final int BLAKE2F_INPUT_LENGTH = 213;
    private static final byte BLAKE2F_FINAL_BLOCK_BYTE = 1;
    private static final byte BLAKE2F_NON_FINAL_BLOCK_BYTE = 0;

    public Blake2F(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        if (Bytes.length(input) != BLAKE2F_INPUT_LENGTH) {
            return 0L;
        }
        var blake2Round = price.lookForGasPrice("blake2Round");
        var rounds = extractParam(input, 0, 4).longValueExact();
        return Math.multiplyExact(blake2Round, rounds);
    }

    @Override
    public FnExecResult run(byte[] input) {
        if (Bytes.length(input) != BLAKE2F_INPUT_LENGTH) {
            return error(VmStatus.VM_INVALID_ARGUMENT, "Blake2F: Invalid input length");
        }
        var finalByte = input[212];
        if (finalByte != BLAKE2F_FINAL_BLOCK_BYTE && finalByte != BLAKE2F_NON_FINAL_BLOCK_BYTE) {
            return error(VmStatus.VM_INVALID_ARGUMENT, "Blake2F: Invalid final flag");
        }
        var result = Hash.blake2bf(org.apache.tuweni.bytes.Bytes.wrap(input));
        return success(result.toArray());
    }
}
