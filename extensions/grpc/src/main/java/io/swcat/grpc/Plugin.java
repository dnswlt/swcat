package io.swcat.grpc;

import swcat.plugin.v1.Plugin.ExecuteRequest;
import swcat.plugin.v1.Plugin.ExecuteResponse;

/**
 * Interface for swcat plugin implementations in Java.
 * <p>
 * To implement a new plugin, create a class that implements this interface
 * and register it in {@link io.swcat.grpc.PluginService}.
 */
public interface Plugin {
    /**
     * Executes the plugin logic for a given request.
     *
     * @param request The execution request containing the entity and configuration.
     * @return The execution response containing annotations and generated files.
     */
    ExecuteResponse execute(ExecuteRequest request);
}
