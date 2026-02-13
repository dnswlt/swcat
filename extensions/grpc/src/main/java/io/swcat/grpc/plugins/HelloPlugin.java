package io.swcat.grpc.plugins;

import io.swcat.grpc.Plugin;
import swcat.plugin.v1.Plugin.ExecuteRequest;
import swcat.plugin.v1.Plugin.ExecuteResponse;
import com.google.protobuf.Value;

/**
 * A simple example plugin that adds a greeting annotation to the processed entity.
 * <p>
 * This plugin demonstrates:
 * 1. How to read configuration values from the request.
 * 2. How to return a success response with annotations.
 * 3. How to return an error response when validation fails.
 */
public class HelloPlugin implements Plugin {
    /**
     * Executes the greeting plugin logic.
     * <p>
     * Expects a 'name' field in the plugin configuration.
     *
     * @param request The execution request.
     * @return A response containing a personalized greeting or an error message.
     */
    @Override
    public ExecuteResponse execute(ExecuteRequest request) {
        // Check if the required 'name' field is present in the configuration
        if (!request.hasConfig() || !request.getConfig().containsFields("name")) {
            return ExecuteResponse.newBuilder()
                    .setSuccess(false)
                    .setError("Missing required configuration field: 'name'")
                    .build();
        }

        String name = request.getConfig().getFieldsOrThrow("name").getStringValue();
        String greeting = "hello, " + name;

        return ExecuteResponse.newBuilder()
                .setSuccess(true)
                .putAnnotations("swcat/plugin-test", Value.newBuilder().setStringValue(greeting).build())
                .build();
    }
}
