package io.swcat.grpc;

import io.grpc.Server;
import io.grpc.ServerBuilder;

import java.io.IOException;
import java.util.concurrent.TimeUnit;
import java.util.logging.Logger;

public class PluginServer {
    private static final Logger logger = Logger.getLogger(PluginServer.class.getName());

    private final int port;
    private final Server server;

    public PluginServer(int port) {
        this.port = port;
        this.server = ServerBuilder.forPort(port)
                .addService(new PluginService())
                .build();
    }

    public void start() throws IOException {
        server.start();
        logger.info("Server started, listening on " + port);
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.err.println("*** shutting down gRPC server since JVM is shutting down");
            try {
                PluginServer.this.stop();
            } catch (InterruptedException e) {
                e.printStackTrace(System.err);
            }
            System.err.println("*** server shut down");
        }));
    }

    public void stop() throws InterruptedException {
        if (server != null) {
            server.shutdown().awaitTermination(30, TimeUnit.SECONDS);
        }
    }

    private void blockUntilShutdown() throws InterruptedException {
        if (server != null) {
            server.awaitTermination();
        }
    }

    public static void main(String[] args) throws IOException, InterruptedException {
        int port = 50051;
        if (args.length > 0) {
            port = Integer.parseInt(args[0]);
        }
        PluginServer server = new PluginServer(port);
        server.start();
        server.blockUntilShutdown();
    }
}
