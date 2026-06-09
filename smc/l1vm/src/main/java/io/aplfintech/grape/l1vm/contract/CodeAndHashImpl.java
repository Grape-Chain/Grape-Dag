package io.aplfintech.grape.l1vm.contract;

import io.aplfintech.grape.model.Hash;
import io.aplfintech.grape.vm.contract.Code;
import io.aplfintech.grape.vm.contract.CodeAndHash;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.experimental.Delegate;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class CodeAndHashImpl implements CodeAndHash {
    @Delegate
    private final Code code;
    private byte[] hash;

    public CodeAndHashImpl(@NonNull Code code, @NonNull Hash codeHash) {
        this.code = code;
        this.hash = codeHash.bytes();
    }

    @Override
    public byte[] codeHash() {
        return hash;
    }

    @Override
    public String toString() {
        return "CodeAndHashImpl{" +
            "code=" + code +
            ", hash=" + HexUtils.toHex(hash) +
            '}';
    }
}
