package io.aplfintech.grape.vm.impl;

import io.aplfintech.grape.bcei.CachedReadStateAccess;
import io.aplfintech.grape.bcei.DEI;
import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.bcei.VmStateManager;
import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ExecutionConfig;
import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.exception.ExecutionException;
import io.aplfintech.grape.exception.IntrinsicGasException;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.l1vm.VmImpl;
import io.aplfintech.grape.l1vm.VmMessage;
import io.aplfintech.grape.l1vm.VmResult;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.utils.Exceptions;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.MessageExecutor;
import io.aplfintech.grape.vm.MessageResult;
import io.aplfintech.grape.vm.Receipt;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.env.VmContext;
import lombok.Getter;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class GasEstimator implements MessageExecutor {
    private static final String EXECUTE_ESTIMATION_PREFIX = "###+++ EXECUTE ESTIMATION";
    private final ChainConfig chainConfig;
    private final ExecutionConfig executionConfig;
    private final DEI stateAccess;
    private final GasPrice priceMap;

    public GasEstimator(@NonNull ChainConfig chainConfig, @NonNull ExecutionConfig cfg, @NonNull DEI stateAccess) {
        this.executionConfig = cfg;
        this.chainConfig = chainConfig;
        this.stateAccess = stateAccess;
        this.priceMap = chainConfig.gasPriceMap();
    }

    @Override
    public Receipt executeMessage(@NonNull final Message inMessage, @NonNull final BlockContext block) {
        Address sender;
        if (inMessage.from().equals(VmAddress.UNDEFINED_ADDRESS)) {
            sender = VmAddress.ZERO_ADDRESS;
        } else {
            sender = inMessage.from();
        }
        var balance = Math256.MAX_UNSIGNED_BIGINTEGER;//set unlimited balance
        VmAccount account = new VmAccount(sender, inMessage.nonce(), balance);

        var message = VmMessage.toBuilder(inMessage)
            .from(sender)
            .nonce(0)
            .fake(true)
            .build();

        Receipt receipt;
        try {
            //estimate gas
            BigInteger usedGas = estimateGas(message, block, account);
            //
            ContractResult contractResult = VmResult.success(usedGas.longValue(), message.to());
            var messageResult = new VmMessageResult(contractResult, inMessage.gasLimit().longValue(), 0, usedGas.longValue(), 0);
            receipt = Receipts.from(messageResult);
        } catch (Exception e) {
            receipt = Receipts.error(e.getMessage());
        }

        return receipt;
    }

    private BigInteger estimateGas(VmMessage message, BlockContext block, VmAccount account) {
        //TODO use BigInteger instead of long for all gasLimit operations
        long lo = priceMap.lookForGasPrice("tx");
        long hi = block.gasLimit().longValueExact();
        var cap = hi;
        EstimateResult rc;
        int i = 0;
        while (lo + 1 < hi) {
            var mid = (lo + hi) / 2;
            if (log.isTraceEnabled()) {
                log.trace("*** ESTIMATOR: iteration={}, mid gas={}", ++i, mid);
            }
            rc = estimateMessageExecution(message, block, mid, account);
            if (rc.isError()) {
                throw Exceptions.from(VmStatus.VM_FAILURE, rc.errorMessage);
            } else {
                if (rc.isFailed()) {
                    lo = mid;
                } else {
                    hi = mid;
                }
            }
        }

        if (hi == cap) {
            rc = estimateMessageExecution(message, block, hi, account);
            if (rc.isError()) {
                throw Exceptions.from(VmStatus.VM_FAILURE, rc.errorMessage);
            } else {
                if (rc.isFailed()) {
                    throw Exceptions.from(VmStatus.VM_FAILURE, "Gas required exceeds allowance=" + cap);
                }
            }
        }
        return BigInteger.valueOf(hi);
    }

    private EstimateResult estimateMessageExecution(VmMessage message, BlockContext block, long proposedGasLimit, VmAccount account) {
        message.setGasLimit(BigInteger.valueOf(proposedGasLimit));
        CachedReadStateAccess cachedReadStateAccess = new CachedReadStateAccess(stateAccess);
        VmStateManager vmStateManager = new VmStateManager(cachedReadStateAccess);
        if (account != null) {//put account in the cached state
            cachedReadStateAccess.putAccount(account.address(), account);
        }

        var context = new VmContext(message.from(), message.gasPrice(), block);
        var vm = new VmImpl(context, executionConfig, chainConfig.vmConfig(), vmStateManager);
        try {
            var messageResult = applyMessage(message, vm, vmStateManager);
            /*if (messageResult.hasError()) {
                if (messageResult.executionStatus() == VmStatus.VM_OUT_OF_GAS ||
                    messageResult.executionStatus() == VmStatus.VM_CODESTORE_OUT_OF_GAS) {
                    return new EstimateResult(false, true, messageResult);
                }
                if (messageResult.executionStatus() == VmStatus.VM_REVERT) {
                    return new EstimateResult(false, true, messageResult);
                }
                return new EstimateResult(true, true, messageResult);
            }*/
            return new EstimateResult(false, messageResult.hasError(), messageResult);
        } catch (IntrinsicGasException e) {
            return new EstimateResult(false, true, e.getMessage());
        } catch (Exception e) {
            return new EstimateResult(true, false, e.getMessage());
        }
    }

    private MessageResult applyMessage(Message message, Vm vm, VmStateAccess stateAccess) throws ExecutionException {
        log.debug(EXECUTE_ESTIMATION_PREFIX + ": Incoming message from={} to={} gas limit={}", message.from(), message.to(), message.gasLimit());
        if (log.isTraceEnabled()) {
            log.trace(EXECUTE_ESTIMATION_PREFIX + ": Incoming message={}", message);
        }
        var st = new StateTransition(message, vm, stateAccess, executionConfig);
        //try to apply message without the state changes
        var result = st.apply();
        if (log.isTraceEnabled()) {
            log.trace(EXECUTE_ESTIMATION_PREFIX + ": Result={} message={}", result, message);
        }
        log.debug(EXECUTE_ESTIMATION_PREFIX + ": Result status={} initGas={} refundGas={} usedGas={} gas={}",
            result.contractResult().executionStatus(), result.initGas(), result.refundGas(), result.usedGas(), result.gas());
        return result;
    }

    @Getter
    private static final class EstimateResult {
        //execution status has error
        private final boolean error;
        //final contract execution status during estimation
        private final boolean failed;
        //contract execution result
        private MessageResult result;
        //error message in case exception occurs
        private String errorMessage;

        private EstimateResult(boolean error, boolean failed, @NonNull MessageResult result) {
            this.error = error;
            this.failed = failed;
            this.result = result;
        }

        private EstimateResult(boolean error, boolean failed, @NonNull String errorMessage) {
            this.error = error;
            this.failed = failed;
            this.errorMessage = errorMessage;
        }
    }

}
