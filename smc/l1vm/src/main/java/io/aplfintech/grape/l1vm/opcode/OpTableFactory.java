package io.aplfintech.grape.l1vm.opcode;

import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ConfigurationException;
import io.aplfintech.grape.config.PriceItem;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.utils.Exceptions;
import io.aplfintech.grape.vm.DynamicGasHandler;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.opcode.ExecFn;
import io.aplfintech.grape.vm.opcode.OpCode;
import io.aplfintech.grape.vm.opcode.OpTable;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.util.HexFormat;
import java.util.Locale;
import java.util.Map;

import static io.aplfintech.grape.l1vm.opcode.OpCodes.INVALID;

/**
 * Factory for all stuff related with opCodes
 * This one loads configuration and creates opCode info, dynamic gas handlers and opCode functions
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class OpTableFactory {
    private final ChainConfig chainConfig;
    private final CryptoLib cryptoLib;

    private OpTableFactory(ChainConfig chainConfig, CryptoLib cryptoLib) {
        this.chainConfig = chainConfig;
        this.cryptoLib = cryptoLib;
    }

    public static OpTableFactory newFactory(@NonNull ChainConfig chainConfig, @NonNull CryptoLib cryptoLib) {
        return new OpTableFactory(chainConfig, cryptoLib);
    }

    public OpTable createTable() {
        return createTable(false);
    }

    public OpTable createTable(boolean traceEnabled) {
        //TODO change to array to increase performance
        //Set fee for op codes
        var opCodes = OpCodes.getOpCodes(chainConfig, cryptoLib);
        var gasPrice = chainConfig.gasPriceMap();
        var defaultPrice = new PriceItem(gasPrice.get("base").getValue(), "base setup");
        //set base fee from config
        for (int i = 0; i < opCodes.length; i++) {
            if (opCodes[i] != null) {
                var op = opCodes[i];
                var name = op.getName().toLowerCase(Locale.ROOT);
                var fee = gasPrice.getOrDefault(name, defaultPrice);
                op.setFee(fee.getValue());
            } else {
                opCodes[i] = INVALID;
            }
        }
        if (traceEnabled) {
            //wrap opCode functions
            opCodes = Instructions.createFunctionsWithLogging(opCodes);
        }

        //set opCode dynamic gas handlers
        var gasTableFactory = new GasTableFactory(chainConfig);
        var dynamicGasHandlers = gasTableFactory.createGasHandlersMap();

        var opTable = new OpTableImpl(chainConfig, opCodes, dynamicGasHandlers);
        var error = validate(opTable);
        if (error != 0) {
            throw new ConfigurationException("OpTable validation: found " + error + " errors.");
        }
        return opTable;
    }

    /**
     * Returns found errors count
     */
    private static int validate(@NonNull OpTableImpl opTable) {
        int errorCount = 0;
        var opCodes = opTable.opCodes();
        for (var op : opCodes) {
            if (op != null) {
                if (op.getFee() < 0) {
                    errorCount++;
                    log.error("The base fee isn't set, opCode=0x{}", HexFormat.of().toHexDigits(op.getCode()));
                }
                if (op.isDynamicGas()) {
                    if (!opTable.validateGasHandler(op.getCode())) {
                        errorCount++;
                        log.error("Gas dynamic handler not found, opCode=0x{}", HexFormat.of().toHexDigits(op.getCode()));
                    }
                }
            }
        }
        for (var aByte : opTable.dynamicGasHandlers.keySet()) {
            int code = aByte & 0x00ff;
            var op = opCodes[code];
            if (op == null) {
                errorCount++;
                log.error("The dynamic gas handler exists for unknown opCode=0x{}", HexFormat.of().toHexDigits(aByte));
            } else if (!op.isDynamicGas()) {
                errorCount++;
                log.error("The dynamic gas handler exists for opCode=0x{} but dynamicGas=false", HexFormat.of().toHexDigits(aByte));
            }
        }
        log.info("OpTable validation: Found {} errors.", errorCount);
        return errorCount;
    }

    static class OpTableImpl implements OpTable {
        private static final DynamicGasHandler DEFAULT_GAS_HANDLER = runContext -> 0L;
        private final OpCode[] opCodes;
        private final Map<Byte, DynamicGasHandler> dynamicGasHandlers;

        OpTableImpl(ChainConfig config, OpCode[] opCodes, Map<Byte, DynamicGasHandler> dynamicGasHandlers) {
            this.opCodes = opCodes;
            this.dynamicGasHandlers = dynamicGasHandlers;
        }

        @Override
        public ExecFn locateFn(byte opCode) {
            var op = locateOpCode(opCode);
            var fn = op.getFn();
            if (fn == null) {
                Exceptions.trap(VmStatus.VM_INTERNAL_ERROR,
                    "OpCode function error: Function not found, code=" + opCode);
            }
            return fn;
        }

        @Override
        public DynamicGasHandler locateDynamicGasHandler(byte opCode) {
            var dh = dynamicGasHandlers.get(opCode);
            if (dh == null) {
                log.error("Used default handler because Dynamic Handler not found, opCode=0x{}",
                    HexFormat.of().toHexDigits(opCode));
/*
                Exceptions.trap(VmStatus.VM_INTERNAL_ERROR,
                    "OpCode dynamic gas handlers: Handler not found, opCode=0x" + HexFormat.of().toHexDigits((byte) opCode));
*/
                return DEFAULT_GAS_HANDLER;
            }
            return dh;
        }

        @Override
        public OpCode locateOpCode(byte opCode) {
            int code = opCode & 0x00ff;
            var op = opCodes[code];
            if (op == null) {
                return INVALID;
            }
            return op;
        }

        @Override
        public OpCode[] opCodes() {
            return opCodes;
        }

        public boolean validateGasHandler(byte opCode) {
            return dynamicGasHandlers.get(opCode) != null;
        }

    }

}
