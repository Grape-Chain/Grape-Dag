package io.aplfintech.luna.config;

/**
 * Loads config for application
 *
 * @param <T> identify type of the loaded config
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ConfigLoader<T> {
    /**
     * Returns the loaded config
     *
     * @return loaded config
     */
    T load();
}
