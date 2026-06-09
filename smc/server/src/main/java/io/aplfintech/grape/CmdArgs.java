package io.aplfintech.grape;


import picocli.CommandLine;

import java.nio.file.Paths;

@CommandLine.Command(sortOptions = false)
public class CmdArgs {
    @CommandLine.Option(names = {"--state-server-port", "-ssp"}, description = "Port of the state server where from fetch accounts, mappings, etc (Typically - a Grape1 peer's state grpc port)", defaultValue = "39399")
    Integer stateServerPort;
    @CommandLine.Option(names = {"--state-server-host", "-ssh"}, description = "Host of the state server where from fetch accounts, mappings, etc (Typically - a Grape1 peer's state grpc port)", defaultValue = "localhost")
    String stateServerHost;
    @CommandLine.Option(names = {"--port", "-p"}, description = "Port to listen for incoming transactions for this VM Server", defaultValue = "29299")
    int port;
    @CommandLine.Option(names = {"--log-dir", "-l"}, description = "Directory where VM and its server can write logs about tx execution and internal operations. Default is temp directory")
    String logDir = Paths.get(System.getProperty("java.io.tmpdir"), "l1vm").toAbsolutePath().toString();
    @CommandLine.Option(names = {"--disable-tracer"}, description = "Disable tracer for message execution. By Default tracer is enabled")
    boolean disableTracer = false;
    @CommandLine.Option(names = {"--disable-mdLogger"}, description = "Disable MD logger for message execution. By default MD logger is enabled")
    boolean disableMdLogger = false;
    @CommandLine.Option(names = {"--enable-estimator-mdLogger"}, description = "Enable MD logger for message estimation. By default logger is disabled")
    boolean enableEstimatorMdLog = false;
    @CommandLine.Option(names = {"-h", "-?", "-help", "--help"}, usageHelp = true, description = "display a help message")
    private boolean helpRequested = false;
}
