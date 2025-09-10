# Keruta Agent - Kotlin Version

This is the Kotlin version of the keruta-agent, converted from the original Go implementation.

## Project Structure

```
src/
├── main/kotlin/net/kigawa/keruta/
│   ├── Main.kt                 # Application entry point
│   ├── config/
│   │   └── Config.kt          # Configuration management
│   ├── logger/
│   │   └── Logger.kt          # Logging functionality
│   ├── commands/
│   │   └── Commands.kt        # CLI command handling
│   ├── api/
│   │   └── Client.kt          # API client for keruta server
│   └── utils/
│       └── FileUtils.kt       # File utility functions
└── test/kotlin/net/kigawa/keruta/
    ├── config/
    │   └── ConfigTest.kt
    └── utils/
        └── FileUtilsTest.kt
```

## Building and Running

### Build the project
```bash
./gradlew build
```

### Run tests
```bash
./gradlew test
```

### Create executable JAR
```bash
./gradlew shadowJar
```

### Run the application
```bash
java -jar build/libs/keruta-agent-1.0.0-all.jar
```

Or using Gradle:
```bash
./gradlew run
```

## Dependencies

- **Kotlin**: 1.9.10
- **Clikt**: Command line interface framework
- **Logback**: Logging framework
- **Jackson**: JSON/YAML processing
- **Ktor**: HTTP client
- **Coroutines**: Asynchronous programming
- **JUnit 5**: Testing framework
- **MockK**: Mocking framework

## Configuration

The application reads configuration from:
1. Environment variables (highest priority)
2. Configuration files: `config.yaml`, `~/.keruta/config.yaml`, `/etc/keruta/config.yaml`
3. Default values (lowest priority)

### Environment Variables

- `KERUTA_API_URL`: API server URL (required)
- `KERUTA_API_TOKEN`: API authentication token
- `KERUTA_TASK_ID`: Task ID (required for non-daemon mode)
- `KERUTA_SESSION_ID`: Session ID (for daemon mode)
- `KERUTA_WORKSPACE_ID`: Workspace ID
- `KERUTA_LOG_LEVEL`: Log level (DEBUG, INFO, WARN, ERROR)
- `KERUTA_POLL_INTERVAL`: Polling interval in seconds
- `KERUTA_DAEMON_PORT`: HTTP server port for daemon mode

## Usage

### Daemon Mode
```bash
java -jar keruta-agent.jar daemon
java -jar keruta-agent.jar daemon --port 8080
```

### With Environment Variables
```bash
export KERUTA_API_URL="http://api.keruta.example.com"
export KERUTA_TASK_ID="task-123"
java -jar keruta-agent.jar
```

## Differences from Go Version

1. **Dependency Injection**: Uses constructor injection instead of global variables
2. **Coroutines**: Asynchronous operations use Kotlin coroutines instead of goroutines
3. **Type Safety**: Leverages Kotlin's null safety and type system
4. **Testing**: Uses JUnit 5 and MockK for better testing experience
5. **Configuration**: Uses Jackson for YAML parsing instead of Viper
6. **CLI**: Uses Clikt instead of Cobra for command-line interface

## Migration Notes

The Kotlin version maintains functional compatibility with the Go version while taking advantage of Kotlin/JVM ecosystem and features. All environment variables and configuration options remain the same.