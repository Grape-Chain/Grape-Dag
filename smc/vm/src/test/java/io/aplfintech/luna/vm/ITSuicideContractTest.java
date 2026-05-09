package io.aplfintech.luna.vm;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.vm.contract.ContractResult;
import io.aplfintech.luna.vm.tx.MockBlock;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.util.Date;
import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.luna.vm.contract.ContractExecutor.address;
import static io.aplfintech.luna.vm.contract.ContractExecutor.message;
import static io.aplfintech.luna.vm.contract.ContractExecutor.state;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITSuicideContractTest {

    @SneakyThrows
    @Test
    void suicideContract_InvalidInstruction() {
        //GIVEN
        //SelfDestruct feature is not enabled
        BlockContext block = new MockBlock();
        var contractFile = "contracts/suicide-bytecode.json";
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
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
            .call("destroy")
            .then()
            .statusIsEqualTo(VmStatus.VM_INVALID_INSTRUCTION)
            .outputIsNull();
        //retrieve an address of the created contract
        var contractAddress = contractResult.get().contract();
        assertEquals(expectedContractAddress, contractAddress);

    }

    @SneakyThrows
    @Test
    void suicideContract() {
        //GIVEN
        BlockContext block = new MockBlock(new Date().getTime() / 1000);
        var contractFile = "contracts/suicide-bytecode.json";
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        long balance = 1_000_000L;
        var publisher = new VmAccount(address(0), 0, balance);
        AtomicReference<ContractResult> contractResult = new AtomicReference<>();
        var gasLimit = 100_000L;
        //add the publisher account to the current state
        var state = state(block, publisher);

        //publish contract
        long value = 100L;
        long gasPrice = 1L;
        var messageResult = message(state)
            .contractSpec(contractFile)
            .from(publisher)
            .value(value)
            .gasLimit(gasLimit)
            .gasPrice(gasPrice)
            .when()
            .publish()
            .then()
            .isSuccess()
            .contractAddressIs(expectedContractAddress)
            .resultContract(contractResult::set)
            .result();

        //retrieve an address of the created contract
        var contractAddress = contractResult.get().contract();
        assertEquals(expectedContractAddress, contractAddress);
        assertThat(publisher.balance().longValue())
            .isEqualTo(balance - messageResult.usedGas() * gasPrice - value);
        //call next message
        balance = publisher.balance().longValue();
        messageResult = state.nextMessage() //WHEN call destroy() method
            .to(expectedContractAddress)
            .value(0L)
            .when()
            .call("destroy")
            .then()
            .isSuccess()
            .outputIsNull()
            .result();
        //value is transferred to owner after the suicide
        assertThat(publisher.balance().longValue())
            .isEqualTo(balance - messageResult.usedGas() * gasPrice + value);

    }

    @SneakyThrows
    @Test
    void suicideContract_not_owner() {
        //GIVEN
        BlockContext block = new MockBlock(new Date().getTime() / 1000);
        var contractFile = "contracts/suicide-bytecode.json";
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        var nextSender = new VmAccount(address(1), 0, 1_000_000_000L);
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
            .resultContract(contractResult::set);
        //retrieve an address of the created contract
        var contractAddress = contractResult.get().contract();
        //Send message from another account
        var rc = state.account(nextSender)//put new account in the state
            .newMessage()
            .from(nextSender)
            .to(contractAddress)
            .value(0L)
            .when()//WHEN call retrieve() method
            .call("destroy")
            .then()
            .isRevert()//msg.sender is not the owner
            .resultContract();

        //Thees assertions added to prevent the Sonar warning message
        assertNotNull(rc);

    }

}