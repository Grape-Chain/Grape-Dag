package io.aplfintech.grape.grap3.ether.crypto;

import io.aplfintech.grape.grap3.crypto.CryptoLibException;
import io.aplfintech.grape.grap3.crypto.KeyPair;
import io.aplfintech.grape.utils.Bytes;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;
import org.bouncycastle.asn1.x9.X9ECParameters;
import org.bouncycastle.asn1.x9.X9IntegerConverter;
import org.bouncycastle.crypto.digests.SHA256Digest;
import org.bouncycastle.crypto.ec.CustomNamedCurves;
import org.bouncycastle.crypto.generators.ECKeyPairGenerator;
import org.bouncycastle.crypto.params.ECDomainParameters;
import org.bouncycastle.crypto.params.ECKeyGenerationParameters;
import org.bouncycastle.crypto.params.ECPrivateKeyParameters;
import org.bouncycastle.crypto.params.ECPublicKeyParameters;
import org.bouncycastle.crypto.signers.ECDSASigner;
import org.bouncycastle.crypto.signers.HMacDSAKCalculator;
import org.bouncycastle.math.ec.ECAlgorithms;
import org.bouncycastle.math.ec.ECPoint;
import org.bouncycastle.math.ec.FixedPointCombMultiplier;
import org.bouncycastle.math.ec.custom.sec.SecP256K1Curve;
import org.bouncycastle.util.Arrays;

import java.math.BigInteger;
import java.security.SecureRandom;

import static com.google.common.base.Preconditions.checkArgument;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
public class Secp256 {
    static final ECDomainParameters CURVE_PARAMS;

    static {
        X9ECParameters params = CustomNamedCurves.getByName("secp256k1");
        CURVE_PARAMS = new ECDomainParameters(params.getCurve(), params.getG(), params.getN(), params.getH());
    }

    static final BigInteger HALF_CURVE = CURVE_PARAMS.getN().shiftRight(1);

    public static ECDSASignature sign(byte[] privateKeyBytes, byte[] message) throws CryptoLibException {
        ECDSASigner signer = new ECDSASigner(new HMacDSAKCalculator(new SHA256Digest()));
        BigInteger privateKey = new BigInteger(1, privateKeyBytes);
        ECPrivateKeyParameters pKey = new ECPrivateKeyParameters(privateKey, CURVE_PARAMS);
        signer.init(true, pKey);
        var components = signer.generateSignature(message);
        var sig = new ECDSASignature(components[0], components[1]);
        return sig.toCanonical();
    }

    public static byte[] recoverableSign(ECDSASignature sig, byte[] privateKeyBytes, byte[] message) throws CryptoLibException {
        BigInteger privateKey = new BigInteger(1, privateKeyBytes);
        BigInteger publicKey = publicKeyFromPrivate(privateKey);
        int recId = -1;
        for (int i = 0; i < 4; i++) {
            BigInteger k = recoverFromSignature(i, sig, message);
            if (k != null && k.equals(publicKey)) {
                recId = i;
                break;
            }
        }
        if (recId == -1) {
            throw new CryptoLibException("Could not construct a recoverable key.");
        }
        int h = recId + 27;
        return sig.encode(h);
    }

    public static boolean verify(byte[] pubKey, byte[] signature, byte[] message) throws CryptoLibException {
        ECDSASigner signer = new ECDSASigner(new HMacDSAKCalculator(new SHA256Digest()));
        ECPoint p = CURVE_PARAMS.getCurve().decodePoint(pubKey);
        ECPublicKeyParameters pKey = new ECPublicKeyParameters(p, CURVE_PARAMS);
        signer.init(false, pKey);
        var sig = ECDSASignature.from(signature);
        return signer.verifySignature(message, sig.r, sig.s);
    }

    public static boolean validateSignatureValues(byte v, BigInteger r, BigInteger s) {
        if (r.signum() < 0 || s.signum() < 0) return false;
        return r.compareTo(CURVE_PARAMS.getN()) < 0 && s.compareTo(CURVE_PARAMS.getN()) < 0 && (v == 0 || v == 1);
    }

    /**
     * Returns public key from the given private key.
     *
     * @param privateKey the private key to derive the public key from
     * @return BigInteger encoded public key
     */
    public static BigInteger publicKeyFromPrivate(BigInteger privateKey) {
        ECPoint point = publicPointFromPrivate(privateKey);
        byte[] encoded = point.getEncoded(false);
        return new BigInteger(1, Arrays.copyOfRange(encoded, 1, encoded.length));
    }

    /**
     * Returns public key point from the given private key.
     *
     * @param privateKey the private key to derive the public key from
     * @return ECPoint public key
     */
    private static ECPoint publicPointFromPrivate(BigInteger privateKey) {
        if (privateKey.bitLength() > CURVE_PARAMS.getN().bitLength()) {
            privateKey = privateKey.mod(CURVE_PARAMS.getN());
        }
        return new FixedPointCombMultiplier().multiply(CURVE_PARAMS.getG(), privateKey);
    }

    public static BigInteger recoverFromSignature(int recId, ECDSASignature sig, byte @NonNull [] message) {
        checkArgument(recId >= 0, "recId must be positive");
        checkArgument(sig.r.signum() >= 0, "r must be positive");
        checkArgument(sig.s.signum() >= 0, "s must be positive");

        // 1.0 For j from 0 to h   (h == recId here and the loop is outside this function)
        //   1.1 Let x = r + jn
        BigInteger n = CURVE_PARAMS.getN(); // Curve order.
        BigInteger i = BigInteger.valueOf((long) recId / 2);
        BigInteger x = sig.r.add(i.multiply(n));
        //   1.2. Convert the integer x to an octet string X of length mlen using the conversion
        //        routine specified in Section 2.3.7, where mlen = ⌈(log2 p)/8⌉ or mlen = ⌈m/8⌉.
        //   1.3. Convert the octet string (16 set binary digits)||X to an elliptic curve point R
        //        using the conversion routine specified in Section 2.3.4. If this conversion
        //        routine outputs "invalid", then do another iteration of Step 1.
        //
        // More concisely, what these points mean is to use X as a compressed public key.
        BigInteger prime = SecP256K1Curve.q;
        if (x.compareTo(prime) >= 0) {
            // Cannot have point co-ordinates larger than this as everything takes place modulo Q.
            return null;
        }
        // Compressed keys require you to know an extra bit of data about the y-coord as there are
        // two possibilities. So it's encoded in the recId.
        ECPoint R;
        try {
            R = decompressKey(x, (recId & 1) == 1);
        } catch (Exception e) {
            log.error("Error decompressing key, cause={}", e.getMessage());
            return null;
        }
        //   1.4. If nR != point at infinity, then do another iteration of Step 1 (callers
        //        responsibility).
        if (!R.multiply(n).isInfinity()) {
            return null;
        }
        //   1.5. Compute e from M using Steps 2 and 3 of ECDSA signature verification.
        BigInteger e = new BigInteger(1, message);
        //   1.6. For k from 1 to 2 do the following.   (loop is outside this function via
        //        iterating recId)
        //   1.6.1. Compute a candidate public key as:
        //               Q = mi(r) * (sR - eG)
        //
        // Where mi(x) is the modular multiplicative inverse. We transform this into the following:
        //               Q = (mi(r) * s ** R) + (mi(r) * -e ** G)
        // Where -e is the modular additive inverse of e, that is z such that z + e = 0 (mod n).
        // In the above equation ** is point multiplication and + is point addition (the EC group
        // operator).
        //
        // We can find the additive inverse by subtracting e from zero then taking the mod. For
        // example the additive inverse of 3 modulo 11 is 8 because 3 + 8 mod 11 = 0, and
        // -3 mod 11 = 8.
        BigInteger eInv = BigInteger.ZERO.subtract(e).mod(n);
        BigInteger rInv = sig.r.modInverse(n);
        BigInteger srInv = rInv.multiply(sig.s).mod(n);
        BigInteger eInvrInv = rInv.multiply(eInv).mod(n);
        ECPoint q = ECAlgorithms.sumOfTwoMultiplies(CURVE_PARAMS.getG(), eInvrInv, R, srInv);

        byte[] qBytes = q.getEncoded(false);
        return new BigInteger(1, qBytes);
    }

    private static ECPoint decompressKey(BigInteger xBN, boolean yBit) {
        X9IntegerConverter x9 = new X9IntegerConverter();
        byte[] compEnc = x9.integerToBytes(xBN, 1 + x9.getByteLength(CURVE_PARAMS.getCurve()));
        compEnc[0] = (byte) (yBit ? 0x03 : 0x02);
        return CURVE_PARAMS.getCurve().decodePoint(compEnc);
    }

    public static KeyPair generateRandom() {
        var generator = new ECKeyPairGenerator();
        generator.init(new ECKeyGenerationParameters(CURVE_PARAMS, new SecureRandom()));
        var keyPair = generator.generateKeyPair();
        return new KeyPair(
            Bytes.asUnsignedByteArray(((ECPrivateKeyParameters) keyPair.getPrivate()).getD()),
            ((ECPublicKeyParameters) keyPair.getPublic()).getQ().getEncoded(false)
        );
    }

    record ECDSASignature(BigInteger r, BigInteger s) {
        static ECDSASignature from(byte[] signature) {
            var r = new BigInteger(1, Bytes.slice(signature, 0, 32));
            var s = new BigInteger(1, Bytes.slice(signature, 32, 64));
            return new ECDSASignature(r, s);
        }

        public boolean isCanonical() {
            return s.compareTo(HALF_CURVE) <= 0;
        }

        public ECDSASignature toCanonical() {
            if (!isCanonical()) {
                return new ECDSASignature(r, CURVE_PARAMS.getN().subtract(s));
            }
            return this;
        }

        /**
         * Returns 64-byte signature
         */
        public byte[] encode() {
            byte[] r = Bytes.leftPadBytes(Bytes.asUnsignedByteArray(this.r), 32);
            byte[] s = Bytes.leftPadBytes(Bytes.asUnsignedByteArray(this.s), 32);
            return Bytes.concat(r, s);
        }

        /**
         * Returns 65-byte signature, the last element is recId
         */
        public byte[] encode(int recId) {
            byte v = (byte) recId;
            return Bytes.concat(encode(), new byte[]{v});
        }
    }
}
