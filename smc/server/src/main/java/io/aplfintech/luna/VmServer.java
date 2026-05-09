package io.aplfintech.luna;

import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.VmMessage;
import io.aplfintech.luna.l1vm.opcode.OpTableFactory;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.tracers.LoggerConfig;
import io.aplfintech.luna.utils.TracerUtils;
import io.aplfintech.luna.vm.Executors;
import io.aplfintech.luna.vm.Message;
import io.aplfintech.luna.vm.MessageResult;
import io.aplfintech.luna.vm.Receipt;
import io.aplfintech.luna.vm.impl.ExecutorFactory;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import io.grpc.Server;
import io.grpc.ServerBuilder;
import io.grpc.stub.StreamObserver;
import lombok.AllArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;
import pb.SCVmServiceGrpc;
import pb.TxvX;
import pb.Vm;

import java.io.IOException;
import java.io.PrintWriter;
import java.nio.file.Paths;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static io.aplfintech.luna.lunach.utils.Util.unsignedBigIntFromBytes;

/**
 * Grape VM Server
 *
 * @author Andrii Boiarskyi
 */
@AllArgsConstructor
@Slf4j
public class VmServer extends SCVmServiceGrpc.SCVmServiceImplBase {
    private static final int DEFAULT_PORT = 29299;
    private final Server server;
    private final StateClient client;
    private final int port;

    public VmServer() {
        this(DEFAULT_PORT, StateClient.DEFAULT_HOST, StateClient.DEFAULT_PORT);
    }

    public VmServer(int port, String stateServerHost, int stateServerPort) {
        var serv = ServerBuilder.forPort(port);
        server = serv.addService(this).build();
        this.port = port;
        client = new StateClient(stateServerHost, stateServerPort);
    }

    public void start() {
        try {
            server.start();
        } catch (IOException e) {
            throw new IllegalStateException("VM gRPC Server may be already running, port " + port + " is busy", e);
        }
        log.info("VM gRPC Server started, listening on " + port + " for incoming transactions");
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            log.info("Shutting down VM gRPC Server since JVM is shutting down");
            try {
                this.stop();
            } catch (InterruptedException e) {
                log.error("VM gRPC Server shutdown interrupted", e);
                Thread.currentThread().interrupt();
            }
            log.error("VM gRPC Server shutdown completed");
        }));
    }

    public void stop() throws InterruptedException {
        server.shutdown().awaitTermination(30, TimeUnit.SECONDS);
    }

    public void blockUntilShutdown() throws InterruptedException {
        server.awaitTermination();
    }

    /**
     * Runs the smart-contract transaction
     */
    @Override
    public void runCall(Vm.WriteContractRequest request, StreamObserver<Vm.CallResponse> responseObserver) {
        Message message = mapToVmMessage(request.getTx());
        String txIdHex = getTxIdHex(request.getTx());
        log.info("*** RunCall: runs smc tx {}->{}", txIdHex, message);
        var startTime = System.nanoTime();
        AtomicReference<ExecutionConfig> executionConfig = new AtomicReference<>();
        try {
            var block = new PinTxContext(request.getHeader().getTxNumber(), request.getHeader().getTimestamp(),
                VmAddress.from(request.getHeader().getCoinbaseAccountAddress().getAddBytes().toByteArray()));
            String tracerFileName = logFullName(txIdHex + "-tx-trace.log");
            String loggerFileName = logFullName(txIdHex + "-tx-md.log");
            try (var tracerWriter = createMessageTraceWriter(tracerFileName);
                 var loggerWriter = createMessageMdLogWriter(loggerFileName)) {

                var execBuilder = prepareExecBuilder(block, loggerWriter);

                var executor = execBuilder.buildExecutionConfig(executionConfig::set)
                    .executor(new StateServerDei(client));

                var receipt = executor.executeMessage(message, block);
                if (Config.isTracerEnabled()) {
                    //Write the collected trace data to the writer
                    executionConfig.get().tracer().writeTrace(tracerWriter);
                }
                //Prepare response
                prepareResponse("runCall", receipt, responseObserver);
            }
        } catch (Exception e) {
            log.error("System error during RunCall: txId=" + txIdHex, e);
            prepareOnExceptionResponse(responseObserver, e);
        } finally {
            responseObserver.onCompleted();
            log.info("RunCall finished in {}ns, txId={}", System.nanoTime() - startTime, txIdHex);
        }
    }

    /**
     * Calls the contract method with parameters
     */
    @Override
    public void runCode(Vm.ReadContractRequest request, StreamObserver<Vm.CallResponse> responseObserver) {
        var startTime = System.nanoTime();
        var contractAddress = VmAddress.from(request.getContractAddress().getAddBytes().toByteArray());
        var sender = VmAddress.from(request.getSender());
        BlockContext block = new PinTxContext(request.getHeader().getTxNumber(), request.getHeader().getTimestamp(),
            VmAddress.from(request.getHeader().getCoinbaseAccountAddress().getAddBytes().toByteArray()));
        log.info("*** RunCode: calls readMethod, sender={}, contract={}, method={}, params={}",
            request.getSender(), contractAddress.hexAddress(), request.getMethodSignature(), request.getMethodParameters());
        AtomicReference<ExecutionConfig> executionConfig = new AtomicReference<>();
        try {
            byte[] input = HexUtils.parseHex(request.getMethodSignature() + request.getMethodParameters());
            var message = VmMessage.builder()
                .fake(true)//don't check nonce consistence
                .from(sender)
                .to(contractAddress)
                .gasLimit(block.gasLimit())
                //set contract data, init data for CREATE or arguments for CALL
                .data(input)
                .build();

            String commonFileName = contractAddress.hexAddress() + "-" + request.getMethodSignature() + "-" + TracerUtils.addDateTimePrefix("");
            String tracerFileName = logFullName(commonFileName + "-read-trace.log");
            String loggerFileName = logFullName(commonFileName + "-read-md.log");
            try (var tracerWriter = createMessageTraceWriter(tracerFileName);
                 var loggerWriter = createMessageMdLogWriter(loggerFileName)) {

                var execBuilder = prepareExecBuilder(block, loggerWriter);
                var applier = execBuilder.buildExecutionConfig(executionConfig::set)
                    .applier(new StateServerDei(client));

                var receipt = applier.executeMessage(message, block);
                if (Config.isTracerEnabled()) {
                    //Write the collected trace data to the writer
                    executionConfig.get().tracer().writeTrace(tracerWriter);
                }
                //Prepare response
                String hexOutput = HexUtils.toHex(receipt.result().output(), true);
                prepareResponse(
                    String.format("RunCode, contract=%s, method=%s, parameters=%s, sender=%s, result=%s",
                        contractAddress.hexAddress(), request.getMethodSignature(), request.getMethodParameters(), request.getSender(), hexOutput),
                    receipt,
                    responseObserver);
            }
        } catch (Exception e) {
            log.error("System error during call read method: contract={}, method={}, parameters={}, sender={}", contractAddress.hexAddress(), request.getMethodSignature(), request.getMethodParameters(), request.getSender(), e);
            prepareOnExceptionResponse(responseObserver, e);
        } finally {
            responseObserver.onCompleted();
            log.info("RunCode finished in {}ns, contract: {}, method: {}, params: {}, sender: {}", System.nanoTime() - startTime, contractAddress.hexAddress(), request.getMethodSignature(), request.getMethodParameters(), request.getSender());
        }
    }

    @Override
    public void estimateGas(Vm.EstimateGasRequest request, StreamObserver<Vm.CallResponse> responseObserver) {
        var dei = new StateServerDei(client);
        Message message;
        BlockContext block = new PinTxContext(request.getHeader().getTxNumber(), request.getHeader().getTimestamp(),
            VmAddress.from(request.getHeader().getCoinbaseAccountAddress().getAddBytes().toByteArray()));
        if (request.hasTx()) {
            var receivedTx = request.getTx();
            message = mapToVmMessage(receivedTx);
        } else {
            Vm.CallMessage callMessage = request.getMessage();
            message = VmMessage.builder()
                .from(mapPbAddressToVm(callMessage.getFrom()))
                .to(mapPbAddressToVm(callMessage.getTo()))
                .amount(unsignedBigIntFromBytes(callMessage.getAmount()))
                //set contract data, init data for CREATE or arguments for CALL
                .data(callMessage.getData().toByteArray())
                .build();
        }
        log.info("*** EstimateGas received {}, message={}", request.hasTx() ? "tx" : "msg", message);
        try (PrintWriter loggerWriter = createEstimatorMdLoggerWriter(message, request.hasTx())) {
            ChainConfig chainConfig = Config.chainConfigAt(block.timestamp());
            var vm = Executors.createEstimator(chainConfig, dei, loggerWriter);
            var receipt = vm.executeMessage(message, block);
            if (receipt.success()) {
                log.info("Estimated gas={}", receipt.result().usedGas());
            }
            prepareResponse("Estimate", receipt, responseObserver);
        } catch (Exception e) {
            prepareOnExceptionResponse(responseObserver, e);
        } finally {
            responseObserver.onCompleted();
        }
    }

    private static String getTxIdHex(TxvX.Txv1 tx) {
        var txId = CryptoConfig.crypto().sha256sum(tx.toByteArray());
        return HexUtils.toHex(txId, true);
    }

    private static void prepareResponse(String callInfo, Receipt receipt, StreamObserver<Vm.CallResponse> responseObserver) {
        if (receipt.success()) {
            prepareSolidityResultResponse(callInfo, receipt, responseObserver);
        } else {
            prepareGeneralErrorResponse(receipt, responseObserver);
        }
    }

    private static void prepareSolidityResultResponse(String callInfo, Receipt receipt, StreamObserver<Vm.CallResponse> responseObserver) {
        MessageResult result = receipt.result();
        String hexOutput = HexUtils.toHex(result.output(), true);
        if (result.isSuccess()) {
            responseObserver.onNext(Vm.CallResponse.newBuilder()
                .setError("0x")
                .setMsg(hexOutput)
                .setStatus(0)
                .setGasUsed(String.valueOf(result.usedGas()))
                .build());
            log.info("{} success with output: {}", callInfo, hexOutput);
        } else {
            int status = result.executionStatus().isInternalError() ? -2 : result.executionStatus().getErrorCode();
            String vmStatusName = result.executionStatus().fullName();
            long gasUsed = result.usedGas();
            responseObserver.onNext(Vm.CallResponse.newBuilder()
                .setError(vmStatusName)
                .setMsg(receipt.errorMessage() != null ? receipt.errorMessage() : hexOutput)
                .setGasUsed(String.valueOf(gasUsed))
                .setStatus(status)
                .build());
            log.info("{} failed with solidity error={}, encodedOutput={}", callInfo, vmStatusName, hexOutput);
        }
    }

    private static void prepareGeneralErrorResponse(Receipt receipt, StreamObserver<Vm.CallResponse> responseObserver) {
        // system unexpected error
        responseObserver.onNext(Vm.CallResponse.newBuilder()
            .setError(receipt.errorMessage())
            .setMsg("0x")
            .setStatus(-2)
            .setGasUsed(String.valueOf(0))
            .build());
        log.info("Execution failed with system error: {}", receipt.errorMessage());
    }

    private static void prepareOnExceptionResponse(StreamObserver<Vm.CallResponse> responseObserver, Exception e) {
        responseObserver.onNext(Vm.CallResponse.newBuilder()
            .setError(e.getMessage())
            .setMsg("0x")
            .setStatus(-2)
            // -2 - bad txs (don't put in DAG) / system error in code occurred (programming error)
            // -1 - expected VM general Error - VM error occurred by executing Solidity code/tx must be saved in the dag with error status
            // 0 - success;
            // Positive code - contract execution error on require/assert: tx must be saved in the dag with error status
            .build());
    }

    public static Address mapPbAddressToVm(Vm.Address address) {
        if (address == null || address.getAddBytes() == null || address.getAddBytes().toByteArray() == null || address.getAddBytes().toByteArray().length == 0) {
            return VmAddress.UNDEFINED_ADDRESS;
        } else {
            return VmAddress.from(address.getAddBytes().toByteArray());
        }
    }

    /**
     * Converts the protoBuf transaction object to the VM transaction
     *
     * @param txv1 protoBuf transaction
     * @return the VM transaction object converted from the protoBuf object
     */
    public static Message mapToVmMessage(TxvX.Txv1 txv1) {
        VmAddress sender = VmAddress.from(txv1.getSender().toByteArray());
        var recipient = Bytes.isAllZero(txv1.getRecepient().toByteArray()) ? VmAddress.UNDEFINED_ADDRESS : VmAddress.from(txv1.getRecepient().toByteArray());
        return VmMessage.builder()
            .nonce(txv1.getNonce())
            .from(sender)
            .to(recipient)
            .amount(unsignedBigIntFromBytes(txv1.getAmount()))
            .gasLimit(unsignedBigIntFromBytes(txv1.getFuelLimit()))
            .gasPrice(unsignedBigIntFromBytes(txv1.getFuelPrice()))
            //set contract data, init data for CREATE or arguments for CALL
            .data(txv1.getData().toByteArray())
            .build();
    }

    private static String logFullName(String file) {
        return Paths.get(Config.logDir(), file).toAbsolutePath().toString();
    }

    private static PrintWriter createMessageTraceWriter(String tracerFileName) {
        if (Config.isTracerDisabled()) {
            return null;
        }
        return TracerUtils.openWriterToAppend(tracerFileName);
    }

    private static PrintWriter createMessageMdLogWriter(String tracerFileName) {
        if (Config.isMdLoggerDisabled()) {
            return null;
        }
        return TracerUtils.openWriterToAppend(tracerFileName);
    }

    private static PrintWriter createEstimatorMdLoggerWriter(Message message, boolean isTx) {
        switch (Config.estimatorMdLogger()) {
            case FILE -> {
                String loggerExt = isTx ? "-tx-md.log" : "-msg-md.log";
                String loggerFileName;
                if (message.to().isUndefined()) {
                    loggerFileName = "create-" + message.from().hexAddress() + "-" + message.nonce();
                } else {
                    loggerFileName = "call-" + message.to().hexAddress() + "-" + getMethodSelector(message.data());
                }
                var fileName = logFullName("estimate-" + loggerFileName + "-" + TracerUtils.addDateTimePrefix(loggerExt));
                return TracerUtils.openWriterToAppend(fileName);
            }
            case CONSOLE -> {
                return TracerUtils.stdOutWriter();
            }
            default -> {
                return null;
            }
        }
    }

    private ExecutorFactory.ExecConfigBuilder prepareExecBuilder(BlockContext block, PrintWriter loggerWriter) {
        var chainConfig = Config.chainConfigAt(block.timestamp());
        CryptoLib crypto = CryptoConfig.crypto();
        var opTableFactory = OpTableFactory.newFactory(chainConfig, crypto);
        var execBuilder = ExecutorFactory.newFactory(chainConfig)
            .configuration().debugEnabled(true).opTable(opTableFactory.createTable()).cryptoLib(crypto).noBaseFee(false);
        if (Config.isTracerEnabled() || Config.isMdLoggerEnabled()) {
            var tracerCfg = execBuilder.tracers(LoggerConfig.defaultConfig());
            if (Config.isTracerEnabled()) {
                tracerCfg.addStructTracer();
            }
            if (Config.isMdLoggerEnabled() && loggerWriter != null) {
                tracerCfg.addMdLogger(loggerWriter);
            }
            tracerCfg.buildTracer();
        }
        return execBuilder;
    }

    private static String getMethodSelector(byte @NonNull [] data) {
        return HexUtils.toHex(Bytes.slice(data, 0, 4));
    }
}
