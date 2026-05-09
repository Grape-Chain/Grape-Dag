package io.aplfintech.luna.config;

import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.tracers.NullTracer;
import io.aplfintech.luna.tracers.Tracer;
import io.aplfintech.luna.vm.opcode.OpTable;
import lombok.NonNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmExecConfig implements ExecutionConfig {
    private final boolean debugEnabled;
    private final Tracer tracer;
    private final OpTable opTable;
    private final boolean noBaseFee;
    private final CryptoLib cryptoLib;
    private final ChainConfig chainConfig;

    private VmExecConfig(ChainConfig chainConfig, OpTable opTable, CryptoLib cryptoLib, Tracer tracer, boolean debugEnabled, boolean noBaseFee) {
        this.debugEnabled = debugEnabled;
        this.tracer = tracer;
        this.opTable = opTable;
        this.cryptoLib = cryptoLib;
        this.noBaseFee = noBaseFee;
        this.chainConfig = chainConfig;
    }

    public static VmExecConfig create(@NonNull ChainConfig chainConfig,
                                      boolean debugEnabled,
                                      boolean noBaseFee,
                                      Tracer tracer,
                                      @NonNull OpTable opTable,
                                      @NonNull CryptoLib cryptoLib) {
        boolean isDebug = debugEnabled;
        Tracer tr;
        if (tracer == null) {
            isDebug = false;
            tr = new NullTracer();
        } else {
            tr = tracer;
        }
        return new VmExecConfig(chainConfig, opTable, cryptoLib, tr, isDebug, noBaseFee);
    }

    public static VmExecConfigBuilder builder() {
        return new VmExecConfigBuilder();
    }

    @Override
    public boolean isDebugEnabled() {
        return debugEnabled;
    }

    @Override
    public Tracer tracer() {
        return tracer;
    }

    @Override
    public OpTable opTable() {
        return opTable;
    }

    @Override
    public ChainConfig chainConfig() {
        return chainConfig;
    }

    @Override
    public boolean isNoBaseFee() {
        return noBaseFee;
    }

    @Override
    public CryptoLib cryptoLib() {
        return cryptoLib;
    }

    /**
     * @author andrew.zinchenko@gmail.com
     * @since 0.1
     */
    public static final class VmExecConfigBuilder {
        private boolean debugEnabled;
        private boolean noBaseFee;
        private Tracer tracer;
        private OpTable opTable;
        private CryptoLib cryptoLib;

        private VmExecConfigBuilder() {
            //set default values
            this.debugEnabled = false;
            this.noBaseFee = true;
        }

        public VmExecConfigBuilder debugEnabled(boolean debugEnabled) {
            this.debugEnabled = debugEnabled;
            return this;
        }

        public VmExecConfigBuilder noBaseFee(boolean noBaseFee) {
            this.noBaseFee = noBaseFee;
            return this;
        }

        public VmExecConfigBuilder tracer(Tracer tracer) {
            this.tracer = tracer;
            return this;
        }

        public VmExecConfigBuilder opTable(OpTable opTable) {
            this.opTable = opTable;
            return this;
        }

        public VmExecConfigBuilder cryptoLib(CryptoLib cryptoLib) {
            this.cryptoLib = cryptoLib;
            return this;
        }

        public VmExecConfig build(@NonNull ChainConfig chainConfig) {
            return create(chainConfig, debugEnabled, noBaseFee, tracer, opTable, cryptoLib);
        }
    }
}
