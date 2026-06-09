package io.aplfintech.grape.vm.contract;

import com.google.common.base.Preconditions;
import com.google.common.base.Strings;
import io.aplfintech.grape.bcei.InMemoryStateAccess;
import io.aplfintech.grape.bcei.VmStateManager;
import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ChainConfigFactory;
import io.aplfintech.grape.config.ChainConfigLoader;
import io.aplfintech.grape.config.CryptoConfig;
import io.aplfintech.grape.config.VmExecConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.l1vm.VmMessage;
import io.aplfintech.grape.l1vm.opcode.OpTableFactory;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Account;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.tracers.LogTracer;
import io.aplfintech.grape.tracers.LoggerConfig;
import io.aplfintech.grape.tracers.MdLogger;
import io.aplfintech.grape.tracers.StructTracer;
import io.aplfintech.grape.tracers.Tracer;
import io.aplfintech.grape.utils.TracerUtils;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.MessageExecutor;
import io.aplfintech.grape.vm.MessageResult;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.impl.VmMessageExecutor;
import io.aplfintech.grape.vm.utils.ContractHelper;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;

import java.io.FileNotFoundException;
import java.io.PrintWriter;
import java.math.BigInteger;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.function.Consumer;

import static io.aplfintech.grape.vm.Executors.EXECUTOR_CONTEXT;
import static io.aplfintech.grape.vm.tx.TxData.PREDEFINED_ADDRESSES;
import static io.aplfintech.grape.vm.tx.TxData.PREDEFINED_PRIVATE_KEYS;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class ContractExecutor implements StateSpec {
    private static final BigInteger CHAIN_ID = BigInteger.TWO;
    private static ContractExecutor CURRENT_STATE;
    private static final CryptoLib cryptoLib = CryptoConfig.crypto();
    private static final PrintWriter TRACE_WRITER;

    static {
        TRACE_WRITER = TracerUtils.nullWriter();//TracerUtils.openFileToAppend("trace.log");
    }

    private final ChainConfig chainConfig;
    private final VmStateManager vmStateManager;
    private final MessageExecutor messageApplier;
    private final Tracer tracer;
    private final LoggerConfig loggerConfig;
    private final PrintWriter traceWriter;
    private BlockContext block;
    private VmMessage latestMessage;
    private MessageSpec messageSpec;
    private CompiledContract latestContract;

    private ContractExecutor(ChainConfig chainConfig, VmStateManager vmStateManager, MessageExecutor messageApplier, Tracer tracer, LoggerConfig loggerConfig, PrintWriter traceWriter) {
        this.chainConfig = chainConfig;
        this.vmStateManager = vmStateManager;
        this.messageApplier = messageApplier;
        this.tracer = tracer;
        this.loggerConfig = loggerConfig;
        this.traceWriter = traceWriter;
    }

    /**
     * Creates new state for the message execution
     *
     * @param block    the current block
     * @param accounts accounts that will be keep in the created state
     * @return the new state for the message execution
     */
    @SneakyThrows
    public static ContractExecutor state(BlockContext block, Account... accounts) {
        ContractExecutor state = createState(block, accounts);
        updateCurrentState(state);
        return CURRENT_STATE;
    }

    /**
     * Returns the current state for the message execution
     *
     * @return the current state for the message execution
     */
    @SneakyThrows
    public static ContractExecutor inState() {
        return CURRENT_STATE;
    }

    @SneakyThrows
    public static MessageSpec message(ContractExecutor state) {
        updateCurrentState(state);
        return CURRENT_STATE.newMessage();
    }

    private static void updateCurrentState(ContractExecutor state) throws Exception {
        //set the current state instance up
        if (CURRENT_STATE != null && CURRENT_STATE != state) {
            writeTraceForCurrentState();
        }
        CURRENT_STATE = state;
    }

    private static ContractExecutor createState(BlockContext block, Account... accounts) throws FileNotFoundException {
        var grapeChainConfig = new ChainConfigLoader().load();
        var chainConfig = new ChainConfigFactory(grapeChainConfig).configAt(block.timestamp());
        var opTableFactory = OpTableFactory.newFactory(chainConfig, cryptoLib);
        var stateAccess = new InMemoryStateAccess(CHAIN_ID, accounts);
        var vmStateManager = new VmStateManager(stateAccess);
        var tracerCfg = LoggerConfig.defaultConfig();
        Tracer tracer = Tracer.link(
            new LogTracer(
                new MdLogger(/*Tracer.openFileToAppend("md.log")*/TracerUtils.stdOutWriter(), tracerCfg)
            ),
            new StructTracer(tracerCfg)
        );
        var cfg = VmExecConfig.builder()
            .debugEnabled(true)
            .tracer(tracer)
            .cryptoLib(cryptoLib)
            .opTable(opTableFactory.createTable())
            .build(chainConfig);
        var messageApplier = new VmMessageExecutor(cfg, vmStateManager, EXECUTOR_CONTEXT);

        //create state executor instance
        var state = new ContractExecutor(chainConfig, vmStateManager, messageApplier, tracer, tracerCfg, TRACE_WRITER);
        state.block(block);
        return state;
    }

    @Override
    public StateSpec block(@NonNull BlockContext block) {
        this.block = block;
        return this;
    }

    @Override
    public StateSpec account(Account... accounts) {
        for (Account acc : accounts) {
            vmStateManager.putAccount(acc.address(), acc);
        }
        return this;
    }

    @Override
    public StateSpec contract(@NonNull Address address, CompiledContract contract) {
        vmStateManager.putContractCode(address, contract.runtimeCode());
        latestContract = contract;
        return this;
    }

    @Override
    public StateSpec balanceIsEqual(Address address, BigInteger balance) {
        var actualBalance = CURRENT_STATE.vmStateManager.getAccount(address).balance();
        checkExpectation(balance.equals(actualBalance), "Expected balance=%s but actual balance=%s", balance, actualBalance);
        return this;
    }

    @Override
    public MessageSpec newMessage() {
        this.messageSpec = new MessageSpecImpl();
        return messageSpec;
    }

    @Override
    public MessageSpec nextMessage() {
        if (messageSpec == null) {
            throw new IllegalStateException("The state message not created");
        }
        return messageSpec.incrementNonce();
    }

    public void writeTrace() {
        tracer.writeTrace(traceWriter);
    }

    /**
     * Returns one of 20 predefined addresses by its index.
     * By start, Neither of addresses keep in the state.
     *
     * @param idx the index of the predefined address, its range from 0 up to 19
     * @return the address by number
     */
    public static Address address(int idx) {
        Preconditions.checkPositionIndex(idx, PREDEFINED_ADDRESSES.size());
        return PREDEFINED_ADDRESSES.get(idx);
    }

    /**
     * Returns one of 20 predefined keys by its index.
     *
     * @param idx the index of the predefined key, its range from 0 up to 19
     * @return the 32 byte key
     */
    public static byte[] key(int idx) {
        Preconditions.checkPositionIndex(idx, PREDEFINED_PRIVATE_KEYS.size());
        return PREDEFINED_PRIVATE_KEYS.get(idx);
    }

    private static class MessageSpecImpl implements MessageSpec {
        private CompiledContract contract;
        private final VmMessage message;

        public MessageSpecImpl() {
            if (CURRENT_STATE.latestMessage == null) {
                CURRENT_STATE.latestMessage = createEmptyMessage();
            }
            message = CURRENT_STATE.latestMessage;
        }

        private VmMessage createEmptyMessage() {
            return VmMessage.builder().build();
        }

        @Override
        public MessageSpec contractSpec(@NonNull CompiledContract contract) {
            this.contract = contract;
            return this;
        }

        @Override
        public MessageSpec contractSpec(@NonNull String contractFile) {
            this.contract = ContractHelper.createCompiledContract(contractFile);
            return this;
        }

        @Override
        public MessageSpec incrementNonce() {
            this.message.setNonce(message.nonce() + 1);
            return this;
        }

        @Override
        public MessageSpec nonce(long nonce) {
            this.message.setNonce(nonce);
            return this;
        }

        @Override
        public MessageSpec from(@NonNull Address address) {
            this.message.setFrom(address);
            return this;
        }

        @Override
        public MessageSpec to(@NonNull Address address) {
            this.message.setTo(address);
            return this;
        }

        @Override
        public MessageSpec value(long value) {
            Preconditions.checkArgument(value >= 0, "The transferred amount must be positive, actual value=%s", value);
            this.message.setAmount(BigInteger.valueOf(value));
            return this;
        }

        @Override
        public MessageSpec gasLimit(long value) {
            Preconditions.checkArgument(value > 0, "The gas limit must be greater then zero, actual value=%s", value);
            this.message.setGasLimit(BigInteger.valueOf(value));
            return this;
        }

        @Override
        public MessageSpec gasPrice(long value) {
            Preconditions.checkArgument(value >= 0, "The gas price value must be positive, actual value=%s", value);
            this.message.setGasPrice(BigInteger.valueOf(value));
            return this;
        }

        @Override
        public ContractCallSpec when() {
            if (contract != null) {
                CURRENT_STATE.latestContract = contract;
            } else {
                contract = CURRENT_STATE.latestContract;
            }
            return new ContractCallSpecImpl(this);
        }

    }

    private static class ContractCallSpecImpl implements ContractCallSpec {
        private final MessageSpecImpl messageSpec;
        String input;

        public ContractCallSpecImpl(MessageSpecImpl messageSpec) {
            this.messageSpec = messageSpec;
            this.input = null;
        }

        @Override
        public MessageResultSpec publish(String... args) {
            try {
                //create contract
                var msg = messageSpec.message;
                msg.setTo(VmAddress.UNDEFINED_ADDRESS);
                var formattedInput = formatInput(args);
                byte[] code = Bytes.concat(messageSpec.contract.code(), HexUtils.fromHex(formattedInput));
                msg.setData(code);
                log.debug("=== Publish contract input=[{}]", formattedInput);
                var result = CURRENT_STATE.messageApplier.executeMessage(msg, CURRENT_STATE.block);

                return new MessageResultSpecImpl(result.result());
            } catch (Exception e) {
                checkExpectation(false, "Contract publishing: caught unexpected exception=%s:%s",
                    e.getClass().getSimpleName(), e.getMessage());
                throw new IllegalStateException("Unexpected flow, Unreachable state exception");
            }
        }

        @Override
        public MessageResultSpec call(String method, String... args) {
            checkExpectation(messageSpec.contract != null,
                "The called contract specification is not specified. called method=%s, contract address=%s",
                method, messageSpec.message.to().hexAddress());
            var functionHash = messageSpec.contract.functions().get(method);
            checkExpectation(functionHash != null,
                "Can't find ABI hash for function='%s', json file=%s", method, messageSpec.contract.fileName());
            try {
                String formattedArgs = formatInput(args);
                var inputHex = functionHash + formattedArgs;
                var input = HexUtils.parseHex(inputHex);
                //call contract
                var msg = messageSpec.message;
                msg.setData(input);
                log.debug("=== Call method={} args=[{}]", method, formattedArgs);
                var result = CURRENT_STATE.messageApplier.executeMessage(msg, CURRENT_STATE.block);

                return new MessageResultSpecImpl(result.result());
            } catch (Exception e) {
                checkExpectation(false, "Contract calling method=%s: caught unexpected exception=%s:%s",
                    method, e.getClass().getSimpleName(), e.getMessage());
                throw new IllegalStateException("Unexpected flow, Unreachable state exception");
            }
        }

        private static String formatInput(String[] args) {
            StringBuilder params = new StringBuilder();
            for (var arg : args) {
                params.append(Strings.padStart(arg.replace("0x", ""), 64, '0'));
            }
            return params.toString();
        }

    }

    private static class MessageResultSpecImpl implements MessageResultSpec {
        private final MessageResult messageResult;
        private final ContractResult result;
        private static final String OUTPUT_MISMATCH_PATTERN = "Output mismatch, expected=%s actual=%s";

        public MessageResultSpecImpl(@NonNull MessageResult result) {
            this.messageResult = result;
            this.result = result.contractResult();
        }

        @Override
        public MessageResultSpec then() {
            return this;
        }

        @Override
        public MessageResultSpec isSuccess() {
            checkExpectation(!result.hasError(), "Expected SUCCESS status but actual status=%s", result.executionStatus().fullName());
            return this;
        }

        @Override
        public MessageResultSpec isRevert() {
            return statusIsEqualTo(VmStatus.VM_REVERT);
        }

        @Override
        public MessageResultSpec statusIsEqualTo(@NonNull ExecutionStatus expected) {
            checkExpectation(expected.equals(result.executionStatus()),
                "Execution status mismatch, expected=%s actual=%s",
                expected.fullName(), result.executionStatus().fullName());
            return this;
        }

        @Override
        public MessageResultSpec outputIsEqualTo(byte[]... expects) {
            var expected = Bytes.concat(expects);
            checkExpectation(Arrays.equals(expected, result.output()),
                OUTPUT_MISMATCH_PATTERN,
                HexUtils.toHex(expected, true), HexUtils.toHex(result.output(), true));
            return this;
        }

        @Override
        public MessageResultSpec outputIsEqualTo(String expectedHex) {
            byte[] bytes = HexUtils.parseHex(expectedHex);
            if (bytes.length < 32) {
                bytes = Math256.padToWord(bytes);
            }
            return outputIsEqualTo(bytes);
        }

        @Override
        public MessageResultSpec outputIsNull() {
            checkExpectation(result.output() == null || result.output().length == 0,
                "Expected Empty output but actual output=%s",
                HexUtils.toHex(result.output(), true));
            return this;
        }

        @Override
        public MessageResultSpec outputStartsWith(byte[] expected) {
            checkExpectation(expected.length <= result.output().length,
                OUTPUT_MISMATCH_PATTERN,
                HexUtils.toHex(expected, true), HexUtils.toHex(result.output(), true));
            var rc = Bytes.slice(result.output(), 0, expected.length);
            checkExpectation(Arrays.equals(expected, rc),
                OUTPUT_MISMATCH_PATTERN,
                HexUtils.toHex(expected, true), HexUtils.toHex(result.output(), true));
            return this;
        }

        @Override
        public MessageResultSpec contractAddressIs(Address expected) {
            checkExpectation(expected.equals(result.contract()),
                "Contract address mismatch, expected=%s actual=%s",
                expected.hexAddress(), result.contract().hexAddress());
            return this;
        }

        @Override
        public ContractResult resultContract() {
            return result;
        }

        @Override
        public MessageResult result() {
            return messageResult;
        }

        @Override
        public MessageResultSpec resultContract(Consumer<ContractResult> consumer) {
            consumer.accept(result);
            return this;
        }

        @Override
        public MessageResultSpec result(Consumer<MessageResult> consumer) {
            consumer.accept(messageResult);
            return this;
        }

        @Override
        public StateSpec inState() {
            return CURRENT_STATE;
        }

        @Override
        public MessageSpec nextMessage() {
            return CURRENT_STATE.newMessage().incrementNonce();
        }


    }

    private static void checkExpectation(boolean condition, String formatPattern, Object... args) {
        if (!condition) {
            var err = String.format(formatPattern, args);
            log.error(err);
            writeTraceForCurrentState();
            throw new ContractExecutionException(err);
        }
    }

    private static void writeTraceForCurrentState() {
        if (CURRENT_STATE != null && CURRENT_STATE.tracer != null && CURRENT_STATE.traceWriter != null) {
            CURRENT_STATE.tracer.writeTrace(CURRENT_STATE.traceWriter);
        }
    }

    private static class ContractExecutionException extends RuntimeException {
        public ContractExecutionException(String message) {
            super(message);
        }
    }

    public static VmExecConfig createVmExecConfig(ChainConfig chainConfig, boolean enableStructTracer) {
        //apply all needed configs and instantiate the appropriate VM execution config
        var opTableFactory = OpTableFactory.newFactory(chainConfig, cryptoLib);
        var tracerCfg = LoggerConfig.defaultConfig();
        var chain = new ArrayList<Tracer>();
        if (enableStructTracer) {
            chain.add(new StructTracer(tracerCfg));
        }

        Tracer tracer = Tracer.link(
            new LogTracer(
                new MdLogger(TracerUtils.stdOutWriter(), tracerCfg)
            ),
            chain.toArray(new Tracer[0])
        );

        return VmExecConfig.builder()
            .debugEnabled(true)
            .tracer(tracer)
            .cryptoLib(cryptoLib)
            .opTable(opTableFactory.createTable())
            .build(chainConfig);
    }
}
