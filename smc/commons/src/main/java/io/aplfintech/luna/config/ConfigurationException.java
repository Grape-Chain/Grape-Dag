package io.aplfintech.luna.config;

/**
 * Configuration exception
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class ConfigurationException extends RuntimeException {
    public ConfigurationException(String message) {
        super(message);
    }

    public ConfigurationException(Throwable cause) {
        super(cause);
    }
}
