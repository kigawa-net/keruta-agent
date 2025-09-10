package net.kigawa.keruta.commands

import com.github.ajalt.clikt.core.CliktCommand
import com.github.ajalt.clikt.core.subcommands
import com.github.ajalt.clikt.parameters.options.flag
import com.github.ajalt.clikt.parameters.options.option
import net.kigawa.keruta.config.Config
import net.kigawa.keruta.logger.Logger
import org.slf4j.LoggerFactory

private val logger = LoggerFactory.getLogger("Commands")

class KerutaCommand : CliktCommand(
    name = "keruta",
    help = "keruta-agent - Kubernetes Pod内でタスクを実行するCLIツール"
) {
    private val verbose by option("-v", "--verbose", help = "詳細ログを出力").flag()
    private val taskId by option("--task-id", help = "タスクID（環境変数KERUTA_TASK_IDから自動取得）")
    
    override fun run() {
        // ログレベルの設定
        if (verbose) {
            // Debug level設定をここで行う
        }
        
        // タスクIDの設定
        taskId?.let { id ->
            System.setProperty("KERUTA_TASK_ID", id)
        }
        
        Logger.withTaskId().debug("keruta-agentを開始しました")
    }
}

class DaemonCommand : CliktCommand(
    name = "daemon",
    help = "デーモンモードで起動"
) {
    private val port by option("--port", help = "HTTPサーバーのポート番号")
    
    override fun run() {
        val actualPort = port ?: Config.getDaemonPort()
        
        echo("デーモンモードで起動中 (Port: $actualPort)")
        
        // デーモンの実装はここに追加
        // 現在は基本構造のみ
    }
}

fun execute() {
    val kerutaCommand = KerutaCommand().subcommands(DaemonCommand())
    
    try {
        kerutaCommand.main(emptyArray())
    } catch (e: Exception) {
        logger.error("コマンドの実行に失敗しました", e)
        throw e
    }
}