package io.aplfintech.luna.l1vm.opcode;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.ChainConfigFactory;
import io.aplfintech.luna.config.ChainConfigLoader;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.config.GrapeChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.interpreter.Interpreter;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.l1vm.VmMemory;
import io.aplfintech.luna.l1vm.VmStack;
import io.aplfintech.luna.l1vm.interpreter.RunState;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.math.UInt256;
import io.aplfintech.luna.utils.HexArgs;
import io.aplfintech.luna.vm.opcode.ExecFn;
import io.aplfintech.luna.vm.opcode.OpTable;
import io.aplfintech.luna.utils.FileUtils;
import io.aplfintech.luna.utils.HexUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.AllArgsConstructor;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import java.util.Arrays;
import java.util.Map;
import java.util.TreeMap;
import java.util.function.Supplier;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class InstructionsTest {
    static ChainConfig chainConfig;
    static CryptoLib crypto;

    private Interpreter interpreter;
    static OpTable opTable;
    RunContext runState;

    static Map<String, Byte> twoArgOps = new TreeMap<>();

    static {
        twoArgOps.put("add", (byte) 0x01);
        twoArgOps.put("mul", (byte) 0x02);
        twoArgOps.put("sub", (byte) 0x03);
        twoArgOps.put("div", (byte) 0x04);
        twoArgOps.put("sdiv", (byte) 0x05);
        twoArgOps.put("mod", (byte) 0x06);
        twoArgOps.put("smod", (byte) 0x07);
        twoArgOps.put("exp", (byte) 0x0a);
        twoArgOps.put("signext", (byte) 0x0b);
        twoArgOps.put("lt", (byte) 0x10);
        twoArgOps.put("gt", (byte) 0x11);
        twoArgOps.put("slt", (byte) 0x12);
        twoArgOps.put("sgt", (byte) 0x13);
        twoArgOps.put("eq", (byte) 0x14);
        twoArgOps.put("and", (byte) 0x16);
        twoArgOps.put("or", (byte) 0x17);
        twoArgOps.put("xor", (byte) 0x18);
        twoArgOps.put("byte", (byte) 0x1a);
        twoArgOps.put("shl", (byte) 0x1b);
        twoArgOps.put("shr", (byte) 0x1c);
        twoArgOps.put("sar", (byte) 0x1d);
    }

    @SneakyThrows
    @BeforeAll
    static void beforeAll() {
        var loader = new ChainConfigLoader("test-chain.json");
        GrapeChainConfig grapeChainConfig = loader.load();
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(0);
        crypto = CryptoConfig.crypto();
        var factory = OpTableFactory.newFactory(chainConfig, crypto);
        opTable = factory.createTable();
    }

    @AfterAll
    static void afterAll() {
    }

    @BeforeEach
    void setUp() {
        crypto = CryptoConfig.crypto();
        interpreter = mock(Interpreter.class);//new BaseInterpreter(vm, cfg);
    }

    @Test
    void testTwoArgOps() {
        boolean rc = true;
        var ops = twoArgOps.keySet();
        for (var name : ops) {
            log.info("Start testing '{}' testcase", name);
            if (!testTwoArgOpJson(name)) {
                rc = false;
            }
        }
        assertTrue(rc);
    }

    @CsvSource({
        "ABCDEF0908070605040302010000000000000000000000000000000000000000, 00, AB",
        "ABCDEF0908070605040302010000000000000000000000000000000000000000, 01, CD",
        "00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff, 00, 00",
        "00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff, 01, CD",
        "0000000000000000000000000000000000000000000000000000000000102030, 1F, 30",
        "0000000000000000000000000000000000000000000000000000000000102030, 1E, 20",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 20, 00",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, FFFFFFFFFFFFFFFF, 00"
    })
    @ParameterizedTest
    void testBYTE(String xArg, String yArg, String expectedArg) {
        //GIVEN
        var name = "byte";
        var tests = new TwoOperandTestData[]{new TwoOperandTestData(xArg, yArg, expectedArg)};
        var fn = getExecFn(name);
        //WHEN
        var rc = testTwoArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }

    @CsvSource({
        "0000000000000000000000000000000000000000000000000000000000000001, 01, 0000000000000000000000000000000000000000000000000000000000000002",
        "0000000000000000000000000000000000000000000000000000000000000001, ff, 8000000000000000000000000000000000000000000000000000000000000000",
        "0000000000000000000000000000000000000000000000000000000000000001, 0100, 0000000000000000000000000000000000000000000000000000000000000000",
        "0000000000000000000000000000000000000000000000000000000000000001, 0101, 0000000000000000000000000000000000000000000000000000000000000000",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 00, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 01, fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, ff, 8000000000000000000000000000000000000000000000000000000000000000",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0100, 0000000000000000000000000000000000000000000000000000000000000000",
        "0000000000000000000000000000000000000000000000000000000000000000, 01, 0000000000000000000000000000000000000000000000000000000000000000",
        "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 01, fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"
    })
    @ParameterizedTest
    void testSHL(String xArg, String yArg, String expectedArg) {
        //GIVEN
        var name = "shl";
        var tests = new TwoOperandTestData[]{new TwoOperandTestData(xArg, yArg, expectedArg)};
        var fn = getExecFn(name);
        //WHEN
        var rc = testTwoArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }

    @CsvSource({
        "0000000000000000000000000000000000000000000000000000000000000001, 00, 0000000000000000000000000000000000000000000000000000000000000001",
        "0000000000000000000000000000000000000000000000000000000000000001, 01, 0000000000000000000000000000000000000000000000000000000000000000",
        "8000000000000000000000000000000000000000000000000000000000000000, 01, 4000000000000000000000000000000000000000000000000000000000000000",
        "8000000000000000000000000000000000000000000000000000000000000000, ff, 0000000000000000000000000000000000000000000000000000000000000001",
        "8000000000000000000000000000000000000000000000000000000000000000, 0100, 0000000000000000000000000000000000000000000000000000000000000000",
        "8000000000000000000000000000000000000000000000000000000000000000, 0101, 0000000000000000000000000000000000000000000000000000000000000000",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 00, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 01, 7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, ff, 0000000000000000000000000000000000000000000000000000000000000001",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0100, 0000000000000000000000000000000000000000000000000000000000000000",
        "0000000000000000000000000000000000000000000000000000000000000000, 01, 0000000000000000000000000000000000000000000000000000000000000000",
    })
    @ParameterizedTest
    void testSHR(String xArg, String yArg, String expectedArg) {
        //GIVEN
        var name = "shr";
        var tests = new TwoOperandTestData[]{new TwoOperandTestData(xArg, yArg, expectedArg)};
        var fn = getExecFn(name);
        //WHEN
        var rc = testTwoArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }

    @CsvSource({
        "0000000000000000000000000000000000000000000000000000000000000001, 00, 0000000000000000000000000000000000000000000000000000000000000001",
        "0000000000000000000000000000000000000000000000000000000000000001, 01, 0000000000000000000000000000000000000000000000000000000000000000",
        "8000000000000000000000000000000000000000000000000000000000000000, 01, c000000000000000000000000000000000000000000000000000000000000000",
        "8000000000000000000000000000000000000000000000000000000000000000, ff, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "8000000000000000000000000000000000000000000000000000000000000000, 0100, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "8000000000000000000000000000000000000000000000000000000000000000, 0101, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 00, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 01, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, ff, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0100, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0000000000000000000000000000000000000000000000000000000000000000, 01, 0000000000000000000000000000000000000000000000000000000000000000",
        "4000000000000000000000000000000000000000000000000000000000000000, fe, 0000000000000000000000000000000000000000000000000000000000000001",
        "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, f8, 000000000000000000000000000000000000000000000000000000000000007f",
        "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, fe, 0000000000000000000000000000000000000000000000000000000000000001",
        "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, ff, 0000000000000000000000000000000000000000000000000000000000000000",
        "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0100, 0000000000000000000000000000000000000000000000000000000000000000",
    })
    @ParameterizedTest
    void testSAR(String xArg, String yArg, String expectedArg) {
        //GIVEN
        var name = "sar";
        var tests = new TwoOperandTestData[]{new TwoOperandTestData(xArg, yArg, expectedArg)};
        var fn = getExecFn(name);
        //WHEN
        var rc = testTwoArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }

    @CsvSource({
        "1, 2, 2, 1",
        "-1, -2, 2, 1",
        "-6, 1, 3, 2",
        "4, 1, -3, 5",
        "-1, 0, 5, 0",
        "-1, 1, 5, 1",
        "-1, 2, 5, 2",
        "-1, -2, 5, 4",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 1, 5, 1",
        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe, ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
        "4, 1, 0, 0",
        "0, 1, 0, 0",
        "1, 0, 0, 0",
        "0, 0, 0, 0"
    })
    @ParameterizedTest
    void testAddMod(String xArg, String yArg, String zArg, String expectedArg) {
        //GIVEN
        var name = "addMod";//0x08
        var tests = new ThreeOperandTestData[]{new ThreeOperandTestData(xArg, yArg, zArg, expectedArg)};
        var fn = opTable.locateFn((byte) 0x08);
        //WHEN
        var rc = testThreeArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }

    @CsvSource({
        "1, 2, 2, 0",
        "-1, -2, 3, 0",
        "5, 2, -3, 0x0a",
        "-5, 1, 3, 2",
        "0x1b, 0x25, 0x64, 0x63",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0000000000000000000000000000000000000000000000000000000000000003, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "-1, 0x0000000000000000000000000000000000000000000000000000000000000002, 0x0000000000000000000000000000000000000000000000000000000000000005, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000001, 2, 5, 3",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 2, 5, 1",
        "0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 2, 5, 4",
        "0, 1, 0, 0",
        "1, 2, 0, 0",
        "0, 0, 0, 0",

    })
    @ParameterizedTest
    void testMulMod(String xArg, String yArg, String zArg, String expectedArg) {
        //GIVEN
        var name = "mulMod";//0x08
        var tests = new ThreeOperandTestData[]{new ThreeOperandTestData(xArg, yArg, zArg, expectedArg)};
        var fn = opTable.locateFn((byte) 0x09);
        //WHEN
        var rc = testThreeArgOp(name, fn, tests);
        //THEN
        assertTrue(rc);
    }


    boolean testTwoArgOpJson(@NonNull String name) {
        boolean rc = true;
        var tests = loadTestData(name);
        ExecFn fn = getExecFn(name);
        if (!testTwoArgOp(name, fn, tests)) {
            rc = false;
        }
        return rc;
    }

    boolean testTwoArgOp(@NonNull String name, ExecFn fn, TwoOperandTestData[] tests) {
        boolean rc = true;
        int failed = 0;
        for (int i = 0; i < tests.length; i++) {
            var test = tests[i];
            var memory = new VmMemory();
            var stack = new VmStack();
            runState = RunState.builder()
                .pc(0)
                //state fields
                .opCode(OpCodes.INVALID.getCode())
                .memory(memory)
                .stack(stack)
                .interpreter(interpreter)
                .build();
            if (!testTwoArgOpInContext(test, runState, fn, name, i)) {
                rc = false;
                failed++;
            }
        }
        log.info("Testcase '{}' passed {} tests, failed {}", name, tests.length, failed);
        return rc;
    }

    boolean testThreeArgOp(@NonNull String name, ExecFn fn, ThreeOperandTestData[] tests) {
        boolean rc = true;
        int failed = 0;
        for (int i = 0; i < tests.length; i++) {
            var test = tests[i];
            var memory = new VmMemory();
            var stack = new VmStack();
            runState = RunState.builder()
                .pc(0)
                //state fields
                .opCode(OpCodes.INVALID.getCode())
                .memory(memory)
                .stack(stack)
                .interpreter(interpreter)
                .build();
            if (!testThreeArgOpInContext(test, runState, fn, name, i)) {
                rc = false;
                failed++;
            }
        }
        log.info("Testcase '{}' passed {} tests, failed {}", name, tests.length, failed);
        return rc;
    }

    boolean testTwoArgOpInContext(TwoOperandTestData test, RunContext runContext, ExecFn fn, String opName, int idx) {
        var x = HexArgs.uintFromHexArg(test.x).asWord();
        var y = HexArgs.uintFromHexArg(test.y).asWord();
        var expected = Math256.uint256(HexUtils.parseHex(test.expected));

        runContext.getStack().push(x);
        runContext.getStack().push(y);
        return runOp(runContext, fn, opName, idx, expected,
            () -> String.format("%s(%s,%s): expected=%s", opName, x.hex(), y.hex(), expected.hex()));
    }

    boolean testThreeArgOpInContext(ThreeOperandTestData test, RunContext runContext, ExecFn fn, String opName, int idx) {
        var x = HexArgs.uintFromHexArg(test.x).asWord();
        var y = HexArgs.uintFromHexArg(test.y).asWord();
        var z = HexArgs.uintFromHexArg(test.z).asWord();
        var expected = Math256.uint256(HexUtils.parseHex(test.expected));

        runContext.getStack().push(z);
        runContext.getStack().push(y);
        runContext.getStack().push(x);
        return runOp(runContext, fn, opName, idx, expected,
            () -> String.format("%s(%s,%s,%s): expected=%s", opName, x.hex(), y.hex(), z.hex(), expected.hex()));
    }

    private static boolean runOp(RunContext runContext, ExecFn fn, String opName, int idx, UInt256 expected, Supplier<String> msgSupplier) {
        try {
            fn.apply(runContext);
        } catch (ArithmeticException e) {
            log.error("Testcase {}:{} caught exception {}, {}", opName, idx + 1, e.getMessage(), msgSupplier.get());
            return false;
        }
        if (runContext.getStack().size() != 1) {
            log.error("Expected 1 item on stack after {}, got {}", opName, runContext.getStack().size());
            return false;
        }
        var actual = runContext.getStack().pop();
        if (!Arrays.equals(actual.bytes32(), expected.asWord().bytes32())) {
            log.error("Testcase {}:{} {}, got {}", opName, idx + 1, msgSupplier.get(), actual.hex());
            return false;
        }
        return true;
    }

    private static ExecFn getExecFn(String name) {
        if (!twoArgOps.containsKey(name)) {
            throw new IllegalArgumentException("Can't find execution function for two-argument operation: " + name);
        }
        return opTable.locateFn(twoArgOps.get(name));
    }

    @SneakyThrows
    private static TwoOperandTestData[] loadTestData(String name) {
        String fileName = "test-data/two-operand/" + name + ".json";
        var json = FileUtils.readResourceContent(fileName);
        assertThat(json).withFailMessage("resource not found: " + fileName).isNotNull();
        return JsonUtils.HEX_MAPPER.readValue(json, TwoOperandTestData[].class);
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    @AllArgsConstructor
    @NoArgsConstructor
    private static class TwoOperandTestData {
        @JsonProperty("X")
        String x;
        @JsonProperty("Y")
        String y;
        @JsonProperty("Expected")
        String expected;
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    @AllArgsConstructor
    @NoArgsConstructor
    private static class ThreeOperandTestData {
        @JsonProperty("X")
        String x;
        @JsonProperty("Y")
        String y;
        @JsonProperty("Z")
        String z;
        @JsonProperty("Expected")
        String expected;
    }
}
