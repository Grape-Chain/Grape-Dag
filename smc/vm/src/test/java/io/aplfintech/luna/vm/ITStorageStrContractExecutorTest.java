package io.aplfintech.luna.vm;

import io.aplfintech.luna.bcei.CachedReadStateAccess;
import io.aplfintech.luna.bcei.InMemoryStateAccess;
import io.aplfintech.luna.bcei.StateAccess;
import io.aplfintech.luna.bcei.VmStateManager;
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
import io.aplfintech.luna.l1vm.VmMessage;
import io.aplfintech.luna.utils.TracerUtils;
import io.aplfintech.luna.vm.contract.ContractExecutor;
import io.aplfintech.luna.vm.impl.VmMessageExecutor;
import io.aplfintech.luna.vm.tx.MockBlock;
import io.aplfintech.luna.vm.utils.ContractHelper;
import io.aplfintech.luna.vm.utils.MessageHelper;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import java.io.File;
import java.io.PrintWriter;
import java.math.BigInteger;

import static io.aplfintech.luna.vm.Executors.APPLIER_CONTEXT;
import static io.aplfintech.luna.vm.Executors.EXECUTOR_CONTEXT;
import static io.aplfintech.luna.vm.VmStatus.VM_SUCCESS;
import static io.aplfintech.luna.vm.utils.ContractHelper.txData;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class ITStorageStrContractExecutorTest {

    private final PrintWriter printWriter = TracerUtils.nullWriter();
    static GrapeChainConfig grapeChainConfig;
    static ChainConfig chainConfig;
    static CryptoLib crypto;
    BlockContext block = new MockBlock();
    VmExecConfig cfg;
    StateAccess dei;
    VmStateManager stateManagerRo;
    VmStateManager stateManager;
    MessageExecutor messageApplier;
    MessageExecutor messageExecutor;
    long nonce = 1234567890;
    VmAccount publisher = new VmAccount(VmAddress.from(crypto.recoverAddress(txData.senderPublicKey)), nonce, 1_000_000_000L);

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
        var zAccount = new VmAccount(VmAddress.ZERO_ADDRESS, 0, BigInteger.valueOf(Long.MAX_VALUE));
        cfg = ContractExecutor.createVmExecConfig(chainConfig, false);
        dei = new InMemoryStateAccess(BigInteger.TWO, publisher, zAccount);
        stateManagerRo = new VmStateManager(new CachedReadStateAccess(dei));
        stateManager = new VmStateManager(dei);
        messageApplier = new VmMessageExecutor(cfg, stateManagerRo, APPLIER_CONTEXT);
        messageExecutor = new VmMessageExecutor(cfg, stateManager, EXECUTOR_CONTEXT);
    }

    @SneakyThrows
    @ValueSource(strings = {"storageStr_4-bytecode.json", "storageStr_8-bytecode.json"})
    @ParameterizedTest
    void storageContract(String contractFileName) {
        //GIVEN
        var contractFile = "contracts" + File.separator + contractFileName;
        var contract = ContractHelper.createCompiledContract(contractFile);
        //param1 = "S111111"
        var param1 = HexUtils.fromHex("000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000075331313131313100000000000000000000000000000000000000000000000000");
        //param2 = "S222222"
        var param2 = HexUtils.fromHex("000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000075332323232323200000000000000000000000000000000000000000000000000");
        var initValue = HexUtils.fromHex("000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000007533131313131310000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000075332323232323200000000000000000000000000000000000000000000000000");
        var contractInitCode = Bytes.concat(contract.code(), initValue);
        var tx = MessageHelper.createPublishMessage(nonce, 0, 10_000_000L, 5L, contractInitCode);
        var expectedContractAddress = VmAddress.from(crypto.createAddress(publisher.address().bytes(), nonce));
        //WHEN
        //publish
        var receipt = messageExecutor.executeMessage(tx, block);
        //THEN
        assertNotNull(receipt);
        assertTrue(receipt.success(), "must be Success result");
        assertEquals(VM_SUCCESS, receipt.result().executionStatus());
        // next call in the same state
        //GIVEN
        var callRetrieveHash = HexUtils.fromHex(contract.functions().get("retrieve1"));
        var message = VmMessage.builder()
            .from(VmAddress.ZERO_ADDRESS)
            .to(expectedContractAddress)
            .amount(BigInteger.ZERO)
            .gasLimit(BigInteger.valueOf(10_000_000))
            .gasPrice(BigInteger.valueOf(5))
            .data(callRetrieveHash)
            .build();
        //WHEN
        var rc = messageApplier.executeMessage(message, block);
        //THEN
        assertNotNull(rc);
        assertTrue(rc.result().contractResult().isSuccess(), "must be Success result");
        assertArrayEquals(param1, rc.result().contractResult().output(), "contract result Output is unexpected");


        // next call in the same state
        //GIVEN
        callRetrieveHash = HexUtils.fromHex(contract.functions().get("retrieve2"));
        message = VmMessage.builder()
            .nonce(1)
            .from(VmAddress.ZERO_ADDRESS)
            .to(expectedContractAddress)
            .amount(BigInteger.ZERO)
            .gasLimit(BigInteger.valueOf(10_000_000))
            .gasPrice(BigInteger.valueOf(5))
            .data(callRetrieveHash)
            .build();
        //WHEN
        rc = messageApplier.executeMessage(message, block);
        //THEN
        assertNotNull(rc);
        assertTrue(rc.result().contractResult().isSuccess(), "must be Success result");
        assertArrayEquals(param2, rc.result().contractResult().output(), "contract result Output is unexpected");

    }

}