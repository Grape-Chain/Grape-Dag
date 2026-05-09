package io.aplfintech.luna.vm.impl;

import io.aplfintech.luna.bcei.VmStateAccess;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.env.Context;
import io.aplfintech.luna.exception.ExecutionException;
import io.aplfintech.luna.l1vm.VmImpl;
import io.aplfintech.luna.vm.Message;
import io.aplfintech.luna.vm.MessageExecutor;
import io.aplfintech.luna.vm.MessageResult;
import io.aplfintech.luna.vm.Receipt;
import io.aplfintech.luna.vm.Vm;
import io.aplfintech.luna.vm.env.ContextSupplier;
import lombok.Getter;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public abstract class AbstractMessageExecutor implements MessageExecutor {
    protected final ChainConfig chainConfig;
    @Getter
    protected final ExecutionConfig executionConfig;
    protected final VmStateAccess stateAccess;
    protected final ContextSupplier contextSupplier;

    protected AbstractMessageExecutor(@NonNull ExecutionConfig cfg, @NonNull VmStateAccess stateAccess, @NonNull ContextSupplier contextSupplier) {
        this.executionConfig = cfg;
        this.chainConfig = cfg.chainConfig();
        this.stateAccess = stateAccess;
        this.contextSupplier = contextSupplier;
    }

    @Override
    public Receipt executeMessage(@NonNull Message message, @NonNull BlockContext blockContext) {
        //TODO use ReadContext() for 'STATIC CALL' i.e. read method calling
        var context = contextSupplier.get(message, blockContext);
        var vm = createMachine(context, executionConfig);
        try {
            var result = applyMessage(message, vm, stateAccess);
            Receipt receipt = Receipts.from(result);
            log.debug("Execution receipt={}", receipt);
            return receipt;
        } catch (ExecutionException e) {
            log.error("Caught {}:{}", e.getClass().getSimpleName(), e.getMessage());
            return Receipts.error(message, e.getClass().getSimpleName() + ':' + e.getMessage());
        } catch (Exception e) {
            String errMessage = "Execution error, type=" + e.getClass().getSimpleName() + " cause: " + e.getMessage() + ", given Message=" + message;
            log.error("Unexpected behaviour, caught unexpected exception type=" + e.getClass(), e);
            return Receipts.error(errMessage);
        }
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
    protected abstract MessageResult applyMessage(Message message, Vm vm, VmStateAccess stateAccess) throws ExecutionException;

    /**
     * Creates VM instance
     *
     * @param context current context
     * @param cfg     given execution config
     * @return VM instance
     */
    protected Vm createMachine(Context context, ExecutionConfig cfg) {
        return new VmImpl(context, cfg, chainConfig.vmConfig(), stateAccess);
    }

}
