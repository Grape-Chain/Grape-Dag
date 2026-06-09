package io.aplfintech.grape;

import com.google.protobuf.ByteString;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.model.Account;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.utils.HexUtils;
import io.grpc.ManagedChannelBuilder;
import io.grpc.StatusRuntimeException;
import lombok.extern.slf4j.Slf4j;
import pb.NodeStorageServiceGrpc;
import pb.Vm;

import java.math.BigInteger;
import java.util.Arrays;
import java.util.Optional;
import java.util.stream.Collectors;

@Slf4j
public class StateClient {

    static final String DEFAULT_HOST = "localhost";
    static final int DEFAULT_PORT = 39399;
    private final NodeStorageServiceGrpc.NodeStorageServiceBlockingStub grpcSender;

    public StateClient() {
        this(DEFAULT_HOST, DEFAULT_PORT);
    }

    public StateClient(String host, int port) {
        var chan = ManagedChannelBuilder.forAddress(host, port).usePlaintext().build();
        grpcSender = NodeStorageServiceGrpc.newBlockingStub(chan);
    }

    public Optional<Account> getAccount(byte[] address) {
        try {
            var account = grpcSender.getAccount(Vm.Address.newBuilder()
                .setAddBytes(
                    ByteString.copyFrom(address))
                .build()
            );
            return
                Optional.of(
                    new LnAccount(account.getAddress().getAddBytes().toByteArray(), account.getBalance().isBlank() ? BigInteger.ZERO : new BigInteger(account.getBalance()),
                        HexUtils.parseHex(account.getCodeHash()), account.getNonce()));
        } catch (StatusRuntimeException e) {
            log.debug("Got error when fetching account", e);
            log.warn("Account wasn't found, address={}", HexUtils.toHex(address, true));
            return Optional.empty();
        }
    }

    public void createAccount(byte[] address) {
        var result = grpcSender.createAccount(Vm.Address.newBuilder()
            .setAddBytes(
                ByteString.copyFrom(address))
            .build()
        );
        if (result.getStatus() != 0) {
            log.error("FATAL! Account creation failed, address={} reason={}, code={}"
                , HexUtils.toHex(address, true), result.getMessage(), result.getStatus());
        } else {
            log.info("Account={} created with message={}", HexUtils.toHex(address, true), result.getMessage());
        }
    }


    public void addBalance(byte[] address, BigInteger amount) {
        var result = grpcSender.addBalance(
            Vm.AddBalanceRequest.newBuilder()
                .setAccountAddress(
                    Vm.Address.newBuilder()
                        .setAddBytes(
                            ByteString.copyFrom(address))
                )
                .setAmount(amount.toString())
                .build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Balance add operation failed, address={}, amount={}, reason={}, code={}"
                , HexUtils.toHex(address, true), amount, result.getMessage(), result.getStatus());
        } else {
            log.info("Account={} amount={} added with message={}", HexUtils.toHex(address, true), amount, result.getMessage());
        }
    }

    public void subBalance(byte[] address, BigInteger amount) {
        var result = grpcSender.subBalance(
            Vm.SubBalanceRequest.newBuilder()
                .setAccountAddress(
                    Vm.Address.newBuilder()
                        .setAddBytes(
                            ByteString.copyFrom(address))
                )
                .setAmount(amount.toString())
                .build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Balance sub operation failed, address={}, amount={}, reason={}, code={}"
                , HexUtils.toHex(address, true), amount, result.getMessage(), result.getStatus());
        } else {
            log.info("Account={} amount={} subtracted with message={}", HexUtils.toHex(address, true), amount, result.getMessage());
        }
    }

    public void setNonceForContract(byte[] address, long nonce) {
        String hexAddress = HexUtils.toHex(address, true);
        try {
            var result = grpcSender.setNonce(Vm.SetNonceRequest.newBuilder()
                .setNonce(nonce)
                .setContractAddress(Vm.Address.newBuilder()
                    .setAddBytes(ByteString.copyFrom(address)).build())
                .build());

            if (result.getStatus() != 0) {
                log.error("Unable to set nonce={} for account={}, errorCode={}, message={}", nonce, hexAddress,
                    result.getStatus(), result.getMessage());
            } else {
                log.info("Nonce={} has been successfully set for account={}", nonce, hexAddress);
            }
        } catch (StatusRuntimeException e) {
            log.warn("Unable to set nonce={} for account={}, reason={}", nonce, hexAddress, e.getMessage());
        }
    }

    public void saveLogs(Log[] logs) {
        if (logs == null || logs.length == 0) {
            return;
        }
        String contracts = Arrays.stream(logs).map(e -> e.getAddress().hexAddress()).distinct().collect(Collectors.joining(","));
        try {
            Vm.SaveLogsRequest.Builder reqBuilder = createSaveLogsRequest(logs);
            var result = grpcSender.saveLogs(reqBuilder.build());
            if (result.getStatus() != 0) {
                log.error("Unable to save logs for contracts={}, errorCode={}, message={}, logs={}", contracts,
                    result.getStatus(), result.getMessage(), Arrays.toString(logs));
            } else {
                log.info("Logs (quantity={}) have been successfully saved for contracts={}", logs.length, contracts);
            }
        } catch (StatusRuntimeException e) {
            log.warn("Unable to save logs for contracts={}, reason={}", contracts, e.getMessage());
        }
    }

    private static Vm.SaveLogsRequest.Builder createSaveLogsRequest(Log[] logs) {
        var reqBuilder = Vm.SaveLogsRequest.newBuilder();
        for (Log l : logs) {
            reqBuilder
                .addLogs(Vm.Log.newBuilder()
                    .setBlock(l.getBlockNumber().longValueExact())
                    .setContractAddress(Vm.Address.newBuilder().setAddBytes(ByteString.copyFrom(l.getAddress().bytes())).build())
                    .setData(ByteString.copyFrom(l.getData()))
                    .addAllTopics(Arrays.stream(l.getTopics()).map(e ->
                            Vm.Topic.newBuilder()
                                .setHash(
                                    ByteString.copyFrom(e.bytes()))
                                .build())
                        .collect(Collectors.toList()))
                    .build());
        }
        return reqBuilder;
    }


    public void putContractCode(byte[] address, byte[] code) {
        var result = grpcSender.putContractCode(Vm.PutContractCodeRequest
            .newBuilder()
            .setContractCode(Vm.ContractCode.newBuilder()
                .setBytecode(ByteString.copyFrom(code))
                .build())
            .setContractAddress(Vm.Address.newBuilder()
                .setAddBytes(ByteString.copyFrom(address))
                .build())
            .build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Unable to store contract code, address={}, code={}, reason={}, code={}"
                , HexUtils.toHex(address, true), HexUtils.toHex(code, true), result.getMessage(), result.getStatus());
        } else {
            log.info("Contract={} code.length={} saved with message={}", HexUtils.toHex(address, true), code.length, result.getMessage());
            log.trace("Contract={} code={} saved with message={}", HexUtils.toHex(address, true), HexUtils.toHex(code, true), result.getMessage());
        }
    }

    public byte[] getContractCode(byte[] address) {
        try {
            var result = grpcSender.getContractCode(
                Vm.Address.newBuilder()
                    .setAddBytes(ByteString.copyFrom(address))
                    .build());
            log.info("Retrieved contract code by address={}, code.length={}",
                HexUtils.toHex(address, true), result.getBytecode().toByteArray().length);
            log.trace("Retrieved contract code by address={}, code={}",
                HexUtils.toHex(address, true), HexUtils.toHex(result.getBytecode().toByteArray()));
            return result.getBytecode().toByteArray();
        } catch (StatusRuntimeException e) {
            log.warn("Unable to retrieve contract's code, address={}, reason: {}", HexUtils.toHex(address, true), e.getMessage());
            return new byte[0];
        }
    }

    public byte[] getContractStorage(byte[] address, byte[] key) {
        var result = grpcSender.getContractCodeStorage(
            Vm.GetFromStorageByKeyRequest.newBuilder()
                .setContractAddress(Vm.Address.newBuilder()
                    .setAddBytes(ByteString.copyFrom(address))
                    .build())
                .setKeyToQuery(Vm.Key.newBuilder()
                    .setContent(ByteString.copyFrom(key)))
                .build());

        log.info("Retrieved contract storage by address={} and key={}, value={}",
            HexUtils.toHex(address, true),
            HexUtils.toHex(key, true),
            HexUtils.toHex((result.getContent().toByteArray()), true));
        return result.getContent().toByteArray();
    }

    public byte[] getCommittedContractStorage(byte[] address, byte[] key) {
        //TODO grpc endpoint not yet implemented
        return getContractStorage(address, key);
    }

    public void putContractStorage(byte[] address, byte[] key, byte[] value) {
        var result = grpcSender.putContractStorage(
            Vm.PutIntoStorageRequest.newBuilder()
                .setContractAddress(Vm.Address.newBuilder()
                    .setAddBytes(ByteString.copyFrom(address))
                    .build())
                .setKeyToPut(Vm.Key.newBuilder()
                    .setContent(ByteString.copyFrom(key)))
                .setValueToPut(Vm.Value.newBuilder()
                    .setContent(ByteString.copyFrom(value))
                    .build())
                .build());

        log.info("Put contract storage by address={} and key={}, value={}",
            HexUtils.toHex(address, true), HexUtils.toHex(key, true), HexUtils.toHex(value, true));

        if (result.getStatus() != 0) {
            log.error("FATAL! Unable to store contract value by key, address={}, reason={}, code={}"
                , HexUtils.toHex(address, true), result.getMessage(), result.getStatus());
        } else {
            log.info("Contract={} key={}, value={} saved with message={}", HexUtils.toHex(address, true),
                HexUtils.toHex(key, true), HexUtils.toHex(value, true), result.getMessage());
        }
    }

    public void checkpoint() {
        log.info("Calling state checkpoint");
        var result = grpcSender.stateCheckpoint(Vm.TransactionalRequest.newBuilder().build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Unable to checkpoint state: {}", result.getMessage());
        } else {
            log.info("State checkpoint is successful");
        }
    }

    public void commit() {
        log.info("Calling commit state");
        var result = grpcSender.stateCommit(Vm.TransactionalRequest.newBuilder().build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Unable to commit state: {}", result.getMessage());
        } else {
            log.info("State committed");
        }
    }

    public void revert() {
        log.info("Calling revert state");
        var result = grpcSender.stateRevert(Vm.TransactionalRequest.newBuilder().build());
        if (result.getStatus() != 0) {
            log.error("FATAL! Unable to revert state: {}", result.getMessage());
        } else {
            log.info("State reverted");
        }
    }

    public BigInteger getChainId() {
        //TODO grpc endpoint not yet implemented
        return BigInteger.TWO;
    }

    private static final class LnAccount implements Account {
        private final BigInteger balance;
        private final byte[] storageRoot = new byte[0];

        private final long nonce;
        private final Address address;
        private final byte[] codeHash;

        private LnAccount(byte[] address, BigInteger balance, byte[] codeHash, long nonce) {
            this.address = VmAddress.from(address);
            this.balance = balance;
            this.codeHash = codeHash;
            this.nonce = nonce;
        }

        @Override
        public BigInteger balance() {
            return balance;
        }

        @Override
        public void addBalance(BigInteger value) {
            throw new UnsupportedOperationException("Operation denied. Use StateClient.addBalance instead");
        }

        @Override
        public void subBalance(BigInteger amount) {
            throw new UnsupportedOperationException("Operation denied. Use StateClient.subBalance instead");
        }

        @Override
        public long nonce() {
            return nonce;
        }

        @Override
        public void setNonce(long nonce) {
            throw new UnsupportedOperationException("Operation denied. Use StateClient.setNonceForContract instead");
        }

        @Override
        public byte[] storageRoot() {
            return storageRoot;
        }

        @Override
        public byte[] codeHash() {
            return codeHash;
        }

        @Override
        public Address address() {
            return address;
        }
    }

}
