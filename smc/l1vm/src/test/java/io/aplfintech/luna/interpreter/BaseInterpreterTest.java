package io.aplfintech.luna.interpreter;

import io.aplfintech.luna.bcei.VmStateAccess;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ChainConfigFactory;
import io.aplfintech.luna.config.ChainConfigLoader;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.config.GrapeChainConfig;
import io.aplfintech.luna.config.VmExecConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.env.Context;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.VmImpl;
import io.aplfintech.luna.l1vm.code.Codes;
import io.aplfintech.luna.l1vm.contract.CodeAndHashImpl;
import io.aplfintech.luna.l1vm.contract.VmContract;
import io.aplfintech.luna.l1vm.interpreter.BaseInterpreter;
import io.aplfintech.luna.l1vm.opcode.OpTableFactory;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Hash;
import io.aplfintech.luna.tracers.DebugLogger;
import io.aplfintech.luna.tracers.LogTracer;
import io.aplfintech.luna.tracers.LoggerConfig;
import io.aplfintech.luna.tracers.StructTracer;
import io.aplfintech.luna.tracers.Tracer;
import io.aplfintech.luna.utils.TracerUtils;
import io.aplfintech.luna.vm.Vm;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.contract.Code;
import io.aplfintech.luna.vm.contract.ContractRef;
import io.aplfintech.luna.vm.opcode.OpTable;
import io.aplfintech.luna.utils.HexUtils;
import lombok.SneakyThrows;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.PrintWriter;
import java.math.BigInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class BaseInterpreterTest {
    public String recipientHex = "168426fdc4c8a51b96b4bed827907b5fa6491ad0";//20 bytes
    public Address from = VmAddress.from(HexUtils.fromHex(recipientHex));
    Vm vm;
    static ChainConfig chainConfig;
    static CryptoLib crypto;
    static OpTableFactory opTableFactory;
    OpTable opTable;
    ExecutionConfig cfg;

    public String callerHex = "168426fdc4c8a51b96b4bed827907b5fa6491ad0";//20 bytes
    public Address caller = VmAddress.from(HexUtils.fromHex(callerHex));

    Tracer tracer;

    static PrintWriter traceWriter;
    private Interpreter interpreter;

    @SneakyThrows
    @BeforeAll
    static void beforeAll() {
        var loader = new ChainConfigLoader("test-chain.json");
        GrapeChainConfig grapeChainConfig = loader.load();
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(0);
        crypto = CryptoConfig.crypto();
        opTableFactory = OpTableFactory.newFactory(chainConfig, crypto);
        traceWriter = TracerUtils.nullWriter();
    }

    @AfterAll
    static void afterAll() {
        traceWriter.close();
    }

    @BeforeEach
    void setUp() {
        var vmStateAccess = mock(VmStateAccess.class);
        var context = mock(Context.class);
        var tracerCfg = LoggerConfig.defaultConfig();
        opTable = opTableFactory.createTable();
        tracer = Tracer.link(
            new LogTracer(
                new DebugLogger(tracerCfg)
            ),
            new StructTracer(tracerCfg)
        );
        cfg = VmExecConfig.create(chainConfig, true, true, tracer, opTable, crypto);
        vm = new VmImpl(context, cfg, chainConfig.vmConfig(), vmStateAccess);
        interpreter = new BaseInterpreter(vm, cfg);
    }

    @Test
    void run() {
        //GIVEN
        VmContract contract = createContract(BigInteger.ZERO);
        tracer.notifyExecutionStart(vm.stateAccess(), from, contract.address(), true, null, 10000, BigInteger.ZERO);
        //WHEN
        try {
            var result = interpreter.run(contract, null, false);
            assertNotNull(result);
            assertEquals(336, result.output().length);
            assertEquals(VmStatus.VM_SUCCESS, result.executionStatus());

        } finally {
            //write tracer output
            cfg.tracer().writeTrace(traceWriter);

        }//THEN
    }

    @Test
    void runWithRevert() {
        //GIVEN
        VmContract contract = createContract(BigInteger.TEN);
        tracer.notifyExecutionStart(vm.stateAccess(), from, contract.address(), true, null, 10000, BigInteger.TEN);
        //WHEN
        try {
            var result = interpreter.run(contract, null, false);
            //THEN
            assertNotNull(result);
            assertEquals(0, result.output().length);
            assertEquals(VmStatus.VM_REVERT, result.executionStatus());
        } finally {
            //write tracer output
            cfg.tracer().writeTrace(traceWriter);
        }
    }

    private VmContract createContract(BigInteger value) {
        var codeBytes = HexUtils.fromHex(contractCodeHex);
        var contractAddress = VmAddress.from(crypto.createAddress(caller.bytes(), 1));
        var callerRef = asContractRef(caller, value);
        var contract = new VmContract(callerRef, contractAddress, value, 1000L);
        Code code = Codes.from(codeBytes);
        Hash codeHash = new Hash(crypto.keccak256(code.bytes()));
        var codeAndHash = new CodeAndHashImpl(code, codeHash);
        contract.setCallCode(contractAddress, codeAndHash);
        return contract;
    }

    private ContractRef asContractRef(final Address caller, final BigInteger value) {
        return new ContractRef() {
            @Override
            public Address address() {
                return caller;
            }

            @Override
            public BigInteger value() {
                return value;
            }
        };
    }

    private String contractCodeHex = "608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";
}