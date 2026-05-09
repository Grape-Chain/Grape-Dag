package io.aplfintech.luna.vm.utils;

import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.VmMessage;
import io.aplfintech.luna.vm.Message;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class MessageHelper {
    static String senderPublicKeyHex = "897938b42aa41961ff960ea2aafedfe11dbe1628a73506322ffd3819e601bee5";//32 bytes
    static byte[] senderPublicKey = HexUtils.fromHex(senderPublicKeyHex);

    public static Message createPublishMessage(long nonce, long amount, long fuelLimit, long fuelPrice, byte @NonNull [] contractCode) {
        return createMessage(new byte[0], nonce, amount, fuelLimit, fuelPrice, contractCode);
    }

    public static Message createCallMessage(byte[] to, long nonce, long amount, long fuelLimit, long fuelPrice, byte @NonNull [] contractCode) {
        return createMessage(to, nonce, amount, fuelLimit, fuelPrice, contractCode);
    }

    public static Message createMessage(byte[] to, long nonce, long amount, long fuelLimit, long fuelPrice, byte @NonNull [] contractCode) {
        VmAddress sender = VmAddress.from(CryptoConfig.crypto().recoverAddress(senderPublicKey));
        var recipient = Bytes.isAllZero(to) ? VmAddress.UNDEFINED_ADDRESS : VmAddress.from(to);
        return VmMessage.builder()
            .nonce(nonce)
            .from(sender)
            .to(recipient)
            .amount(BigInteger.valueOf(amount))
            .gasLimit(BigInteger.valueOf(fuelLimit))
            .gasPrice(BigInteger.valueOf(fuelPrice))
            .data(contractCode)
            .build();
    }
}
