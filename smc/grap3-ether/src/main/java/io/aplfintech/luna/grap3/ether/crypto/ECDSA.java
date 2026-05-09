package io.aplfintech.luna.grap3.ether.crypto;

import com.google.common.base.Preconditions;
import io.aplfintech.luna.grap3.crypto.CryptoLibException;
import io.aplfintech.luna.grap3.crypto.DSARecoverable;
import io.aplfintech.luna.utils.Bytes;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
public class ECDSA implements DSARecoverable {

    @Override
    public byte[] sign(byte[] privateKeyBytes, byte[] message) throws CryptoLibException {
        var sig = Secp256.sign(privateKeyBytes, message);
        return sig.encode();
    }

    @Override
    public boolean verify(byte[] pubKey, byte[] signature, byte[] message) throws CryptoLibException {
        return Secp256.verify(pubKey, signature, message);
    }

    @Override
    public byte[] generatePublicKey(byte[] privateKeyBytes) {
        var publicKey = Secp256.publicKeyFromPrivate(new BigInteger(1, privateKeyBytes));
        return Bytes.asUnsignedByteArray(publicKey);
    }

    @Override
    public boolean validateSignatureValues(byte v, BigInteger r, BigInteger s) {
        return Secp256.validateSignatureValues(v, r, s);
    }

    @Override
    public byte[] recover(byte[] message, byte[] signature) throws CryptoLibException {
        Preconditions.checkArgument(message.length == 32, "Invalid Message length");
        Preconditions.checkArgument(signature.length == 65, "Invalid Signature length");
        Preconditions.checkArgument(signature[64] < 4, "Invalid recovery ID");
        var v = signature[64];
        var sig = Secp256.ECDSASignature.from(signature);
        var pubKey = Secp256.recoverFromSignature(v, sig, message);
        return pubKey != null ? Bytes.asUnsignedByteArray(pubKey) : new byte[0];
    }

}
