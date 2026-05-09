package io.aplfintech.luna.vm;

import io.aplfintech.luna.bcei.DEI;
import io.aplfintech.luna.bcei.HostNode;
import io.aplfintech.luna.bcei.InMemoryStateAccess;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ChainConfigFactory;
import io.aplfintech.luna.config.ChainConfigLoader;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.config.GrapeChainConfig;
import io.aplfintech.luna.config.VmExecConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.vm.contract.ContractExecutor;
import io.aplfintech.luna.vm.impl.GasEstimator;
import io.aplfintech.luna.vm.tx.MockBlock;
import io.aplfintech.luna.vm.utils.ContractHelper;
import io.aplfintech.luna.vm.utils.MessageHelper;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.math.BigInteger;

import static io.aplfintech.luna.vm.utils.ContractHelper.txData;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITStorage2ContractGasEstimatorTest {

    static GrapeChainConfig grapeChainConfig;
    static ChainConfig chainConfig;
    static CryptoLib crypto;

    BlockContext block = new MockBlock();
    VmExecConfig cfg;
    DEI dei;
    GasEstimator gasEstimator;
    long nonce = 1234567890;
    VmAccount sender;

    @BeforeAll
    static void beforeAll() {
        var chainConfigLoader = new ChainConfigLoader();
        grapeChainConfig = chainConfigLoader.load();
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(0);
        crypto = CryptoConfig.crypto();
    }

    @BeforeEach
    void setUp() {
        sender = new VmAccount(VmAddress.from(crypto.recoverAddress(txData.senderPublicKey)), nonce, 1_000_000_000L);
        cfg = ContractExecutor.createVmExecConfig(chainConfig, false);
        dei = new HostNode(new InMemoryStateAccess(BigInteger.TWO, sender));
        gasEstimator = new GasEstimator(chainConfig, cfg, dei);
    }

    @SneakyThrows
    @Test
    void estimateStorageContract() {
        //GIVEN
        var balance = sender.balance();
        var contractFile = "contracts/storage2-bytecode.json";
        var contract = ContractHelper.createCompiledContract(contractFile);
        var initValue = Bytes.leftPadBytes(HexUtils.fromHex("aabbccddeeff"), 32);
        var contractInitCode = Bytes.concat(contract.code(), initValue);
        var tx = MessageHelper.createPublishMessage(nonce, 0, 10_000_000L, 5L, contractInitCode);
        var expectedContractAddress = VmAddress.from(crypto.createAddress(sender.address().bytes(), nonce));
        long expectedUsedGasTx = 69278;
        long expectedUsedGasContract = 0;
        //WHEN
        var estimatedGasReceipt = gasEstimator.executeMessage(tx, block);
        //THEN
        assertNotNull(estimatedGasReceipt);
        assertTrue(estimatedGasReceipt.success(), "must be Success result");
        assertEquals(expectedUsedGasTx, estimatedGasReceipt.result().usedGas());
        assertEquals(expectedUsedGasContract, estimatedGasReceipt.result().contractResult().gasUsed());
        assertEquals(balance, sender.balance());

        // *** next call in the same state ***

        //GIVEN
        var callRetrieveData = HexUtils.fromHex(contract.functions().get("retrieve"));
        expectedUsedGasTx = 21272;
        expectedUsedGasContract = 0;
        tx = MessageHelper.createCallMessage(expectedContractAddress.bytes(), nonce + 1, 0, 10_000_000L, 5L, callRetrieveData);

        //WHEN
        estimatedGasReceipt = gasEstimator.executeMessage(tx, block);
        //THEN
        assertNotNull(estimatedGasReceipt);
        assertTrue(estimatedGasReceipt.success(), "must be Success result");
        assertEquals(expectedUsedGasTx, estimatedGasReceipt.result().usedGas());
        assertEquals(expectedUsedGasContract, estimatedGasReceipt.result().contractResult().gasUsed());
        assertEquals(balance, sender.balance());

    }

}