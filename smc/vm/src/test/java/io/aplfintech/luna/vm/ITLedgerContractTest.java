package io.aplfintech.luna.vm;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.vm.contract.ContractResult;
import io.aplfintech.luna.vm.tx.MockBlock;
import io.aplfintech.luna.vm.utils.ContractHelper;
import io.aplfintech.luna.utils.Bytes;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.math.BigInteger;
import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.luna.vm.contract.ContractExecutor.address;
import static io.aplfintech.luna.vm.contract.ContractExecutor.state;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITLedgerContractTest {

    private static final String CONTRACT_FILE = "contracts/ledger-bytecode.json";
    BlockContext block = new MockBlock();

    @SneakyThrows
    @Test
    void publish() {
        //GIVEN
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        var receiver = address(1);
        var mintBeneficiary = address(2);
        var nextSender = new VmAccount(mintBeneficiary, 0, 1_000_000_000L);
        AtomicReference<ContractResult> contractResult = new AtomicReference<>();

        //add the publisher account to the current state
        state(block, publisher, nextSender)
            //call the message (publish contract)
            .newMessage()
            .contractSpec(CONTRACT_FILE)
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

        ;
    }

    @SneakyThrows
    @Test
    void checkNotPayableConstructor() {
        //GIVEN
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        //add the publisher account to the current state
        state(block, publisher)
            //call message
            .newMessage()
            .contractSpec(CONTRACT_FILE)
            .from(publisher)
            .value(10)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()
            .publish()
            .then()
            .isRevert()//because the contract constructor is not payable
            .outputIsNull()
        ;
    }

    @SneakyThrows
    @Test
    void checkNotPayableMethod() {
        var sender = new VmAccount(address(0), 0, 1_000_000_000L);
        Address contractAddress = address(1);
        state(block, sender)
            //put contract in the state
            .contract(contractAddress, ContractHelper.createCompiledContract(CONTRACT_FILE))
            //call message
            .newMessage()
            .from(sender)
            .to(contractAddress)
            .value(10)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()//call minter() method
            .call("withdrawMoney")
            .then()
            .isRevert()//because the contract constructor is not payable
            .outputIsNull()
        ;
    }

    @SneakyThrows
    @Test
    void deposit() {
        long balance = 1_000_000_000L;
        var sender = new VmAccount(address(0), 0, balance);
        var contractAccount = new VmAccount(address(1), 0, 0L);

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
            .when()//call send(receiver, 0x20000) method
            .call("deposit")
            .then()
            .isSuccess()
            .outputIsNull()
            .inState()
            .balanceIsEqual(contractAccount, BigInteger.valueOf(depositValue))
            //call next message
            .nextMessage()
            .value(0L)
            .when()//call getMinter() method
            .call("getDepositNum", sender.address().hexAddress(), "0")
            .then()
            .isSuccess()
            .outputStartsWith(Math256.padToWord(Bytes.toBytes(depositValue)))
            //call next message
            .nextMessage()
        ;
    }

}
