# Java gRPC Plugin Server for swcat

This is a simple, lightweight gRPC server that allows you to implement `swcat` 
plugins in Java.

## Project Structure

- `src/main/java/io/swcat/grpc/PluginServer.java`: The main entry point that starts the gRPC server.
- `src/main/java/io/swcat/grpc/PluginService.java`: The gRPC service implementation and plugin dispatcher.
- `src/main/java/io/swcat/grpc/Plugin.java`: The interface that individual plugins must implement.
- `src/main/java/io/swcat/grpc/plugins/HelloPlugin.java`: A simple example plugin.

## Building

To build the project and generate the fat JAR:

```bash
mvn clean package
```

The JAR will be located at `target/grpc-plugin-server-1.0-SNAPSHOT.jar`.

## Running

Start the server on the default port (50051):

```bash
java -jar target/grpc-plugin-server-1.0-SNAPSHOT.jar
```

To use a different port:

```bash
java -jar target/grpc-plugin-server-1.0-SNAPSHOT.jar 9090
```

## Configuring swcat

To use this server in `swcat`, add a `GRPCPlugin` to your `plugins.yml`:

```yaml
plugins:
  javaHello:
    kind: GRPCPlugin
    trigger: |-
      kind:Component
    spec:
      address: localhost:50051
      config:
        pluginName: hello
        name: world
```

### Dispatching Logic

The server dispatches incoming requests to a `Plugin` implementation based on the following priority:

1.  **Plugin Name Override**: If `spec.config` contains a `pluginName` key (as shown above), the server uses that value to find the matching implementation in its registry. This allows multiple plugin configurations to share the same Java class.
2.  **Plugin Instance Name**: If no `pluginName` is specified in the config, the server falls back to using the plugin instance name (the key used in `plugins.yml`, e.g., `javaHello`).

## Registering New Plugins

To add a new plugin implementation:

1.  Create a new class that implements `io.swcat.grpc.Plugin`.
2.  Open `io.swcat.grpc.PluginService.java`.
3.  Add your new plugin to the `implementations` map in the constructor:

```java
public PluginService() {
    // ...
    implementations.put("my-new-plugin", new MyNewPlugin());
}
```

4.  Rebuild the server with `mvn package`.
5.  Update your `plugins.yml` to use your new plugin.
