package io.aplfintech.grape.grap3.crypto;

import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters;
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters;
import org.bouncycastle.math.ec.rfc8032.Ed25519;

public class Ed25519DSA implements DSA {
    @Override
    public byte[] sign(byte[] privateKey, byte[] message) {
        Ed25519PrivateKeyParameters pk = new Ed25519PrivateKeyParameters(
            privateKey, 0);
        byte[] signature = new byte[64];
        pk.sign(0, null, message, 0, message.length, signature, 0);
        return signature;
    }

    @Override
    public boolean verify(byte[] pubKey, byte[] signature, byte[] message) {
        Ed25519PublicKeyParameters pubKeyObj = new Ed25519PublicKeyParameters(pubKey, 0);
        return Ed25519.verify(signature, 0, pubKeyObj.getEncoded(), 0, message, 0, message.length);
    }

    @Override
    public byte[] generatePublicKey(byte[] privateKey) {
        Ed25519PrivateKeyParameters pk = new Ed25519PrivateKeyParameters(
            privateKey, 0);
        return pk.generatePublicKey().getEncoded();
    }
}
