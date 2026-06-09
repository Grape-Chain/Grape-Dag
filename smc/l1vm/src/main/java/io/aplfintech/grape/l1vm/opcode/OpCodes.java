package io.aplfintech.grape.l1vm.opcode;

import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ConfigurationException;
import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.opcode.ExecFn;
import io.aplfintech.grape.vm.opcode.OpCode;
import io.aplfintech.grape.utils.Bytes;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.util.concurrent.atomic.AtomicBoolean;

import static io.aplfintech.grape.l1vm.opcode.Instructions.makeCallFunction;
import static io.aplfintech.grape.l1vm.opcode.Instructions.makeCreateFunction;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opAdd;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opAddMod;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opAddress;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opAnd;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opBalance;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opBaseFee;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opBeginSub;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opBlockHash;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opByte;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCallDataCopy;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCallDataLoad;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCallDataSize;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCallValue;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCaller;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opChainId;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCodeCopy;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCodeSize;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opCoinBase;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opDifficulty;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opDiv;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opDup;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opEq;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opExp;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opExtCodeCopy;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opExtCodeHash;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opExtCodeSize;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opGas;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opGasLimit;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opGasPrice;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opGt;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opInvalid;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opIsZero;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opJump;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opJumpDest;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opJumpI;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opJumpSub;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opLog;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opLt;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMLoad;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMSize;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMStore;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMStore8;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMod;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMul;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opMulMod;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opNot;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opNumber;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opOr;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opOrigin;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opPc;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opPop;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opPush;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opPush0;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opReturn;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opReturnDataCopy;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opReturnDataSize;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opReturnSub;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opRevert;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSDiv;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSGt;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSLoad;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSLt;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSMod;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSStore;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSar;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSelfBalance;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSelfDestruct;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSha3;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opShl;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opShr;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSignExtend;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opStop;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSub;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opSwap;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opTLoad;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opTStore;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opTimestamp;
import static io.aplfintech.grape.l1vm.opcode.Instructions.opXor;

/**
 * The opcodes map factory
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class OpCodes {
    public static final OpCode STOP = op(0x00, "STOP", false, opStop());
    public static final OpCode INVALID = op(0xfe, "INVALID", false, opInvalid());

    public static OpCode[] getOpCodes(@NonNull ChainConfig chainConfig, @NonNull CryptoLib crypto) {
        var gasPrice = chainConfig.gasPriceMap();
        var factory = new OpCodeFactory(gasPrice, crypto);
        OpCode[] opCodes;
        if (chainConfig.isForkEnabled("fuelOptimization")) {
            opCodes = factory.createFuelOptimization();
        } else {//Initial start opcodes
            opCodes = factory.createInitialOpCodes();
        }
        return opCodes;
    }

    private static class OpCodeFactory {
        private final CryptoLib crypto;

        private final long callStipend;

        public OpCodeFactory(GasPrice gasPrice, CryptoLib crypto) {
            this.crypto = crypto;
            this.callStipend = gasPrice.lookForGasPrice("callStipend");
        }

        private OpCode[] create_Auth() {
            var opCodes = createFuelOptimization();
            opCodes[0xf6] = op(0xf6, "AUTH", true, opInvalid());
            opCodes[0xf7] = op(0xf7, "AUTHCALL", true, opInvalid());
            return opCodes;
        }

        private OpCode[] createFuelOptimization() {
            var opCodes = createInitialOpCodes();
            // 0xb0 range
            opCodes[0xb3] = op(0xb3, "TLOAD", false, opTLoad());
            opCodes[0xb4] = op(0xb4, "TSTORE", false, opTStore());

            opCodes[0xff] = op(0xff, "SELFDESTRUCT", true, opSelfDestruct());
            return opCodes;
        }

        private OpCode[] createInitialOpCodes() {
            OpCode[] opCodes = new OpCode[256];

            opCodes[0x00] = STOP;
            // 0x0 range - arithmetic ops
            opCodes[0x01] = op(0x01, "ADD", false, opAdd());
            opCodes[0x02] = op(0x02, "MUL", false, opMul());
            opCodes[0x03] = op(0x03, "SUB", false, opSub());
            opCodes[0x04] = op(0x04, "DIV", false, opDiv());
            opCodes[0x05] = op(0x05, "SDIV", false, opSDiv());
            opCodes[0x06] = op(0x06, "MOD", false, opMod());
            opCodes[0x07] = op(0x07, "SMOD", false, opSMod());
            opCodes[0x08] = op(0x08, "ADDMOD", false, opAddMod());
            opCodes[0x09] = op(0x09, "MULMOD", false, opMulMod());
            opCodes[0x0a] = op(0x0a, "EXP", true, opExp());
            opCodes[0x0b] = op(0x0b, "SIGNEXTEND", false, opSignExtend());
            // 0x10 range - bit ops
            opCodes[0x10] = op(0x10, "LT", false, opLt());
            opCodes[0x11] = op(0x11, "GT", false, opGt());
            opCodes[0x12] = op(0x12, "SLT", false, opSLt());
            opCodes[0x13] = op(0x13, "SGT", false, opSGt());
            opCodes[0x14] = op(0x14, "EQ", false, opEq());
            opCodes[0x15] = op(0x15, "ISZERO", false, opIsZero());
            opCodes[0x16] = op(0x16, "AND", false, opAnd());
            opCodes[0x17] = op(0x17, "OR", false, opOr());
            opCodes[0x18] = op(0x18, "XOR", false, opXor());
            opCodes[0x19] = op(0x19, "NOT", false, opNot());
            opCodes[0x1a] = op(0x1a, "BYTE", false, opByte());
            opCodes[0x1b] = op(0x1b, "SHL", false, opShl());
            opCodes[0x1c] = op(0x1c, "SHR", false, opShr());
            opCodes[0x1d] = op(0x1d, "SAR", false, opSar());
            // 0x20 range - crypto
            opCodes[0x20] = op(0x20, "SHA3", true, opSha3(crypto));
            // 0x30 range - closure state
            opCodes[0x30] = op(0x30, "ADDRESS", false, opAddress());
            opCodes[0x31] = op(0x31, "BALANCE", true, opBalance());
            opCodes[0x32] = op(0x32, "ORIGIN", false, opOrigin());
            opCodes[0x33] = op(0x33, "CALLER", false, opCaller());
            opCodes[0x34] = op(0x34, "CALLVALUE", false, opCallValue());
            opCodes[0x35] = op(0x35, "CALLDATALOAD", false, opCallDataLoad());
            opCodes[0x36] = op(0x36, "CALLDATASIZE", false, opCallDataSize());
            opCodes[0x37] = op(0x37, "CALLDATACOPY", true, opCallDataCopy());
            opCodes[0x38] = op(0x38, "CODESIZE", false, opCodeSize());
            opCodes[0x39] = op(0x39, "CODECOPY", true, opCodeCopy());
            opCodes[0x3a] = op(0x3a, "GASPRICE", false, opGasPrice());
            opCodes[0x3b] = op(0x3b, "EXTCODESIZE", true, opExtCodeSize());
            opCodes[0x3c] = op(0x3c, "EXTCODECOPY", true, opExtCodeCopy());
            opCodes[0x3d] = op(0x3d, "RETURNDATASIZE", false, opReturnDataSize());
            opCodes[0x3e] = op(0x3e, "RETURNDATACOPY", true, opReturnDataCopy());
            opCodes[0x3f] = op(0x3f, "EXTCODEHASH", true, opExtCodeHash());
            // '0x40' range - block operations
            opCodes[0x40] = op(0x40, "BLOCKHASH", false, opBlockHash());
            opCodes[0x41] = op(0x41, "COINBASE", false, opCoinBase());
            opCodes[0x42] = op(0x42, "TIMESTAMP", false, opTimestamp());
            opCodes[0x43] = op(0x43, "NUMBER", false, opNumber());
            opCodes[0x44] = op(0x44, "DIFFICULTY", false, opDifficulty());
            opCodes[0x45] = op(0x45, "GASLIMIT", false, opGasLimit());
            opCodes[0x46] = op(0x46, "CHAINID", false, opChainId());
            opCodes[0x47] = op(0x47, "SELFBALANCE", false, opSelfBalance());
            opCodes[0x48] = op(0x48, "BASEFEE", false, opBaseFee());
            // 0x50 range - 'storage' and execution
            opCodes[0x50] = op(0x50, "POP", false, opPop());
            opCodes[0x51] = op(0x51, "MLOAD", true, opMLoad());
            opCodes[0x52] = op(0x52, "MSTORE", true, opMStore());
            opCodes[0x53] = op(0x53, "MSTORE8", true, opMStore8());
            opCodes[0x54] = op(0x54, "SLOAD", true, opSLoad());
            opCodes[0x55] = op(0x55, "SSTORE", true, opSStore());
            opCodes[0x56] = op(0x56, "JUMP", false, opJump());
            opCodes[0x57] = op(0x57, "JUMPI", false, opJumpI());
            opCodes[0x58] = op(0x58, "PC", false, opPc());
            opCodes[0x59] = op(0x59, "MSIZE", false, opMSize());
            opCodes[0x5a] = op(0x5a, "GAS", false, opGas());
            opCodes[0x5b] = op(0x5b, "JUMPDEST", false, opJumpDest());
            opCodes[0x5c] = op(0x5c, "BEGINSUB", false, opBeginSub());
            opCodes[0x5d] = op(0x5d, "RETURNSUB", false, opReturnSub());
            opCodes[0x5e] = op(0x5e, "JUMPSUB", false, opJumpSub());
            //0x5f - PUSH0
            opCodes[0x5f] = op(0x5f, "PUSH", false, opPush0());
            // 0x60 range - PUSH xx
            opCodes[0x60] = op(0x60, "PUSH", false, opPush(1));
            opCodes[0x61] = op(0x61, "PUSH", false, opPush(2));
            opCodes[0x62] = op(0x62, "PUSH", false, opPush(3));
            opCodes[0x63] = op(0x63, "PUSH", false, opPush(4));
            opCodes[0x64] = op(0x64, "PUSH", false, opPush(5));
            opCodes[0x65] = op(0x65, "PUSH", false, opPush(6));
            opCodes[0x66] = op(0x66, "PUSH", false, opPush(7));
            opCodes[0x67] = op(0x67, "PUSH", false, opPush(8));
            opCodes[0x68] = op(0x68, "PUSH", false, opPush(9));
            opCodes[0x69] = op(0x69, "PUSH", false, opPush(10));
            opCodes[0x6a] = op(0x6a, "PUSH", false, opPush(11));
            opCodes[0x6b] = op(0x6b, "PUSH", false, opPush(12));
            opCodes[0x6c] = op(0x6c, "PUSH", false, opPush(13));
            opCodes[0x6d] = op(0x6d, "PUSH", false, opPush(14));
            opCodes[0x6e] = op(0x6e, "PUSH", false, opPush(15));
            opCodes[0x6f] = op(0x6f, "PUSH", false, opPush(16));
            opCodes[0x70] = op(0x70, "PUSH", false, opPush(17));
            opCodes[0x71] = op(0x71, "PUSH", false, opPush(18));
            opCodes[0x72] = op(0x72, "PUSH", false, opPush(19));
            opCodes[0x73] = op(0x73, "PUSH", false, opPush(20));
            opCodes[0x74] = op(0x74, "PUSH", false, opPush(21));
            opCodes[0x75] = op(0x75, "PUSH", false, opPush(22));
            opCodes[0x76] = op(0x76, "PUSH", false, opPush(23));
            opCodes[0x77] = op(0x77, "PUSH", false, opPush(24));
            opCodes[0x78] = op(0x78, "PUSH", false, opPush(25));
            opCodes[0x79] = op(0x79, "PUSH", false, opPush(26));
            opCodes[0x7a] = op(0x7a, "PUSH", false, opPush(27));
            opCodes[0x7b] = op(0x7b, "PUSH", false, opPush(28));
            opCodes[0x7c] = op(0x7c, "PUSH", false, opPush(29));
            opCodes[0x7d] = op(0x7d, "PUSH", false, opPush(30));
            opCodes[0x7e] = op(0x7e, "PUSH", false, opPush(31));
            opCodes[0x7f] = op(0x7f, "PUSH", false, opPush(32));
            // 0x80 range - DUP xx
            opCodes[0x80] = op(0x80, "DUP", false, opDup(1));
            opCodes[0x81] = op(0x81, "DUP", false, opDup(2));
            opCodes[0x82] = op(0x82, "DUP", false, opDup(3));
            opCodes[0x83] = op(0x83, "DUP", false, opDup(4));
            opCodes[0x84] = op(0x84, "DUP", false, opDup(5));
            opCodes[0x85] = op(0x85, "DUP", false, opDup(6));
            opCodes[0x86] = op(0x86, "DUP", false, opDup(7));
            opCodes[0x87] = op(0x87, "DUP", false, opDup(8));
            opCodes[0x88] = op(0x88, "DUP", false, opDup(9));
            opCodes[0x89] = op(0x89, "DUP", false, opDup(10));
            opCodes[0x8a] = op(0x8a, "DUP", false, opDup(11));
            opCodes[0x8b] = op(0x8b, "DUP", false, opDup(12));
            opCodes[0x8c] = op(0x8c, "DUP", false, opDup(13));
            opCodes[0x8d] = op(0x8d, "DUP", false, opDup(14));
            opCodes[0x8e] = op(0x8e, "DUP", false, opDup(15));
            opCodes[0x8f] = op(0x8f, "DUP", false, opDup(16));
            // 0x90 range - SWAP xx
            opCodes[0x90] = op(0x90, "SWAP", false, opSwap(1));
            opCodes[0x91] = op(0x91, "SWAP", false, opSwap(2));
            opCodes[0x92] = op(0x92, "SWAP", false, opSwap(3));
            opCodes[0x93] = op(0x93, "SWAP", false, opSwap(4));
            opCodes[0x94] = op(0x94, "SWAP", false, opSwap(5));
            opCodes[0x95] = op(0x95, "SWAP", false, opSwap(6));
            opCodes[0x96] = op(0x96, "SWAP", false, opSwap(7));
            opCodes[0x97] = op(0x97, "SWAP", false, opSwap(8));
            opCodes[0x98] = op(0x98, "SWAP", false, opSwap(9));
            opCodes[0x99] = op(0x99, "SWAP", false, opSwap(10));
            opCodes[0x9a] = op(0x9a, "SWAP", false, opSwap(11));
            opCodes[0x9b] = op(0x9b, "SWAP", false, opSwap(12));
            opCodes[0x9c] = op(0x9c, "SWAP", false, opSwap(13));
            opCodes[0x9d] = op(0x9d, "SWAP", false, opSwap(14));
            opCodes[0x9e] = op(0x9e, "SWAP", false, opSwap(15));
            opCodes[0x9f] = op(0x9f, "SWAP", false, opSwap(16));
            // 0xa0 range - LOG xx
            opCodes[0xa0] = op(0xa0, "LOG", true, opLog(0));
            opCodes[0xa1] = op(0xa1, "LOG", true, opLog(1));
            opCodes[0xa2] = op(0xa2, "LOG", true, opLog(2));
            opCodes[0xa3] = op(0xa3, "LOG", true, opLog(3));
            opCodes[0xa4] = op(0xa4, "LOG", true, opLog(4));
            // 0xf0 range - closure
            opCodes[0xf0] = op(0xf0, "CREATE", true, makeCreateFunction(Vm.CreateKind.CREATE));
            opCodes[0xf1] = op(0xf1, "CALL", true, makeCallFunction(Vm.CallKind.CALL, callStipend));
            opCodes[0xf2] = op(0xf2, "CALLCODE", true, makeCallFunction(Vm.CallKind.CALL_CODE, callStipend));
            opCodes[0xf3] = op(0xf3, "RETURN", true, opReturn());
            opCodes[0xf4] = op(0xf4, "DELEGATECALL", true, makeCallFunction(Vm.CallKind.DELEGATE_CALL, callStipend));
            opCodes[0xf5] = op(0xf5, "CREATE2", true, makeCreateFunction(Vm.CreateKind.CREATE2));
            //0xfe range - other
            opCodes[0xfa] = op(0xfa, "STATICCALL", true, makeCallFunction(Vm.CallKind.STATIC_CALL, callStipend));
            opCodes[0xfd] = op(0xfd, "REVERT", true, opRevert());
            opCodes[0xfe] = INVALID;

            return opCodes;
        }
    }

    /**
     * Returns true if current op code is INVALID for current virtual machine
     *
     * @return true iif current op code is INVALID for current virtual machine
     */
    public static boolean isInvalidOpcode(int code) {
        return code == INVALID.getCode();
    }

    /**
     * Returns all valid jump and jumpsub destinations,
     * array of values where validJumps[index] has value 0 (default), 1 (jumpdest), 2 (beginsub)
     * <p>
     * <p>
     * isCode returns true if the provided PC location is an actual opcode, as opposed to a data-segment following a PUSHN operation.
     *
     * @param code given code
     * @return valid jumps array
     */
    public static byte[] getValidJumps(byte[] code) {
        var jumps = Bytes.alloc(code.length, 0);
        for (var i = 0; i < code.length; i++) {
            var opcode = code[i];
            // skip over PUSH0-32 since no jump destinations in the middle of a push block
            if (opcode <= ((byte) 0x7f)) {
                if (opcode >= 0x60) {
                    i += opcode - 0x5f;//skip over data segment occupied by PUSHxx arguments
                } else if (opcode == 0x5b) {
                    // Define a JUMPDEST as a 1 (it's a valid destination)
                    jumps[i] = 1;
                } else if (opcode == 0x5c) {
                    // Define a BEGINSUB as a 2 (it's a valid destination)
                    jumps[i] = 2;
                }
            }
        }
        return jumps;
    }

    /**
     * Creates the OpCode instance
     */
    private static OpCode op(int code, String name, boolean dynamicGas, ExecFn fn) {
        return new OpCodeImpl(code, name, dynamicGas, fn);
    }

    public static void validate(OpCode[] opcodes) {
        AtomicBoolean valid = new AtomicBoolean(true);
        for (var opCode : opcodes) {
            if (!opCode.validate()) {
                log.error("Wrong opcode configuration, opcode=" + opCode);
                valid.set(false);
            }
        }
        if (!valid.get()) {
            throw new ConfigurationException("Wrong opcode configuration");
        }
    }

}
