package io.swcat.grpc;

import io.grpc.stub.StreamObserver;
import io.swcat.grpc.plugins.HelloPlugin;
import io.swcat.grpc.plugins.maven.MavenArtifactExtractorPlugin;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import swcat.plugin.v1.Plugin.ExecuteRequest;
import swcat.plugin.v1.Plugin.ExecuteResponse;
import swcat.plugin.v1.PluginServiceGrpc;

import java.util.HashMap;
import java.util.Map;

public class PluginService extends PluginServiceGrpc.PluginServiceImplBase {
    private static final Logger logger = LoggerFactory.getLogger(PluginService.class);
    private final Map<String, Plugin> implementations = new HashMap<>();

    public PluginService() {
        // --- PLUGIN REGISTRATION START ---
        // Register your plugin implementations here.
        // The key used here is what users specify in plugins.yml (either as the 
        // plugin name or via the 'pluginName' config key).
        
        implementations.put("hello", new HelloPlugin());
        implementations.put("maven-extractor", new MavenArtifactExtractorPlugin());
        
        // Example: implementations.put("my-custom-plugin", new MyCustomPlugin());
        // --- PLUGIN REGISTRATION END ---
    }

    @Override
    public void execute(ExecuteRequest request, StreamObserver<ExecuteResponse> responseObserver) {
        String pluginName = request.getPluginName();

        // Check if a specific implementation is configured in the config struct
        if (request.hasConfig() && request.getConfig().containsFields("pluginName")) {
            pluginName = request.getConfig().getFieldsOrThrow("pluginName").getStringValue();
        }

        Plugin impl = implementations.get(pluginName);
        if (impl == null) {
            logger.warn("Invalid plugin requested: {}", pluginName);
            responseObserver.onNext(ExecuteResponse.newBuilder()
                    .setSuccess(false)
                    .setError("Unknown plugin " + pluginName)
                    .build());
        } else {
            try {
                logger.info("Executing plugin: {}", pluginName);
                ExecuteResponse response = impl.execute(request);
                responseObserver.onNext(response);
            } catch (Exception e) {
                logger.error("Plugin execution failed: {}", pluginName, e);
                responseObserver.onNext(ExecuteResponse.newBuilder()
                        .setSuccess(false)
                        .setError("Plugin execution failed: " + e.getMessage())
                        .build());
            }
        }
        responseObserver.onCompleted();
    }
}
