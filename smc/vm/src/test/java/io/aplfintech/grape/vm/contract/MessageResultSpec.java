package io.aplfintech.grape.vm.contract;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.MessageResult;

import java.util.function.Consumer;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface MessageResultSpec {
    MessageResultSpec then();

    MessageResultSpec resultContract(Consumer<ContractResult> consumer);

    MessageResultSpec result(Consumer<MessageResult> consumer);

    ContractResult resultContract();

    MessageResult result();

    MessageResultSpec isSuccess();

    MessageResultSpec isRevert();

    MessageResultSpec statusIsEqualTo(ExecutionStatus expected);

    MessageResultSpec outputIsEqualTo(byte[]... expected);

    MessageResultSpec outputIsEqualTo(String expectedHex);

    MessageResultSpec outputIsNull();

    MessageResultSpec outputStartsWith(byte[] expected);

    MessageResultSpec contractAddressIs(Address expected);

    StateSpec inState();

    MessageSpec nextMessage();
}
