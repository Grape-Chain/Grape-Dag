package io.aplfintech.grape.grap3.ether.precompile;

import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.utils.Bytes;

import static io.aplfintech.grape.math.Math256.WORD_SIZE;
import static io.aplfintech.grape.math.Math256.padToWord;
import static io.aplfintech.grape.utils.Bytes.isAllZero;
import static io.aplfintech.grape.utils.Bytes.slice;
import static java.lang.System.arraycopy;

/**
 * EC-RECOVER from ECDSA signature
 * 0x01
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class EcRecover extends PrecompiledContract {
    public EcRecover(GasPrice price, CryptoLib crypto) {
        super(price, crypto);
    }

    @Override
    public long requiredGas(byte[] input) {
        return price.lookForGasPrice("ecRecover");
    }

    @Override
    public FnExecResult run(byte[] input) {
        var ecRecoverInputLength = 128;
        input = Bytes.rightPadBytes(input, ecRecoverInputLength);
        // "input" is (hash, v, r, s), each 32 bytes
        var r = extractParam(input, 64, WORD_SIZE);
        var s = extractParam(input, 96, WORD_SIZE);
        byte v = (byte) (input[63] - 27);
        // tighter sig s values input homestead only apply to tx sigs
        if (!isAllZero(slice(input, 32, 63)) || !crypto.validateSignatureValues(v, r, s)) {
            return EMPTY_SUCCESS_RESULT;
        }
        // We must make sure not to modify the 'input', so placing the 'v' along with
        // the signature needs to be done on a new allocation
        var sig = new byte[65];
        arraycopy(input, 64, sig, 0, 64);
        sig[64] = v;
        // v needs to be at the end for libsecp256k1
        var pubKey = crypto.ecRecover(slice(input, 0, 32), sig);
        // make sure the public key is a valid one
        if (pubKey == null || pubKey.length == 0) {
            return EMPTY_SUCCESS_RESULT;
        }
        // the first byte of pubkey is bitcoin heritage
        byte[] address = crypto.recoverAddress(slice(pubKey, 1));
        return success(padToWord(address));
    }
}
