package io.aplfintech.grape.vm.impl;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.config.ExecutionConfig;
import io.aplfintech.grape.exception.ExecutionException;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.MessageResult;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.env.ContextSupplier;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class VmMessageExecutor extends AbstractMessageExecutor {

    public VmMessageExecutor(@NonNull ExecutionConfig cfg, @NonNull VmStateAccess stateAccess, @NonNull ContextSupplier contextSupplier) {
        super(cfg, stateAccess, contextSupplier);
    }

    /**
     * Computes the new state by applying the given message
     * and returns the result the 'call message' execution
     *
     * @param message     the message sent to a contract
     * @param vm          the execution VM
     * @param stateAccess the access to the state
     * @return the result of the 'call message' execution
     */
    @Override
    protected MessageResult applyMessage(Message message, Vm vm, VmStateAccess stateAccess) throws ExecutionException {
        if (log.isDebugEnabled()) {
            String data;
            if (message.data() != null) {
                int end = Math.min(message.data().length, 32);
                data = HexUtils.toHex(Bytes.slice(message.data(), 0, end), true) + (message.data().length > 32 ? " ..." : "");
            } else {
                data = "null";
            }
            log.debug("###+++ EXECUTE: Incoming message from={} to={} gasLimit={} input={}", message.from(), message.to(), message.gasLimit(), data);
        }
        if (log.isTraceEnabled()) {
            log.trace("###+++ EXECUTE: Incoming message={}", message);
        }
        var st = new StateTransition(message, vm, stateAccess, executionConfig);
        //apply state changes
        var result = st.apply();

        if (log.isTraceEnabled()) {
            log.trace("###+++ EXECUTE: Result={} message={}", result, message);
        }
        log.debug("###+++ EXECUTE: Result status={} initGas={} refundGas={} usedGas={} gas={}",
            result.contractResult().executionStatus(), result.initGas(), result.refundGas(), result.usedGas(), result.gas());

        return result;
    }

}
