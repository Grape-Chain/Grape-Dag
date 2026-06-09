package io.aplfintech.grape.vm.impl;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.config.ExecutionConfig;
import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.exception.ExecutionException;
import io.aplfintech.grape.exception.InsufficientFoundsException;
import io.aplfintech.grape.exception.IntrinsicGasException;
import io.aplfintech.grape.exception.NonceModificationException;
import io.aplfintech.grape.exception.SenderNoEOAException;
import io.aplfintech.grape.l1vm.Constants;
import io.aplfintech.grape.l1vm.VmResult;
import io.aplfintech.grape.l1vm.code.Codes;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.vm.MessageResult;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.contract.ContractRef;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.util.Arrays;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class StateTransition {
    private final Message message;
    private final Vm vm;
    private final ExecutionConfig executionConfig;
    private final VmStateAccess stateAccess;
    /**
     * Gas left after the state transition
     */
    private long gas;
    /**
     * The initial message gas limit
     */
    private final long initialGas;
    /**
     * Message gas price
     */
    private final long gasPrice;
    /**
     * Message value
     */
    private final BigInteger value;
    /**
     * Message input data
     */
    private final byte[] data;

    private final GasPrice priceMap;

    public StateTransition(Message message, Vm vm, VmStateAccess stateAccess, ExecutionConfig executionConfig) {
        this.message = message;
        this.vm = vm;
        this.stateAccess = stateAccess;
        this.executionConfig = executionConfig;
        this.gas = message.gasLimit().longValueExact();
        this.initialGas = message.gasLimit().longValueExact();
        this.gasPrice = message.gasPrice().longValueExact();
        this.value = message.amount();
        this.data = message.data();
        this.priceMap = executionConfig.chainConfig().gasPriceMap();
    }

    /**
     * Transitions the state by applying the message
     *
     * @return the contract execution result
     * @throws ExecutionException in case of <li>Insufficient funds for buying gas</li>
     *                            <li>Intrinsic gas too low</li> <li>Insufficient funds for transfer</li>
     */
    public MessageResult apply() throws ExecutionException {
        var startTime = System.nanoTime();
        var caller = asContractRef(message);
        var isContractCreation = message.to().isUndefined();
        //Checks the nonce consistency
        preCheck();
        //buys the requested (gasLimit) amount of gas
        buyGas();

        captureStart(message);
        var intrinsicGas = intrinsicGas(data, isContractCreation);
        if (gas < intrinsicGas) {
            // the transaction is specified to use less gas than required to start the invocation
            var msg = String.format("Intrinsic gas too low: have=%s want=%s", gas, intrinsicGas);
            throw new IntrinsicGasException(intrinsicGas, msg);
        }
        gas = Math.subtractExact(gas, intrinsicGas);
        if (message.amount().signum() > 0 && !vm.context().canTransfer(stateAccess, message.from(), value)) {
            throw new InsufficientFoundsException(String.format("Insufficient funds for transfer, from=%s value=%s",
                message.from().hexAddress(), value.toString()));
        }

        ContractResult result = executeContract(caller, isContractCreation);

        gas = result.gas();
        //refunds are capped to (gasUsed / gasConfig.MaxRefundQuotient)
        var refundGas = refundGas(executionConfig.chainConfig().gasConfig().getMaxRefundQuotient());
        long usedGas = gasUsed();
        log.trace("GAS: After contract execution: init gas={}, used gas={}, refunded gas={}, remaining gas={}", initialGas, usedGas, refundGas, gas);
        captureEnd(initialGas, gas, Math.multiplyExact(gas, gasPrice), startTime);
        if (result.hasError()) {
            log.error(">>> Message: Contract execution status={}", result.executionStatus().fullName());
        }
        if (executionConfig.isNoBaseFee()) {
            //skip fee payment when simulating call,
            //currently it's default behavior
            log.info("!!! Skip fee payment to the coinbase={}, because noBaseFee == true", vm.context().coinbase().hexAddress());
        } else {
            var fee = Math256.mul(usedGas, gasPrice);
            if (fee.signum() > 0) {
                log.info("Fee payment: transfer fee={} to coinbase={}", fee, vm.context().coinbase().hexAddress());
                stateAccess.addBalance(vm.context().coinbase(), fee);
            } else {
                log.info("!!! Skip Fee payment: transfer fee={} to coinbase={}", fee, vm.context().coinbase().hexAddress());
            }
        }

        var msgResult = new VmMessageResult(result, initialGas, refundGas, usedGas, gas);
        log.info(">>> Message applying result={}", msgResult);
        return msgResult;
    }

    private static ContractRef asContractRef(Message message) {
        return new ContractRef() {
            @Override
            public Address address() {
                return message.from();
            }

            @Override
            public BigInteger value() {
                return message.amount();
            }
        };
    }

    /**
     * Executes contract on the VM
     */
    private ContractResult executeContract(ContractRef caller, boolean isContractCreation) {
        ContractResult result;
        log.trace("GAS: Before contract execution gas={}", gas);
        try {
            /*====== Contract execution ======*/
            if (isContractCreation) {
                //use data as contract deployment code
                result = vm.create(caller, Codes.from(data), gas, value);
            } else {
                //increment the nonce for the next transaction
                stateAccess.setNonce(message.from(), stateAccess.getNonce(message.from()) + 1);
                //use data as input for contract call
                result = vm.runCall(caller, message.to(), data, gas, value);
            }
            /*================================*/
        } catch (Exception e) {
            result = VmResult.error(VmStatus.VM_INTERNAL_ERROR, 0, message.to(), e.getMessage());
        }
        return result;
    }


    /**
     * Returns the intrinsic gas for a given data and
     * based on the price for contract creation and data length
     */
    private long intrinsicGas(byte[] data, boolean isCreate) {
        long realGas = 0;
        log.debug("Start calculating of the intrinsic gas={}", realGas);
        if (isCreate) {
            int creationPrice = priceMap.lookForGasPrice("txCreation");//32000
            realGas = Math.addExact(realGas, creationPrice);
            log.debug("add tx creation cost={}, gas={}", creationPrice, realGas);
        } else {
            int creationPrice = priceMap.lookForGasPrice("tx");//21000
            realGas = Math.addExact(realGas, creationPrice);
            log.debug("add tx cost={}, gas={}", creationPrice, realGas);
        }
        long nz = 0;
        //calculate non-zero bytes
        for (var byt : data) {
            if (byt != 0) {
                nz++;
            }
        }
        if (nz > 0) {
            var nonZeroPrice = priceMap.lookForGasPrice("txDataNonZero");
            realGas = Math.addExact(realGas, Math.multiplyExact(nz, nonZeroPrice));
            log.debug("add nonZero bytes cost={}, nz number={}, gas={}", nonZeroPrice, nz, realGas);
        }
        //calculate zero bytes
        long z = data.length - nz;
        if (z > 0) {
            var zeroPrice = priceMap.lookForGasPrice("txDataZero");
            realGas = Math.addExact(realGas, Math.multiplyExact(z, zeroPrice));
            log.debug("add Zero bytes cost={}, z number={}, gas={}", zeroPrice, z, realGas);
        }
        log.info("GAS: The Intrinsic gas={}", realGas);
        return realGas;
    }

    /**
     * Checks the nonce consistency
     */
    private void preCheck() throws ExecutionException {
        if (!message.isFake()) {
            var stNonce = stateAccess.getNonce(message.from());
            var msgNonce = message.nonce();
            if (stNonce < msgNonce) {
                throw new NonceModificationException(String.format("Nonce too High, address=%s tx nonce=%d, state nonce=%d", message.from().hexAddress(), msgNonce, stNonce));
            } else if (stNonce > msgNonce) {
                throw new NonceModificationException(String.format("Nonce too Low, address=%s tx nonce=%d, state nonce=%d", message.from().hexAddress(), msgNonce, stNonce));
            }
            // Make sure the sender is an EOA
            log.debug("Make sure the sender is an EOA ...");
            var codeHash = stateAccess.getContractCodeHash(message.from());
            if (codeHash != null && !Arrays.equals(codeHash, Constants.KECCAK256_NULL)) {
                throw new SenderNoEOAException(String.format("Sender isn't EOA, address=%s codeHash=%s", message.from().hexAddress(), HexUtils.toHex(codeHash, true)));
            }
        }
    }

    /**
     * Buys the requested gas.
     * State is changed
     */
    private void buyGas() throws InsufficientFoundsException {
        var want = Math256.mul(gas, gasPrice);
        var have = stateAccess.getBalance(message.from());
        log.debug("Buy requested gas: gas={}, address={} have={} want={}", gas, message.from().hexAddress(), have, want);
        if (have.compareTo(want) < 0) {
            throw new InsufficientFoundsException(String.format("Insufficient funds for buying the gas, address=%s have=%s want=%s",
                message.from().hexAddress(), have, want));
        }
        stateAccess.subBalance(message.from(), want);
    }

    /**
     * Returns coins for remaining gas exchanged at the original rate.
     * State is changed.
     */
    private long refundGas(long refundQuotient) {
        log.debug("Apply refund counter capped to a refund quotient={}", refundQuotient);
        //Check state refund counter
        var refundGas = stateAccess.getRefundGas();
        var refund = Math.min(gasUsed() / refundQuotient, refundGas);
        log.debug("Refund counter={}, capped to={}", refundGas, refund);
        gas = Math.addExact(gas, refund);
        // Return coins for remaining gas, exchanged at the original rate
        var remaining = Math256.mul(gas, gasPrice);
        log.debug("Gas={}, refund remaining={} neutrino to address={}", gas, remaining, message.from().hexAddress());
        stateAccess.addBalance(message.from(), remaining);
        return refund;
    }

    /**
     * returns the amount of gas used up by the state transition
     */
    public long gasUsed() {
        return Math.subtractExact(initialGas, gas);
    }

    private void captureStart(Message message) {
        if (executionConfig.isDebugEnabled()) {
            executionConfig.tracer().notifyMessageStart(stateAccess, message);
        } else {
            log.trace("Message Capture Start, cfg.debug=false");
        }
    }

    private void captureEnd(long startGas, long gas, long remaining, long startTime) {
        if (executionConfig.isDebugEnabled()) {
            executionConfig.tracer().notifyMessageEnd(message, startGas, startGas - gas, remaining, System.nanoTime() - startTime);
        } else {
            log.trace("Message Capture End, cfg.debug=false");
        }
    }

}
