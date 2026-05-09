package io.aplfintech.luna.vm;

import io.aplfintech.luna.env.BlockContext;
import lombok.NonNull;

/**
 * The contract message executor
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface MessageExecutor {

    /**
     * Executes the given message in the given block context and computes the new state
     *
     * @param message      given message
     * @param blockContext current block context
     * @return the transaction execution receipt
     */
    Receipt executeMessage(@NonNull Message message, @NonNull BlockContext blockContext);
}
