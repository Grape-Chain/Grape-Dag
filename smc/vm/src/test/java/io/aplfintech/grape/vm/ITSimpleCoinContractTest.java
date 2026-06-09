package io.aplfintech.grape.vm;

import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.tx.MockBlock;
import io.aplfintech.grape.vm.utils.ContractHelper;
import io.aplfintech.grape.utils.HexUtils;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.grape.vm.contract.ContractExecutor.address;
import static io.aplfintech.grape.vm.contract.ContractExecutor.state;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITSimpleCoinContractTest {

    private static final String CONTRACT_FILE = "contracts/simplecoin-bytecode.json";
    BlockContext block = new MockBlock();

    @SneakyThrows
    @Test
    void trueCase() {
        //GIVEN
        var expectedContractAddress = VmAddress.from("0x5fbdb2315678afecb367f032d93f642f64180aa3");
        var publisher = new VmAccount(address(0), 0, 1_000_000_000L);
        var receiver = address(1);
        var mintBeneficiary = address(2);
        var nextSender = new VmAccount(mintBeneficiary, 5, 1_000_000_000L);
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
            //call next message
            .nextMessage()
            .to(contractResult.get().contract())
            .value(0L)
            .when()//call getMinter() method
            .call("getMinter")
            .then()
            .isSuccess()
            .outputIsEqualTo(Math256.padToWord(publisher.address().bytes()))
            //call next message
            .nextMessage()
            .when()//call minter() method
            .call("minter")
            .then()
            .isSuccess()
            .outputIsEqualTo(Math256.padToWord(publisher.address().bytes()))
            //call next message
            .nextMessage()
            .when()//call mint(beneficiary, 0x12345) method
            .call("mint", mintBeneficiary.hexAddress(), "0x12345")
            .then()
            .isSuccess()
            .outputIsNull()
            //call next message
            .nextMessage()
            .when()//WHEN call balance(beneficiary) method
            .call("balance", mintBeneficiary.hexAddress())
            .then()
            .isSuccess()
            .outputIsEqualTo("0x12345")
            //call next message
            .nextMessage()
            .from(nextSender)
            .when()//call send(receiver, 0x2345) method
            .call("send", receiver.hexAddress(), "0x2345")
            .then()
            .isSuccess()
            .outputIsNull()
            //call next message
            .nextMessage()
            .when()
            .call("balance", receiver.hexAddress())
            .then()
            .isSuccess()
            .outputIsEqualTo("0x2345")
            //call next message
            .nextMessage()
            .when()//WHEN call balance(beneficiary) method
            .call("balance", mintBeneficiary.hexAddress())
            .then()
            .isSuccess()
            .outputIsEqualTo("0x10000")
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
            .call("minter")
            .then()
            .isRevert()//because the contract constructor is not payable
            .outputIsNull()
        ;
    }

    @SneakyThrows
    @Test
    void checkRequirement_InsufficientBalance() {
        var sender = new VmAccount(address(0), 0, 1_000_000_000L);
        var contractAccount = new VmAccount(address(1), 0, 0L);
        var receiver = address(2);

        state(block, sender, contractAccount)
            //put contract in the state
            .contract(contractAccount.address(), ContractHelper.createCompiledContract(CONTRACT_FILE))
            //call message
            .newMessage()
            .from(sender)
            .to(contractAccount)
            .value(0)
            .gasLimit(1_000_000L)
            .gasPrice(10L)
            .when()//call send(receiver, 0x20000) method
            .call("send", receiver.hexAddress(), "0x20000")
            .then()
            //TODO analyze output, it must contain the message - "Insufficient balance."
            .isRevert()
            //ABI encoded returned object
            // 0x08c379a0 - Function selector for Error(string)
            // 0000000000000000000000000000000000000000000000000000000000000020 - Data offset
            // 0000000000000000000000000000000000000000000000000000000000000015 - String length
            // 496e73756666696369656e742062616c616e63652e0000000000000000000000 - String data = 'Insufficient balance.'
            .outputIsEqualTo("0x08c379a000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000015496e73756666696369656e742062616c616e63652e0000000000000000000000")
        ;

        //just to display the expected error message
        log.trace("error=[{}]", new String(HexUtils.fromHex("496e73756666696369656e742062616c616e63652e"), StandardCharsets.UTF_8));
        //state.writeTrace();
    }

}
