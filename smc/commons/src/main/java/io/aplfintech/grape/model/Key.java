package io.aplfintech.grape.model;

/**
 * General interface for storage key
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Key {
    byte[] bytes();

    String hex();
}
