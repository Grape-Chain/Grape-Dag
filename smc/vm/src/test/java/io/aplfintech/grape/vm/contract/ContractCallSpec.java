package io.aplfintech.grape.vm.contract;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContractCallSpec {
    MessageResultSpec publish(String... args);

    MessageResultSpec call(String method, String... args);
}
