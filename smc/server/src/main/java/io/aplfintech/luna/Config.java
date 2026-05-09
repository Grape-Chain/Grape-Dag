package io.aplfintech.luna;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ChainConfigFactory;
import io.aplfintech.luna.config.ChainConfigLoader;
import io.aplfintech.luna.config.GrapeChainConfig;
import lombok.NonNull;

import java.util.Date;


public class Config {
    static Config instance;
    private static volatile boolean initialized;
    private ChainConfigFactory configFactory;
    private String logDir;
    private MdLoggerType estimatorMdLogger;
    private boolean tracerDisabled;
    private boolean mdLoggerDisabled;

    private Config() {
    }

    public static synchronized void init(@NonNull String logDir, boolean tracerDisabled, boolean mdLoggerDisabled, boolean enableEstimatorMdLogger) {
        if (initialized) {
            throw new IllegalStateException("Config is already loaded");
        }
        ChainConfigLoader configLoader = new ChainConfigLoader();
        instance = new Config();
        var grapeChainConfig = configLoader.load();
        instance.configFactory = new ChainConfigFactory(grapeChainConfig);
        instance.logDir = logDir;
        instance.estimatorMdLogger = toMdLoggerType(enableEstimatorMdLogger);
        instance.tracerDisabled = tracerDisabled;
        instance.mdLoggerDisabled = mdLoggerDisabled;

        initialized = true;
    }

    public static synchronized GrapeChainConfig grapeChainConfig() {
        requireLoadedConfig();
        return instance.configFactory.getGrapeChainConfig();
    }

    /**
     * Returns the actual chain config at the given date
     *
     * @param date the date in UTC
     * @return the actual chain config at the given date
     */
    public static synchronized ChainConfig chainConfigAt(Date date) {
        requireLoadedConfig();
        return instance.configFactory.configAt(date);
    }

    /**
     * Returns the actual chain config at the given timestamp
     *
     * @param timestamp timestamp in seconds
     * @return the actual chain config at the given timestamp
     */
    public static synchronized ChainConfig chainConfigAt(long timestamp) {
        requireLoadedConfig();
        return instance.configFactory.configAt(timestamp);
    }

    private static void requireLoadedConfig() {
        if (!initialized) {
            throw new IllegalStateException("Config is not loaded yet");
        }
    }

    public static synchronized String logDir() {
        requireLoadedConfig();
        return instance.logDir;
    }

    public static MdLoggerType estimatorMdLogger() {
        requireLoadedConfig();
        return instance.estimatorMdLogger;
    }

    public static boolean isTracerDisabled() {
        return instance.tracerDisabled;
    }

    public static boolean isTracerEnabled() {
        return !instance.tracerDisabled;
    }

    public static boolean isMdLoggerDisabled() {
        return instance.mdLoggerDisabled;
    }

    public static boolean isMdLoggerEnabled() {
        return !instance.mdLoggerDisabled;
    }

    private static MdLoggerType toMdLoggerType(boolean enabled) {
        if (enabled) {
            return MdLoggerType.FILE;
        }
        return MdLoggerType.DISABLED;
    }

    public enum MdLoggerType {
        DISABLED,
        FILE,
        CONSOLE
    }

}
