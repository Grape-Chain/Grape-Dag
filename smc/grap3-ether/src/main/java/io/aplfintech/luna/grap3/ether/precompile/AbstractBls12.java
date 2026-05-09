package io.aplfintech.luna.grap3.ether.precompile;

import com.sun.jna.ptr.IntByReference;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;

import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.utils.Bytes;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;
import org.hyperledger.besu.nativelib.bls12_381.LibEthPairings;

import static java.nio.charset.StandardCharsets.UTF_8;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public abstract class AbstractBls12 extends PrecompiledContract {
    private static final String BLS_12_381 = "BLS12 381";
    private final Integer[] discount;
    private final String opName;
    private final byte opId;
    private final int inputLength;

    public AbstractBls12(@NonNull String opName, byte opId, int inputLength,
                         @NonNull ChainConfig chainConfig, @NonNull CryptoLib crypto) {
        super(chainConfig.gasPriceMap(), crypto);
        if (!chainConfig.isFeatureEnabled(BLS_12_381)) {
            throw new IllegalStateException(BLS_12_381 + " feature is not enabled");
        }
        var blsDiscountTable = chainConfig.getIntProperties(BLS_12_381, "Bls12381MultiExpGasDiscount");
        if (blsDiscountTable.isEmpty() || blsDiscountTable.get().size() < 2 || blsDiscountTable.get().get(0) != -1) {
            throw new IllegalStateException(BLS_12_381 + ": Discount table is not defined or has wrong format");
        }
        this.discount = blsDiscountTable.get().toArray(new Integer[0]);
        this.opName = opName;
        this.opId = opId;
        this.inputLength = inputLength + 1;

    }

    @Override
    public FnExecResult run(byte[] input) {
        final byte[] result = new byte[LibEthPairings.EIP2537_PREALLOCATE_FOR_RESULT_BYTES];
        final byte[] error = new byte[LibEthPairings.EIP2537_PREALLOCATE_FOR_ERROR_BYTES];
        var inputSize = Math.min(inputLength, input.length);

        final IntByReference outputLength = new IntByReference(LibEthPairings.EIP2537_PREALLOCATE_FOR_RESULT_BYTES);
        final IntByReference errorLength = new IntByReference(LibEthPairings.EIP2537_PREALLOCATE_FOR_ERROR_BYTES);

        var errorNo = LibEthPairings.eip2537_perform_operation(
            opId,
            Bytes.slice(input, 0, inputSize),
            inputSize,
            result,
            outputLength,
            error,
            errorLength);

        if (errorNo == 0) {
            return success(Bytes.slice(result, 0, outputLength.getValue()));
        } else {
            var errorDescription = new String(error, 0, errorLength.getValue(), UTF_8);
            log.trace("Error executing precompiled contract {}:{} - '{}'", BLS_12_381, opName, errorDescription);
            return error(VmStatus.VM_PRECOMPILE_ERROR, errorDescription);
        }
    }

    protected int getDiscount(int k) {
        if (k >= discount.length) {
            return discount[discount.length - 1];
        }
        return discount[k];
    }


}
