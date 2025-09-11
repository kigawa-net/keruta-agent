package net.kigawa.keruta.logger

import ch.qos.logback.classic.Level
import ch.qos.logback.classic.LoggerContext
import ch.qos.logback.classic.encoder.PatternLayoutEncoder
import ch.qos.logback.classic.spi.ILoggingEvent
import ch.qos.logback.core.AppenderBase
import ch.qos.logback.core.ConsoleAppender
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import net.kigawa.keruta.config.Config
import net.logstash.logback.encoder.LogstashEncoder
import org.slf4j.LoggerFactory
import org.slf4j.MDC
import ch.qos.logback.classic.Logger as LogbackLogger

interface LogSender {
    suspend fun sendLog(taskId: String, level: String, message: String)
}

class ApiLogAppender(private val client: LogSender) : AppenderBase<ILoggingEvent>() {

    override fun append(event: ILoggingEvent) {
        // API関連のログは送信しない（無限ループ防止）
        if (event.mdcPropertyMap["component"] == "api") {
            return
        }

        val taskId = Config.getTaskId()
        if (taskId.isEmpty()) {
            return
        }

        val level = event.level.toString().uppercase()

        // 非同期でAPIにログを送信
        CoroutineScope(Dispatchers.IO).launch {
            try {
                client.sendLog(taskId, level, event.formattedMessage)
            } catch (e: Exception) {
                // ログ送信エラーは無視
            }
        }
    }
}

object Logger {
    private var apiLogAppender: ApiLogAppender? = null

    fun init() {
        val context = LoggerFactory.getILoggerFactory() as LoggerContext
        val rootLogger = context.getLogger(LogbackLogger.ROOT_LOGGER_NAME)

        // 既存のアペンダーをクリア
        rootLogger.detachAndStopAllAppenders()

        // ログレベルの設定
        val level = when (System.getenv("KERUTA_LOG_LEVEL")?.lowercase()) {
            "debug" -> Level.DEBUG
            "info" -> Level.INFO
            "warn" -> Level.WARN
            "error" -> Level.ERROR
            else -> Level.INFO
        }
        rootLogger.level = level

        // コンソールアペンダーの設定
        val consoleAppender = ConsoleAppender<ILoggingEvent>()
        consoleAppender.context = context
        consoleAppender.name = "console"

        // フォーマットの設定
        if (System.getenv("KERUTA_LOG_FORMAT") == "json") {
            val jsonEncoder = LogstashEncoder()
            jsonEncoder.context = context
            jsonEncoder.start()
            consoleAppender.encoder = jsonEncoder
        } else {
            val patternEncoder = PatternLayoutEncoder()
            patternEncoder.context = context
            patternEncoder.pattern = "%d{yyyy-MM-dd HH:mm:ss} [%thread] %-5level %logger{36} - %msg%n"
            patternEncoder.start()
            consoleAppender.encoder = patternEncoder
        }

        consoleAppender.start()
        rootLogger.addAppender(consoleAppender)
    }

    fun setApiClient(client: LogSender) {
        val context = LoggerFactory.getILoggerFactory() as LoggerContext
        val rootLogger = context.getLogger(LogbackLogger.ROOT_LOGGER_NAME)

        // 既存のAPIアペンダーを削除
        apiLogAppender?.let {
            rootLogger.detachAppender(it)
            it.stop()
        }

        // 新しいAPIアペンダーを追加
        apiLogAppender = ApiLogAppender(client)
        apiLogAppender?.let {
            it.context = context
            it.name = "api"
            it.start()
            rootLogger.addAppender(it)
        }
    }

    fun withTaskId(): org.slf4j.Logger {
        MDC.put("task_id", Config.getTaskId())
        return LoggerFactory.getLogger("TaskLogger")
    }

    fun withComponent(component: String): org.slf4j.Logger {
        MDC.put("component", component)
        return LoggerFactory.getLogger("ComponentLogger")
    }

    fun withTaskIdAndComponent(component: String): org.slf4j.Logger {
        MDC.put("task_id", Config.getTaskId())
        MDC.put("component", component)
        return LoggerFactory.getLogger("TaskComponentLogger")
    }
}
