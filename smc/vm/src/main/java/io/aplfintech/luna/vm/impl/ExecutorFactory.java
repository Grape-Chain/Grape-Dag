package io.aplfintech.luna.vm.impl;

import io.aplfintech.luna.bcei.DEI;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.config.VmExecConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.tracers.DebugLogger;
import io.aplfintech.luna.tracers.LogTracer;
import io.aplfintech.luna.tracers.LoggerConfig;
import io.aplfintech.luna.tracers.MdLogger;
import io.aplfintech.luna.tracers.StructTracer;
import io.aplfintech.luna.tracers.Tracer;
import io.aplfintech.luna.utils.TracerUtils;
import io.aplfintech.luna.vm.Executors;
import io.aplfintech.luna.vm.MessageExecutor;
import io.aplfintech.luna.vm.opcode.OpTable;
import lombok.NonNull;

import java.io.PrintWriter;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.function.Consumer;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class ExecutorFactory {
    public static final String THE_EXECUTION_CONFIG_NOT_CONFIGURED = "The execution config not configured.";
    private final ChainConfig chainConfig;
    private ExecutionConfig executionConfig;

    private ExecutorFactory(ChainConfig chainConfig) {
        this.chainConfig = chainConfig;
    }

    public static ExecutorFactory newFactory(@NonNull ChainConfig chainConfig) {
        return new ExecutorFactory(chainConfig);
    }

    public ExecutorFactory executionConfig(ExecutionConfig cfg) {
        this.executionConfig = cfg;
        return this;
    }

    public MessageExecutor executor(@NonNull DEI dei) {
        Objects.requireNonNull(executionConfig, THE_EXECUTION_CONFIG_NOT_CONFIGURED);
        return Executors.createExecutor(dei, executionConfig);
    }

    public MessageExecutor applier(@NonNull DEI dei) {
        Objects.requireNonNull(executionConfig, THE_EXECUTION_CONFIG_NOT_CONFIGURED);
        return Executors.createApplier(dei, executionConfig);
    }

    public MessageExecutor estimator(@NonNull DEI dei) {
        Objects.requireNonNull(executionConfig, THE_EXECUTION_CONFIG_NOT_CONFIGURED);
        return new GasEstimator(chainConfig, executionConfig, dei);
    }


    public ExecConfigBuilder configuration() {
        return new ExecConfigBuilder(this);
    }

    public static class TracerBuilder {
        private final ExecConfigBuilder execConfigBuilder;
        private final LoggerConfig loggerConfig;
        List<Tracer> tracers;
        List<Tracer> loggers;

        public TracerBuilder(@NonNull ExecConfigBuilder execConfigBuilder,
                             @NonNull LoggerConfig loggerConfig) {
            this.execConfigBuilder = execConfigBuilder;
            this.loggerConfig = loggerConfig;
            this.tracers = new ArrayList<>();
            this.loggers = new ArrayList<>();
        }

        public TracerBuilder addTracer(@NonNull Tracer tracer) {
            tracers.add(tracer);
            return this;
        }

        public TracerBuilder addStructTracer() {
            addStructTracer(this.loggerConfig);
            return this;
        }

        public TracerBuilder addStructTracer(@NonNull LoggerConfig loggerConfig) {
            var tracer = new StructTracer(loggerConfig);
            tracers.add(tracer);
            return this;
        }

        public TracerBuilder addMdLogger() {
            var printWriter = TracerUtils.stdOutWriter();
            addMdLogger(printWriter, this.loggerConfig);
            return this;
        }

        public TracerBuilder addMdLogger(@NonNull LoggerConfig loggerConfig) {
            var printWriter = TracerUtils.stdOutWriter();
            addMdLogger(printWriter, loggerConfig);
            return this;
        }

        public TracerBuilder addMdLogger(@NonNull PrintWriter writer) {
            addMdLogger(writer, this.loggerConfig);
            return this;
        }

        public TracerBuilder addMdLogger(@NonNull PrintWriter writer, @NonNull LoggerConfig loggerConfig) {
            var logger = (Tracer) new LogTracer(
                new MdLogger(writer, loggerConfig)
            );
            loggers.add(logger);
            return this;
        }

        public TracerBuilder addDebugLogger() {
            var logger = new LogTracer(
                new DebugLogger(this.loggerConfig)
            );
            loggers.add(logger);
            return this;
        }

        public TracerBuilder addDebugLogger(@NonNull LoggerConfig loggerConfig) {
            var logger = new LogTracer(
                new DebugLogger(loggerConfig)
            );
            loggers.add(logger);
            return this;
        }

        public ExecConfigBuilder buildTracer() {
            if (tracers.isEmpty() && loggers.isEmpty()) {
                throw new IllegalStateException("Neither Tracer nor Logger is configured.");
            }
            tracers.addAll(loggers);
            var first = tracers.remove(0);
            var tracer = Tracer.link(first, tracers.toArray(new Tracer[0]));
            execConfigBuilder.setTracer(tracer);
            return execConfigBuilder;
        }
    }

    public static class ExecConfigBuilder {
        private final ExecutorFactory executorFactory;
        private boolean debugEnabled = true;
        private boolean noBaseFee = true;
        private Tracer tracer;
        private OpTable opTable;
        private CryptoLib cryptoLib;

        private ExecConfigBuilder(ExecutorFactory executorFactory) {
            this.executorFactory = executorFactory;
        }

        void setTracer(Tracer tracer) {
            this.tracer = tracer;
        }

        public ExecConfigBuilder debugEnabled(boolean debugEnabled) {
            this.debugEnabled = debugEnabled;
            return this;
        }

        public ExecConfigBuilder noBaseFee(boolean noBaseFee) {
            this.noBaseFee = noBaseFee;
            return this;
        }

        public TracerBuilder tracers(@NonNull LoggerConfig loggerConfig) {
            return new TracerBuilder(this, loggerConfig);
        }

        public ExecConfigBuilder opTable(OpTable opTable) {
            this.opTable = opTable;
            return this;
        }

        public ExecConfigBuilder cryptoLib(CryptoLib cryptoLib) {
            this.cryptoLib = cryptoLib;
            return this;
        }

        public ExecConfigBuilder buildExecutionConfig(@NonNull Consumer<ExecutionConfig> consumer) {
            consumer.accept(buildExecConfig());
            return this;
        }

        public MessageExecutor executor(@NonNull DEI dei) {
            var cfg = buildExecConfig();
            executorFactory.executionConfig(cfg);
            return Executors.createExecutor(dei, cfg);
        }

        public MessageExecutor applier(@NonNull DEI dei) {
            var cfg = buildExecConfig();
            executorFactory.executionConfig(cfg);
            return Executors.createApplier(dei, cfg);
        }

        private ExecutionConfig buildExecConfig() {
            return VmExecConfig.create(executorFactory.chainConfig,
                debugEnabled, noBaseFee, tracer, opTable, cryptoLib);
        }
    }
}
