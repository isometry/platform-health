# Platform Health Providers

Platform Health Providers are extensions to the platform-health server. They enable the server to report on the health and status of a variety of external systems. This extensibility allows the platform-health server to be a versatile tool for monitoring and maintaining the health of your entire platform.

## Interface

To create a new provider, there are a few requirements:

* **Implement the `provider.Service` interface**: The [`provider.Service`](provider.go) interface defines the methods that a provider must implement. This includes methods for checking the health and status of the external system that the provider is designed to interact with.

* **Register with the internal registry**: Providers must register themselves with the platform-health server's internal registry. This is done with a call to [`provider.Register`](registry.go) in an `init()` function. The `init()` function is automatically called when the program starts, registering the provider before the server begins handling requests.

* **Include via blank import**: To include the provider in the server, it must be imported using a blank import statement (i.e., `_ path/to/module`) in the [server command](../../cmd/phs).

By following these guidelines, you can extend the platform-health server to interact with any external system, making it a powerful tool for platform health monitoring.
