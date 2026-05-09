package io.aplfintech.luna.vm;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.utils.Hex;
import io.aplfintech.luna.vm.contract.ContractResult;
import io.aplfintech.luna.vm.tx.MockBlock;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.luna.vm.contract.ContractExecutor.address;
import static io.aplfintech.luna.vm.contract.ContractExecutor.message;
import static io.aplfintech.luna.vm.contract.ContractExecutor.state;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITStorage2ContractTest {
    BlockContext block = new MockBlock();

    @SneakyThrows
    @Test
    void storageContract() {
        //GIVEN
        var contractFile = "contracts/storage2-bytecode.json";
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        var nextSender = new VmAccount(address(1), 2, 1_000_000_000L);
        var initValue = "aabbccddeeff";
        var storageValue = "1234567890";
        AtomicReference<ContractResult> contractResult = new AtomicReference<>();

        //add the publisher account to the current state
        var state = state(block, publisher);

        //publish contract
        message(state)
            .contractSpec(contractFile)
            .from(publisher)
            .value(0)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()
            .publish(initValue)
            .then()
            .isSuccess()
            .contractAddressIs(expectedContractAddress)
            .resultContract(contractResult::set)//it's an example usage
            //call next message
            .nextMessage() //WHEN call retrieve() method
            .to(expectedContractAddress)
            .value(0L)
            .when()
            .call("retrieve")
            .then()
            .isSuccess()
            .outputIsEqualTo(initValue)
            //call next message
            .nextMessage()
            .to(expectedContractAddress)
            .value(0L)
            .when()//WHEN call store(0x123456789aabbccddeeff)
            .call("store", storageValue)
            .then()
            .isSuccess()
            .outputIsNull();
        //retrieve an address of the created contract
        var contractAddress = contractResult.get().contract();
        //Send message from another account
        var rc = state.account(nextSender)//put new account in the state
            .newMessage()
            .from(nextSender)
            .to(contractAddress)
            .value(0L)
            .when()//WHEN call retrieve() method
            .call("retrieve")
            .then()
            .isSuccess()
            .outputIsEqualTo(storageValue)
            .resultContract();

        //Thees assertions added to prevent the Sonar warning message
        //In general thees are duplicate all above verifications
        assertNotNull(rc);
        assertThat(rc.output())
            .isEqualTo(Hex.fromHexToWord(storageValue));

        //Send message with nonzero value
        message(state)
            .nonce(3)
            .from(nextSender)
            .to(contractAddress)
            .value(10L)
            .when()//WHEN call retrieve() method
            .call("retrieve")
            .then()
            .isRevert()
            .outputIsNull();

    }

    @SneakyThrows
    @Test
    void publishContract_withoutConstructorParameters() {
        //GIVEN
        var contractFile = "contracts/storage2-bytecode.json";
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);

        //add the publisher account to the current state
        state(block, publisher)
            .newMessage()//publish contract
            .contractSpec(contractFile)
            .from(publisher)
            .value(0)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()
            .publish(/*initValue*/)
            .then()
            .statusIsEqualTo(VmStatus.VM_REVERT)
            .outputIsNull();
    }

}