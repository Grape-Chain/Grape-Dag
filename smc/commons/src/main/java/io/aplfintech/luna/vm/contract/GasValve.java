package io.aplfintech.luna.vm.contract;

/**
 * Gas supplier interface
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface GasValve {
    /**
     * Returns the contract gas left
     *
     * @return the contract gas left
     */
    long gas();

    /**
     * Returns the used gas during the contract execution
     *
     * @return the used gas
     */
    long gasUsed();

    /**
     * Increase the contract gas on gas value
     *
     * @param gas value
     */
    void addGas(long gas);

    /**
     * Attempts to use gas and subtracts it and returns true on success
     *
     * @param gas specified gas to use
     * @return true iff contract has enough gas value to use specified gas
     */
    boolean useGas(long gas);

    /**
     * Resets gas
     */
    void resetGas();
}
