package net.kigawa.keruta.api

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.cio.CIO
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.bearerAuth
import io.ktor.client.request.forms.formData
import io.ktor.client.request.forms.submitFormWithBinaryData
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.Headers
import io.ktor.http.HttpHeaders
import io.ktor.http.contentType
import io.ktor.http.isSuccess
import io.ktor.serialization.jackson.jackson
import net.kigawa.keruta.config.Config
import net.kigawa.keruta.logger.Logger
import org.slf4j.LoggerFactory
import java.io.File

private val logger = LoggerFactory.getLogger("ApiClient")

data class TaskStatusRequest(
    val status: String,
    val message: String? = null
)

data class LogRequest(
    val level: String,
    val message: String,
    val timestamp: Long = System.currentTimeMillis()
)

class ApiClient {
    private val httpClient = HttpClient(CIO) {
        install(ContentNegotiation) {
            jackson()
        }
    }

    private val baseUrl = Config.getApiUrl()
    private val token = Config.getApiToken()

    suspend fun updateTaskStatus(taskId: String, status: String, message: String? = null) {
        try {
            val url = "$baseUrl/api/v1/tasks/$taskId/status"
            val request = TaskStatusRequest(status, message)

            val response = httpClient.put(url) {
                contentType(ContentType.Application.Json)
                if (token.isNotEmpty()) {
                    bearerAuth(token)
                }
                setBody(request)
            }

            logApiCall("PUT", url, response.status.value, response.status.description)

            if (!response.status.isSuccess()) {
                val body = response.body<String>()
                throw Exception("API呼び出しが失敗しました: ${response.status.value} - $body")
            }

            Logger.withTaskIdAndComponent("api").info("タスクステータスを更新しました: $status")
        } catch (e: Exception) {
            Logger.withTaskIdAndComponent("api").error("タスクステータスの更新に失敗しました", e)
            throw e
        }
    }

    suspend fun sendLog(taskId: String, level: String, message: String) {
        try {
            val url = "$baseUrl/api/v1/tasks/$taskId/logs"
            val request = LogRequest(level, message)

            val response = httpClient.post(url) {
                contentType(ContentType.Application.Json)
                if (token.isNotEmpty()) {
                    bearerAuth(token)
                }
                setBody(request)
            }

            logApiCall("POST", url, response.status.value, response.status.description)

            if (!response.status.isSuccess()) {
                val body = response.body<String>()
                throw Exception("API呼び出しが失敗しました: ${response.status.value} - $body")
            }
        } catch (e: Exception) {
            // ログ送信のエラーは無視（無限ループを防ぐため）
        }
    }

    suspend fun uploadArtifact(taskId: String, filePath: String, description: String = "") {
        try {
            val url = "$baseUrl/api/v1/tasks/$taskId/artifacts"
            val file = File(filePath)

            if (!file.exists()) {
                throw Exception("ファイルが存在しません: $filePath")
            }

            val response = httpClient.submitFormWithBinaryData(
                url = url,
                formData = formData {
                    append(
                        "file",
                        file.readBytes(),
                        Headers.build {
                            append(HttpHeaders.ContentType, "application/octet-stream")
                            append(HttpHeaders.ContentDisposition, "filename=\"${file.name}\"")
                        }
                    )
                    if (description.isNotEmpty()) {
                        append("description", description)
                    }
                }
            ) {
                if (token.isNotEmpty()) {
                    bearerAuth(token)
                }
            }

            logApiCall("POST", url, response.status.value, response.status.description)

            if (!response.status.isSuccess()) {
                val body = response.body<String>()
                throw Exception("API呼び出しが失敗しました: ${response.status.value} - $body")
            }

            Logger.withTaskIdAndComponent("api").info("成果物をアップロードしました: $filePath")
        } catch (e: Exception) {
            Logger.withTaskIdAndComponent("api").error("成果物のアップロードに失敗しました", e)
            throw e
        }
    }

    private fun logApiCall(method: String, url: String, statusCode: Int, statusDescription: String) {
        Logger.withTaskIdAndComponent("api").debug("API Call: $method $url -> $statusCode $statusDescription")
    }

    fun close() {
        httpClient.close()
    }

    companion object {
        fun newClient(): ApiClient {
            return ApiClient()
        }
    }
}
