package io.aplfintech.grape.vm.env;

import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.env.Context;
import io.aplfintech.grape.vm.Message;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ContextSupplier {
    Context get(Message message, BlockContext block);
}
