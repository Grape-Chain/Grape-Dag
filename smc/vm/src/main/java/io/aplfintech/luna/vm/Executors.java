package io.aplfintech.luna.vm;

import io.aplfintech.luna.bcei.CachedReadStateAccess;
import io.aplfintech.luna.bcei.DEI;
import io.aplfintech.luna.bcei.VmStateManager;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.config.VmExecConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.opcode.OpTableFactory;
import io.aplfintech.luna.tracers.LogTracer;
import io.aplfintech.luna.tracers.LoggerConfig;
import io.aplfintech.luna.tracers.MdLogger;
import io.aplfintech.luna.vm.env.ContextSupplier;
import io.aplfintech.luna.vm.env.VmContext;
import io.aplfintech.luna.vm.impl.AbstractMessageExecutor;
import io.aplfintech.luna.vm.impl.ExecutorFactory;
import io.aplfintech.luna.vm.impl.VmMessageExecutor;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;

import java.io.PrintWriter;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Executors {
    static CryptoLib crypto = CryptoConfig.crypto();
    public static final ContextSupplier EXECUTOR_CONTEXT = (message, block) -> new VmContext(message.from(), message.gasPrice(), block);
    public static final ContextSupplier APPLIER_CONTEXT = (message, block) -> new VmContext(message.from(), message.gasPrice(), block);

    public static MessageExecutor createEstimator(@NonNull ChainConfig chainConfig, @NonNull DEI dei, PrintWriter printWriter) {
        var opTableFactory = OpTableFactory.newFactory(chainConfig, CryptoConfig.crypto());
        var vmExecConfigBuilder = VmExecConfig.builder()
            .noBaseFee(true)
            .debugEnabled(true)
            .opTable(opTableFactory.createTable())
            .cryptoLib(crypto);

        if (printWriter != null) {
            var tracerCfg = LoggerConfig.defaultConfig();
            vmExecConfigBuilder.tracer(new LogTracer(new MdLogger(printWriter, tracerCfg)));
        }

        VmExecConfig cfg = vmExecConfigBuilder.build(chainConfig);
        return ExecutorFactory.newFactory(chainConfig)
            .executionConfig(cfg)
            .estimator(dei);
    }

    public static AbstractMessageExecutor createExecutor(@NonNull DEI dei, @NonNull ExecutionConfig executionConfig) {
        var stateManager = new VmStateManager(dei);
        return new VmMessageExecutor(executionConfig, stateManager, EXECUTOR_CONTEXT);
    }

    public static AbstractMessageExecutor createApplier(@NonNull DEI dei, @NonNull ExecutionConfig executionConfig) {
        var zAccount = new VmAccount(VmAddress.ZERO_ADDRESS, 0, 0);
        var stateManager = new VmStateManager(new CachedReadStateAccess(dei, zAccount));
        return new VmMessageExecutor(executionConfig, stateManager, APPLIER_CONTEXT);
    }

}
