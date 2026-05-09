package io.aplfintech.luna.vm.env;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.env.Context;
import io.aplfintech.luna.vm.Message;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContextSupplier {
    Context get(Message message, BlockContext block);
}
