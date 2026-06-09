package io.aplfintech.grape.grapech;

import com.google.common.base.Preconditions;
import com.google.protobuf.ByteString;
import com.google.protobuf.Timestamp;
import io.aplfintech.grape.grap3.crypto.Crypto;
import io.aplfintech.grape.grap3.crypto.DSA;
import io.aplfintech.grape.grap3.crypto.Hashers;
import io.aplfintech.grape.grap3.crypto.wallet.Addresses;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;
import pb.TxvX;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class TxBuilderImpl implements TxBuilder {

    private static final String ENCODED_TX_FORMAT = "{ \"encodedTx\": \"%s\" }";
    /**
     * Transaction type
     */
    private Type type;

    /**
     * Chain id for which transaction is designed
     * <p>0 - MAIN_NET; 1- PUBLIC TEST_NET; etc
     */
    private byte chainId;

    /**
     * Sender public key,
     * it's a compressed sender’s account public key of the EC secp256k1 cryptography
     */
    private byte[] senderPublicKey;
    private byte[] senderAddress;

    /**
     * Recipient address
     */
    private byte[] recipient;

    /**
     * Amount of coin units (cents) being transferred,
     * must be >=0
     */
    private BigInteger amount;

    /**
     * Ordinal for the transactions sent by account, starting from 0 and incrementing for every next transaction by 1.
     * Transactions are getting confirmed one-by-one in nonce ASC order
     */
    private long nonce;

    /**
     * UTC milliseconds since UNIX epoch when tx is created
     */
    private final long timestamp;

    /**
     * Amount of fuel allowed by the user to be used for tx confirmation (static for PAYMENT and dynamic for CONTRACT)
     */
    private BigInteger fuelLimit;

    /**
     * Price for each unit of FuelLimit (in cents, minimal coin units)
     * Resulting max tx fee going to be: FuelLimit * FuelPrice (in cents)
     */
    private BigInteger fuelPrice;

    /**
     * user-defined information of the transaction
     */
    private byte[] data;

    TxBuilderImpl() {
        //set default values
        this.type = Type.PAYMENT;
        this.chainId = 2;//Private Testnet
        this.recipient = new byte[0];
        this.amount = BigInteger.ZERO;
        this.nonce = 0;
        this.timestamp = System.currentTimeMillis();
        this.fuelLimit = BigInteger.ZERO;
        this.fuelPrice = BigInteger.ZERO;
        this.data = new byte[0];
    }

    @Override
    public TxBuilderImpl type(@NonNull Type type) {
        this.type = type;
        if (Type.PAYMENT == type) {
            this.nonce = -1;
        } else if (Type.PUBLISH_CONTRACT == type) {
            this.recipient = new byte[0];
        }
        return this;
    }

    @Override
    public TxBuilderImpl chainId(int chainId) {
        this.chainId = (byte) (chainId & 0xff);
        return this;
    }

    @Override
    public TxBuilderImpl senderPublicKey(@NonNull String publicKeyHex) {
        return senderPublicKey(HexUtils.parseHex(publicKeyHex));
    }

    @Override
    public TxBuilderImpl senderPublicKey(byte @NonNull [] pk) {
        this.senderPublicKey = pk;
        this.senderAddress = Addresses.createAddress(pk);
        return this;
    }

    @Override
    public TxBuilderImpl recipient(@NonNull String recipientHex) {
        return recipient(HexUtils.parseHex(recipientHex));
    }

    @Override
    public TxBuilderImpl recipient(byte @NonNull [] recipient) {
        this.recipient = recipient;
        return this;
    }

    @Override
    public TxBuilderImpl nonce(long nonce) {
        Preconditions.checkArgument(nonce >= 0, "Nonce=%s must be positive");
        this.nonce = nonce;
        return this;
    }

    @Override
    public TxBuilderImpl amount(long amount) {
        return amount(BigInteger.valueOf(amount));
    }

    @Override
    public TxBuilderImpl amount(@NonNull BigInteger amount) {
        Preconditions.checkArgument(amount.signum() >= 0, "Amount=%s, is not positive value", amount.toString());
        this.amount = amount;
        return this;
    }

    @Override
    public TxBuilderImpl fuelLimit(long fuelLimit) {
        return fuelLimit(BigInteger.valueOf(fuelLimit));
    }

    @Override
    public TxBuilderImpl fuelLimit(@NonNull BigInteger fuelLimit) {
        Preconditions.checkArgument(fuelLimit.signum() >= 0, "FuelLimit=%s, is not positive value", fuelLimit.toString());
        this.fuelLimit = fuelLimit;
        return this;
    }

    @Override
    public TxBuilderImpl fuelPrice(long fuelPrice) {
        return fuelPrice(BigInteger.valueOf(fuelPrice));
    }

    @Override
    public TxBuilderImpl fuelPrice(@NonNull BigInteger fuelPrice) {
        Preconditions.checkArgument(fuelPrice.signum() >= 0, "FuelPrice=%s, is not positive value", fuelPrice.toString());
        this.fuelPrice = fuelPrice;
        return this;
    }

    @Override
    public TxBuilderImpl data(@NonNull String dataHex) {
        return data(HexUtils.parseHex(dataHex));
    }

    @Override
    public TxBuilderImpl data(byte @NonNull [] data) {
        this.data = data;
        return this;
    }

    /**
     * Returns the message signature as hex string
     *
     * @param privateKeyHex private key as hex string
     * @return the message signature as hex string
     */
    @Override
    public String sign(@NonNull String privateKeyHex) {
        return HexUtils.toHex(
                sign(HexUtils.parseHex(privateKeyHex)),
                true);
    }

    /**
     * Returns the message signature
     *
     * @param privateKey private key
     * @return the message signature
     */
    @Override
    public byte[] sign(byte @NonNull [] privateKey) {
        DSA dsa = Crypto.currentDSA();
        var msg = getPbTxBytes(null).toByteArray();
        var hash = Hashers.sha256().digest(msg);
        //sign
        return dsa.sign(privateKey, hash);
    }

    /**
     * Returns the signed transaction as hex string
     *
     * @param privateKeyHex the private key as hex string
     * @return the signed transaction as hex string
     */
    @Override
    public String build(@NonNull String privateKeyHex) {
        var privKey = HexUtils.parseHex(privateKeyHex);
        return HexUtils.toHex(build(privKey), true);
    }

    /**
     * Returns the signed transaction
     *
     * @param privateKey the private key
     * @return the signed transaction
     */
    @Override
    public byte[] build(byte @NonNull [] privateKey) {
        var signature = sign(privateKey);
        var tx = getPbTxBytes(signature);
        return tx.toByteArray();
    }

    /**
     * Returns the Raw (Unsigned) transaction
     *
     * @return the Raw (Unsigned) transaction
     */
    @Override
    public byte[] build() {
        return getPbTxBytes(null).toByteArray();
    }


    /**
     * Returns the Json object with signed transaction
     *
     * @param privateKeyHex the private key as hex string
     * @return the Json object with signed transaction
     */
    @Override
    public String buildJson(@NonNull String privateKeyHex) {
        return buildJson(HexUtils.parseHex(privateKeyHex));
    }

    /**
     * Returns the Json object with signed transaction
     *
     * @param privateKey the private key
     * @return the Json object with signed transaction
     */
    @Override
    public String buildJson(byte @NonNull [] privateKey) {
        var tx = build(privateKey);
        return String.format(ENCODED_TX_FORMAT, HexUtils.toHex(tx, true));
    }

    /**
     * Returns the Json object with Raw (Unsigned) transaction
     *
     * @return the Json object with Raw (Unsigned) transaction
     */
    @Override
    public String buildJson() {
        var tx = build();
        return String.format(ENCODED_TX_FORMAT, HexUtils.toHex(tx, true));
    }

    private TxvX.Txv1 getPbTxBytes(byte[] signature) {
        var builder = TxvX.Txv1.newBuilder()
                .setTxTypeValue(this.type.ordinal())
                .setChainTypeValue(this.chainId)
                .setNonce(this.nonce)
                .setTimestamp(Timestamp.newBuilder().setSeconds(this.timestamp / 1000)
                        .setNanos((int) ((this.timestamp % 1000) * 1000000)).build())
                .setSenderPubk(ByteString.copyFrom(this.senderPublicKey))
                .setSender(ByteString.copyFrom(this.senderAddress))
                .setRecepient(ByteString.copyFrom(this.recipient))
                .setAmount(ByteString.copyFrom(Bytes.asUnsignedByteArray(this.amount)))
                .setFuelLimit(ByteString.copyFrom(Bytes.asUnsignedByteArray(this.fuelLimit)))
                .setFuelPrice(ByteString.copyFrom(Bytes.asUnsignedByteArray(this.fuelPrice)))
                .setData(ByteString.copyFrom(this.data));
        if (signature != null) {
            builder.setSignature(ByteString.copyFrom(signature));
        }
        return builder.build();
    }
}
