package io.aplfintech.luna.vm.contract;

import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Addressable;
import lombok.NonNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface MessageSpec {
    MessageSpec contractSpec(CompiledContract contract);

    MessageSpec contractSpec(@NonNull String contractFile);

    MessageSpec incrementNonce();

    MessageSpec nonce(long nonce);

    MessageSpec from(Address address);

    default MessageSpec from(Addressable object) {
        return from(object.address());
    }

    MessageSpec to(Address address);

    default MessageSpec to(Addressable object) {
        return to(object.address());
    }

    MessageSpec value(long value);

    MessageSpec gasLimit(long value);

    MessageSpec gasPrice(long value);

    ContractCallSpec when();
}
