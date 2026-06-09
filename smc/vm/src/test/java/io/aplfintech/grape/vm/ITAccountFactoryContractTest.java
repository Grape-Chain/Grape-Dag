package io.aplfintech.grape.vm;

import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.tx.MockBlock;
import io.aplfintech.grape.vm.utils.ContractHelper;
import io.aplfintech.grape.utils.Bytes;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.grape.vm.contract.ContractExecutor.address;
import static io.aplfintech.grape.vm.contract.ContractExecutor.inState;
import static io.aplfintech.grape.vm.contract.ContractExecutor.state;
import static org.assertj.core.api.Assertions.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITAccountFactoryContractTest {
    private static final String CONTRACT_FILE = "contracts/accountfactory-bytecode.json";
    private static final String ACCOUNT_CONTRACT_FILE = "contracts/account-bytecode.json";
    BlockContext block = new MockBlock();
    Address expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");

    @SneakyThrows
    @Test
    void publish() {
        //GIVEN
        long balance = 1_000_000_000L;
        var publisher = new VmAccount(address(0), 0, balance);

        //add the publisher account to the current state
        long gasPrice = 10L;
        var messageResult = state(block, publisher)
            //call the message (publish contract)
            .newMessage()
            .contractSpec(CONTRACT_FILE)
            .from(publisher)
            .value(0)
            .gasLimit(1_000_000L)
            .gasPrice(gasPrice)
            .when()
            .publish()
            .then()
            .isSuccess()
            .contractAddressIs(expectedContractAddress)
            .result();

        assertThat(publisher.balance().longValue())
            .isEqualTo(balance - messageResult.usedGas() * gasPrice);

    }

    @SneakyThrows
    @Test
    void createAccount() {
        var contractAccount = new VmAccount(expectedContractAddress, 0, 0L);
        long balance = 1_000_000_000L;
        var sender = new VmAccount(address(1), 0, balance);
        var owner = address(2);
        AtomicReference<ContractResult> contractResult = new AtomicReference<>();
        long depositValue = 1000;
        state(block, sender, contractAccount)
            //put contract in the state
            .contract(contractAccount.address(), ContractHelper.createCompiledContract(CONTRACT_FILE))
            //call message
            .newMessage()
            .from(sender)
            .to(contractAccount)
            .value(depositValue)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()//call createAccount(owner) method
            .call("createAccount", owner.hexAddress())
            .then()
            .isSuccess()
            .outputIsNull()
            //call next message
            .nextMessage()
            .to(contractAccount)
            .value(0L)
            .when()//call getMinter() method
            .call("accounts", "0")
            .then()
            .isSuccess()
            .resultContract(contractResult::set)
        ;

        var accountContractAddress0 = VmAddress.from(Bytes.trimLeftZeros(contractResult.get().output()));
        //call owner() that leads to call another smart-contract
        inState()
            .balanceIsEqual(accountContractAddress0, depositValue)
            .newMessage()
            .nonce(2)
            .when()
            .call("owner", accountContractAddress0.hexAddress())
            .then()
            .outputIsEqualTo((owner.hexAddress()))
        ;
        //call new created contract - Account contract from AccountFactory.sol
        inState()
            .newMessage()
            .contractSpec(ContractHelper.createCompiledContract(ACCOUNT_CONTRACT_FILE))
            .nonce(3)
            .from(sender)
            .to(accountContractAddress0)
            .value(0)
            .when()
            .call("owner")
            .then()
            .outputIsEqualTo(owner.hexAddress())
            .nextMessage()
            .when()
            .call("bank")
            .then()
            .outputIsEqualTo(contractAccount.address().hexAddress())
        ;

        inState().writeTrace();

    }

}
