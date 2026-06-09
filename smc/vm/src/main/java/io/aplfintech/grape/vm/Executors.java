package io.aplfintech.grape.vm;

import io.aplfintech.grape.bcei.CachedReadStateAccess;
import io.aplfintech.grape.bcei.DEI;
import io.aplfintech.grape.bcei.VmStateManager;
import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.CryptoConfig;
import io.aplfintech.grape.config.ExecutionConfig;
import io.aplfintech.grape.config.VmExecConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.l1vm.opcode.OpTableFactory;
import io.aplfintech.grape.tracers.LogTracer;
import io.aplfintech.grape.tracers.LoggerConfig;
import io.aplfintech.grape.tracers.MdLogger;
import io.aplfintech.grape.vm.env.ContextSupplier;
import io.aplfintech.grape.vm.env.VmContext;
import io.aplfintech.grape.vm.impl.AbstractMessageExecutor;
import io.aplfintech.grape.vm.impl.ExecutorFactory;
import io.aplfintech.grape.vm.impl.VmMessageExecutor;
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
