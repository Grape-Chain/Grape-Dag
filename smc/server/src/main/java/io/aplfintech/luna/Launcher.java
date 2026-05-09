package io.aplfintech.luna;

import org.slf4j.LoggerFactory;
import picocli.CommandLine;

import java.util.stream.Collectors;

public class Launcher {

    public static void main(String[] args) throws InterruptedException {
        System.out.println("Start gRPC VM Server");
        CmdArgs parsedArgs = new CmdArgs();
        CommandLine commandLine = new CommandLine(parsedArgs);
        CommandLine.ParseResult parseResult = commandLine.parseArgs(args);
        if (!parseResult.errors().isEmpty()) {
            throw new IllegalArgumentException("Bad cmd args input: " + parseResult.errors().stream()
                .map(Exception::getMessage).collect(Collectors.joining(", ")));
        }
        if (parseResult.isUsageHelpRequested()) {
            commandLine.usage(System.out);
            System.exit(0);
        }
        System.setProperty("LOG_DIR", parsedArgs.logDir);
        Config.init(parsedArgs.logDir, parsedArgs.disableTracer, parsedArgs.disableMdLogger, parsedArgs.enableEstimatorMdLog);
        var log = LoggerFactory.getLogger(Launcher.class);
        log.info("Log dir set to {}", parsedArgs.logDir);
        log.info("Use chain config: {}", Config.grapeChainConfig());
        log.info("Message loggers: tracer={}, mdLogger={}, estimator mdLogger={}",
            disabledToString(Config.isTracerDisabled()),
            disabledToString(Config.isMdLoggerDisabled()),
            Config.estimatorMdLogger());
        log.info("Server ports: state-server-port: {}, state-server-host: {}, vm-port: {}", parsedArgs.stateServerPort,
            parsedArgs.stateServerHost, parsedArgs.port);
        VmServer vmServer = new VmServer(parsedArgs.port, parsedArgs.stateServerHost, parsedArgs.stateServerPort);
        vmServer.start();

        vmServer.blockUntilShutdown();
        log.info("gRPC VM Server done");
    }

    private static String disabledToString(boolean isDisabled) {
        return isDisabled ? "DISABLED" : "ENABLED";
    }
}