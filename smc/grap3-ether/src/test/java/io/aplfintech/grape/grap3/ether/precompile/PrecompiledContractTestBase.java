package io.aplfintech.grape.grap3.ether.precompile;

import com.fasterxml.jackson.annotation.JsonEnumDefaultValue;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.common.io.CharStreams;
import io.aplfintech.grape.grap3.ether.CryptoLibProvider;
import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ChainConfigFactory;
import io.aplfintech.grape.config.ChainConfigLoader;
import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.config.GrapeChainConfig;
import io.aplfintech.grape.config.HardForkConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.utils.FileUtils;
import io.aplfintech.grape.utils.HexUtils;
import io.aplfintech.grape.utils.JsonUtils;
import lombok.Data;
import lombok.NonNull;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;

import java.io.FileWriter;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Objects;

import static io.aplfintech.grape.vm.VmStatus.VM_PRECOMPILE_ERROR;
import static io.aplfintech.grape.vm.VmStatus.VM_SUCCESS;
import static io.aplfintech.grape.utils.JsonUtils.HEX_MAPPER;
import static java.nio.charset.StandardCharsets.UTF_8;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class PrecompiledContractTestBase {
    static final CryptoLib crypto  = CryptoLibProvider.crypto();
    static GrapeChainConfig grapeChainConfig;
    static HardForkConfig fuelOptimizationForkConfig;

    ChainConfig chainConfig;
    GasPrice price;

    @BeforeAll
    static void beforeAll() {
        //crypto = CryptoConfig.crypto();
        var chainConfigLoader = new ChainConfigLoader("test-chain.json");
        grapeChainConfig = chainConfigLoader.load();
        var fork = grapeChainConfig.locateHardFork("fuelOptimization");
        assertTrue(fork.isPresent(), "Fork is defined");
        fuelOptimizationForkConfig = fork.get();
    }


    void setUpInitialConfig() {
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(0);
        price = chainConfig.gasPriceMap();
    }

    void setUpFuelOptimizationConfig() {
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(fuelOptimizationForkConfig.getTimestamp().getTime() / 1000 + 1);
        price = chainConfig.gasPriceMap();
    }

    boolean testJson(String name, PrecompiledContract contract) {
        var tests = loadTestData(name);
        boolean rc = true;
        int failed = 0;
        for (var test : tests) {
            if (!testPrecompiled(contract, test)) {
                log.error("Precompiled contract fails the '{}' sample", test.name);
                rc = false;
                failed++;
            }
        }
        log.info("=== Testcase '{}' passed {} tests, failed {}", name, tests.length, failed);
        return rc;
    }

    boolean testPrecompiled(PrecompiledContract contract, PrecompiledTestData testData) {
        boolean success = true;
        List<String> messages = new ArrayList<>();
        var input = HexUtils.parseHex(testData.input);
        ExecutionStatus expectedStatus = testData.status;
        FnExecResult result;
        long actualGas;
        try {
            actualGas = contract.requiredGas(input);
            log.info("Data sample={} gas={}", testData.name, actualGas);
            result = contract.run(input);
        } catch (Exception e) {
            log.error("Precompiled contract execution error.", e);
            log.error("Test data={}", testData);
            return false;
        }

        if (!expectedStatus.equals(VM_SUCCESS)) {
            if (!expectedStatus.equals(result.executionStatus())) {
                success = false;
                messages.add(
                    String.format("Expected Execution Status %s, got %s",
                        testData.status,
                        result.executionStatus().getName())
                );
            }
        } else { //status == VM_SUCCESS
            if (result.hasError()) {
                success = false;
                messages.add(result.toString());
            } else {
                if (!Arrays.equals(HexUtils.parseHex(testData.expected), result.output())) {
                    success = false;
                    messages.add(
                        String.format("Expected %s, got %s",
                            testData.expected,
                            HexUtils.toHex(result.output(), false))
                    );
                }
            }
            if (actualGas != testData.gas) {
                success = false;
                messages.add(
                    String.format("%s: gas wrong, expected %d, got %d", testData.name, testData.gas, actualGas)
                );
            }
            if (!Arrays.equals(HexUtils.parseHex(testData.input), input)) {
                success = false;
                messages.add(
                    String.format("Precompiled %x modified the input data", contract)
                );
            }
        }
        for (String msg : messages) {
            log.error(msg);
        }

        return success;
    }

    @SneakyThrows
    protected PrecompiledTestData[] loadTestData(String name) {
        String fileName = "test-data/precompiled/" + name + ".json";
        var json = FileUtils.readResourceContent(fileName);
        assertThat(json).withFailMessage("resource not found: " + fileName).isNotNull();
        return JsonUtils.HEX_MAPPER.readValue(json, PrecompiledTestData[].class);
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    @JsonInclude(JsonInclude.Include.NON_NULL)
    @Data
    static class PrecompiledTestData {
        @JsonProperty("Name")
        String name;
        @JsonProperty("Input")
        String input;
        @JsonProperty("Expected")
        String expected;
        @JsonProperty("Gas")
        long gas;
        @JsonProperty(value = "Status")
        @JsonEnumDefaultValue
        VmStatus status = VM_SUCCESS;
        @JsonProperty("errorMessage")
        String errorMessage;
    }

    @SneakyThrows
    void generateJsonTestData(String inputName) {
        var fileName = "test-data/precompiled/" + inputName + ".csv";
        var items = getItemsList(fileName);
        int idx = 0;
        List<PrecompiledTestData> out = new ArrayList<>();
        for (var item : items) {
            idx++;
            var tData = new PrecompiledTestData();
            var fullName = inputName + "_input_";
            tData.input = item[0].trim();
            if (!item[1].isBlank()) {
                tData.expected = item[1].trim();
            } else {
                tData.expected = "";
            }
            if (!item[2].isBlank()) {
                tData.gas = Long.parseLong(item[2].trim());
            }
            if (!item[3].isBlank()) {
                tData.status = VM_PRECOMPILE_ERROR;
                tData.errorMessage = item[3].trim();
                fullName += "error_";
            }
            tData.name = fullName + idx;
            out.add(tData);
        }
        writeJson(inputName + ".json", out);
    }

    /**
     * Reads testdata from CSV file
     *
     * @param fileName
     */
    private static List<String[]> getItemsList(String fileName) throws IOException {
        InputStream inputStream = PrecompiledContractTestBase.class.getClassLoader().getResourceAsStream(fileName);
        InputStreamReader reader = new InputStreamReader(Objects.requireNonNull(inputStream), UTF_8);
        return CharStreams.readLines(reader)
            .stream()
            .map(line -> line.split(",", 4))
            .toList();
    }

    @SneakyThrows
    private static void writeJson(@NonNull String fileName, @NonNull Object data) {
        var file = new FileWriter(fileName);
        HEX_MAPPER.writeValue(file, data);
    }
}