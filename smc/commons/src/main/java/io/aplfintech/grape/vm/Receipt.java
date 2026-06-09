package io.aplfintech.grape.vm;

/**
 * Generic interface for the transaction execution output
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Receipt {
    /**
     * Returns true if transaction completed with a SUCCESS status
     *
     * @return true iff transaction completed with a SUCCESS status
     */
    boolean success();

    /**
     * Returns the error message
     *
     * @return the error message
     */
    String errorMessage();

    /**
     * Returns the contract execution result
     *
     * @return the contract execution result
     */
    MessageResult result();

    /**
     * Returns a huma-readable version of the output receipt
     *
     * @return a huma-readable version of the output receipt
     */
    default String humanString() {
        return this.toString();
    }

}
