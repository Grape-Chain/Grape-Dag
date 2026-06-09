package io.aplfintech.grape.vm;

import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.utils.Hex;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.tx.MockBlock;
import io.aplfintech.grape.utils.Bytes;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.grape.vm.contract.ContractExecutor.address;
import static io.aplfintech.grape.vm.contract.ContractExecutor.message;
import static io.aplfintech.grape.vm.contract.ContractExecutor.state;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITStorageContractTest {
    BlockContext block = new MockBlock();

    @SneakyThrows
    @Test
    void storageContract() {
        //GIVEN
        var contractFile = "contracts/storage-bytecode.json";
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        var nextSender = new VmAccount(address(1), 2, 1_000_000_000L);
        var storageValue = "123456789aabbccddeeff";
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
            .publish()
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
            .outputIsEqualTo(Bytes.alloc(32, 0))
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
        //In general thees one are duplicate all above verifications
        assertNotNull(rc);
        assertThat(rc.output())
            .isEqualTo(Hex.fromHexToWord(storageValue));

        //Send message with nonzero value and expect the REVERT status
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

}