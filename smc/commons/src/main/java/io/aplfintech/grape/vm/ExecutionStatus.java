package io.aplfintech.grape.vm;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ExecutionStatus {
    boolean isSuccess();

    boolean isFailure();

    /**
     * Returns true if current error is related with the system/internal error of the VM
     */
    default boolean isInternalError() {
        return getErrorCode() < 0;
    }

    /**
     * Returns the short error name
     */
    String getName();

    /**
     * Returns the error code
     */
    int getErrorCode();

    default String fullName() {
        return getName() + '(' + getErrorCode() + ')';
    }

}
