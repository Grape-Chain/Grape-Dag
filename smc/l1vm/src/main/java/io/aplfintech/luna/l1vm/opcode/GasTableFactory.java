package io.aplfintech.luna.l1vm.opcode;

import io.aplfintech.luna.bcei.VmStateAccess;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.GasPrice;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.math.BigNum;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.utils.Exceptions;
import io.aplfintech.luna.vm.DynamicGasHandler;
import io.aplfintech.luna.vm.Vm;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;

import static io.aplfintech.luna.l1vm.opcode.Feature.INIT_CODE_WORD_COST;
import static io.aplfintech.luna.l1vm.opcode.Feature.WARMED_ACCESS;
import static io.aplfintech.luna.math.Math256.WORD_SIZE;
import static io.aplfintech.luna.math.Math256.toWordSize;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class GasTableFactory {
    //TODO change long to UBigInt for all Gas-related routines and check that the callCost is not previously overflowed
    private final ChainConfig config;
    private final GasPrice gasPrice;

    public GasTableFactory(@NonNull ChainConfig config) {
        this.config = config;
        this.gasPrice = config.gasPriceMap();
    }

    private boolean isFeatureEnabled(@NonNull Feature feature) {
        return config.isFeatureEnabled(feature.key());
    }

    /**
     * Returns the dynamic gas handlers map for opcodes which have dynamic gas
     * These are not the <i>pure</i> functions (opcodes).
     *
     * @return dynamic gas handlers map
     */
    public Map<Byte, DynamicGasHandler> createGasHandlersMap() {
        var handlers = createBaseGasHandlersMap();
        if (isFeatureEnabled(WARMED_ACCESS)) {
            handlers.put(
                /* SSTORE */
                (byte) 0x55,
                makeSStoreHandlerWarmed(gasPrice)
            );
        }

        if (isFeatureEnabled(Feature.SELF_DESTRUCT)) {
            handlers.put(
                /* SELFDESTRUCT */
                (byte) 0xff,
                makeSelfDestructHandler(gasPrice)
            );
        }

        return handlers;
    }

    private Map<Byte, DynamicGasHandler> createBaseGasHandlersMap() {
        var handlers = new HashMap<Byte, DynamicGasHandler>();
        handlers.put(
            /*EXP*/
            (byte) 0x0a,
            runState -> {
                var params = runState.getStack().peek(2);//0-base, 1-exponent
                var exponent = params.get(1).asBigNum();
                if (exponent.isZero()) {
                    return 0L;
                }
                var byteLength = exponent.byteLength();
                Exceptions.trap(byteLength < 1 || byteLength > 32,
                    VmStatus.VM_ARGUMENT_OUT_OF_RANGE, "Gas handlers definition error");
                var expPricePerByte = gasPrice.lookForGasPrice("expByte");
                return Math.multiplyExact(byteLength, (long) expPricePerByte);
            }
        );
        handlers.put(
            /* SHA3 */
            (byte) 0x20,
            runState -> {
                var params = runState.getStack().peek(2);//0-offset, 1-length
                var offset = params.get(0).asBigNum().longValue();
                var length = params.get(1).asBigNum().intValue();
                long sha3Word = Math.multiplyFull(gasPrice.lookForGasPrice("sha3Word"), Math256.toWordSize(length));
                return subMemUsage(runState, offset, length, gasPrice) + sha3Word;
            }
        );
        handlers.put(
            /* BALANCE */
            (byte) 0x31,
            makeAddressAccessHandler(gasPrice, config)
        );
        handlers.put(
            /* CALLDATACOPY */
            (byte) 0x37,
            makeMemCopyHandler(3, 0, 2, gasPrice)
        );
        handlers.put(
            /* CODECOPY */
            (byte) 0x39,
            makeMemCopyHandler(3, 0, 2, gasPrice)
        );
        handlers.put(
            /* EXTCODESIZE */
            (byte) 0x3b,
            makeAddressAccessHandler(gasPrice, config)
        );
        handlers.put(
            /* EXTCODECOPY */
            (byte) 0x3c,
            runState -> {
                var params = runState.getStack().peek(4);//address, mem_offset, data_offset, length
                var memOffset = params.get(1).asBigNum().longValue();
                var length = params.get(3).asBigNum().intValue();

                long fee = getMemUsageFee(runState, memOffset, length, gasPrice);
                if (isFeatureEnabled(WARMED_ACCESS)) {
                    var address = VmAddress.from(params.get(0).bytes());
                    VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();
                    fee = Math.addExact(fee, checkAddressAccessFee(address, gasPrice, stateAccess));
                }
                return fee;
            }
        );
        handlers.put(
            /* RETURNDATACOPY */
            (byte) 0x3e,
            makeMemCopyHandler(4, 1, 3, gasPrice)
        );
        handlers.put(
            /* EXTCODEHASH */
            (byte) 0x3f,
            makeAddressAccessHandler(gasPrice, config)
        );
        handlers.put(
            /* MLOAD */
            (byte) 0x51,
            runState -> {
                var params = runState.getStack().peek(1);//0-pos
                var pos = params.get(0).asBigNum().longValue();
                return subMemUsage(runState, pos, WORD_SIZE, gasPrice);
            }
        );
        handlers.put(
            /* MSTORE */
            (byte) 0x52,
            runState -> {
                var params = runState.getStack().peek(1);//0-pos
                var pos = params.get(0).asBigNum().longValue();
                return subMemUsage(runState, pos, WORD_SIZE, gasPrice);
            }
        );
        handlers.put(
            /* MSTORE8 */
            (byte) 0x53,
            runState -> {
                var params = runState.getStack().peek(1);//0-pos
                var pos = params.get(0).asBigNum().longValue();
                return subMemUsage(runState, pos, 1, gasPrice);
            }
        );
        handlers.put(
            /* SLOAD */
            (byte) 0x54,
            runState -> {
                if (isFeatureEnabled(WARMED_ACCESS)) {
                    var params = runState.getStack().peek(1);
                    var key = params.get(0).bytes32();
                    var address = runState.getContract().address();
                    VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();
                    long fee;
                    if (!stateAccess.isWarmedSlot(address, key)) {
                        stateAccess.addWarmedSlot(address, key);
                        fee = gasPrice.lookForGasPrice("coldsload");
                    } else {
                        fee = gasPrice.lookForGasPrice("warmstorageread");
                    }
                    return fee;
                }
                return 0L;
            }
        );
        handlers.put(
            /* SSTORE */
            (byte) 0x55,
            makeSStoreHandlerBase(gasPrice)
        );
        handlers.put(
            /* LOG0 */
            (byte) 0xa0,
            makeLogHandler(0, gasPrice)
        );
        handlers.put(
            /* LOG1 */
            (byte) 0xa1,
            makeLogHandler(1, gasPrice)
        );
        handlers.put(
            /* LOG2 */
            (byte) 0xa2,
            makeLogHandler(2, gasPrice)
        );
        handlers.put(
            /* LOG3 */
            (byte) 0xa3,
            makeLogHandler(3, gasPrice)
        );
        handlers.put(
            /* LOG4 */
            (byte) 0xa4,
            makeLogHandler(4, gasPrice)
        );
        handlers.put(
            /* CREATE */
            (byte) 0xf0,
            runState -> {
                var params = runState.getStack().peek(3);//value, offset, length
                var offset = params.get(1).asBigNum().longValue();
                var length = params.get(2).asBigNum().longValue();
                long fee;
                if (isFeatureEnabled(INIT_CODE_WORD_COST)) {
                    fee = Math.multiplyFull(gasPrice.lookForGasPrice("initCodeWordCost"), Math256.toWordSize(length));
                } else {
                    fee = 0;
                }
                return Math.addExact(fee, subMemUsage(runState, offset, length, gasPrice));
            }
        );
        handlers.put(
            /* CALL */
            (byte) 0xf1,
            makeCallHandler(Vm.CallKind.CALL, gasPrice, config)
        );
        handlers.put(
            /* CALLCODE */
            (byte) 0xf2,
            makeCallHandler(Vm.CallKind.CALL_CODE, gasPrice, config)
        );
        handlers.put(
            /* RETURN */
            (byte) 0xf3,
            runState -> {
                var params = runState.getStack().peek(2);//0-offset, 1-length
                var offset = params.get(0).asBigNum().longValue();
                var length = params.get(1).asBigNum().longValue();
                return subMemUsage(runState, offset, length, gasPrice);
            }
        );
        handlers.put(
            /* DELEGATECALL */
            (byte) 0xf4,
            makeStaticOrDelegateCallHandler(gasPrice, config)
        );
        handlers.put(
            /* CREATE2 */
            (byte) 0xf5,
            runState -> {
                var params = runState.getStack().peek(3);//value, offset, length, (salt not used)
                var offset = params.get(1).asBigNum().intValue(false);
                var length = params.get(2).asBigNum().intValue(false);
                log.debug("CREATE2 offset={}, length={}", offset, length);
                int wordFee = gasPrice.lookForGasPrice("sha3Word");
                if (isFeatureEnabled(INIT_CODE_WORD_COST)) {
                    wordFee += gasPrice.lookForGasPrice("initCodeWordCost");
                }
                long fee = Math.multiplyFull(wordFee, Math256.toWordSize(length));
                return Math.addExact(fee, subMemUsage(runState, offset, length, gasPrice));
            }
        );
        handlers.put(
            /* STATICCALL */
            (byte) 0xfa,
            makeStaticOrDelegateCallHandler(gasPrice, config)
        );
        handlers.put(
            /* REVERT */
            (byte) 0xfd,
            runState -> {
                var params = runState.getStack().peek(2);//0-offset, 1-length
                var offset = params.get(0).asBigNum().longValue();
                var length = params.get(1).asBigNum().longValue();
                return subMemUsage(runState, offset, length, gasPrice);
            }
        );

        return handlers;
    }

    private static DynamicGasHandler makeSStoreHandlerWarmed(GasPrice gasPrice) {
        return runState -> {
            var address = runState.getContract().address();
            var params = runState.getStack().peek(2);//0-key, 1-value
            var key = params.get(0).bytes32();
            var val = params.get(1);

            VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();
            var current = stateAccess.getContractStorage(address, key);
            var value = val.bytes32();
            int sentryGas = gasPrice.lookForGasPrice("sstoreSentryGasEIP2200");
            if (runState.getContract().gas() < sentryGas) {
                log.error("Not enough gas for reentrancy sentry, required={} available={}", sentryGas, runState.getContract().gas());
                throw Exceptions.from(VmStatus.VM_OUT_OF_GAS, "Not enough gas for reentrancy sentry");
            }
            long cost = checkStorageAccessFee(address, key, gasPrice, stateAccess);
            if (Arrays.equals(current, value)) { //noop
                //value is unchanged, deduct SLOAD price
                return cost + gasPrice.lookForGasPrice("warmstorageread");
            }
            var original = stateAccess.getCommittedContractStorage(address, key);
            final var clearRefund = gasPrice.lookForGasPrice("sstoreClearRefundEIP2200");
            if (Arrays.equals(original, current)) {
                if (Bytes.length(original) == 0) {//create slot
                    return cost + gasPrice.lookForGasPrice("sstoreInitGasEIP2200");
                }
                if (Bytes.length(value) == 0) {//delete slot
                    stateAccess.addRefundGas(clearRefund);
                }
                return cost + (gasPrice.lookForGasPrice("sstoreCleanGasEIP2200") - gasPrice.lookForGasPrice("coldsload"));
            }
            if (Bytes.length(original) > 0) {
                if (Bytes.length(current) == 0) {//recreate slot
                    stateAccess.subRefundGas(clearRefund);
                } else if (Bytes.length(value) == 0) {//delete slot
                    stateAccess.addRefundGas(clearRefund);
                }
            }
            if (Arrays.equals(original, value)) {
                int refund;
                if (Bytes.length(original) == 0) {//reset to original inexistent slot
                    refund = gasPrice.lookForGasPrice("sstoreInitGasEIP2200") - gasPrice.lookForGasPrice("warmstorageread");
                } else {//reset to original existing slot
                    refund = (gasPrice.lookForGasPrice("sstoreCleanGasEIP2200") - gasPrice.lookForGasPrice("coldsload")) - gasPrice.lookForGasPrice("warmstorageread");
                }
                stateAccess.addRefundGas(refund);
            }
            //dirty update
            return cost + gasPrice.lookForGasPrice("warmstorageread");
        };
    }

    private static DynamicGasHandler makeSStoreHandlerBase(GasPrice gasPrice) {
        return runState -> {
            var address = runState.getContract().address();
            var params = runState.getStack().peek(2);//0-key, 1-value
            var key = params.get(0).bytes32();
            var value = params.get(1).asBigNum();
            VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();
            var current = stateAccess.getContractStorage(address, key);
            long fee;
            // Here are three scenarios of gas calculation:
            // 1. From a zero-value address to a non-zero value         (NEW VALUE)
            // 2. From a non-zero value address to a zero-value address (DELETE)
            // 3. From a non-zero to a non-zero                         (CHANGE)
            if ((Bytes.length(current) == 0) && value.isNonZero()) { // 0 => non 0
                fee = gasPrice.lookForGasPrice("sstoreSet");
            } else if ((Bytes.length(current) > 0) && value.isZero()) {// non 0 => 0
                var refund = gasPrice.lookForGasPrice("sstoreRefund");
                stateAccess.addRefundGas(refund);
                fee = gasPrice.lookForGasPrice("sstoreReset");
            } else {// non 0 => non 0 (or 0 => 0)
                fee = gasPrice.lookForGasPrice("sstoreReset");
            }

            return fee;
        };
    }

    private static DynamicGasHandler makeSelfDestructHandler(GasPrice gasPrice) {
        return runState -> {
            var params = runState.getStack().peek(1);//0-offset, 1-length
            var beneficiary = VmAddress.from(params.get(0).bytes());
            var suicide = runState.getContract().address();
            var balance = runState.getInterpreter().getVm().stateAccess().getBalance(suicide);
            VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();

            var fee = checkAddressAccessFee(beneficiary, gasPrice, stateAccess);
            if (balance.signum() != 0 &&
                (!stateAccess.isAccountExists(beneficiary) || (stateAccess.accountIsEmpty(beneficiary)))) {
                fee = Math.addExact(fee, gasPrice.lookForGasPrice("callNewAccount"));
            }
            if (!stateAccess.hasSuicided(beneficiary)) {
                var refund = gasPrice.lookForGasPrice("selfdestructRefund");
                stateAccess.addRefundGas(refund);
            }
            return fee;
        };
    }

    /**
     * Checks whether the given slot is present in the access set.
     * If it is, this method returns 0L,
     * otherwise added the slot to the access set and returns 'coldsload' fee
     */
    private static long checkStorageAccessFee(Address address, byte[] key, GasPrice gasPrice, VmStateAccess stateAccess) {
        if (!stateAccess.isWarmedSlot(address, key)) {
            stateAccess.addWarmedSlot(address, key);
            return gasPrice.lookForGasPrice("coldsload");
        }
        return 0L;
    }

    /**
     * Checks whether the given address is present in the access set.
     * If it is, this method returns '0',
     * otherwise added the address to the access set and returns 'coldaccountaccess' fee
     */
    private static long checkAddressAccessFee(Address address, GasPrice gasPrice, VmStateAccess stateAccess) {
        if (!stateAccess.isWarmedAddress(address)) {
            stateAccess.addWarmedAddress(address);
            return gasPrice.lookForGasPrice("coldaccountaccess");
        }
        return 0L;
    }

    /**
     * Creates the handler that checks whether the first stack item (as address) is present in the access list.
     */
    private static DynamicGasHandler makeAddressAccessHandler(GasPrice gasPrice, ChainConfig config) {
        return runState -> {
            if (config.isFeatureEnabled(WARMED_ACCESS.key())) {
                var params = runState.getStack().peek(1);
                var address = VmAddress.from(params.get(0).bytes());
                VmStateAccess stateAccess = runState.getInterpreter().getVm().stateAccess();
                return checkAddressAccessFee(address, gasPrice, stateAccess);
            }
            return 0L;
        };
    }

    /**
     * Returns handler to calculate the gas fee for LOG commands
     *
     * @param topicsCount the number of topics
     * @param price       the price
     */
    private static DynamicGasHandler makeLogHandler(int topicsCount, GasPrice price) {
        return runState -> {
            var params = runState.getStack().peek(2);//0-offset, 1-length
            var offset = params.get(0).asBigNum().longValue();
            var length = params.get(1).asBigNum().intValue();
            long logFee = price.lookForGasPrice("log") + Math.multiplyFull(price.lookForGasPrice("logTopic"), topicsCount);
            long memSizeFee = Math.multiplyFull(price.lookForGasPrice("logData"), length);
            long memUsageFee = subMemUsage(runState, offset, length, price);

            return Math.addExact(Math.addExact(logFee, memSizeFee), memUsageFee);
        };
    }

    private static long limitCallGas(long availableGas, long baseFee, BigNum callCost) {
        var gas = availableGas - baseFee;
        if (gas < 0) {
            //TODO use uint64 instead of long
            log.trace("==== LIMIT Call gas: Potentially OUT_OF_GAS error, available gas={} baseFee={}, shortage={}", availableGas, baseFee, gas);
            //gas = baseFee;
        }
        gas = gas - gas / 64;

        var longCallCost = callCost.longValue(true);
        var limit = Math.min(gas, longCallCost);
        log.trace("==== LIMIT Call gas: limit={}, available gas={} callCost={} long callCost={}", limit, gas, callCost, longCallCost);
        return limit;
    }

    /**
     * Returns handler to calculate the gas fee for copy commands based by size of copied bytes
     *
     * @param argsCount the number of arguments peeked from the stack
     * @param offsetIdx index of the 'offset' argument
     * @param lengthIdx index of the 'length' argument
     * @param price     the price
     */
    private static DynamicGasHandler makeMemCopyHandler(int argsCount, int offsetIdx, int lengthIdx, GasPrice price) {
        return runState -> {
            var params = runState.getStack().peek(argsCount);
            var memOffset = params.get(offsetIdx).asBigNum().longValue();
            var length = params.get(lengthIdx).asBigNum().intValue();

            return getMemUsageFee(runState, memOffset, length, price);
        };
    }

    private static long getMemUsageFee(RunContext runState, long memOffset, int length, GasPrice price) {
        var fee = subMemUsage(runState, memOffset, length, price);
        if (length != 0) {
            fee = Math.addExact(fee, Math.multiplyFull(price.lookForGasPrice("copy"), Math256.toWordSize(length)));
        }
        return fee;
    }

    /**
     * Returns handler to calculate the gas fee for CALL or CALLCODE commands
     */
    private static DynamicGasHandler makeCallHandler(@NonNull Vm.CallKind callKind, GasPrice price, ChainConfig config) {
        return runState -> {
            var params = runState.getStack().peek(7);//gasLimit, toAddr, value, inOffset, inLength, outOffset, outLength
            var gasLimit = params.get(0).asBigNum();
            var to = VmAddress.from(params.get(1).bytes());
            var isNonZeroValue = params.get(2).asBigNum().isNonZero();
            var inOffset = params.get(3).asBigNum().longValue();
            var inLength = params.get(4).asBigNum().longValue();
            var outOffset = params.get(5).asBigNum().longValue();
            var outLength = params.get(6).asBigNum().longValue();
            long fee = 0;
            if (config.isFeatureEnabled(WARMED_ACCESS.key())) {
                long coldCost = checkAddressAccessFee(to, price, runState.getInterpreter().getVm().stateAccess());
                if (runState.getContract().gas() < coldCost) {
                    log.error("Not enough gas for cold calling, required={} available={}", coldCost, runState.getContract().gas());
                    throw Exceptions.from(VmStatus.VM_OUT_OF_GAS, "Not enough gas for cold calling");
                }
                fee = fee + coldCost;
            }
            //old calculator
            fee = Math.addExact(fee, subMemUsage(runState, inOffset, inLength, price));
            fee = Math.addExact(fee, subMemUsage(runState, outOffset, outLength, price));

            if (callKind == Vm.CallKind.CALL) {
                if (!runState.getInterpreter().getVm().stateAccess().isAccountExists(to)) {
                    fee = Math.addExact(fee, price.lookForGasPrice("callNewAccount"));
                }
            }
            if (isNonZeroValue) {
                fee = Math.addExact(fee, price.lookForGasPrice("callValueTransfer"));
            }
            var cap = limitCallGas(runState.getContract().gas(), fee, gasLimit);

            runState.getInterpreter().getVm().setCallGas(cap);

            return fee;
        };
    }

    /**
     * Returns handler to calculate the gas fee for STATICCALL or DELEGATECALL commands
     */
    private static DynamicGasHandler makeStaticOrDelegateCallHandler(GasPrice price, ChainConfig config) {
        return runState -> {
            var params = runState.getStack().peek(6);//gasLimit, toAddr, inOffset, inLength, outOffset, outLength
            var gasLimit = params.get(0).asBigNum();
            var to = VmAddress.from(params.get(1).bytes());
            var inOffset = params.get(2).asBigNum().longValue();
            var inLength = params.get(3).asBigNum().longValue();
            var outOffset = params.get(4).asBigNum().longValue();
            var outLength = params.get(5).asBigNum().longValue();
            long fee = 0;
            if (config.isFeatureEnabled(WARMED_ACCESS.key())) {
                long coldCost = checkAddressAccessFee(to, price, runState.getInterpreter().getVm().stateAccess());
                if (runState.getContract().gas() < coldCost) {
                    log.error("Not enough gas for cold static/delegate calling, required={} available={}", coldCost, runState.getContract().gas());
                    throw Exceptions.from(VmStatus.VM_OUT_OF_GAS, "Not enough gas for cold static/delegate calling");
                }
                fee = fee + coldCost;
            }

            fee = Math.addExact(fee, subMemUsage(runState, inOffset, inLength, price));
            fee = Math.addExact(fee, subMemUsage(runState, outOffset, outLength, price));
            var cap = limitCallGas(runState.getContract().gas(), fee, gasLimit);
            runState.getInterpreter().getVm().setCallGas(cap);

            return fee;
        };
    }

    /**
     * Calculates the amount needed for memory usage
     */
    private static long subMemUsage(RunContext runContext, long offset, long length, GasPrice gasPrice) {
        log.trace("SubMemCopy: offset={}({}) length={}({}) end={}({})",
            offset, HexUtils.toHex(offset, true),
            length, HexUtils.toHex(length, true),
            offset + length, HexUtils.toHex(offset + length, true));
        // Access with zero length will not extend the memory
        if (length == 0) return 0;
        var newMemoryWordCount = toWordSize(offset + length);
        //if memory doesn't grow returns 0
        if (newMemoryWordCount <= runContext.getMemoryWordCount()) return 0;
        //calculates gas fee for memory expansion
        var fee = gasPrice.lookForGasPrice("memory");
        var quadCoeff = gasPrice.lookForGasPrice("quadCoeffDiv");
        // words * 3 + words ^2 / 512
        var cost = Math.addExact(
            Math.multiplyFull(newMemoryWordCount, fee),
            Math.multiplyFull(newMemoryWordCount, newMemoryWordCount) / quadCoeff
        );
        if (cost > runContext.getHighestMemCost()) {
            var currentHighestMemCost = runContext.getHighestMemCost();
            runContext.setHighestMemCost(cost);
            cost -= currentHighestMemCost;
        }
        runContext.setMemoryWordCount(newMemoryWordCount);
        return cost;
    }

}
