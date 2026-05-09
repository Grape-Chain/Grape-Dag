package io.aplfintech.luna.bcei;

import lombok.NonNull;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

/**
 * It's a stub for DEI interface
 * Don't use in production
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class HostNode implements StateAccess, DEI {

    @Delegate
    protected final StateAccess stateAccess;

    public HostNode(@NonNull StateAccess stateAccess) {
        this.stateAccess = stateAccess;
    }
    
}
