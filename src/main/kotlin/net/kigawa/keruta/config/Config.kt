package net.kigawa.keruta.config

import com.fasterxml.jackson.databind.ObjectMapper
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory
import com.fasterxml.jackson.module.kotlin.KotlinModule
import com.fasterxml.jackson.module.kotlin.readValue
import java.io.File
import java.time.Duration
import kotlin.system.exitProcess

data class AppConfig(
    val api: ApiConfig = ApiConfig(),
    val logging: LoggingConfig = LoggingConfig(),
    val artifacts: ArtifactsConfig = ArtifactsConfig(),
    val errorHandling: ErrorHandlingConfig = ErrorHandlingConfig()
)

data class ApiConfig(
    val url: String = "",
    val token: String = "",
    val timeout: Duration = Duration.ofSeconds(30)
)

data class LoggingConfig(
    val level: String = "INFO",
    val format: String = "json"
)

data class ArtifactsConfig(
    val maxSize: Long = 100 * 1024 * 1024, // 100MB
    val directory: String = "/.keruta/doc",
    val extensions: String = ""
)

data class ErrorHandlingConfig(
    val autoFix: Boolean = true,
    val retryCount: Int = 3
)

object Config {
    private var globalConfig: AppConfig = AppConfig()
    private val objectMapper = ObjectMapper(YAMLFactory()).registerModule(KotlinModule.Builder().build())

    fun init() {
        setDefaults()
        loadFromEnv()
        loadFromFile()
        validate()
    }

    private fun setDefaults() {
        globalConfig = AppConfig()
    }

    private fun loadFromEnv() {
        val apiUrl = System.getenv("KERUTA_API_URL") ?: globalConfig.api.url
        val apiToken = System.getenv("KERUTA_API_TOKEN") ?: globalConfig.api.token
        val timeout = System.getenv("KERUTA_TIMEOUT")?.let { Duration.parse("PT$it") } ?: globalConfig.api.timeout

        val logLevel = System.getenv("KERUTA_LOG_LEVEL") ?: globalConfig.logging.level

        val artifactsDir = System.getenv("KERUTA_ARTIFACTS_DIR") ?: globalConfig.artifacts.directory
        val maxSize = System.getenv("KERUTA_MAX_FILE_SIZE")?.toLongOrNull()?.let { it * 1024 * 1024 } ?: globalConfig.artifacts.maxSize

        val autoFix = System.getenv("KERUTA_AUTO_FIX_ENABLED")?.toBoolean() ?: globalConfig.errorHandling.autoFix
        val retryCount = System.getenv("KERUTA_RETRY_COUNT")?.toIntOrNull() ?: globalConfig.errorHandling.retryCount

        globalConfig = globalConfig.copy(
            api = globalConfig.api.copy(url = apiUrl, token = apiToken, timeout = timeout),
            logging = globalConfig.logging.copy(level = logLevel),
            artifacts = globalConfig.artifacts.copy(directory = artifactsDir, maxSize = maxSize),
            errorHandling = globalConfig.errorHandling.copy(autoFix = autoFix, retryCount = retryCount)
        )
    }

    private fun loadFromFile() {
        val configPaths = listOf(
            "config.yaml",
            "${System.getProperty("user.home")}/.keruta/config.yaml",
            "/etc/keruta/config.yaml"
        )

        for (path in configPaths) {
            val file = File(path)
            if (file.exists()) {
                try {
                    val fileConfig: AppConfig = objectMapper.readValue(file)
                    globalConfig = mergeConfigs(globalConfig, fileConfig)
                    break
                } catch (e: Exception) {
                    // ファイルが読み込めない場合は無視
                }
            }
        }
    }

    private fun mergeConfigs(envConfig: AppConfig, fileConfig: AppConfig): AppConfig {
        return AppConfig(
            api = ApiConfig(
                url = if (envConfig.api.url.isNotEmpty()) envConfig.api.url else fileConfig.api.url,
                token = if (envConfig.api.token.isNotEmpty()) envConfig.api.token else fileConfig.api.token,
                timeout = envConfig.api.timeout
            ),
            logging = LoggingConfig(
                level = envConfig.logging.level,
                format = if (envConfig.logging.format != "json") envConfig.logging.format else fileConfig.logging.format
            ),
            artifacts = ArtifactsConfig(
                maxSize = envConfig.artifacts.maxSize,
                directory = if (envConfig.artifacts.directory != "/.keruta/doc") envConfig.artifacts.directory else fileConfig.artifacts.directory,
                extensions = if (envConfig.artifacts.extensions.isNotEmpty()) envConfig.artifacts.extensions else fileConfig.artifacts.extensions
            ),
            errorHandling = ErrorHandlingConfig(
                autoFix = envConfig.errorHandling.autoFix,
                retryCount = envConfig.errorHandling.retryCount
            )
        )
    }

    private fun validate() {
        if (globalConfig.api.url.isEmpty()) {
            System.err.println("KERUTA_API_URL が設定されていません")
            exitProcess(1)
        }

        val isDaemonMode = System.getProperty("sun.java.command")?.contains("daemon") == true

        if (getTaskId().isEmpty()) {
            if (!isDaemonMode) {
                System.err.println("KERUTA_TASK_ID が設定されていません")
                exitProcess(1)
            }

            val sessionId = getSessionId()
            val workspaceId = getWorkspaceId()
            val coderWorkspaceId = System.getenv("CODER_WORKSPACE_ID")
            val coderWorkspaceName = System.getenv("CODER_WORKSPACE_NAME")

            val hasSessionId = sessionId.isNotEmpty()
            val hasWorkspaceId = workspaceId.isNotEmpty() || coderWorkspaceId?.isNotEmpty() == true || coderWorkspaceName?.isNotEmpty() == true

            if (!hasSessionId && !hasWorkspaceId) {
                if (globalConfig.api.url.isEmpty()) {
                    System.err.println("デーモンモードでは KERUTA_SESSION_ID、KERUTA_WORKSPACE_ID、CODER_WORKSPACE_ID、CODER_WORKSPACE_NAME のいずれか、またはワークスペース名から自動取得するためのKERUTA_API_URLが必要です")
                    exitProcess(1)
                }
            }
        }
    }

    fun getTaskId(): String = System.getenv("KERUTA_TASK_ID") ?: ""

    fun getApiUrl(): String = globalConfig.api.url

    fun getTimeout(): Duration = globalConfig.api.timeout

    fun getApiToken(): String = globalConfig.api.token

    fun getSessionId(): String {
        val sessionId = System.getenv("KERUTA_SESSION_ID")
        if (sessionId?.isNotEmpty() == true) {
            return sessionId
        }

        val workspaceId = getWorkspaceId()
        if (workspaceId.isNotEmpty()) {
            val sessionFromWorkspace = getSessionIdFromWorkspace(workspaceId)
            if (sessionFromWorkspace.isNotEmpty()) {
                return sessionFromWorkspace
            }
        }

        return ""
    }

    private fun getSessionIdFromWorkspace(workspaceId: String): String {
        // HTTP client implementation would be needed here
        // For now, return empty string
        return ""
    }

    fun getWorkspaceId(): String {
        return System.getenv("KERUTA_WORKSPACE_ID") ?: System.getenv("CODER_WORKSPACE_ID") ?: ""
    }

    fun getCoderWorkspaceName(): String = System.getenv("CODER_WORKSPACE_NAME") ?: ""

    fun getPollInterval(): Duration {
        val interval = System.getenv("KERUTA_POLL_INTERVAL")
        return if (interval?.isNotEmpty() == true) {
            try {
                Duration.ofSeconds(interval.toLong())
            } catch (e: NumberFormatException) {
                Duration.ofSeconds(5)
            }
        } else {
            Duration.ofSeconds(5)
        }
    }

    fun getUseHttpInput(): Boolean = System.getenv("KERUTA_USE_HTTP_INPUT") == "true"

    fun getDaemonPort(): String = System.getenv("KERUTA_DAEMON_PORT") ?: "8080"
}
