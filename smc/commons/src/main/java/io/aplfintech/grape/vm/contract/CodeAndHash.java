package io.aplfintech.grape.vm.contract;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface CodeAndHash extends Code {
    /**
     * Returns the hash of contract code
     *
     * @return hash of contracts code
     */
    byte[] codeHash();
}
