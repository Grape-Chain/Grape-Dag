package io.aplfintech.luna.tracers;

/**
 * Configuration options for the tracer/logger
 *
 * @param memoryEnabled     enable the memory capture
 * @param stackDisabled     disable the stack capture
 * @param storageDisabled   disable the storage capture
 * @param returnDataEnabled enable the return data capture
 * @param outputLength      maximum length of output, but zero means unlimited
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public record LoggerConfig(boolean memoryEnabled, boolean stackDisabled, boolean storageDisabled,
                           boolean returnDataEnabled, int outputLength) {
    public static LoggerConfig defaultConfig() {
        return new LoggerConfig(true, false, false, true, 1024);
    }
}
