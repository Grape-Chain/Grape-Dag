package io.aplfintech.luna.grap3.crypto.wallet;

import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.ToString;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Getter
@ToString
@EqualsAndHashCode
public class Wallet {
    private String address;
    private String privateKey;
    private String publicKey;

    public Wallet(String address, String privateKey, String publicKey) {
        this.address = address;
        this.privateKey = privateKey;
        this.publicKey = publicKey;
    }
}
