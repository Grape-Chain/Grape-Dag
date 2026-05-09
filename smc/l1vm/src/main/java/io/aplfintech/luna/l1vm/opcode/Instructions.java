package io.aplfintech.luna.l1vm.opcode;

import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.exception.InterpreterExecutionException;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.code.Codes;
import io.aplfintech.luna.math.BigNum;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.math.UInt256;
import io.aplfintech.luna.model.Hash;
import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.Log;
import io.aplfintech.luna.vm.Vm;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.contract.ContractResult;
import io.aplfintech.luna.vm.opcode.ExecFn;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.vm.opcode.FnResult;
import io.aplfintech.luna.vm.opcode.OpCode;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

import static io.aplfintech.luna.l1vm.Constants.KECCAK256_NULL;
import static io.aplfintech.luna.math.Math256.UINT_256_256;
import static io.aplfintech.luna.math.Math256.UINT_256_ZERO;
import static io.aplfintech.luna.math.Math256.WORD_SIZE;


/**
 * Factory of functions to execute opCodes
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Instructions {
    /*0x00 - STOP*/
    static ExecFn opStop() {
        return runState -> status(VmStatus.VM_STOP_TOKEN);
    }

    /*0x01 - ADD*/
    static ExecFn opAdd() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.add(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x02 - MUL*/
    static ExecFn opMul() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.mul(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x03 - SUB*/
    static ExecFn opSub() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.sub(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x04 - DIV*/
    static ExecFn opDiv() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.div(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x05 - SDIV*/
    static ExecFn opSDiv() {
        return runState -> {//a two's complement signed number
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.sdiv(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x06 - MOD*/
    static ExecFn opMod() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.mod(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x07 - SMOD*/
    static ExecFn opSMod() {
        return runState -> {//a two's complement signed number
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.smod(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x08 - ADDMOD*/
    static ExecFn opAddMod() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var z = runState.getStack().pop().asBigNum();
            var r = x.modadd(y, z);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x09 - MULMOD*/
    static ExecFn opMulMod() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var z = runState.getStack().pop().asBigNum();
            var r = x.modmul(y, z);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x0a - EXP*/
    static ExecFn opExp() {
        return runState -> {
            var base = runState.getStack().pop().asBigNum();
            var exponent = runState.getStack().pop().asBigNum();
            var r = base.pow(exponent);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x0b - SIGNEXTEND*/
    static ExecFn opSignExtend() {
        return runState -> {//a two's complement signed number
            var byteNum = runState.getStack().pop().asBigNum();
            var number = runState.getStack().pop().asBigNum();
            var r = Math256.signExt(number, byteNum);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x10 - LT*/
    static ExecFn opLt() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.lt(y);
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x11 - GT*/
    static ExecFn opGt() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.gt(y);
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x12 - SLT*/
    static ExecFn opSLt() {
        return runState -> {//a two's complement signed number
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.slt(y);
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x13 - SGT*/
    static ExecFn opSGt() {
        return runState -> {//a two's complement signed number
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.sgt(y);
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x14 - EQ*/
    static ExecFn opEq() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.eq(y);
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x15 - ISZERO*/
    static ExecFn opIsZero() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var r = x.isZero();
            runState.getStack().push(asWord(r));
            return success();
        };
    }

    /*0x16 - AND*/
    static ExecFn opAnd() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.and(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x17 - OR*/
    static ExecFn opOr() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.or(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x18 - XOR*/
    static ExecFn opXor() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var y = runState.getStack().pop().asBigNum();
            var r = x.xor(y);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x19 - NOT*/
    static ExecFn opNot() {
        return runState -> {
            var x = runState.getStack().pop().asBigNum();
            var r = x.not();
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x1a - BYTE*/
    static ExecFn opByte() {
        return runState -> {
            var pos = runState.getStack().pop().asBigNum().intValue(true);
            var word = runState.getStack().pop().asBigNum();
            var r = Math256.getByte(pos, word);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x1b - SHL*/
    static ExecFn opShl() {
        return runState -> {
            var shift = runState.getStack().pop().asBigNum();
            var word = runState.getStack().pop().asBigNum();
            var r = shift.lt(UINT_256_256) ? word.shl(shift.intValue()) : UINT_256_ZERO;
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x1c - SHR*/
    static ExecFn opShr() {
        return runState -> {
            var shift = runState.getStack().pop().asBigNum();
            var word = runState.getStack().pop().asBigNum();
            var r = shift.lt(UINT_256_256) ? word.shr(shift.intValue()) : UINT_256_ZERO;
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x1d - SAR*/
    static ExecFn opSar() {
        return runState -> {//(Signed/Arithmetic right shift)
            var shift = runState.getStack().pop().asBigNum();
            var word = runState.getStack().pop().asBigNum();
            var r = word.sar(shift);
            runState.getStack().push(r.asWord());
            return success();
        };
    }

    /*0x20 - SHA3*/
    static ExecFn opSha3(CryptoLib crypto) {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var size = runState.getStack().pop().asBigNum().intValue();
            byte[] hash;
            if (size > 0) {
                var data = runState.getMemory().read(offset, size);
                hash = crypto.keccak256(data);
            } else {
                hash = KECCAK256_NULL;
            }
            runState.getStack().push(Math256.uint256(hash));
            return success();
        };
    }

    /*0x30 - ADDRESS*/
    static ExecFn opAddress() {
        return runState -> {
            var r = runState.getContract().address().bytes();
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x31 - BALANCE*/
    static ExecFn opBalance() {
        return runState -> {
            var slot = runState.getStack().pop().bytes();
            var address = VmAddress.from(slot);
            var balance = runState.getInterpreter().getVm().stateAccess().getBalance(address).toByteArray();
            runState.getStack().push(Math256.uint256(balance));
            return success();
        };
    }

    /*0x32 - ORIGIN*/
    static ExecFn opOrigin() {
        return runState -> {
            var r = runState.getInterpreter().getVm().context().getOrigin().bytes();
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x33 - CALLER*/
    static ExecFn opCaller() {
        return runState -> {
            var r = runState.getContract().caller().bytes();
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x34 - CALLVALUE*/
    static ExecFn opCallValue() {
        return runState -> {
            var r = runState.getContract().value();

            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x35 - CALLDATALOAD*/
    static ExecFn opCallDataLoad() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var r = Bytes.slicePadded(runState.getContract().getInput(), offset, WORD_SIZE);
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x36 - CALLDATASIZE*/
    static ExecFn opCallDataSize() {
        return runState -> {
            var r = runState.getContract().getInput().length;
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x37 - CALLDATACOPY*/
    static ExecFn opCallDataCopy() {
        return runState -> {
            var memOffset = runState.getStack().pop().asBigNum().intValue();
            var dataOffset = runState.getStack().pop().asBigNum().intValue(true);
            var length = runState.getStack().pop().asBigNum().intValue();
            var data = Bytes.slicePadded(runState.getContract().getInput(), dataOffset, length);
            runState.getMemory().write(memOffset, length, data);
            return success();
        };
    }

    /*0x38 - CODESIZE*/
    static ExecFn opCodeSize() {
        return runState -> {
            var length = runState.getContract().code().size();
            runState.getStack().push(Math256.uint256(length));
            return success();
        };
    }

    /*0x39 - CODECOPY*/
    static ExecFn opCodeCopy() {
        return runState -> {
            var memOffset = runState.getStack().pop().asBigNum().intValue();
            var codeOffset = runState.getStack().pop().asBigNum().intValue(true);
            var length = runState.getStack().pop().asBigNum().intValue();
            var data = Bytes.slicePadded(runState.getContract().code().bytes(), codeOffset, length);
            runState.getMemory().write(memOffset, length, data);
            return success();
        };
    }

    /*0x3a - GASPRICE*/
    static ExecFn opGasPrice() {
        return runState -> {
            var price = runState.getInterpreter().getVm().context().gasPrice();
            runState.getStack().push(Math256.uint256(price));
            return success();
        };
    }

    /*0x3b - EXTCODESIZE*/
    static ExecFn opExtCodeSize() {
        return runState -> {
            var slot = runState.getStack().pop().bytes();
            var extContractAddress = VmAddress.from(slot);
            var codeSize = runState.getInterpreter().getVm().stateAccess().getContractCodeSize(extContractAddress);
            runState.getStack().push(Math256.uint256(codeSize));
            return success();
        };
    }

    /*0x3c - EXTCODECOPY*/
    static ExecFn opExtCodeCopy() {
        return runState -> {
            var slot = runState.getStack().pop().bytes();
            var extContractAddress = VmAddress.from(slot);
            var memOffset = runState.getStack().pop().asBigNum().intValue();
            var codeOffset = runState.getStack().pop().asBigNum().intValue(true);
            var length = runState.getStack().pop().asBigNum().intValue();
            byte[] extContractCode = runState.getInterpreter().getVm().stateAccess().getContractCode(extContractAddress);
            var data = Bytes.slicePadded(extContractCode, codeOffset, length);
            runState.getMemory().write(memOffset, length, data);
            return success();
        };
    }

    /*0x3d - RETURNDATASIZE*/
    static ExecFn opReturnDataSize() {
        return runState -> {
            var length = Bytes.length(runState.getInterpreter().getReturnData());
            runState.getStack().push(Math256.uint256(length));
            return success();
        };
    }

    /*0x3e - RETURNDATACOPY*/
    static ExecFn opReturnDataCopy() {
        return runState -> {
            try {
                var memOffset = runState.getStack().pop().asBigNum().intValue();
                var dataOffset = runState.getStack().pop().asBigNum().intValue();
                var length = runState.getStack().pop().asBigNum().intValue();
                var end = Math.addExact(dataOffset, length);
                if (Bytes.length(runState.getInterpreter().getReturnData()) < end) {
                    return status(VmStatus.VM_RETURN_DATA_OUT_OF_BOUNDS);
                }
                var data = Bytes.slicePadded(runState.getInterpreter().getReturnData(), dataOffset, length);
                runState.getMemory().write(memOffset, length, data);
                return success();
            } catch (ArithmeticException e) {
                return status(VmStatus.VM_RETURN_DATA_OUT_OF_BOUNDS);
            }
        };
    }

    /*0x3f - EXTCODEHASH*/
    static ExecFn opExtCodeHash() {
        return runState -> {
            var slot = runState.getStack().pop().bytes();
            var extContractAddress = VmAddress.from(slot);
            UInt256 hash;
            if (!runState.getInterpreter().getVm().stateAccess().isAccountExists(extContractAddress)
                || runState.getInterpreter().getVm().stateAccess().accountIsEmpty(extContractAddress)) {
                hash = Math256.UINT_256_ZERO;
            } else {
                hash = Math256.uint256(runState.getInterpreter().getVm().stateAccess().getContractCodeHash(extContractAddress));
            }
            runState.getStack().push(hash);
            return success();
        };
    }

    /*0x40 - BLOCKHASH*/
    static ExecFn opBlockHash() {
        return runState -> {//provided for the latest 256 blocks
            var num = runState.getStack().pop().asBigNum();
            var upper = Math256.uint256(runState.getInterpreter().getVm().context().blockNumber());
            BigNum lower;
            BigNum hash;
            if (upper.lt(Math256.uint256(257))) {
                lower = Math256.UINT_256_ZERO;
            } else {
                lower = upper.sub(Math256.uint256(256));
            }
            if (num.gte(lower) && num.lt(upper)) {
                var block = new BigInteger(num.asWord().bytes32());
                hash = Math256.uint256(runState.getInterpreter().getVm().stateAccess().getBlockHash(block));
            } else {
                hash = Math256.UINT_256_ZERO;
            }
            runState.getStack().push(hash.asWord());
            return success();
        };
    }

    /*0x41 - COINBASE*/
    static ExecFn opCoinBase() {
        return runState -> {
            var coinbase = runState.getInterpreter().getVm().context().coinbase().bytes();
            runState.getStack().push(Math256.uint256(coinbase));
            return success();
        };
    }

    /*0x42 - */
    static ExecFn opTimestamp() {
        return runState -> {
            var timestamp = runState.getInterpreter().getVm().context().timestamp();
            runState.getStack().push(Math256.uint256(timestamp));
            return success();
        };
    }

    /*0x43 - NUMBER*/
    static ExecFn opNumber() {
        return runState -> {
            var number = runState.getInterpreter().getVm().context().blockNumber().toByteArray();
            runState.getStack().push(Math256.uint256(number));
            return success();
        };
    }

    /*0x44 - DIFFICULTY, RANDOM, PREVRANDAO*/
    static ExecFn opDifficulty() {
        return runState -> {
            //INFO: the latest version of EVM uses PREVRANDAO instead of DIFFICULTY
            var random = runState.getInterpreter().getVm().context().prevRandao();
            runState.getStack().push(Math256.uint256(random));
            return success();
        };
    }

    /*0x45 - GASLIMIT*/
    static ExecFn opGasLimit() {
        return runState -> {
            var gasLimit = runState.getInterpreter().getVm().context().gasLimit().toByteArray();
            runState.getStack().push(Math256.uint256(gasLimit));
            return success();
        };
    }

    /*0x46 - CHAINID*/
    static ExecFn opChainId() {
        return runState -> {
            var chainId = runState.getInterpreter().getVm().stateAccess().chainId();
            runState.getStack().push(Math256.uint256(chainId));
            return success();
        };
    }

    /*0x47 - SELFBALANCE*/
    static ExecFn opSelfBalance() {
        return runState -> {
            var address = runState.getContract().address();
            var balance = runState.getInterpreter().getVm().stateAccess().getBalance(address).toByteArray();
            runState.getStack().push(Math256.uint256(balance));
            return success();
        };
    }

    /*0x48 - BASEFEE*/
    static ExecFn opBaseFee() {
        return runState -> {
            var baseFee = runState.getInterpreter().getVm().context().baseFeePerGas().toByteArray();
            runState.getStack().push(Math256.uint256(baseFee));
            return success();
        };
    }

    /*0x50 - POP*/
    static ExecFn opPop() {
        return runState -> {
            runState.getStack().pop();
            return success();
        };
    }

    /*0x51 - MLOAD*/
    static ExecFn opMLoad() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var r = runState.getMemory().read(offset, WORD_SIZE);
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x52 - MSTORE*/
    static ExecFn opMStore() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var value = runState.getStack().pop().bytes32();
            runState.getMemory().write(offset, WORD_SIZE, value);
            return success();
        };
    }

    /*0x53 - MSTORE8*/
    static ExecFn opMStore8() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var value = (byte) runState.getStack().pop().asBigNum().intValue();
            runState.getMemory().write(offset, 1, new byte[]{value});
            return success();
        };
    }

    /*0x54 - SLOAD*/
    static ExecFn opSLoad() {
        return runState -> {
            var key = runState.getStack().pop().bytes32();
            var address = runState.getContract().address();
            var r = runState.getInterpreter().getVm().stateAccess().getContractStorage(address, key);
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0x55 - SSTORE*/
    static ExecFn opSStore() {
        return runState -> {
            if (runState.getInterpreter().isReadonly()) {
                return status(VmStatus.VM_STATIC_MODE_VIOLATION);
            }
            var key = runState.getStack().pop().bytes32();
            var value = runState.getStack().pop().bytes();
            var address = runState.getContract().address();
            runState.getInterpreter().getVm().stateAccess().putContractStorage(address, key, value);
            return success();
        };
    }

    /*0x56 - JUMP*/
    static ExecFn opJump() {
        return runState -> {
            var pos = runState.getStack().pop().asBigNum().longValue();
            if (!runState.getContract().isValidJumpDest(pos, 1)) {
                return status(VmStatus.VM_BAD_JUMP_DESTINATION);
            }
            runState.setPc(pos - 1);// pc will be increased by the interpreter loop
            return success();
        };
    }

    /*0x57 - JUMPI*/
    static ExecFn opJumpI() {
        return runState -> {
            var pos = runState.getStack().pop().asBigNum().longValue();
            var isZero = runState.getStack().pop().asBigNum().isZero();
            if (!isZero) {
                if (!runState.getContract().isValidJumpDest(pos, 1)) {
                    return status(VmStatus.VM_BAD_JUMP_DESTINATION);
                }
                runState.setPc(pos - 1);// pc will be increased by the interpreter loop
            }
            return success();
        };
    }

    /*0x58 - PC*/
    static ExecFn opPc() {
        return runState -> {
            runState.getStack().push(Math256.uint256(runState.getPc()));
            return success();
        };
    }

    /*0x59 - MSIZE*/
    static ExecFn opMSize() {
        return runState -> {
            runState.getStack().push(Math256.uint256(runState.getMemory().size()));
            return success();
        };
    }

    /*0x5a - GAS*/
    static ExecFn opGas() {
        return runState -> {
            runState.getStack().push(Math256.uint256(runState.getContract().gas()));
            return success();
        };
    }

    /*0x5b - JUMPDEST*/
    static ExecFn opJumpDest() {
        return runState -> success();
    }

    /*0x5c - BEGINSUB*/
    static ExecFn opBeginSub() {
        return runState -> status(VmStatus.VM_INVALID_BEGINSUB);
    }

    /*0x5d - RETURNSUB*/
    static ExecFn opReturnSub() {
        return runState -> {
            var pos = runState.getStack().pop().asBigNum().longValue();
            runState.setPc(pos);
            return success();
        };
    }

    /*0x5e - JUMPSUB*/
    static ExecFn opJumpSub() {
        return runState -> {
            var pos = runState.getStack().pop().asBigNum().longValue();
            if (!runState.getContract().isValidJumpDest(pos, 2)) {
                return status(VmStatus.VM_INVALID_JUMPSUB);
            }
            runState.getStack().push(Math256.uint256(pos));
            runState.setPc(pos);
            return success();
        };
    }

    /*0x5f - PUSH0*/
    static ExecFn opPush0() {
        return runState -> {
            runState.getStack().push(Math256.UINT_256_ZERO);
            return success();
        };
    }

    /*0x60-0x7f - PUSHxx*/
    static ExecFn opPush(int numToPush) {
        return runState -> {
            var pc = runState.getPc();
            long codeSize = runState.getContract().code().size();
            var startMin = codeSize;
            if (pc + 1 < startMin) {
                startMin = pc + 1;
            }
            var endMin = codeSize;
            if (startMin + numToPush < endMin) {
                endMin = startMin + numToPush;
            }
            if (pc + numToPush > codeSize) {
                return status(VmStatus.VM_ARGUMENT_OUT_OF_RANGE);
            }
            byte[] data = runState.getContract().code().slice(startMin, endMin);
            runState.getStack().push(Math256.uint256(data));
            runState.addPc(numToPush);

            return success(data);
        };
    }

    /*0x80-0x8f - DUP*/
    static ExecFn opDup(int stackDups) {
        return runState -> {
            runState.getStack().dup(stackDups);
            return success();
        };
    }

    /*0x90-0x9f - SWAP*/
    static ExecFn opSwap(int stackPops) {
        return runState -> {
            runState.getStack().swap(stackPops);
            return success();
        };
    }

    /*0xa0-0xa4 - LOG*/
    static ExecFn opLog(int topicsCount) {
        return runState -> {
            try {
                if (runState.getInterpreter().isReadonly()) {
                    return status(VmStatus.VM_STATIC_MODE_VIOLATION);
                }
                var stack = runState.getStack();
                Hash[] topics = new Hash[topicsCount];
                var memOffset = runState.getStack().pop().asBigNum().intValue();
                var memSize = runState.getStack().pop().asBigNum().intValue();
                for (int i = 0; i < topicsCount; i++) {
                    topics[i] = new Hash(stack.pop().bytes32());
                }
                var data = runState.getMemory().read(memOffset, memSize);
                var event = new Log(runState.getContract().address(), topics, data);
                runState.getInterpreter().getVm().addLog(event);
                return success();
            } catch (ArithmeticException e) {
                return status(VmStatus.VM_RETURN_DATA_OUT_OF_BOUNDS);
            }
        };
    }

    /*0xb3 - TLOAD*/
    static ExecFn opTLoad() {
        return runState -> {
            var key = runState.getStack().pop().bytes32();
            var address = runState.getContract().address();
            var r = runState.getInterpreter().getVm().stateAccess().getTransientStorage(address, key);
            runState.getStack().push(Math256.uint256(r));
            return success();
        };
    }

    /*0xb4 - TSTORE*/
    static ExecFn opTStore() {
        return runState -> {
            if (runState.getInterpreter().isReadonly()) {
                return status(VmStatus.VM_STATIC_MODE_VIOLATION);
            }
            var key = runState.getStack().pop().bytes32();
            var value = runState.getStack().pop().bytes();
            var address = runState.getContract().address();
            runState.getInterpreter().getVm().stateAccess().putTransientStorage(address, key, value);
            return success();
        };
    }

    /*0xf3 - RETURN*/
    static ExecFn opReturn() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var size = runState.getStack().pop().asBigNum().intValue();
            var returnResult = runState.getMemory().read(offset, size);
            return status(returnResult, VmStatus.VM_STOP_TOKEN);
        };
    }

    /*0xfd - REVERT*/
    static ExecFn opRevert() {
        return runState -> {
            var offset = runState.getStack().pop().asBigNum().intValue();
            var size = runState.getStack().pop().asBigNum().intValue();
            var returnResult = runState.getMemory().read(offset, size);
            runState.getInterpreter().setReturnData(returnResult);
            return status(returnResult, VmStatus.VM_REVERT);
        };
    }

    /*0xfe - INVALID*/
    static ExecFn opInvalid() {
        return runState -> {
            var pc = runState.getPc();
            var opCode = runState.getContract().getOPCode(pc);
            log.error("Invalid opCode{}", HexUtils.toHex(opCode));
            return status(VmStatus.VM_INVALID_INSTRUCTION);
        };
    }

    /*0xff - SELFDESTRUCT*/
    static ExecFn opSelfDestruct() {
        return runState -> {
            if (runState.getInterpreter().isReadonly()) {
                return status(VmStatus.VM_STATIC_MODE_VIOLATION);
            }
            var beneficiary = VmAddress.from(runState.getStack().pop().bytes());
            var suicide = runState.getContract().address();
            var balance = runState.getInterpreter().getVm().stateAccess().getBalance(suicide);
            runState.getInterpreter().getVm().stateAccess().addBalance(beneficiary, balance);
            runState.getInterpreter().getVm().stateAccess().suicide(suicide);
            return status(VmStatus.VM_STOP_TOKEN);
        };
    }

    static ExecFn makeCreateFunction(Vm.CreateKind contractKind) {
        return runState -> {
            if (runState.getInterpreter().isReadonly()) {
                return status(VmStatus.VM_STATIC_MODE_VIOLATION);
            }
            var value = runState.getStack().pop().asBigNum().bigIntegerValue();
            var offset = runState.getStack().pop().asBigNum().intValue(false);
            var size = runState.getStack().pop().asBigNum().intValue(false);

            var input = runState.getMemory().read(offset, size);
            var gas = runState.getContract().gas();
            gas -= gas / 64;
            runState.getContract().useGas(gas);
            final Vm vm = runState.getInterpreter().getVm();
            var caller = runState.getContract();
            ContractResult result = switch (contractKind) {
                case CREATE -> vm.create(caller, Codes.from(input), gas, value);
                case CREATE2 -> vm.create2(caller, Codes.from(input), gas, value, runState.getStack().pop().bytes32());
            };
            if (result == null) {
                throw new InterpreterExecutionException(VmStatus.VM_UNEXPECTED_BEHAVIOR, "Unexpected behavior, the create result is null");
            }
            UInt256 address;
            if (result.hasError() && VmStatus.VM_CODESTORE_OUT_OF_GAS != result.executionStatus()) {
                address = Math256.UINT_256_ZERO;
            } else {
                address = Math256.uint256(result.contract().bytes());
            }
            //push address of the created contract
            runState.getStack().push(address);
            //add returned gas
            runState.getContract().addGas(result.gas());
            if (result.hasError() && VmStatus.VM_REVERT == result.executionStatus()) {
                //set REVERT data to return on the up level
                runState.getInterpreter().setReturnData(result.output());
                return success(result.output());
            }
            //clear dirty array
            runState.getInterpreter().clearReturnData();
            return success();
        };
    }

    static ExecFn makeCallFunction(Vm.CallKind callKind, long callStipend) {
        return runState -> {
            var stack = runState.getStack();
            stack.pop();//unused because the actual available gas is kept in the temporary var - runState.getVm().callGas
            Vm vm = runState.getInterpreter().getVm();
            long gasLimit = vm.getCallGas();
            var toAddr = VmAddress.from(stack.pop().bytes());
            BigNum value;
            if (Vm.CallKind.CALL == callKind || Vm.CallKind.CALL_CODE == callKind) {
                value = stack.pop().asBigNum();
            } else {
                value = Math256.UINT_256_ZERO;
            }
            var inOffset = stack.pop().asBigNum().intValue();
            var inSize = stack.pop().asBigNum().intValue();
            var retOffset = stack.pop().asBigNum().intValue();
            var retSize = stack.pop().asBigNum().intValue();
            //reads the arguments from the memory
            var args = runState.getMemory().read(inOffset, inSize);
            if (runState.getInterpreter().isReadonly() && Vm.CallKind.CALL == callKind && value.isNonZero()) {
                return status(VmStatus.VM_STATIC_MODE_VIOLATION);
            }
            if (!runState.getContract().useGas(gasLimit)) {
                return status(VmStatus.VM_OUT_OF_GAS);
            }
            if (value.isNonZero() && Vm.CallKind.CALL == callKind) {
                gasLimit += callStipend;
            }
            var caller = runState.getContract();
            ContractResult result =
                switch (callKind) {
                    case CALL -> vm.runCall(caller, toAddr, args, gasLimit, value.bigIntegerValue());
                    case CALL_CODE -> vm.runCallCode(caller, toAddr, args, gasLimit, value.bigIntegerValue());
                    case DELEGATE_CALL -> vm.runDelegateCall(caller, toAddr, args, gasLimit);
                    case STATIC_CALL -> vm.runStaticCall(caller, toAddr, args, gasLimit);
                };
            if (result == null) {
                throw new InterpreterExecutionException(VmStatus.VM_UNEXPECTED_BEHAVIOR, "Unexpected behavior, the CALL result is null");
            }
            stack.push(asWord(result.isSuccess()));
            if (result.isSuccess() || VmStatus.VM_REVERT == result.executionStatus()) {
                log.trace("Write CALL instruction output, offset={} size={} output={}", retOffset, retSize, HexUtils.toHex(result.output(), true));
                runState.getMemory().write(retOffset, retSize, result.output());
            }
            //add returned gas
            runState.getContract().addGas(result.gas());
            //set return data
            runState.getInterpreter().setReturnData(result.output());
            return success(result.output());
        };
    }

    private static UInt256 asWord(boolean r) {
        return r ? Math256.UINT_256_ONE : Math256.UINT_256_ZERO;
    }

    private static FnExecResult status(ExecutionStatus status) {
        return new FnResult(status, null);
    }

    private static FnExecResult status(byte[] returnResult, ExecutionStatus status) {
        return new FnResult(status, returnResult);
    }

    private static FnExecResult success() {
        return success(null);
    }

    private static FnExecResult success(byte[] result) {
        return new FnResult(VmStatus.VM_SUCCESS, result);
    }

    /**
     * Returns the wrapped functions map for logging.
     *
     * @return functions map for opcodes
     * @see ExecFn
     */
    public static OpCode[] createFunctionsWithLogging(@NonNull OpCode[] opCodes) {
        var wrapped = new OpCode[opCodes.length];
        for (int i = 0; i < opCodes.length; i++) {
            if (opCodes[i] != null) {
                var fn = new ExecFnWrapper(opCodes[i].getFn());
                wrapped[i] = new OpCodeImpl(opCodes[i], fn);
            }
        }
        return wrapped;
    }

    private record ExecFnWrapper(ExecFn fn) implements ExecFn {

        @Override
        public FnExecResult apply(RunContext runContext) {
            log.trace("---> stack={}", runContext.getStack().toString());
            var rc = fn.apply(runContext);
            log.trace("<--- stack={}", runContext.getStack().toString());
            return rc;
        }
    }
}
